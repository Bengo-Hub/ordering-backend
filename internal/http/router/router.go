package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	httpSwagger "github.com/swaggo/http-swagger"
	"go.uber.org/zap"

	handlers "github.com/bengobox/food-delivery-backend/internal/http/handlers"
	identityhandler "github.com/bengobox/food-delivery-backend/internal/http/handlers/identity"
	sharedmw "github.com/bengobox/food-delivery-backend/internal/shared/middleware"
)

// New constructs the chi router with global middleware and base routes.
func New(log *zap.Logger, healthHandler *handlers.HealthHandler, identityHandler *identityhandler.Handler, authenticator *identityhandler.Authenticator) http.Handler {
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
	r.Get("/swagger/*", httpSwagger.WrapHandler)

	// Domain routes will be mounted on /api/v1.
	r.Route("/api", func(api chi.Router) {
		api.Route("/v1", func(v1 chi.Router) {
			v1.Get("/status", healthHandler.Status)
			if identityHandler != nil && authenticator != nil {
				identityHandler.Register(v1, authenticator)
			}
		})
	})

	return r
}
