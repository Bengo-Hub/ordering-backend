package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"go.uber.org/zap"

	handlers "github.com/bengobox/food-delivery-backend/internal/http/handlers"
	sharedmw "github.com/bengobox/food-delivery-backend/internal/shared/middleware"
)

// New constructs the chi router with global middleware and base routes.
func New(log *zap.Logger, healthHandler *handlers.HealthHandler) http.Handler {
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

	// Domain routes will be mounted on /v1.
	r.Route("/v1", func(api chi.Router) {
		api.Get("/status", healthHandler.Status)
		// api.Mount("/orders", ordersRouter)
	})

	return r
}
