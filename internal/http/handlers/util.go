package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
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

// DecodeJSON decodes the request body into the provided destination.
func DecodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(dst)
}

// ParseInt parses a string to an integer, returning a default value if parsing fails.
func ParseInt(s string, defaultVal int) int {
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return v
}
