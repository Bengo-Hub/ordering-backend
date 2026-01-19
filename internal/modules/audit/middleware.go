package audit

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	authclient "github.com/Bengo-Hub/shared-auth-client"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// MutationAudit returns middleware that logs mutation operations (POST, PUT, PATCH, DELETE).
// It captures user context, request details, and response status for compliance auditing.
func MutationAudit(logger *Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only audit mutation methods
			if !isMutationMethod(r.Method) {
				next.ServeHTTP(w, r)
				return
			}

			// Skip certain paths that don't need auditing
			if shouldSkipPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			start := time.Now()

			// Extract user context from JWT claims
			var tenantID, userID *string
			if claims, ok := authclient.ClaimsFromContext(r.Context()); ok {
				if tid := claims.TenantID; tid != "" {
					tenantID = &tid
				}
				if uid := claims.Subject; uid != "" {
					userID = &uid
				}
			}

			// Capture request body for auditing (sanitized)
			var requestBody map[string]any
			if r.Body != nil && r.ContentLength > 0 && r.ContentLength < 10*1024 { // Max 10KB
				bodyBytes, err := io.ReadAll(r.Body)
				if err == nil {
					r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
					var body map[string]any
					if json.Unmarshal(bodyBytes, &body) == nil {
						requestBody = sanitizeRequestBody(body)
					}
				}
			}

			// Wrap response writer to capture status code
			wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			// Execute the handler
			next.ServeHTTP(wrapped, r)

			// Determine action and resource from the request
			action, resourceType, resourceID := extractActionAndResource(r)

			// Build audit entry
			entry := Entry{
				Action:      action,
				Resource:    resourceType,
				ResourceID:  resourceID,
				HTTPMethod:  r.Method,
				Path:        r.URL.Path,
				StatusCode:  wrapped.statusCode,
				IPAddress:   getClientIP(r),
				UserAgent:   r.UserAgent(),
				RequestBody: requestBody,
				DurationMS:  time.Since(start).Milliseconds(),
				OccurredAt:  start,
			}

			if tenantID != nil {
				entry.TenantID = parseUUID(*tenantID)
			}
			if userID != nil {
				entry.UserID = parseUUID(*userID)
			}

			// Record asynchronously to not block the response
			logger.RecordAsync(entry)
		})
	}
}

// responseWriter wraps http.ResponseWriter to capture status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func isMutationMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func shouldSkipPath(path string) bool {
	// Skip health checks, metrics, and auth endpoints
	skipPrefixes := []string{
		"/healthz",
		"/readyz",
		"/metrics",
		"/api/v1/auth/",       // Auth endpoints handled by auth-service
		"/api/v1/webhooks/",   // Webhooks have their own logging
	}
	for _, prefix := range skipPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func extractActionAndResource(r *http.Request) (action, resourceType, resourceID string) {
	path := r.URL.Path

	// Map HTTP method to action
	switch r.Method {
	case http.MethodPost:
		action = "CREATE"
	case http.MethodPut, http.MethodPatch:
		action = "UPDATE"
	case http.MethodDelete:
		action = "DELETE"
	default:
		action = r.Method
	}

	// Extract resource type from path
	// Pattern: /api/v1/{tenant}/{resource} or /api/v1/{resource}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 3 && parts[0] == "api" && parts[1] == "v1" {
		// Skip tenant UUID if present
		startIdx := 2
		if len(parts) > 3 && isUUID(parts[2]) {
			startIdx = 3
		}

		if startIdx < len(parts) {
			resourceType = normalizeResourceType(parts[startIdx])
		}

		// Extract resource ID if present (next segment after resource type)
		if startIdx+1 < len(parts) && isUUID(parts[startIdx+1]) {
			resourceID = parts[startIdx+1]
		}
	}

	// Try to get resource ID from chi route params
	if resourceID == "" {
		rctx := chi.RouteContext(r.Context())
		if rctx != nil {
			// Check common ID parameter names
			for _, paramName := range []string{"id", "orderId", "cartId", "itemId", "paymentId"} {
				if id := rctx.URLParam(paramName); id != "" && isUUID(id) {
					resourceID = id
					break
				}
			}
		}
	}

	return action, resourceType, resourceID
}

func normalizeResourceType(s string) string {
	// Normalize to singular form for consistency
	s = strings.ToLower(s)
	switch s {
	case "orders":
		return "Order"
	case "carts":
		return "Cart"
	case "payments":
		return "Payment"
	case "items":
		return "Item"
	case "addresses":
		return "Address"
	case "menu":
		return "MenuItem"
	case "categories":
		return "Category"
	case "promos", "promocodes":
		return "PromoCode"
	case "loyalty":
		return "Loyalty"
	case "notifications":
		return "Notification"
	case "fulfilment", "deliveries":
		return "Delivery"
	case "refunds":
		return "Refund"
	default:
		// Capitalize first letter
		if len(s) > 0 {
			return strings.ToUpper(s[:1]) + s[1:]
		}
		return s
	}
}

func sanitizeRequestBody(body map[string]any) map[string]any {
	// List of sensitive fields to redact
	sensitiveFields := map[string]bool{
		"password":        true,
		"password_hash":   true,
		"secret":          true,
		"token":           true,
		"access_token":    true,
		"refresh_token":   true,
		"api_key":         true,
		"apikey":          true,
		"credit_card":     true,
		"card_number":     true,
		"cvv":             true,
		"cvc":             true,
		"pin":             true,
		"ssn":             true,
		"social_security": true,
	}

	sanitized := make(map[string]any)
	for key, value := range body {
		lowerKey := strings.ToLower(key)
		if sensitiveFields[lowerKey] {
			sanitized[key] = "[REDACTED]"
		} else if nested, ok := value.(map[string]any); ok {
			sanitized[key] = sanitizeRequestBody(nested)
		} else {
			sanitized[key] = value
		}
	}
	return sanitized
}

func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first (for proxied requests)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the list
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	if idx := strings.LastIndex(r.RemoteAddr, ":"); idx != -1 {
		return r.RemoteAddr[:idx]
	}
	return r.RemoteAddr
}

func isUUID(s string) bool {
	// Simple UUID check (8-4-4-4-12 format)
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func parseUUID(s string) *uuid.UUID {
	if s == "" {
		return nil
	}
	parsed, err := uuid.Parse(s)
	if err != nil {
		return nil
	}
	return &parsed
}
