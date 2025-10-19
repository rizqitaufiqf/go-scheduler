package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/rizqitaufiqf/go-scheduler/config"
)

// AuthHandler handles authentication-related requests.
type AuthHandler struct {
	Cfg *config.Config
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(cfg *config.Config) *AuthHandler {
	return &AuthHandler{Cfg: cfg}
}

type loginRequest struct {
	UserID    uuid.UUID `json:"user_id" binding:"required"`
	SessionID string    `json:"session_id" binding:"required"`
}

// Login generates a JWT for a given user ID for testing purposes.
// @Summary      Generate JWT for testing
// @Description  Takes a user_id and returns a JWT token for testing authenticated endpoints.
// @Tags         Auth
// @Accept       json
// @Produce      json
// @Param        credentials  body      loginRequest  true  "User ID for token generation"
// @Success      200          {object}  map[string]string
// @Failure      400          {object}  map[string]string
// @Router       /api/v1/auth/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "a valid user_id (UUID) and session_id are required"})
		return
	}

	// Create the claims, including a random session_id as expected by the WebSocket handler.
	claims := jwt.MapClaims{
		"user_id":    req.UserID.String(),
		"session_id": req.SessionID,
		"exp":        time.Now().Add(time.Hour * time.Duration(h.Cfg.JWTExpiryHours)).Unix(),
		"iat":        time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(h.Cfg.JWTSecret))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"token": tokenString})
}
