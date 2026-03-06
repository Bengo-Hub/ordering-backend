package router

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"go.uber.org/zap"

	httpware "github.com/Bengo-Hub/httpware"
	authclient "github.com/Bengo-Hub/shared-auth-client"
	"github.com/bengobox/ordering-backend/internal/config"
	handlers "github.com/bengobox/ordering-backend/internal/http/handlers"
	analyticshandler "github.com/bengobox/ordering-backend/internal/http/handlers/analytics"
	cataloghandler "github.com/bengobox/ordering-backend/internal/http/handlers/catalog"
	compliancehandler "github.com/bengobox/ordering-backend/internal/http/handlers/compliance"
	fulfilmenthandler "github.com/bengobox/ordering-backend/internal/http/handlers/fulfilment"
	confighandler "github.com/bengobox/ordering-backend/internal/http/handlers/config"
	identityhandler "github.com/bengobox/ordering-backend/internal/http/handlers/identity"
	notificationshandler "github.com/bengobox/ordering-backend/internal/http/handlers/notifications"
	orderinghandler "github.com/bengobox/ordering-backend/internal/http/handlers/ordering"
	paymentshandler "github.com/bengobox/ordering-backend/internal/http/handlers/payments"
	slahandler "github.com/bengobox/ordering-backend/internal/http/handlers/sla"
	"github.com/bengobox/ordering-backend/internal/modules/audit"
	"github.com/bengobox/ordering-backend/internal/modules/security"
)

// New constructs the chi router with global middleware and base routes.
// mediaRoot: if non-empty, serves static files at /media/* for menu images and uploads (local dev or persistent volume).
func New(
	log *zap.Logger,
	healthHandler *handlers.HealthHandler,
	mediaRoot string,
	configHandler *confighandler.Handler,
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

	// Global rate limiting by IP (if rate limiter is configured)
	if rateLimiter != nil && securityConfig.RateLimitEnabled {
		r.Use(rateLimiter.IPRateLimiter(securityConfig.RateLimitRequestsPerMin, time.Minute))
	}

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID", "X-Tenant-ID", "X-Tenant-Slug", "X-RateLimit-Limit", "X-RateLimit-Remaining", "X-RateLimit-Reset"},
		ExposedHeaders:   []string{"Link", "X-RateLimit-Limit", "X-RateLimit-Remaining", "X-RateLimit-Reset", "Retry-After"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// System endpoints (no tenant, no auth)
	r.Get("/healthz", healthHandler.Liveness)
	r.Get("/metrics", healthHandler.Metrics)
	r.Get("/v1/docs/*", handlers.SwaggerUI)

	// Local media storage: menu images and uploads (ORDERING_MEDIA_ROOT). Production: use persistent volume mount.
	if mediaRoot != "" {
		r.Handle("/media/*", http.StripPrefix("/media", http.FileServer(http.Dir(mediaRoot))))
	}

	// Redirect root path to Swagger documentation
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/v1/docs/", http.StatusMovedPermanently)
	})

	// TenantV2 config: chained extraction from JWT → headers → URL param
	tenantCfg := httpware.TenantConfig{
		ClaimsExtractor: func(ctx context.Context) (tenantID, tenantSlug string, ok bool) {
			claims, found := authclient.ClaimsFromContext(ctx)
			if !found {
				return "", "", false
			}
			return claims.TenantID, claims.GetTenantSlug(), true
		},
		URLParamFunc: chi.URLParam,
		Required:     true,
	}

	// Domain routes will be mounted on /api/v1/{tenant}.
	r.Route("/api", func(api chi.Router) {
		api.Route("/v1", func(v1 chi.Router) {
			// Apply path-based rate limiting for sensitive endpoints
			if rateLimiter != nil && securityConfig.RateLimitEnabled {
				v1.Use(func(next http.Handler) http.Handler {
					return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						// Stricter rate limiting for auth endpoints
						if strings.HasPrefix(r.URL.Path, "/api/v1/") && strings.Contains(r.URL.Path, "/auth/") {
							rateLimiter.EndpointRateLimiter("auth", securityConfig.RateLimitAuthPerMin, time.Minute)(next).ServeHTTP(w, r)
							return
						}
						// Stricter rate limiting for payment endpoints
						if strings.HasPrefix(r.URL.Path, "/api/v1/") && strings.Contains(r.URL.Path, "/payments/") {
							rateLimiter.EndpointRateLimiter("payments", securityConfig.RateLimitPaymentPerMin, time.Minute)(next).ServeHTTP(w, r)
							return
						}
						next.ServeHTTP(w, r)
					})
				})
			}

			// Serve OpenAPI spec (public, no auth required, outside tenant scope)
			v1.Get("/openapi.json", handlers.OpenAPIJSON)
			v1.Get("/status", healthHandler.Status)

			// Tenant-scoped routes
			v1.Route("/{tenant}", func(tenant chi.Router) {
				// Apply TenantV2 middleware to extract tenant from URL + JWT + headers
				tenant.Use(httpware.TenantV2(tenantCfg))

				// Apply auth-service middleware (excluding /auth/*, /webhooks/*)
				if authMiddleware != nil {
					tenant.Use(func(next http.Handler) http.Handler {
						return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							path := r.URL.Path
							// Skip auth for public routes: auth, webhooks, tenant config, cafes, menu (catalog)
							if strings.Contains(path, "/auth/") || strings.Contains(path, "/webhooks/") ||
								strings.Contains(path, "/config") || strings.Contains(path, "/cafes") || strings.Contains(path, "/menu") {
								next.ServeHTTP(w, r)
								return
							}
							authMiddleware.RequireAuth(next).ServeHTTP(w, r)
						})
					})
				}

				// Audit logging middleware for mutation endpoints
				if auditLogger != nil {
					tenant.Use(audit.MutationAudit(auditLogger))
				}

				// Public tenant/brand config (no auth)
				if configHandler != nil {
					tenant.Get("/config", configHandler.GetConfig)
				}

				// Register identity routes (auth endpoints are public)
				if identityHandler != nil && authenticator != nil {
					identityHandler.Register(tenant, authenticator)
				}

				// Register catalog routes (public menu + admin catalog)
				if catalogHandler != nil && authenticator != nil {
					catalogHandler.Register(tenant, authenticator)
				}

				// Register ordering routes (cart, orders, checkout, promo, loyalty, addresses)
				if authenticator != nil {
					if cartHandler != nil {
						cartHandler.Register(tenant, authenticator)
					}
					if orderHandler != nil {
						orderHandler.Register(tenant, authenticator)
					}
					if promoHandler != nil {
						promoHandler.Register(tenant, authenticator)
					}
					if loyaltyHandler != nil {
						loyaltyHandler.Register(tenant, authenticator)
					}
					if addressHandler != nil {
						addressHandler.Register(tenant, authenticator)
					}

					// Register payment routes
					if paymentHandler != nil {
						paymentHandler.Register(tenant, authenticator)
					}
					if paymentMethodHandler != nil {
						paymentMethodHandler.Register(tenant, authenticator)
					}
				}

				// Register fulfilment routes (delivery tasks, tracking)
				if fulfilmentTaskHandler != nil {
					fulfilmentTaskHandler.Register(tenant, authenticator)
				}

				// Register notifications routes
				if notificationsHandler != nil {
					notificationsHandler.Register(tenant, authenticator)
				}

				// Register SLA routes
				if slaHandler != nil {
					slaHandler.Register(tenant, authenticator)
				}

				// Register analytics routes
				if analyticsHandler != nil {
					analyticsHandler.Register(tenant, authenticator)
				}

				// Register compliance routes
				if complianceHandler != nil {
					complianceHandler.Register(tenant, authenticator)
				}

				// Webhook routes (no auth required - use signature verification)
				if paymentWebhookHandler != nil {
					paymentWebhookHandler.Register(tenant)
				}
				if fulfilmentWebhookHandler != nil {
					fulfilmentWebhookHandler.Register(tenant)
				}
			})
		})
	})

	return r
}
