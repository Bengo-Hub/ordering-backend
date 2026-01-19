package router

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"go.uber.org/zap"

	authclient "github.com/Bengo-Hub/shared-auth-client"
	httpware "github.com/Bengo-Hub/httpware"
	"github.com/bengobox/ordering-backend/internal/config"
	handlers "github.com/bengobox/ordering-backend/internal/http/handlers"
	analyticshandler "github.com/bengobox/ordering-backend/internal/http/handlers/analytics"
	cataloghandler "github.com/bengobox/ordering-backend/internal/http/handlers/catalog"
	compliancehandler "github.com/bengobox/ordering-backend/internal/http/handlers/compliance"
	fulfilmenthandler "github.com/bengobox/ordering-backend/internal/http/handlers/fulfilment"
	identityhandler "github.com/bengobox/ordering-backend/internal/http/handlers/identity"
	notificationshandler "github.com/bengobox/ordering-backend/internal/http/handlers/notifications"
	orderinghandler "github.com/bengobox/ordering-backend/internal/http/handlers/ordering"
	paymentshandler "github.com/bengobox/ordering-backend/internal/http/handlers/payments"
	slahandler "github.com/bengobox/ordering-backend/internal/http/handlers/sla"
	"github.com/bengobox/ordering-backend/internal/modules/audit"
	"github.com/bengobox/ordering-backend/internal/modules/security"
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
	notificationsHandler *notificationshandler.Handler,
	slaHandler *slahandler.Handler,
	analyticsHandler *analyticshandler.Handler,
	complianceHandler *compliancehandler.Handler,
	authenticator *identityhandler.Authenticator,
	authMiddleware *authclient.AuthMiddleware,
	rateLimiter *security.RateLimiter,
	auditLogger *audit.Logger,
	securityConfig config.SecurityConfig,
	allowedOrigins []string,
) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Heartbeat("/readyz"))
	r.Use(httpware.RequestID)
	r.Use(httpware.Logging(log))
	r.Use(httpware.Recover(log))

	// Security headers middleware (OWASP compliance)
	if securityConfig.SecurityHeadersEnabled {
		r.Use(security.SecurityHeaders())
	}

	// Request size limiting middleware
	if securityConfig.MaxRequestBodySize > 0 {
		r.Use(security.RequestSizeLimit(securityConfig.MaxRequestBodySize))
	}

	// Input validation middleware (SQL injection, XSS protection)
	if securityConfig.InputValidationEnabled {
		r.Use(security.InputValidation())
	}

	// Content-Type validation for API requests
	r.Use(security.ContentTypeValidation("application/json"))

	// Tenant ID format validation
	r.Use(security.TenantValidation())

	// Global rate limiting by IP (if rate limiter is configured)
	if rateLimiter != nil && securityConfig.RateLimitEnabled {
		r.Use(rateLimiter.IPRateLimiter(securityConfig.RateLimitRequestsPerMin, time.Minute))
	}

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID", "X-Tenant-ID", "X-RateLimit-Limit", "X-RateLimit-Remaining", "X-RateLimit-Reset"},
		ExposedHeaders:   []string{"Link", "X-RateLimit-Limit", "X-RateLimit-Remaining", "X-RateLimit-Reset", "Retry-After"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	r.Get("/healthz", healthHandler.Liveness)
	r.Get("/metrics", healthHandler.Metrics)
	r.Get("/v1/docs/*", handlers.SwaggerUI)

	// Domain routes will be mounted on /api/v1.
	r.Route("/api", func(api chi.Router) {
		api.Route("/v1", func(v1 chi.Router) {
			// Apply path-based rate limiting for sensitive endpoints
			if rateLimiter != nil && securityConfig.RateLimitEnabled {
				v1.Use(func(next http.Handler) http.Handler {
					return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						// Stricter rate limiting for auth endpoints
						if strings.HasPrefix(r.URL.Path, "/api/v1/auth/") {
							rateLimiter.EndpointRateLimiter("auth", securityConfig.RateLimitAuthPerMin, time.Minute)(next).ServeHTTP(w, r)
							return
						}
						// Stricter rate limiting for payment endpoints
						if strings.HasPrefix(r.URL.Path, "/api/v1/payments/") {
							rateLimiter.EndpointRateLimiter("payments", securityConfig.RateLimitPaymentPerMin, time.Minute)(next).ServeHTTP(w, r)
							return
						}
						next.ServeHTTP(w, r)
					})
				})
			}

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

			// Audit logging middleware for mutation endpoints (POST, PUT, PATCH, DELETE)
			// Must be applied after auth middleware to have access to user claims
			if auditLogger != nil {
				v1.Use(audit.MutationAudit(auditLogger))
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

			// Register notifications routes
			if notificationsHandler != nil {
				notificationsHandler.Register(v1, authenticator)
			}

			// Register SLA routes
			if slaHandler != nil {
				slaHandler.Register(v1, authenticator)
			}

			// Register analytics routes
			if analyticsHandler != nil {
				analyticsHandler.Register(v1, authenticator)
			}

			// Register compliance routes
			if complianceHandler != nil {
				complianceHandler.Register(v1, authenticator)
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
