package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/google/uuid"
	"github.com/rizqitaufiqf/go-scheduler/dto"
)

// SSEClient represents a connected SSE client
type SSEClient struct {
	UserID  uuid.UUID
	Channel chan *dto.NotificationResponse
}

// SSEManager manages all active SSE connections
type SSEManager struct {
	clients map[uuid.UUID][]*SSEClient // userID -> list of clients (support multiple tabs/devices)
	mu      sync.RWMutex
}

var (
	sseManagerInstance *SSEManager
	once               sync.Once
)

// GetSSEManager returns the singleton instance of SSEManager
func GetSSEManager() *SSEManager {
	once.Do(func() {
		sseManagerInstance = &SSEManager{
			clients: make(map[uuid.UUID][]*SSEClient),
		}
		log.Println("✅ SSE Manager initialized")
	})
	return sseManagerInstance
}

// AddClient adds a new SSE client connection
func (m *SSEManager) AddClient(userID uuid.UUID, client *SSEClient) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.clients[userID] = append(m.clients[userID], client)
	log.Printf("✅ SSE Client connected: UserID=%s, Total connections for user=%d, Total online users=%d",
		userID, len(m.clients[userID]), len(m.clients))
}

// RemoveClient removes an SSE client connection
func (m *SSEManager) RemoveClient(userID uuid.UUID, client *SSEClient) {
	m.mu.Lock()
	defer m.mu.Unlock()

	clients := m.clients[userID]
	for i, c := range clients {
		if c == client {
			// Remove client from slice
			m.clients[userID] = append(clients[:i], clients[i+1:]...)
			close(c.Channel)
			break
		}
	}

	// If no more clients for this user, remove the user entry
	if len(m.clients[userID]) == 0 {
		delete(m.clients, userID)
	}

	log.Printf("❌ SSE Client disconnected: UserID=%s, Remaining connections=%d, Total online users=%d",
		userID, len(m.clients[userID]), len(m.clients))
}

// IsUserOnline checks if a user has any active SSE connections
func (m *SSEManager) IsUserOnline(userID uuid.UUID) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	clients, exists := m.clients[userID]
	return exists && len(clients) > 0
}

// SendToUser sends a notification to all connections of a specific user
func (m *SSEManager) SendToUser(userID uuid.UUID, notification *dto.NotificationResponse) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	clients, exists := m.clients[userID]
	if !exists || len(clients) == 0 {
		return fmt.Errorf("user %s is not online", userID)
	}

	// Send to all client connections for this user (multiple tabs/devices)
	successCount := 0
	for _, client := range clients {
		select {
		case client.Channel <- notification:
			successCount++
		default:
			log.Printf("⚠️  Failed to send notification to UserID=%s, channel full or closed", userID)
		}
	}

	if successCount > 0 {
		log.Printf("📨 Sent notification to UserID=%s (%d connections)", userID, successCount)
		return nil
	}

	return fmt.Errorf("failed to send notification to any connection for user %s", userID)
}

// GetOnlineUserCount returns the number of online users
func (m *SSEManager) GetOnlineUserCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.clients)
}

// GetTotalConnectionCount returns total number of SSE connections
func (m *SSEManager) GetTotalConnectionCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	total := 0
	for _, clients := range m.clients {
		total += len(clients)
	}
	return total
}

// FormatSSEMessage formats a notification as SSE message
func FormatSSEMessage(notification *dto.NotificationResponse) (string, error) {
	data, err := json.Marshal(notification)
	if err != nil {
		return "", err
	}

	// SSE format: data: {json}\n\n
	return fmt.Sprintf("data: %s\n\n", string(data)), nil
}
