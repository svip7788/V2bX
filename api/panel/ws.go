package panel

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"encoding/json"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
)

type WSEvent struct {
	Event     string          `json:"event"`
	Data      json.RawMessage `json:"data"`
	Timestamp int64           `json:"timestamp,omitempty"`
}

type WSConfigData struct {
	Config json.RawMessage `json:"config"`
}

type WSUsersData struct {
	Users []UserInfo `json:"users"`
}

type WSUserDeltaData struct {
	Action string     `json:"action"`
	Users  []UserInfo `json:"users"`
}

type WSDevicesData struct {
	Users map[string][]string `json:"users"`
}

type WSClient struct {
	conn     *websocket.Conn
	mu       sync.Mutex
	closed   bool
	stopCh   chan struct{}
	eventCh  chan WSEvent
	dropped  atomic.Bool
	apiHost  string
	token    string
	nodeId   int
	nodeType string
}

type HandshakeResponse struct {
	Websocket struct {
		Enabled bool   `json:"enabled"`
		WsURL   string `json:"ws_url"`
	} `json:"websocket"`
}

func (c *Client) Handshake() (*HandshakeResponse, error) {
	const path = "/api/v2/server/handshake"
	r, err := c.client.R().
		ForceContentType("application/json").
		Post(path)
	if err = c.checkResponse(r, path, err); err != nil {
		return nil, err
	}
	var resp HandshakeResponse
	if err := json.Unmarshal(r.Body(), &resp); err != nil {
		return nil, fmt.Errorf("unmarshal handshake response error: %w", err)
	}
	return &resp, nil
}

func (c *Client) NewWSClient(wsURL string) (*WSClient, error) {
	ws := &WSClient{
		stopCh:   make(chan struct{}),
		eventCh:  make(chan WSEvent, 64),
		apiHost:  c.APIHost,
		token:    c.Token,
		nodeId:   c.NodeId,
		nodeType: c.NodeType,
	}
	if err := ws.connect(wsURL); err != nil {
		return nil, err
	}
	go ws.readLoop()
	go ws.reconnectLoop(wsURL)
	return ws, nil
}

func (ws *WSClient) connect(wsURL string) error {
	u, err := url.Parse(wsURL)
	if err != nil {
		return fmt.Errorf("parse ws url error: %w", err)
	}
	q := u.Query()
	q.Set("token", ws.token)
	q.Set("node_id", strconv.Itoa(ws.nodeId))
	u.RawQuery = q.Encode()

	if !strings.HasPrefix(u.Scheme, "ws") {
		if strings.HasPrefix(u.Scheme, "https") {
			u.Scheme = "wss"
		} else {
			u.Scheme = "ws"
		}
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	conn, _, err := dialer.Dial(u.String(), nil)
	if err != nil {
		return fmt.Errorf("ws dial error: %w", err)
	}
	ws.mu.Lock()
	if ws.closed {
		ws.mu.Unlock()
		conn.Close()
		return fmt.Errorf("ws client already closed")
	}
	ws.conn = conn
	ws.mu.Unlock()
	return nil
}

func (ws *WSClient) readLoop() {
	for {
		select {
		case <-ws.stopCh:
			return
		default:
		}
		ws.mu.Lock()
		conn := ws.conn
		ws.mu.Unlock()
		if conn == nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}

		_, message, err := conn.ReadMessage()
		if err != nil {
			if ws.isClosed() {
				return
			}
			log.WithField("err", err).Warn("WS read error, will reconnect")
			ws.mu.Lock()
			conn.Close()
			ws.conn = nil
			ws.mu.Unlock()
			continue
		}

		var evt WSEvent
		if err := json.Unmarshal(message, &evt); err != nil {
			log.WithField("err", err).Warn("WS unmarshal event error")
			continue
		}

		if evt.Event == "ping" {
			ws.send([]byte(`{"event":"pong"}`))
			continue
		}

		if evt.Event == "auth.success" {
			log.Info("WS auth success")
			continue
		}

		if evt.Event == "error" {
			log.WithField("data", string(evt.Data)).Error("WS received error event")
			continue
		}

		select {
		case ws.eventCh <- evt:
		default:
			ws.dropped.Store(true)
			log.Warn("WS event channel full, dropping event: ", evt.Event)
		}
	}
}

func (ws *WSClient) reconnectLoop(wsURL string) {
	backoff := 2 * time.Second
	const maxBackoff = 60 * time.Second

	for {
		select {
		case <-ws.stopCh:
			return
		case <-time.After(backoff):
		}
		ws.mu.Lock()
		conn := ws.conn
		ws.mu.Unlock()
		if conn != nil {
			backoff = 2 * time.Second
			continue
		}
		if ws.isClosed() {
			return
		}
		log.Info("WS attempting reconnect...")
		if err := ws.connect(wsURL); err != nil {
			log.WithField("err", err).Warn("WS reconnect failed")
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		} else {
			log.Info("WS reconnected")
			backoff = 2 * time.Second
		}
	}
}

func (ws *WSClient) send(data []byte) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	if ws.conn == nil {
		return
	}
	_ = ws.conn.WriteMessage(websocket.TextMessage, data)
}

func (ws *WSClient) Events() <-chan WSEvent {
	return ws.eventCh
}

func (ws *WSClient) SendDeviceReport(data map[int][]string) {
	msg, err := json.Marshal(map[string]interface{}{
		"event": "report.devices",
		"data":  data,
	})
	if err != nil {
		log.WithField("err", err).Warn("WS marshal device report failed")
		return
	}
	ws.send(msg)
}

func (ws *WSClient) DroppedAndReset() bool {
	return ws.dropped.Swap(false)
}

func (ws *WSClient) IsConnected() bool {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	return ws.conn != nil && !ws.closed
}

func (ws *WSClient) isClosed() bool {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	return ws.closed
}

func (ws *WSClient) Close() {
	ws.mu.Lock()
	if ws.closed {
		ws.mu.Unlock()
		return
	}
	ws.closed = true
	close(ws.stopCh)
	if ws.conn != nil {
		ws.conn.Close()
	}
	ws.mu.Unlock()
}
