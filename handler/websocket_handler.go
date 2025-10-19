package handler

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/rizqitaufiqf/go-scheduler/config"
	"github.com/rizqitaufiqf/go-scheduler/dto"
	"github.com/rizqitaufiqf/go-scheduler/middleware"
	"github.com/rizqitaufiqf/go-scheduler/service"
)

type WebSocketHandler struct {
	hub     *service.WebSocketHub
	cfg     *config.Config
	service *service.NotificationService
}

func NewWebSocketHandler(hub *service.WebSocketHub, cfg *config.Config, notifService *service.NotificationService) *WebSocketHandler {
	return &WebSocketHandler{
		hub:     hub,
		cfg:     cfg,
		service: notifService,
	}
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// TODO: Configure allowed origins via ENV
		return true
	},
}

// @Summary WebSocket endpoint for real-time notifications
// @Description Connect to WebSocket for receiving real-time notifications
// @Tags websocket
// @Param token query string true "JWT Token"
// @Router /ws/notifications [get]
func (h *WebSocketHandler) HandleWebSocket(c *gin.Context) {
	// Get token from query parameter
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "token required"})
		return
	}

	// Validate JWT
	claims, err := middleware.ValidateJWT(token, h.cfg)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}

	// Upgrade connection
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[WebSocket] Upgrade error: %v", err)
		return
	}

	// Set connection limits
	conn.SetReadLimit(int64(h.cfg.WSMaxMessageSize))

	// Create client
	client := &service.Client{
		ID:        uuid.New().String(),
		UserID:    claims.UserID,
		SessionID: claims.SessionID,
		Conn:      conn,
		Send:      make(chan []byte, 256),
		Hub:       h.hub,
	}

	// Register client
	h.hub.RegisterClient(client)

	// Save session
	deviceInfo := c.GetHeader("User-Agent")
	ipAddress := c.ClientIP()
	if err := h.service.SaveSession(c.Request.Context(), claims.SessionID, claims.UserID, deviceInfo, ipAddress); err != nil {
		log.Printf("[WebSocket] Error saving session: %v", err)
	}

	// Send auth success message
	authSuccessMsg := dto.WSMessage{
		Type:      dto.WSMessageTypeAuthSuccess,
		Timestamp: time.Now().Unix(),
		SessionID: claims.SessionID,
		Data: map[string]interface{}{
			"user_id":    claims.UserID,
			"session_id": claims.SessionID,
		},
	}
	h.hub.SendToSession(claims.SessionID, &authSuccessMsg)

	// Get pending notifications and send them
	go h.sendPendingNotifications(claims.UserID, claims.SessionID)

	// Start client goroutines
	go client.WritePump()
	go client.ReadPump()
}

// Send pending notifications to newly connected client
func (h *WebSocketHandler) sendPendingNotifications(userID, sessionID string) {
	// Wait a bit for connection to stabilize
	time.Sleep(500 * time.Millisecond)

	notifications, err := h.service.GetUserNotifications(
		context.Background(),
		userID,
		sessionID,
		50, // Get last 50 notifications
		0,
		true, // Unread only
	)

	if err != nil {
		log.Printf("[WebSocket] Error getting pending notifications: %v", err)
		return
	}

	if len(notifications.Notifications) == 0 {
		return
	}

	// Send each notification
	for _, notif := range notifications.Notifications {
		wsMsg := dto.WSMessage{
			Type: dto.WSMessageTypeNotification,
			Data: dto.WSNotificationMessage{
				Notification: notif,
			},
			Timestamp: time.Now().Unix(),
		}

		h.hub.SendToSession(sessionID, &wsMsg)
	}

	log.Printf("[WebSocket] Sent %d pending notifications to session %s", len(notifications.Notifications), sessionID)
}

// Health check endpoint
func (h *WebSocketHandler) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"service": "websocket", // online_users has been removed
	})
}
