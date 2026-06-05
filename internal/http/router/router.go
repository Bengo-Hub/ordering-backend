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
	confighandler "github.com/bengobox/ordering-backend/internal/http/handlers/config"
	fulfilmenthandler "github.com/bengobox/ordering-backend/internal/http/handlers/fulfilment"
	googlebusinesshandler "github.com/bengobox/ordering-backend/internal/http/handlers/googlebusiness"
	identityhandler "github.com/bengobox/ordering-backend/internal/http/handlers/identity"
	notificationshandler "github.com/bengobox/ordering-backend/internal/http/handlers/notifications"
	orderinghandler "github.com/bengobox/ordering-backend/internal/http/handlers/ordering"
	paymentshandler "github.com/bengobox/ordering-backend/internal/http/handlers/payments"
	slahandler "github.com/bengobox/ordering-backend/internal/http/handlers/sla"
	zoneshandler "github.com/bengobox/ordering-backend/internal/http/handlers/zones"
	ordermw "github.com/bengobox/ordering-backend/internal/http/middleware"
	"github.com/bengobox/ordering-backend/internal/modules/audit"
	"github.com/bengobox/ordering-backend/internal/modules/identity"
	"github.com/bengobox/ordering-backend/internal/modules/security"
	"github.com/bengobox/ordering-backend/internal/modules/tenant"
	"github.com/bengobox/ordering-backend/internal/platform/subscriptions"
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
	groupOrderHandler *orderinghandler.GroupOrderHandler,
	paymentHandler *paymentshandler.PaymentHandler,
	paymentMethodHandler *paymentshandler.PaymentMethodHandler,
	paymentWebhookHandler *paymentshandler.WebhookHandler,
	fulfilmentTaskHandler *fulfilmenthandler.TaskHandler,
	fulfilmentWebhookHandler *fulfilmenthandler.WebhookHandler,
	notificationsHandler *notificationshandler.Handler,
	slaHandler *slahandler.Handler,
	analyticsHandler *analyticshandler.Handler,
	complianceHandler *compliancehandler.Handler,
	zonesHandler *zoneshandler.Handler,
	authenticator *identityhandler.Authenticator,
	authMiddleware *authclient.AuthMiddleware,
	rateLimiter *security.RateLimiter,
	auditLogger *audit.Logger,
	securityConfig config.SecurityConfig,
	allowedOrigins []string,
	mediaHandler *handlers.MediaHandler,
	rbacHandler *handlers.RBACHandler,
	tenantSyncer *tenant.Syncer,
	serviceConfigHandler *confighandler.ServiceConfigHandler,
	useCaseHandler *confighandler.UseCaseHandler,
	googleBusinessHandler *googlebusinesshandler.Handler,
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

	// Content-Type validation for API requests (exempt media upload which uses multipart/form-data)
	r.Use(security.ContentTypeValidationWithExclusions(
		[]string{"/api/v1/media/upload"},
		"application/json",
	))

	// CORS must be applied BEFORE rate limiter so 429 responses include CORS headers
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "Origin", "X-Request-ID", "X-Tenant-ID", "X-Tenant-Slug", "X-RateLimit-Limit", "X-RateLimit-Remaining", "X-RateLimit-Reset", "X-Outlet-ID"},
		ExposedHeaders:   []string{"Link", "X-RateLimit-Limit", "X-RateLimit-Remaining", "X-RateLimit-Reset", "Retry-After"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Global rate limiting by IP (if rate limiter is configured)
	if rateLimiter != nil && securityConfig.RateLimitEnabled {
		r.Use(rateLimiter.IPRateLimiter(securityConfig.RateLimitRequestsPerMin, time.Minute))
	}

	// System endpoints (no tenant, no auth)
	r.Get("/healthz", healthHandler.Liveness)
	r.Get("/metrics", healthHandler.Metrics)
	r.Get("/v1/docs/*", handlers.SwaggerUI)

	if mediaHandler != nil {
		r.Post("/api/v1/media/upload", mediaHandler.Upload)
	}

	// Local media storage: menu images and uploads (ORDERING_MEDIA_ROOT). Production: use persistent volume mount.
	if mediaRoot != "" {
		r.Handle("/media/*", http.StripPrefix("/media", http.FileServer(http.Dir(mediaRoot))))
	}

	// Redirect root path to Swagger documentation
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/v1/docs/", http.StatusMovedPermanently)
	})

	// TenantV2 config: chained extraction from JWT -> headers -> URL param
	tenantCfg := httpware.TenantConfig{
		ClaimsExtractor: func(ctx context.Context) (tenantID, tenantSlug string, isPlatformOwner bool, ok bool) {
			claims, found := authclient.ClaimsFromContext(ctx)
			if !found {
				return "", "", false, false
			}
			return claims.TenantID, claims.GetTenantSlug(), claims.IsPlatformOwner, true
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

				// Optional outlet context — extracts X-Outlet-ID if present
				tenant.Use(ordermw.OutletContext)

				// JIT tenant sync: ensure tenant exists in local DB when slug is in context.
				// Also backfills tenant ID into context so getTenantID() works for guest users
				// who only have a slug (no JWT → no tenant UUID in claims).
				if tenantSyncer != nil {
					tenant.Use(func(next http.Handler) http.Handler {
						return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							ctx := r.Context()
							slug := httpware.GetTenantSlug(ctx)
							if slug != "" {
								tenantUUID, err := tenantSyncer.SyncTenant(ctx, slug)
								if err != nil {
									log.Warn("tenant sync failed during request", zap.String("tenant_slug", slug), zap.Error(err))
								} else if tid := tenantUUID.String(); tid != "" && tid != "00000000-0000-0000-0000-000000000000" && httpware.GetTenantID(ctx) == "" {
									// Backfill tenant ID for guest requests that only have the slug
									ctx = context.WithValue(ctx, httpware.TenantIDKey, tid)
									r = r.WithContext(ctx)
								}
							}
							next.ServeHTTP(w, r)
						})
					})
				}

				// Apply auth-service middleware. Skip only for truly public routes (not /auth/me or /auth/logout).
				// This middleware stores JWT claims in context -- required for RequirePermissions checks.
				if authMiddleware != nil {
					tenant.Use(func(next http.Handler) http.Handler {
						return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							path := r.URL.Path
							// Skip auth for public routes: webhooks, tenant config, outlets,
							// catalog GET (public browsing), guest cart, guest checkout, and delivery zones.
							// Do NOT skip /auth/ -- GET /auth/me and POST /auth/logout require a valid JWT.
							// Do NOT skip catalog mutations (POST/PUT/DELETE) -- they need claims for permission checks.
							isPublicCatalog := strings.Contains(path, "/catalog") &&
								!strings.Contains(path, "/catalog/admin") &&
								r.Method == http.MethodGet
							// Public Google endpoints: the OAuth callback (Google redirects here,
							// state-signed) and the review-url deep-link lookup. Admin Google
							// endpoints live under /admin/integrations/google and DO require auth.
							isPublicGoogle := strings.Contains(path, "/integrations/google/callback") ||
								strings.Contains(path, "/integrations/google/review-url")
							if strings.Contains(path, "/webhooks/") ||
								strings.Contains(path, "/config") || strings.Contains(path, "/outlets") ||
								strings.Contains(path, "/cart/guest") ||
								strings.Contains(path, "/cart/fee-breakdown") ||
								strings.Contains(path, "/checkout/guest") ||
								strings.Contains(path, "/orders/guest") ||
								isPublicGoogle ||
								strings.Contains(path, "/zones") ||
								strings.Contains(path, "/ratings") ||
								isPublicCatalog {
								next.ServeHTTP(w, r)
								return
							}
							authMiddleware.RequireAuth(next).ServeHTTP(w, r)
						})
					})

					// Layer 2: Subscription gate — enforces status, grace period, and 402 on hard expiry.
					tenant.Use(subscriptions.SubscriptionGate())
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

				// Register catalog routes (public catalog + admin catalog)
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
					if groupOrderHandler != nil {
						groupOrderHandler.Register(tenant, authenticator)
					}

					// Register delivery zones
					if zonesHandler != nil {
						zonesHandler.Register(tenant, authenticator)
					}

					// Register payment routes
					if paymentHandler != nil {
						paymentHandler.Register(tenant, authenticator)
					}
					if paymentMethodHandler != nil {
						paymentMethodHandler.Register(tenant, authenticator)
						// Tenant gateway-management proxy routes (/payments/gateways/*)
						// that proxy to treasury-api's S2S gateway routes.
						paymentMethodHandler.RegisterGateways(tenant, authenticator)
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

				// Register RBAC routes (role/permission management)
				if rbacHandler != nil {
					rbacHandler.RegisterRoutes(tenant, authenticator)
				}

				// Admin service config routes (platform owner / tenant-scoped settings)
				if serviceConfigHandler != nil {
					// PUBLIC: "Review us on Google" deep link for the guest post-rating CTA.
					// No auth (covered by the public-skip list above) — a plain config read.
					tenant.Get("/integrations/google/review-url", serviceConfigHandler.GetGoogleReviewURL)

					// PLATFORM defaults + UNMASKED secrets: superuser / platform-owner ONLY.
					// RequirePermissions would let tenant admins bypass, so use RequirePlatformOwner
					// which honors claims.IsSuperuser() OR claims.IsPlatformOwner and does NOT honor IsAdmin.
					tenant.Route("/admin/service-config", func(adminCfg chi.Router) {
						adminCfg.Use(authenticator.RequirePlatformOwner)
						adminCfg.Get("/", serviceConfigHandler.ListPlatformSettings)
						adminCfg.Put("/{key}", serviceConfigHandler.UpsertPlatformSetting)
					})
					// Tenant-scoped config (fee-config save path). GET stays read-only;
					// PUT requires config.manage (admin/superuser bypass so tenant admins can save).
					tenant.Route("/settings/service-config", func(settingsCfg chi.Router) {
						settingsCfg.Get("/", serviceConfigHandler.ListTenantSettings)
						settingsCfg.With(authenticator.RequirePermissions(identity.PermissionAdminManage)).
							Put("/{key}", serviceConfigHandler.UpsertTenantSetting)
					})
				}

				// Read-only use-case configuration (tenant + per-outlet use_case).
				if useCaseHandler != nil && authenticator != nil {
					tenant.With(authenticator.RequirePermissions(identity.Permission("ordering.config.view"))).
						Get("/admin/use-case", useCaseHandler.GetUseCaseConfig)
				}

				// Google Business Profile integration (admin connect/reviews + public callback).
				// Safe when OAuth env is unset: endpoints return 503 "not configured".
				if googleBusinessHandler != nil && authenticator != nil {
					googleBusinessHandler.Register(tenant, authenticator)
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
