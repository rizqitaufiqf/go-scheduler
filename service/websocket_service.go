package service

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rizqitaufiqf/go-scheduler/config"
	"github.com/rizqitaufiqf/go-scheduler/dto"
)

type Client struct {
	ID        string
	UserID    string
	SessionID string
	Conn      *websocket.Conn
	Send      chan []byte
	Hub       *WebSocketHub
}

const (
	hubChannelBufferSize    = 256
	broadcastChannelBufSize = 1024
)

type WebSocketHub struct {
	clients    map[string]*Client         // sessionID -> client
	userIndex  map[string]map[string]bool // userID -> map[sessionID]bool
	register   chan *Client
	unregister chan *Client
	broadcast  chan *dto.WSMessage
	mu         sync.RWMutex
	cfg        *config.Config
}

func NewWebSocketHub(cfg *config.Config) *WebSocketHub {
	return &WebSocketHub{
		clients:    make(map[string]*Client),
		userIndex:  make(map[string]map[string]bool),
		register:   make(chan *Client, hubChannelBufferSize),
		unregister: make(chan *Client, hubChannelBufferSize),
		broadcast:  make(chan *dto.WSMessage, broadcastChannelBufSize),
		cfg:        cfg,
	}
}

// Run hub
func (h *WebSocketHub) Run(ctx context.Context) {
	log.Println("[WebSocket Hub] Starting...")

	for {
		select {
		case <-ctx.Done():
			log.Println("[WebSocket Hub] Context cancelled, shutting down...")
			h.shutdown()
			return

		case client := <-h.register:
			h.mu.Lock()
			h.clients[client.SessionID] = client

			if _, ok := h.userIndex[client.UserID]; !ok {
				h.userIndex[client.UserID] = make(map[string]bool)
			}
			h.userIndex[client.UserID][client.SessionID] = true
			h.mu.Unlock()

			log.Printf("[WebSocket] Client registered: userID=%s, sessionID=%s", client.UserID, client.SessionID)

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client.SessionID]; ok {
				delete(h.clients, client.SessionID)
				close(client.Send)

				if sessions, ok := h.userIndex[client.UserID]; ok {
					delete(sessions, client.SessionID)
					if len(sessions) == 0 {
						delete(h.userIndex, client.UserID)
					}
				}
			}
			h.mu.Unlock()

			log.Printf("[WebSocket] Client unregistered: userID=%s, sessionID=%s", client.UserID, client.SessionID)

		case message := <-h.broadcast:
			h.broadcastMessage(message)
		}
	}
}

// RegisterClient sends a client to the register channel.
func (h *WebSocketHub) RegisterClient(client *Client) {
	h.register <- client
}

// Broadcast to all clients
func (h *WebSocketHub) broadcastMessage(message *dto.WSMessage) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	data, err := json.Marshal(message)
	if err != nil {
		log.Printf("[WebSocket] Error marshaling broadcast message: %v", err)
		return
	}

	for _, client := range h.clients {
		select {
		case client.Send <- data:
		default:
			log.Printf("[WebSocket] Client buffer full, skipping message for sessionID=%s", client.SessionID)
		}
	}
}

// Send to specific user (all sessions)
func (h *WebSocketHub) SendToUser(userID string, message *dto.WSMessage) {
	h.mu.RLock()
	sessions, ok := h.userIndex[userID]
	h.mu.RUnlock()

	if !ok {
		log.Printf("[WebSocket] No active sessions for userID=%s", userID)
		return
	}

	data, err := json.Marshal(message)
	if err != nil {
		log.Printf("[WebSocket] Error marshaling message: %v", err)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for sessionID := range sessions {
		if client, ok := h.clients[sessionID]; ok {
			select {
			case client.Send <- data:
				log.Printf("[WebSocket] Message sent to userID=%s, sessionID=%s", userID, sessionID)
			default:
				log.Printf("[WebSocket] Client buffer full for sessionID=%s", sessionID)
			}
		}
	}
}

// Send to specific session
func (h *WebSocketHub) SendToSession(sessionID string, message *dto.WSMessage) {
	h.mu.RLock()
	client, ok := h.clients[sessionID]
	h.mu.RUnlock()

	if !ok {
		log.Printf("[WebSocket] Session not found: %s", sessionID)
		return
	}

	data, err := json.Marshal(message)
	if err != nil {
		log.Printf("[WebSocket] Error marshaling message: %v", err)
		return
	}

	select {
	case client.Send <- data:
	default:
		log.Printf("[WebSocket] Client buffer full for sessionID=%s", sessionID)
	}
}

// Get active sessions count for user
func (h *WebSocketHub) GetUserSessionCount(userID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	if sessions, ok := h.userIndex[userID]; ok {
		return len(sessions)
	}
	return 0
}

// Check if user is online
func (h *WebSocketHub) IsUserOnline(userID string) bool {
	return h.GetUserSessionCount(userID) > 0
}

// Shutdown hub
func (h *WebSocketHub) shutdown() {
	h.mu.Lock()
	defer h.mu.Unlock()

	for _, client := range h.clients {
		close(client.Send)
		client.Conn.Close()
	}

	h.clients = make(map[string]*Client)
	h.userIndex = make(map[string]map[string]bool)
}

// Client read pump
func (c *Client) ReadPump() {
	defer func() {
		c.Hub.unregister <- c
		c.Conn.Close()
	}()

	c.Conn.SetReadDeadline(time.Now().Add(time.Duration(c.Hub.cfg.WSPongTimeoutSeconds) * time.Second))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(time.Duration(c.Hub.cfg.WSPongTimeoutSeconds) * time.Second))
		return nil
	})

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[WebSocket] Read error: %v", err)
			}
			break
		}

		// Handle incoming messages (ping, mark as read, etc)
		var wsMsg dto.WSMessage
		if err := json.Unmarshal(message, &wsMsg); err != nil {
			log.Printf("[WebSocket] Error unmarshaling message: %v", err)
			continue
		}

		// Handle different message types
		switch wsMsg.Type {
		case dto.WSMessageTypePing:
			pongMsg := dto.WSMessage{
				Type:      dto.WSMessageTypePong,
				Timestamp: time.Now().Unix(),
			}
			data, _ := json.Marshal(pongMsg)
			c.Send <- data
		}
	}
}

// Client write pump
func (c *Client) WritePump() {
	ticker := time.NewTicker(time.Duration(c.Hub.cfg.WSPingIntervalSeconds) * time.Second)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
				log.Printf("[WebSocket] Write error: %v", err)
				return
			}

		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
