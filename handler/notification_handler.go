package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rizqitaufiqf/go-scheduler/dto"
	"github.com/rizqitaufiqf/go-scheduler/service"
)

type NotificationHandler struct {
	service *service.NotificationService
}

func NewNotificationHandler(service *service.NotificationService) *NotificationHandler {
	return &NotificationHandler{service: service}
}

// getUserAndSession safely extracts userID and sessionID from the Gin context.
// It returns false if the values are missing or not of the expected type.
func getUserAndSession(c *gin.Context) (userID, sessionID string, ok bool) {
	userIDVal, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user_id not found in context"})
		return "", "", false
	}
	sessionIDVal, exists := c.Get("session_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "session_id not found in context"})
		return "", "", false
	}

	userID, ok = userIDVal.(string)
	sessionID, ok = sessionIDVal.(string)
	return userID, sessionID, ok
}

// @Summary Get user notifications
// @Description Get paginated notifications for the authenticated user
// @Tags notifications
// @Accept json
// @Produce json
// @Param limit query int false "Limit" default(20)
// @Param offset query int false "Offset" default(0)
// @Param unread query bool false "Unread only" default(false)
// @Security BearerAuth
// @Success 200 {object} dto.NotificationListResponse
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/notifications [get]
func (h *NotificationHandler) GetNotifications(c *gin.Context) {
	userID, sessionID, ok := getUserAndSession(c)
	if !ok {
		return // Error response already sent by helper
	}

	var query dto.GetNotificationsQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Set defaults
	if query.Limit == 0 {
		query.Limit = 20
	}

	response, err := h.service.GetUserNotifications(
		c.Request.Context(),
		userID,
		sessionID,
		query.Limit,
		query.Offset,
		query.Unread,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, response)
}

// @Summary Mark notifications as read
// @Description Mark one or more notifications as read for the current session
// @Tags notifications
// @Accept json
// @Produce json
// @Param request body dto.MarkReadRequest true "Notification IDs"
// @Security BearerAuth
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/notifications/read [post]
func (h *NotificationHandler) MarkAsRead(c *gin.Context) {
	userID, sessionID, ok := getUserAndSession(c)
	if !ok {
		return // Error response already sent by helper
	}

	var req dto.MarkReadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err := h.service.MarkAsRead(
		c.Request.Context(),
		req.NotificationIDs,
		sessionID,
		userID,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Notifications marked as read"})
}

// @Summary Get unread count
// @Description Get the count of unread notifications for the authenticated user
// @Tags notifications
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]int64
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/notifications/unread-count [get]
func (h *NotificationHandler) GetUnreadCount(c *gin.Context) {
	userID, sessionID, ok := getUserAndSession(c)
	if !ok {
		return // Error response already sent by helper
	}

	count, err := h.service.GetUnreadCount(
		c.Request.Context(),
		userID,
		sessionID,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"unread_count": count})
}
