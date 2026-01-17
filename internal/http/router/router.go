package router

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"go.uber.org/zap"

	authclient "github.com/Bengo-Hub/shared-auth-client"
	handlers "github.com/bengobox/ordering-backend/internal/http/handlers"
	cataloghandler "github.com/bengobox/ordering-backend/internal/http/handlers/catalog"
	fulfilmenthandler "github.com/bengobox/ordering-backend/internal/http/handlers/fulfilment"
	identityhandler "github.com/bengobox/ordering-backend/internal/http/handlers/identity"
	orderinghandler "github.com/bengobox/ordering-backend/internal/http/handlers/ordering"
	paymentshandler "github.com/bengobox/ordering-backend/internal/http/handlers/payments"
	sharedmw "github.com/bengobox/ordering-backend/internal/shared/middleware"
)

// New constructs the chi router with global middleware and base routes.
func New(
	log *zap.Logger,
	healthHandler *handlers.HealthHandler,
	identityHandler *identityhandler.Handler,
	catalogHandler *cataloghandler.Handler,
	cartHandler *orderinghandler.CartHandler,
	orderHandler *orderinghandler.OrderHandler,
	promoHandler *orderinghandler.PromoHandler,
	loyaltyHandler *orderinghandler.LoyaltyHandler,
	addressHandler *orderinghandler.AddressHandler,
	paymentHandler *paymentshandler.PaymentHandler,
	paymentMethodHandler *paymentshandler.PaymentMethodHandler,
	paymentWebhookHandler *paymentshandler.WebhookHandler,
	fulfilmentTaskHandler *fulfilmenthandler.TaskHandler,
	fulfilmentWebhookHandler *fulfilmenthandler.WebhookHandler,
	authenticator *identityhandler.Authenticator,
	authMiddleware *authclient.AuthMiddleware,
	allowedOrigins []string,
) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Heartbeat("/readyz"))
	r.Use(sharedmw.RequestID)
	r.Use(sharedmw.Logging(log))
	r.Use(sharedmw.Recover(log))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   allowedOrigins,
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
			// Apply auth-service middleware to protected routes only (excluding /auth/* and /webhooks/*)
			// Note: This middleware validates JWT tokens from auth-service
			// Individual routes can still use authenticator.RequireAuth for additional RBAC checks
			if authMiddleware != nil {
				v1.Use(func(next http.Handler) http.Handler {
					return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						// Skip auth middleware for /auth/* routes and /webhooks/* routes
						if strings.HasPrefix(r.URL.Path, "/api/v1/auth/") ||
							strings.HasPrefix(r.URL.Path, "/api/v1/webhooks/") {
							next.ServeHTTP(w, r)
							return
						}
						// Apply auth middleware for all other routes
						authMiddleware.RequireAuth(next).ServeHTTP(w, r)
					})
				})
			}

			// Serve OpenAPI spec (public, no auth required)
			v1.Get("/openapi.json", handlers.OpenAPIJSON)

			v1.Get("/status", healthHandler.Status)

			// Register identity routes (auth endpoints are public)
			// Auth endpoints (/auth/*) are registered without auth middleware
			if identityHandler != nil && authenticator != nil {
				identityHandler.Register(v1, authenticator)
			}

			// Register catalog routes (public menu + admin catalog)
			if catalogHandler != nil && authenticator != nil {
				catalogHandler.Register(v1, authenticator)
			}

			// Register ordering routes (cart, orders, checkout, promo, loyalty, addresses)
			if authenticator != nil {
				if cartHandler != nil {
					cartHandler.Register(v1, authenticator)
				}
				if orderHandler != nil {
					orderHandler.Register(v1, authenticator)
				}
				if promoHandler != nil {
					promoHandler.Register(v1, authenticator)
				}
				if loyaltyHandler != nil {
					loyaltyHandler.Register(v1, authenticator)
				}
				if addressHandler != nil {
					addressHandler.Register(v1, authenticator)
				}

				// Register payment routes
				if paymentHandler != nil {
					paymentHandler.Register(v1, authenticator)
				}
				if paymentMethodHandler != nil {
					paymentMethodHandler.Register(v1, authenticator)
				}
			}

			// Register fulfilment routes (delivery tasks, tracking)
			if fulfilmentTaskHandler != nil {
				fulfilmentTaskHandler.Register(v1, authenticator)
			}

			// Webhook routes (no auth required - use signature verification)
			if paymentWebhookHandler != nil {
				paymentWebhookHandler.Register(v1)
			}
			if fulfilmentWebhookHandler != nil {
				fulfilmentWebhookHandler.Register(v1)
			}
		})
	})

	return r
}
