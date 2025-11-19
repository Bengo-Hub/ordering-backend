package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"go.uber.org/zap"

	handlers "github.com/bengobox/cafe-backend/internal/http/handlers"
	identityhandler "github.com/bengobox/cafe-backend/internal/http/handlers/identity"
	sharedmw "github.com/bengobox/cafe-backend/internal/shared/middleware"
	authclient "github.com/Bengo-Hub/shared-auth-client"
)

// New constructs the chi router with global middleware and base routes.
func New(log *zap.Logger, healthHandler *handlers.HealthHandler, identityHandler *identityhandler.Handler, authenticator *identityhandler.Authenticator, authMiddleware *authclient.AuthMiddleware) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Heartbeat("/readyz"))
	r.Use(sharedmw.RequestID)
	r.Use(sharedmw.Logging(log))
	r.Use(sharedmw.Recover(log))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID", "X-Tenant-ID"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	r.Get("/healthz", healthHandler.Liveness)
	r.Get("/metrics", healthHandler.Metrics)
	r.Get("/v1/docs/*", handlers.SwaggerUI)

	// Domain routes will be mounted on /api/v1.
	r.Route("/api", func(api chi.Router) {
		api.Route("/v1", func(v1 chi.Router) {
			// Serve OpenAPI spec (public, no auth required)
			v1.Get("/openapi.json", handlers.OpenAPIJSON)
			
			v1.Get("/status", healthHandler.Status)
			
			// Apply auth-service middleware to protected routes if configured
			// Legacy authenticator can still be used for backward compatibility
			if authMiddleware != nil {
				v1.Use(authMiddleware.RequireAuth)
			}
			
			if identityHandler != nil && authenticator != nil {
				identityHandler.Register(v1, authenticator)
			}
		})
	})

	return r
}
