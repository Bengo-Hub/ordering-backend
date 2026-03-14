package authclient

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// GinMiddleware provides a Gin-compatible middleware wrapper around AuthMiddleware.
func GinMiddleware(mw *AuthMiddleware) gin.HandlerFunc {
	return func(c *gin.Context) {
		handler := mw.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c.Request = r
			c.Next()
		}))

		handler.ServeHTTP(c.Writer, c.Request)

		if c.IsAborted() {
			return
		}
	}
}

// GinClaimsFromContext extracts claims from Gin context (which wraps the request context).
func GinClaimsFromContext(c *gin.Context) (*Claims, bool) {
	return ClaimsFromContext(c.Request.Context())
}

