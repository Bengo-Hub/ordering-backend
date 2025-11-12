package handlers

import (
	"encoding/json"
	"net/http"
)

// ErrorResponse represents a standard error payload.
type ErrorResponse struct {
	Error   string `json:"error" example:"Unauthorized"`
	Message string `json:"message" example:"invalid credentials"`
}

// RespondJSON writes the provided payload as JSON response.
func RespondJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		_, _ = w.Write([]byte(`{"status":"serialization_error"}`))
	}
}

// RespondError writes an error response with the supplied message.
func RespondError(w http.ResponseWriter, status int, message string) {
	RespondJSON(w, status, ErrorResponse{
		Error:   http.StatusText(status),
		Message: message,
	})
}
