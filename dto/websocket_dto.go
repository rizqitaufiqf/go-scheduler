package dto

type WSMessageType string

const (
	WSMessageTypeNotification     WSMessageType = "notification"
	WSMessageTypePing             WSMessageType = "ping"
	WSMessageTypePong             WSMessageType = "pong"
	WSMessageTypeAuth             WSMessageType = "auth"
	WSMessageTypeAuthSuccess      WSMessageType = "auth_success"
	WSMessageTypeAuthFailed       WSMessageType = "auth_failed"
	WSMessageTypeError            WSMessageType = "error"
	WSMessageTypeSubscribe        WSMessageType = "subscribe"
	WSMessageTypeUnsubscribe      WSMessageType = "unsubscribe"
	WSMessageTypeNotificationRead WSMessageType = "notification_read"
)

type WSMessage struct {
	Type      WSMessageType `json:"type"`
	Data      interface{}   `json:"data,omitempty"`
	Timestamp int64         `json:"timestamp"`
	SessionID string        `json:"session_id,omitempty"`
}

type WSAuthMessage struct {
	Token string `json:"token"`
}

type WSNotificationMessage struct {
	Notification NotificationResponse `json:"notification"`
}

type WSErrorMessage struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
