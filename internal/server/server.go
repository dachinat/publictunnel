package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dachi-pa/publictunnel/internal/protocol"
	"github.com/google/uuid"
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

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all for now
	},
}

type ClientConn struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (c *ClientConn) WriteJSON(v interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.conn.SetWriteDeadline(time.Now().Add(writeWait))
	return c.conn.WriteJSON(v)
}

type TunnelServer struct {
	Domain       string
	TunnelDomain string
	Port         int
	clients      map[string]*ClientConn
	clientsMu    sync.RWMutex
	pendingReqs  map[string]chan *protocol.HttpResponsePayload
	reqsMu       sync.RWMutex
}

func NewTunnelServer(domain string, tunnelDomain string, port int) *TunnelServer {
	if tunnelDomain == "" {
		tunnelDomain = domain
	}
	return &TunnelServer{
		Domain:       domain,
		TunnelDomain: tunnelDomain,
		Port:         port,
		clients:      make(map[string]*ClientConn),
		pendingReqs:  make(map[string]chan *protocol.HttpResponsePayload),
	}
}

func (s *TunnelServer) Start() error {
	addr := fmt.Sprintf(":%d", s.Port)
	log.Printf("Server starting on %s", addr)

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRequest)

	return http.ListenAndServe(addr, mux)
}

func (s *TunnelServer) handleRequest(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	// Remove port if present
	if idx := strings.Index(host, ":"); idx != -1 {
		host = host[:idx]
	}

	// Handle WebSocket registration on the main domain or tunnel domain
	if host == s.Domain || host == s.TunnelDomain || host == "localhost" {
		if r.URL.Path == "/ws" {
			s.handleWebSocket(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("PublicTunnel Server is running. Use the client to connect."))
		return
	}

	// Handle subdomains
	if strings.HasSuffix(host, "."+s.TunnelDomain) {
		subdomain := strings.TrimSuffix(host, "."+s.TunnelDomain)
		s.proxyToClient(subdomain, w, r)
		return
	}

	http.Error(w, "Not Found", http.StatusNotFound)
}

func (s *TunnelServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Upgrade error: %v", err)
		return
	}
	defer conn.Close()

	// Wait for registration message
	_, msg, err := conn.ReadMessage()
	if err != nil {
		return
	}

	var ctrl protocol.ControlMessage
	if err := json.Unmarshal(msg, &ctrl); err != nil || ctrl.Type != protocol.TypeRegister {
		return
	}

	regData, _ := json.Marshal(ctrl.Payload)
	var reg protocol.RegisterPayload
	json.Unmarshal(regData, &reg)

	subdomain := reg.Subdomain
	if subdomain == "" {
		subdomain = uuid.New().String()[:8]
	}

	client := &ClientConn{conn: conn}

	s.clientsMu.Lock()
	if _, exists := s.clients[subdomain]; exists {
		// Subdomain taken, generate new one
		subdomain = subdomain + "-" + uuid.New().String()[:4]
	}
	s.clients[subdomain] = client
	s.clientsMu.Unlock()

	log.Printf("Client registered: %s", subdomain)

	// Send confirmation
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	resp := protocol.ControlMessage{
		Type: protocol.TypeRegisterResp,
		Payload: protocol.RegisterRespPayload{
			Subdomain: subdomain,
			URL:       fmt.Sprintf("%s://%s.%s", scheme, subdomain, s.TunnelDomain),
		},
	}
	client.WriteJSON(resp)

	// Set up health checks
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	// Start ping loop
	go func() {
		for range ticker.C {
			client.mu.Lock()
			client.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := client.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				client.mu.Unlock()
				return
			}
			client.mu.Unlock()
		}
	}()

	// Listen for responses or disconnect
	for {
		_, msg, err := client.conn.ReadMessage()
		if err != nil {
			log.Printf("Client disconnected: %s", subdomain)
			s.clientsMu.Lock()
			delete(s.clients, subdomain)
			s.clientsMu.Unlock()
			break
		}

		var respMsg protocol.ControlMessage
		if err := json.Unmarshal(msg, &respMsg); err != nil {
			continue
		}

		if respMsg.Type == protocol.TypeHttpResponse {
			data, _ := json.Marshal(respMsg.Payload)
			var httpResp protocol.HttpResponsePayload
			json.Unmarshal(data, &httpResp)

			s.reqsMu.RLock()
			ch, ok := s.pendingReqs[httpResp.ID]
			s.reqsMu.RUnlock()

			if ok {
				ch <- &httpResp
			}
		}
	}
}

func (s *TunnelServer) proxyToClient(subdomain string, w http.ResponseWriter, r *http.Request) {
	s.clientsMu.RLock()
	conn, ok := s.clients[subdomain]
	s.clientsMu.RUnlock()

	if !ok {
		http.Error(w, "Tunnel not found", http.StatusNotFound)
		return
	}

	reqID := uuid.New().String()
	body, _ := io.ReadAll(r.Body)

	reqPayload := protocol.HttpRequestPayload{
		ID:      reqID,
		Method:  r.Method,
		Path:    r.URL.Path,
		Headers: r.Header,
		Body:    body,
	}

	ctrlMsg := protocol.ControlMessage{
		Type:    protocol.TypeHttpRequest,
		Payload: reqPayload,
	}

	respCh := make(chan *protocol.HttpResponsePayload, 1)
	s.reqsMu.Lock()
	s.pendingReqs[reqID] = respCh
	s.reqsMu.Unlock()

	defer func() {
		s.reqsMu.Lock()
		delete(s.pendingReqs, reqID)
		s.reqsMu.Unlock()
	}()

	if err := conn.WriteJSON(ctrlMsg); err != nil {
		http.Error(w, "Failed to send request to client", http.StatusInternalServerError)
		return
	}

	// Wait for response from client with timeout
	select {
	case resp := <-respCh:
		for k, vv := range resp.Headers {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(resp.Status)
		w.Write(resp.Body)
	case <-time.After(30 * time.Second):
		http.Error(w, "Gateway timeout", http.StatusGatewayTimeout)
	}
}
