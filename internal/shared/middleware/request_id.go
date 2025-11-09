package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

// requestIDKey is the context key used to store the request identifier.
type requestIDKey struct{}

// RequestID injects a uuid4 into the request context if one is not already present.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := r.Context().Value(requestIDKey{}).(string); !ok {
			r = r.WithContext(context.WithValue(r.Context(), requestIDKey{}, uuid.NewString()))
		}

		next.ServeHTTP(w, r)
	})
}

// FromContext retrieves the request identifier from the context.
func FromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey{}).(string); ok {
		return id
	}

	return ""
}
