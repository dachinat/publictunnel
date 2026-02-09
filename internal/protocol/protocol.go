package protocol

type MessageType string

const (
	TypeRegister     MessageType = "REGISTER"
	TypeRegisterResp MessageType = "REGISTER_RESP"
	TypeHttpRequest  MessageType = "HTTP_REQUEST"
	TypeHttpResponse MessageType = "HTTP_RESPONSE"
	TypeError        MessageType = "ERROR"
)

type ControlMessage struct {
	Type    MessageType `json:"type"`
	Payload interface{} `json:"payload,omitempty"`
}

type RegisterPayload struct {
	Subdomain string `json:"subdomain"`
}

type RegisterRespPayload struct {
	Subdomain string `json:"subdomain"`
	URL       string `json:"url"`
	Error     string `json:"error,omitempty"`
}

type HttpRequestPayload struct {
	ID      string              `json:"id"`
	Method  string              `json:"method"`
	Path    string              `json:"path"`
	Headers map[string][]string `json:"headers"`
	Body    []byte              `json:"body"`
}

type HttpResponsePayload struct {
	ID      string              `json:"id"`
	Status  int                 `json:"status"`
	Headers map[string][]string `json:"headers"`
	Body    []byte              `json:"body"`
}

type ErrorPayload struct {
	Message string `json:"message"`
}
