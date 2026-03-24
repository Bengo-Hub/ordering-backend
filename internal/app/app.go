package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/schema"
	_ "github.com/jackc/pgx/v5/stdlib"
	"go.uber.org/zap"

	sharedcache "github.com/Bengo-Hub/cache"
	authclient "github.com/Bengo-Hub/shared-auth-client"
	eventslib "github.com/Bengo-Hub/shared-events"
	"github.com/bengobox/ordering-backend/internal/config"
	"github.com/bengobox/ordering-backend/internal/ent"
	"github.com/bengobox/ordering-backend/internal/ent/migrate"
	handlers "github.com/bengobox/ordering-backend/internal/http/handlers"
	analyticshandler "github.com/bengobox/ordering-backend/internal/http/handlers/analytics"
	cataloghandler "github.com/bengobox/ordering-backend/internal/http/handlers/catalog"
	compliancehandler "github.com/bengobox/ordering-backend/internal/http/handlers/compliance"
	confighandler "github.com/bengobox/ordering-backend/internal/http/handlers/config"
	zoneshandler "github.com/bengobox/ordering-backend/internal/http/handlers/zones"
	fulfilmenthandler "github.com/bengobox/ordering-backend/internal/http/handlers/fulfilment"
	identityhandler "github.com/bengobox/ordering-backend/internal/http/handlers/identity"
	notificationshandler "github.com/bengobox/ordering-backend/internal/http/handlers/notifications"
	orderinghandler "github.com/bengobox/ordering-backend/internal/http/handlers/ordering"
	paymentshandler "github.com/bengobox/ordering-backend/internal/http/handlers/payments"
	slahandler "github.com/bengobox/ordering-backend/internal/http/handlers/sla"
	httprouter "github.com/bengobox/ordering-backend/internal/http/router"
	"github.com/bengobox/ordering-backend/internal/modules/analytics"
	"github.com/bengobox/ordering-backend/internal/modules/audit"
	"github.com/bengobox/ordering-backend/internal/modules/catalog"
	"github.com/bengobox/ordering-backend/internal/modules/compliance"
	"github.com/bengobox/ordering-backend/internal/modules/fulfilment"
	"github.com/bengobox/ordering-backend/internal/modules/identity"
	"github.com/bengobox/ordering-backend/internal/modules/notifications"
	"github.com/bengobox/ordering-backend/internal/modules/ordering"
	"github.com/bengobox/ordering-backend/internal/modules/rbac"
	"github.com/bengobox/ordering-backend/internal/modules/outbox"
	"github.com/bengobox/ordering-backend/internal/modules/payments"
	"github.com/bengobox/ordering-backend/internal/modules/security"
	"github.com/bengobox/ordering-backend/internal/modules/sla"
	"github.com/bengobox/ordering-backend/internal/modules/tenant"
	"github.com/bengobox/ordering-backend/internal/platform/cache"
	"github.com/bengobox/ordering-backend/internal/platform/database"
	"github.com/bengobox/ordering-backend/internal/platform/events"
	"github.com/bengobox/ordering-backend/internal/platform/inventory"
	"github.com/bengobox/ordering-backend/internal/platform/logistics"
	extnotifications "github.com/bengobox/ordering-backend/internal/platform/notifications"
	"github.com/bengobox/ordering-backend/internal/platform/subscriptions"
	"github.com/bengobox/ordering-backend/internal/platform/superset"
	"github.com/bengobox/ordering-backend/internal/platform/treasury"
	"github.com/bengobox/ordering-backend/internal/shared/logger"
)

type App struct {
	cfg             *config.Config
	log             *zap.Logger
	httpServer      *http.Server
	db              dbCloser
	cache           cacheCloser
	events          eventCloser
	orm             *ent.Client
	outboxPublisher *outbox.Publisher
}

type dbCloser interface {
	Ping(context.Context) error
	Close()
}

type cacheCloser interface {
	Close() error
}

type eventCloser interface {
	Drain() error
	Close()
}

// New initialises the application with all infrastructure dependencies wired.
func New(ctx context.Context) (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	log, err := logger.New(cfg.App.Env)
	if err != nil {
		return nil, fmt.Errorf("app: logger init: %w", err)
	}

	dbPool, err := database.NewPool(ctx, cfg.Postgres)
	if err != nil {
		return nil, fmt.Errorf("app: postgres init: %w", err)
	}

	redisClient := cache.NewClient(cfg.Redis)

	natsConn, err := events.Connect(cfg.Events)
	if err != nil {
		log.Warn("app: nats connect failed", zap.Error(err))
	}

	healthHandler := handlers.NewHealthHandler(log, dbPool, redisClient, natsConn)

	sqlDB, err := sql.Open("pgx", cfg.Postgres.URL)
	if err != nil {
		return nil, fmt.Errorf("app: ent driver init: %w", err)
	}
	// Configure connection pooling for optimal performance
	sqlDB.SetMaxIdleConns(cfg.Postgres.MaxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.Postgres.MaxOpenConns)
	sqlDB.SetConnMaxLifetime(cfg.Postgres.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(5 * time.Minute)

	log.Info("app: database connection pool configured",
		zap.Int("max_open_conns", cfg.Postgres.MaxOpenConns),
		zap.Int("max_idle_conns", cfg.Postgres.MaxIdleConns),
		zap.Duration("conn_max_lifetime", cfg.Postgres.ConnMaxLifetime))

	drv := entsql.OpenDB(dialect.Postgres, sqlDB)
	ormClient := ent.NewClient(ent.Driver(drv))
	if err := ormClient.Schema.Create(ctx, schema.WithDir(migrate.Dir)); err != nil {
		return nil, fmt.Errorf("app: ent schema create: %w", err)
	}

	// Note: Seeding should be done manually via 'go run cmd/seed/main.go' after migrations
	// This ensures roles, permissions, tenant, and demo user are always available
	// The seed command is idempotent and can be run multiple times safely
	log.Info("app: migrations completed - run 'go run cmd/seed/main.go' to seed initial data (idempotent)")

	identityRepo := identity.NewEntRepository(ormClient)
	tenantSyncer := tenant.NewSyncer(ormClient, cfg.Auth.ServiceURL)
	identitySvc, err := identity.NewService(identityRepo, cfg.Auth, log, tenantSyncer)
	if err != nil {
		return nil, fmt.Errorf("app: identity service init: %w", err)
	}

	// Initialize auth-service JWT validator
	var validator *authclient.Validator
	var authMiddleware *authclient.AuthMiddleware
	if cfg.Auth.ServiceURL != "" {
		jwksURL := fmt.Sprintf("%s/api/v1/.well-known/jwks.json", cfg.Auth.ServiceURL)
		authConfig := authclient.DefaultConfig(
			jwksURL,
			cfg.Auth.Issuer,
			cfg.Auth.Audience,
		)
		authConfig.CacheTTL = cfg.Auth.JWKSCacheTTL
		authConfig.RefreshInterval = cfg.Auth.JWKSRefreshInterval
		authConfig.RedisClient = redisClient
		validator, err = authclient.NewValidator(authConfig)
		if err != nil {
			return nil, fmt.Errorf("auth validator init: %w", err)
		}

		// Initialize API key validator if enabled
		if cfg.Auth.EnableAPIKeyAuth {
			apiKeyValidator := authclient.NewAPIKeyValidator(cfg.Auth.ServiceURL, nil)
			authMiddleware = authclient.NewAuthMiddlewareWithAPIKey(validator, apiKeyValidator)
		} else {
			authMiddleware = authclient.NewAuthMiddleware(validator)
		}

		// Subscribe to auth-service events for user sync
		if natsConn != nil {
			eventHandler := identity.NewEventHandler(identitySvc, log)
			if err := eventHandler.SubscribeToAuthEvents(natsConn); err != nil {
				log.Warn("app: failed to subscribe to auth events", zap.Error(err))
			}

			// Initialize NATS event subscribers for proactive provisioning
			eventSub := events.NewSubscriber(natsConn, log)
			branchSub := tenant.NewBranchSubscriber(ormClient, log)
			if err := branchSub.RegisterSubscribers(eventSub); err != nil {
				log.Error("failed to register branch subscribers", zap.Error(err))
			}
		}
	}

	identityHandler := identityhandler.New(log, identitySvc)
	authenticator := identityhandler.NewAuthenticator(log, identitySvc, validator)

	// Public tenant/brand config handler (no auth).
	// Uses shared cache (cache-aside via auth-api) for tenant branding.
	tenantCache := sharedcache.New(redisClient, log)
	configHandler := confighandler.New(log, ormClient, tenantCache, cfg.Auth.ServiceURL)

	// Initialize cache service for catalog read caching
	cacheSvc := cache.NewService(redisClient, cache.DefaultCacheConfig(), log)

	// Initialize inventory client (for stock availability, reservations, and catalog proxy)
	inventoryClient := inventory.NewClient(cfg.Inventory, log)

	// Initialize catalog module (proxy model: inventory-api + local overrides)
	catalogProxySvc := catalog.NewProxyService(ormClient, inventoryClient, cacheSvc, log)
	catalogHandler := cataloghandler.New(log, catalogProxySvc, ormClient)

	// Initialize subscriptions client (for subscription enforcement on order creation)
	subscriptionsClient := subscriptions.NewClient(cfg.Subscriptions, log)

	// Initialize ordering module
	orderingRepo := ordering.NewEntRepository(ormClient)
	cartSvc := ordering.NewCartService(orderingRepo, catalogProxySvc, log)
	promoSvc := ordering.NewPromoService(orderingRepo, log)
	loyaltySvc := ordering.NewLoyaltyService(orderingRepo, log)
	addressSvc := ordering.NewAddressService(orderingRepo, log)
	orderSvc := ordering.NewOrderService(orderingRepo, cartSvc, promoSvc, loyaltySvc, inventoryClient, subscriptionsClient, log)

	// Start order scheduler for scheduled delivery flow
	orderScheduler := ordering.NewOrderScheduler(log, orderSvc)
	go orderScheduler.Start(ctx)
	log.Info("app: order scheduler started (scheduled delivery flow)")

	groupOrderSvc := ordering.NewGroupOrderService(orderingRepo, log)

	// Create ordering handlers
	cartHandler := orderinghandler.NewCartHandler(log, cartSvc)
	orderHandler := orderinghandler.NewOrderHandler(log, orderSvc)
	promoHandler := orderinghandler.NewPromoHandler(log, promoSvc, cartSvc)
	loyaltyHandler := orderinghandler.NewLoyaltyHandler(log, loyaltySvc)
	addressHandler := orderinghandler.NewAddressHandler(log, addressSvc)
	groupOrderHandler := orderinghandler.NewGroupOrderHandler(log, groupOrderSvc)

	// Initialize payments module (treasury-api is source of truth; ordering only keeps payment_intent_id on Order)
	treasuryClient := treasury.NewClient(cfg.Treasury, log)
	orderSvc.SetTreasuryClient(treasuryClient)
	paymentsRepo := payments.NewTreasuryRepository(treasuryClient)
	paymentSvc := payments.NewPaymentService(paymentsRepo, treasuryClient, log)
	paymentMethodSvc := payments.NewPaymentMethodService(paymentsRepo, log)
	paymentWebhookSvc := payments.NewWebhookService(paymentsRepo, cfg.Treasury.WebhookSecret, log)

	// Create payment handlers
	paymentHandler := paymentshandler.NewPaymentHandler(log, paymentSvc)
	paymentMethodHandler := paymentshandler.NewPaymentMethodHandler(log, paymentMethodSvc)
	paymentWebhookHandler := paymentshandler.NewWebhookHandler(log, paymentWebhookSvc)

	// Initialize fulfilment module
	logisticsClient := logistics.NewClient(cfg.Logistics, log)
	fulfilmentRepo := fulfilment.NewEntRepository(ormClient)
	taskSvc := fulfilment.NewTaskService(fulfilmentRepo, logisticsClient, log)
	fulfilmentWebhookSvc := fulfilment.NewWebhookService(fulfilmentRepo, cfg.Logistics.WebhookSecret, log)

	// Create fulfilment handlers
	fulfilmentTaskHandler := fulfilmenthandler.NewTaskHandler(log, taskSvc)
	fulfilmentWebhookHandler := fulfilmenthandler.NewWebhookHandler(log, fulfilmentWebhookSvc)

	// Initialize external notifications client (for sending to notifications-service)
	notificationsClient := extnotifications.NewClient(cfg.Notifications, log)
	_ = notificationsClient // Available for use in services that need to send notifications

	// Initialize event publisher for NATS events
	var eventPublisher *events.Publisher
	var outboxPublisher *outbox.Publisher
	if natsConn != nil {
		eventPublisher = events.NewPublisher(natsConn, log)
		log.Info("app: event publisher initialized")

		// Wire event publisher to order service for publishing order events
		orderSvc.SetEventPublisher(eventPublisher)

		// Subscribe to order events for automatic delivery task creation
		fulfilmentEventHandler := fulfilment.NewEventHandler(taskSvc, orderSvc, orderingRepo, log)
		if err := fulfilmentEventHandler.SubscribeToOrderEvents(natsConn); err != nil {
			log.Warn("app: failed to subscribe to order events for fulfilment", zap.Error(err))
		}

		// Subscribe to inventory events for catalog projection sync
		inventoryEventHandler := catalog.NewInventoryEventHandler(ormClient, log)
		if err := inventoryEventHandler.SubscribeToInventoryEvents(natsConn); err != nil {
			log.Warn("app: failed to subscribe to inventory events for catalog sync", zap.Error(err))
		}

		// Subscribe to inventory stock-out and item-updated events
		stockEventHandler := catalog.NewStockEventHandler(ormClient, log)
		if err := stockEventHandler.SubscribeToStockEvents(natsConn); err != nil {
			log.Warn("app: failed to subscribe to stock events", zap.Error(err))
		}

		// Subscribe to logistics task events for order auto-completion and assignment
		logisticsEventHandler := fulfilment.NewLogisticsEventHandler(fulfilmentRepo, orderSvc, orderingRepo, eventPublisher, log)
		if err := logisticsEventHandler.SubscribeToLogisticsEvents(natsConn); err != nil {
			log.Warn("app: failed to subscribe to logistics events", zap.Error(err))
		}

		// Initialize outbox background publisher (Transactional Outbox Pattern)
		if cfg.Events.OutboxEnabled {
			outboxRepo := eventslib.NewSQLOutboxRepository(sqlDB)
			outboxNatsPublisher := events.NewOutboxPublisher(natsConn, log)
			outboxConfig := outbox.PublisherConfig{
				BatchSize:  cfg.Events.OutboxBatchSize,
				PollPeriod: cfg.Events.OutboxPollPeriod,
			}
			outboxPublisher = outbox.NewPublisher(outboxRepo, outboxNatsPublisher, log, outboxConfig)
			outboxPublisher.Start(ctx)
			log.Info("app: outbox background publisher started",
				zap.Int("batch_size", cfg.Events.OutboxBatchSize),
				zap.Duration("poll_period", cfg.Events.OutboxPollPeriod))
		}
	}

	// Initialize notifications module (notifications-api is source of truth; no local storage)
	notificationsRepo := notifications.NewStubRepository()
	notificationsSvc := notifications.NewService(notificationsRepo, log)
	notificationsHandler := notificationshandler.NewHandler(log, notificationsSvc)

	// Initialize SLA module
	slaRepo := sla.NewEntRepository(ormClient)
	slaSvc := sla.NewService(slaRepo, log)
	slaHandler := slahandler.NewHandler(log, slaSvc)

	// Initialize analytics module (Superset integration)
	supersetClient := superset.NewClient(cfg.Superset, log)
	analyticsSvc := analytics.NewService(supersetClient, cfg.Superset, log)
	analyticsHandler := analyticshandler.NewHandler(log, analyticsSvc)

	// Initialize compliance module (GDPR/DPA data export and deletion)
	complianceRepo := compliance.NewEntRepository(ormClient)
	complianceSvc := compliance.NewService(complianceRepo, log)
	complianceHandler := compliancehandler.NewHandler(log, complianceSvc)

	// Initialize security module (rate limiting)
	rateLimitConfig := security.RateLimitConfig{
		RequestsPerMinute:        cfg.Security.RateLimitRequestsPerMin,
		RequestsPerHour:          cfg.Security.RateLimitRequestsPerHour,
		AuthRequestsPerMinute:    cfg.Security.RateLimitAuthPerMin,
		PaymentRequestsPerMinute: cfg.Security.RateLimitPaymentPerMin,
		BurstMultiplier:          cfg.Security.RateLimitBurstMultiplier,
		KeyPrefix:                cfg.Security.RateLimitKeyPrefix,
		Enabled:                  cfg.Security.RateLimitEnabled,
	}
	rateLimiter := security.NewRateLimiter(redisClient, rateLimitConfig, log)
	log.Info("app: security rate limiter initialized",
		zap.Bool("enabled", cfg.Security.RateLimitEnabled),
		zap.Int("requests_per_min", cfg.Security.RateLimitRequestsPerMin))

	// Initialize audit logging module (compliance requirement)
	auditLogger := audit.New(ormClient, log)
	log.Info("app: audit logger initialized")

	mediaHandler := handlers.NewMediaHandler(log, cfg)
	zonesHandler := zoneshandler.New(log, ormClient)

	// Initialize RBAC module
	rbacRepo := rbac.NewEntRepository(ormClient)
	rbacSvc := rbac.NewService(rbacRepo, log, tenantSyncer)
	rbacHandler := handlers.NewRBACHandler(log, rbacSvc, rbacRepo)

	router := httprouter.New(log, healthHandler, cfg.Media.Root, configHandler, identityHandler, catalogHandler, cartHandler, orderHandler, promoHandler, loyaltyHandler, addressHandler, groupOrderHandler, paymentHandler, paymentMethodHandler, paymentWebhookHandler, fulfilmentTaskHandler, fulfilmentWebhookHandler, notificationsHandler, slaHandler, analyticsHandler, complianceHandler, zonesHandler, authenticator, authMiddleware, rateLimiter, auditLogger, cfg.Security, cfg.HTTP.AllowedOrigins, mediaHandler, rbacHandler, tenantSyncer)

	httpServer := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", cfg.HTTP.Host, cfg.HTTP.Port),
		Handler:           router,
		ReadTimeout:       cfg.HTTP.ReadTimeout,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      cfg.HTTP.WriteTimeout,
		IdleTimeout:       cfg.HTTP.IdleTimeout,
	}

	return &App{
		cfg:             cfg,
		log:             log,
		httpServer:      httpServer,
		db:              dbPool,
		cache:           redisClient,
		events:          natsConn,
		orm:             ormClient,
		outboxPublisher: outboxPublisher,
	}, nil
}

// Run starts the HTTP server and blocks until context cancellation.
func (a *App) Run(ctx context.Context) error {
	a.log.Info("starting http server", zap.String("addr", a.httpServer.Addr))

	errCh := make(chan error, 1)
	go func() {
		errCh <- a.httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := a.httpServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("app: http shutdown: %w", err)
		}

		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("app: http server error: %w", err)
	}
}

// Close releases infrastructure resources.
func (a *App) Close() {
	// Stop outbox publisher first (before NATS connection)
	if a.outboxPublisher != nil {
		a.outboxPublisher.Stop()
		a.log.Info("outbox publisher stopped")
	}

	if a.events != nil {
		if err := a.events.Drain(); err != nil {
			a.log.Warn("failed to drain nats connection", zap.Error(err))
		}
		a.events.Close()
	}

	if a.cache != nil {
		if err := a.cache.Close(); err != nil {
			a.log.Warn("failed to close redis client", zap.Error(err))
		}
	}

	if a.db != nil {
		a.db.Close()
	}

	if a.orm != nil {
		if err := a.orm.Close(); err != nil {
			a.log.Warn("failed to close ent client", zap.Error(err))
		}
	}

	a.log.Sync() //nolint:errcheck // ignore sync error
}
