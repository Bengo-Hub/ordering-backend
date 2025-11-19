package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Bengo-Hub/shared-auth-client"
	"go.uber.org/zap"
)

type contextKey string

const authClaimsKey contextKey = "auth_service_claims"

// AuthServiceMiddleware validates JWT tokens from auth-service using JWKS.
func AuthServiceMiddleware(log *zap.Logger, validator *authclient.Validator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" || !strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
				writeAuthError(w, http.StatusUnauthorized, "missing bearer token")
				return
			}

			tokenStr := strings.TrimSpace(authHeader[7:])
			claims, err := validator.ValidateToken(tokenStr)
			if err != nil {
				log.Warn("token validation failed", zap.Error(err))
				writeAuthError(w, http.StatusUnauthorized, "invalid token")
				return
			}

			ctx := context.WithValue(r.Context(), authClaimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ClaimsFromContext extracts auth-service claims from request context.
func ClaimsFromContext(ctx context.Context) (*authclient.Claims, bool) {
	claims, ok := ctx.Value(authClaimsKey).(*authclient.Claims)
	return claims, ok && claims != nil
}

func writeAuthError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": message,
		"code":  "unauthorized",
	})
}

