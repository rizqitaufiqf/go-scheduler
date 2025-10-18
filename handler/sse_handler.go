package handler

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rizqitaufiqf/go-scheduler/dto"
	"github.com/rizqitaufiqf/go-scheduler/repository"
)

type SSEHandler struct {
	repository repository.NotificationRepository
	sseManager *SSEManager
}

func NewSSEHandler(repository repository.NotificationRepository, sseManager *SSEManager) *SSEHandler {
	return &SSEHandler{
		repository: repository,
		sseManager: sseManager,
	}
}

// HandleSSE handles SSE connections from frontend
// @Summary Subscribe to notifications via SSE
// @Description Establishes a Server-Sent Events connection to receive real-time notifications
// @Tags notifications
// @Param user_id query int true "User ID"
// @Success 200 {string} string "SSE stream"
// @Router /notifications/stream [get]
func (h *SSEHandler) HandleSSE(c *gin.Context) {
	// Get user ID from query parameter
	// In production, extract from JWT token instead
	userIDStr := c.Query("user_id")
	if userIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id is required"})
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user_id"})
		return
	}

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no") // Disable buffering in nginx

	// Check if streaming is supported
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		log.Println("❌ Streaming not supported")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming not supported"})
		return
	}

	// Create SSE client
	client := &SSEClient{
		UserID:  userID,
		Channel: make(chan *dto.NotificationResponse, 10), // Buffer 10 notifications
	}

	// Add client to manager
	h.sseManager.AddClient(userID, client)
	defer h.sseManager.RemoveClient(userID, client)

	// Context for cancellation
	ctx := c.Request.Context()

	// Send pending notifications immediately when user connects
	go h.sendPendingNotifications(ctx, userID, client)

	// Keep-alive ticker (every 30 seconds)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Send initial connection success message
	c.Writer.Write([]byte(": connected\n\n"))
	flusher.Flush()

	log.Printf("🔌 SSE Connection established for UserID=%s", userID)

	// Main event loop
	for {
		select {
		case <-ctx.Done():
			// Client disconnected
			log.Printf("🔌 SSE Connection closed for UserID=%s", userID)
			return

		case notification, ok := <-client.Channel:
			if !ok {
				// Channel closed
				return
			}

			// Format and send notification
			message, err := FormatSSEMessage(notification)
			if err != nil {
				log.Printf("❌ Failed to format SSE message: %v", err)
				continue
			}

			_, err = c.Writer.Write([]byte(message))
			if err != nil {
				log.Printf("❌ Failed to write SSE message: %v", err)
				return
			}
			flusher.Flush()

		case <-ticker.C:
			// Send keep-alive comment to prevent connection timeout
			c.Writer.Write([]byte(": keep-alive\n\n"))
			flusher.Flush()
		}
	}
}

// sendPendingNotifications sends all pending notifications for a user
func (h *SSEHandler) sendPendingNotifications(ctx context.Context, userID uuid.UUID, client *SSEClient) {
	// Small delay to ensure connection is stable
	time.Sleep(500 * time.Millisecond)

	// Fetch all pending notifications from database
	notifications, err := h.repository.FindPendingByUserID(ctx, userID)
	if err != nil {
		log.Printf("❌ Failed to fetch pending notifications for UserID=%s: %v", userID, err)
		return
	}

	if len(notifications) == 0 {
		log.Printf("📭 No pending notifications for UserID=%s", userID)
		return
	}

	log.Printf("📬 Sending %d pending notifications to UserID=%s", len(notifications), userID)

	// Send each pending notification
	for _, notif := range notifications {
		response := &dto.NotificationResponse{
			ID:        notif.ID,
			TaskID:    notif.TaskID,
			Type:      notif.Type,
			Title:     notif.Title,
			Message:   notif.Message,
			Status:    notif.Status,
			Payload:   notif.Payload,
			CreatedAt: notif.CreatedAt,
		}

		// Send to client channel
		select {
		case client.Channel <- response:
			// Mark as sent in database
			if err := h.repository.MarkAsSent(ctx, notif.ID); err != nil {
				log.Printf("⚠️  Failed to mark notification %s as sent: %v", notif.ID, err)
			}
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
			log.Printf("⚠️  Timeout sending notification %s to UserID=%s", notif.ID, userID)
		}
	}
}

// MarkAsRead marks a notification as read
// @Summary Mark notification as read
// @Description Marks a specific notification as read by the user
// @Tags notifications
// @Param id path int true "Notification ID"
// @Success 200 {object} map[string]interface{}
// @Router /notifications/{id}/read [put]
func (h *SSEHandler) MarkAsRead(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid notification id"})
		return
	}

	if err := h.repository.MarkAsRead(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to mark notification as read"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "notification marked as read",
		"id":      id,
	})
}

// GetNotifications gets notifications for a user
// @Summary Get user notifications
// @Description Gets notifications for a specific user with optional limit
// @Tags notifications
// @Param user_id query int true "User ID"
// @Param limit query int false "Limit number of results" default(50)
// @Success 200 {array} dto.Notification
// @Router /notifications [get]
func (h *SSEHandler) GetNotifications(c *gin.Context) {
	userIDStr := c.Query("user_id")
	if userIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id is required"})
		return
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user_id"})
		return
	}

	// Get limit from query parameter (default 50)
	limitStr := c.DefaultQuery("limit", "50")
	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		limit = 50
	}

	notifications, err := h.repository.FindByUserID(c.Request.Context(), userID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch notifications"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"notifications": notifications,
		"count":         len(notifications),
	})
}

// GetStats returns SSE connection statistics
// @Summary Get SSE statistics
// @Description Returns information about active SSE connections
// @Tags notifications
// @Success 200 {object} map[string]interface{}
// @Router /notifications/stats [get]
func (h *SSEHandler) GetStats(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"online_users":      h.sseManager.GetOnlineUserCount(),
		"total_connections": h.sseManager.GetTotalConnectionCount(),
	})
}
