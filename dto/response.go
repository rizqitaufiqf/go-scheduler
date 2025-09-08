package dto

// ErrorResponse defines the standard error response format.
type ErrorResponse struct {
	Error string `json:"error" example:"a description of the error"`
}

// MessageResponse defines the standard message response format.
type MessageResponse struct {
	Message string `json:"message" example:"operation was successful"`
}
