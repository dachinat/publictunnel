package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/dachinat/publictunnel/internal/protocol"
	"github.com/gorilla/websocket"
)

const (
	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second
)

type TunnelClient struct {
	ServerURL  string
	LocalPort  int
	Subdomain  string
	httpClient *http.Client
	writeMu    sync.Mutex
}

func NewTunnelClient(serverURL string, localPort int, subdomain string) *TunnelClient {
	return &TunnelClient{
		ServerURL:  serverURL,
		LocalPort:  localPort,
		Subdomain:  subdomain,
		httpClient: &http.Client{},
	}
}

func (c *TunnelClient) Start() error {
	u, err := url.Parse(c.ServerURL)
	if err != nil {
		return err
	}

	if u.Scheme == "http" {
		u.Scheme = "ws"
	} else if u.Scheme == "https" {
		u.Scheme = "wss"
	}
	u.Path = "/ws"

	log.Printf("Connecting to %s...", u.String())
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return fmt.Errorf("dial: %v", err)
	}
	defer conn.Close()

	// Register
	reg := protocol.ControlMessage{
		Type: protocol.TypeRegister,
		Payload: protocol.RegisterPayload{
			Subdomain: c.Subdomain,
		},
	}
	if err := conn.WriteJSON(reg); err != nil {
		return fmt.Errorf("register: %v", err)
	}

	// Set up health checks
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPingHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		c.writeMu.Lock()
		defer c.writeMu.Unlock()
		conn.SetWriteDeadline(time.Now().Add(writeWait))
		return conn.WriteMessage(websocket.PongMessage, nil)
	})

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read: %v", err)
		}

		var ctrl protocol.ControlMessage
		if err := json.Unmarshal(msg, &ctrl); err != nil {
			log.Printf("Unmarshal error: %v", err)
			continue
		}

		switch ctrl.Type {
		case protocol.TypeRegisterResp:
			data, _ := json.Marshal(ctrl.Payload)
			var resp protocol.RegisterRespPayload
			json.Unmarshal(data, &resp)
			if resp.Error != "" {
				return fmt.Errorf("registration failed: %s", resp.Error)
			}
			log.Printf("Tunnel established!")
			log.Printf("Public URL: %s", resp.URL)
			log.Printf("Forwarding to: http://localhost:%d", c.LocalPort)

		case protocol.TypeHttpRequest:
			data, _ := json.Marshal(ctrl.Payload)
			var reqPayload protocol.HttpRequestPayload
			json.Unmarshal(data, &reqPayload)
			go c.handleRequest(conn, reqPayload)

		case protocol.TypeError:
			data, _ := json.Marshal(ctrl.Payload)
			var errPayload protocol.ErrorPayload
			json.Unmarshal(data, &errPayload)
			log.Printf("Server error: %s", errPayload.Message)
		}
	}
}

func (c *TunnelClient) handleRequest(conn *websocket.Conn, req protocol.HttpRequestPayload) {
	localURL := fmt.Sprintf("http://localhost:%d%s", c.LocalPort, req.Path)
	log.Printf("Proxying: %s %s -> %s", req.Method, req.Path, localURL)

	httpReq, err := http.NewRequest(req.Method, localURL, bytes.NewReader(req.Body))
	if err != nil {
		log.Printf("Failed to create request: %v", err)
		return
	}

	for k, vv := range req.Headers {
		for _, v := range vv {
			httpReq.Header.Add(k, v)
		}
	}

	resp, err := c.httpClient.Do(httpReq)
	var respPayload protocol.HttpResponsePayload
	respPayload.ID = req.ID

	if err != nil {
		log.Printf("Local request failed: %v", err)
		respPayload.Status = http.StatusBadGateway
		respPayload.Body = []byte(fmt.Sprintf("Local request failed: %v", err))
	} else {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		respPayload.Status = resp.StatusCode
		respPayload.Headers = resp.Header
		respPayload.Body = body
	}

	ctrlMsg := protocol.ControlMessage{
		Type:    protocol.TypeHttpResponse,
		Payload: respPayload,
	}

	c.sendResponse(conn, ctrlMsg)
}

func (c *TunnelClient) sendResponse(conn *websocket.Conn, msg protocol.ControlMessage) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	conn.SetWriteDeadline(time.Now().Add(writeWait))
	if err := conn.WriteJSON(msg); err != nil {
		log.Printf("Failed to send response: %v", err)
	}
}
