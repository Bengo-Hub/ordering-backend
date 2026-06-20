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
	"github.com/google/uuid"
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
	fulfilmenthandler "github.com/bengobox/ordering-backend/internal/http/handlers/fulfilment"
	googlebusinesshandler "github.com/bengobox/ordering-backend/internal/http/handlers/googlebusiness"
	identityhandler "github.com/bengobox/ordering-backend/internal/http/handlers/identity"
	notificationshandler "github.com/bengobox/ordering-backend/internal/http/handlers/notifications"
	orderinghandler "github.com/bengobox/ordering-backend/internal/http/handlers/ordering"
	paymentshandler "github.com/bengobox/ordering-backend/internal/http/handlers/payments"
	slahandler "github.com/bengobox/ordering-backend/internal/http/handlers/sla"
	zoneshandler "github.com/bengobox/ordering-backend/internal/http/handlers/zones"
	httprouter "github.com/bengobox/ordering-backend/internal/http/router"
	"github.com/bengobox/ordering-backend/internal/modules/analytics"
	"github.com/bengobox/ordering-backend/internal/modules/audit"
	backupmod "github.com/bengobox/ordering-backend/internal/modules/backup"
	"github.com/bengobox/ordering-backend/internal/modules/catalog"
	"github.com/bengobox/ordering-backend/internal/modules/compliance"
	"github.com/bengobox/ordering-backend/internal/modules/fulfilment"
	"github.com/bengobox/ordering-backend/internal/modules/googlebusiness"
	"github.com/bengobox/ordering-backend/internal/modules/identity"
	"github.com/bengobox/ordering-backend/internal/modules/notifications"
	"github.com/bengobox/ordering-backend/internal/modules/ordering"
	"github.com/bengobox/ordering-backend/internal/modules/payments"
	"github.com/bengobox/ordering-backend/internal/modules/rbac"
	"github.com/bengobox/ordering-backend/internal/modules/security"
	"github.com/bengobox/ordering-backend/internal/modules/sla"
	"github.com/bengobox/ordering-backend/internal/modules/tenant"
	"github.com/bengobox/ordering-backend/internal/platform/cache"
	"github.com/bengobox/ordering-backend/internal/platform/database"
	"github.com/bengobox/ordering-backend/internal/platform/events"
	"github.com/bengobox/ordering-backend/internal/platform/inventory"
	"github.com/bengobox/ordering-backend/internal/platform/logistics"
	"github.com/bengobox/ordering-backend/internal/platform/marketflow"
	extnotifications "github.com/bengobox/ordering-backend/internal/platform/notifications"
	"github.com/bengobox/ordering-backend/internal/platform/posloyalty"
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
	outboxPublisher *eventslib.OutboxPoller
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
	if cfg.Postgres.RunMigrations {
		if err := ormClient.Schema.Create(ctx, schema.WithDir(migrate.Dir)); err != nil {
			return nil, fmt.Errorf("app: ent schema create: %w", err)
		}
		log.Info("app: versioned migrations applied (POSTGRES_RUN_MIGRATIONS=true)")
	}

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

		// Initialize JetStream durable subscribers for outlet sync and subscription cache.
		if natsConn != nil {
			// Outlet sync: auth.outlet.* JetStream events → local outlet projections.
			// Replaces the legacy plain-NATS auth.tenant.branch.created subscription.
			branchSub := tenant.NewBranchSubscriber(ormClient, log)
			if err := branchSub.Start(natsConn); err != nil {
				log.Error("failed to start outlet event subscriber", zap.Error(err))
			}

			// Invalidate tenant branding cache when subscription changes so new plan
			// is reflected in subsequent JWT-enriched responses without a restart.
			subCacheSub := subscriptions.NewCacheSubscriber(redisClient, "", log)
			if err := subCacheSub.Start(natsConn); err != nil {
				log.Warn("app: failed to start subscription cache subscriber", zap.Error(err))
			}
		}
	}

	// RBAC service is initialized later (line ~376) but we need a reference for the identity handler.
	// Create it early — the rbac repo is created just before the router call anyway.
	rbacRepoForIdentity := rbac.NewEntRepository(ormClient)
	rbacSvcForIdentity := rbac.NewService(rbacRepoForIdentity, log, tenantSyncer)
	identityHandler := identityhandler.New(log, identitySvc, rbacSvcForIdentity)
	authenticator := identityhandler.NewAuthenticator(log, identitySvc, validator)

	// Public tenant/brand config handler (no auth).
	// Uses shared cache (cache-aside via auth-api) for tenant branding.
	tenantCache := sharedcache.New(redisClient, log)
	configHandler := confighandler.New(log, ormClient, tenantCache, cfg.Auth.ServiceURL)

	// Admin service config handler for platform/tenant-level configuration CRUD.
	serviceConfigHandler := confighandler.NewServiceConfigHandler(ormClient, log)

	// Read-only use-case configuration handler (tenant + per-outlet use_case).
	useCaseHandler := confighandler.NewUseCaseHandler(ormClient, log)

	// Google Business Profile integration (reviews + connect). INERT until the operator
	// sets GOOGLE_OAUTH_CLIENT_ID/SECRET/REDIRECT_URL — IsConfigured() gates every endpoint.
	googleOAuthCfg := googlebusiness.NewOAuthConfig(
		cfg.Google.OAuthClientID,
		cfg.Google.OAuthClientSecret,
		cfg.Google.OAuthRedirectURL,
	)
	// Shared credential-encryption KeyProvider (DB-first, env-fallback, dev-last).
	// The same instance backs token encryption-at-rest and the platform encryption-key
	// endpoints so a key rotation invalidates a single cache.
	encryptionKeyProvider := googlebusiness.NewKeyProvider(ormClient)
	googleBusinessSvc := googlebusiness.NewService(ormClient, googleOAuthCfg, cfg.Google.FrontendIntegrationsURL, log, encryptionKeyProvider)
	googleBusinessHandler := googlebusinesshandler.NewHandler(log, googleBusinessSvc)

	// Platform-owner credential-encryption key management (GET/PUT /admin/encryption-key).
	encryptionKeyHandler := confighandler.NewEncryptionKeyHandler(ormClient, encryptionKeyProvider, log)
	if googleOAuthCfg.IsConfigured() {
		log.Info("app: Google Business Profile integration enabled")
	} else {
		log.Info("app: Google Business Profile integration inert (GOOGLE_OAUTH_* not set)")
	}

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
	// pos-api is the loyalty source of truth (balances keyed on tenant + customer_phone). Mirror
	// earn/redeem to it over S2S (best-effort); the local LoyaltyAccount remains a legacy fallback.
	posLoyaltyClient := posloyalty.NewClient(cfg.POS.ServiceURL, cfg.POS.APIKey, log)
	loyaltySvc.SetPOSLoyaltyClient(posLoyaltyClient)
	if posLoyaltyClient.Enabled() {
		log.Info("app: pos-api loyalty mirror enabled (pos-api is loyalty SoT)")
	} else {
		log.Info("app: pos-api loyalty mirror disabled (INTERNAL_SERVICE_KEY unset); using local balance only")
	}
	feeSvc := ordering.NewFeeService(orderingRepo, log)
	addressSvc := ordering.NewAddressService(orderingRepo, log)
	orderSvc := ordering.NewOrderService(orderingRepo, cartSvc, promoSvc, loyaltySvc, feeSvc, inventoryClient, subscriptionsClient, log)

	// Start order scheduler for scheduled delivery flow
	orderScheduler := ordering.NewOrderScheduler(log, orderSvc)
	go orderScheduler.Start(ctx)
	log.Info("app: order scheduler started (scheduled delivery flow)")

	// Start cart cleanup job (expires stale carts every 15 minutes)
	cartCleanupJob := ordering.NewCartCleanupJob(orderingRepo, log)
	go cartCleanupJob.Start(ctx)
	log.Info("app: cart cleanup job started (15-minute interval)")

	groupOrderSvc := ordering.NewGroupOrderService(orderingRepo, log)

	// Create ordering handlers
	cartHandler := orderinghandler.NewCartHandler(log, cartSvc, feeSvc)
	orderHandler := orderinghandler.NewOrderHandler(log, orderSvc)
	promoHandler := orderinghandler.NewPromoHandler(log, promoSvc, cartSvc)
	loyaltyHandler := orderinghandler.NewLoyaltyHandler(log, loyaltySvc)
	addressHandler := orderinghandler.NewAddressHandler(log, addressSvc)
	groupOrderHandler := orderinghandler.NewGroupOrderHandler(log, groupOrderSvc)

	// Initialize payments module (treasury-api is source of truth; ordering only keeps payment_intent_id on Order)
	treasuryClient := treasury.NewClient(cfg.Treasury, log)
	orderSvc.SetTreasuryClient(treasuryClient)

	// MarketFlow CRM client — single source of truth for customer contact data. Disabled (no-op)
	// when MARKETFLOW_API_URL is unset; ordering then creates orders without a crm_contact_id.
	crmClient := marketflow.NewClient(cfg.Marketflow.ServiceURL, cfg.Marketflow.APIKey, log)
	orderSvc.SetCrmClient(crmClient)
	if crmClient.Enabled() {
		log.Info("app: marketflow CRM client enabled", zap.String("url", cfg.Marketflow.ServiceURL))
	}
	paymentsRepo := payments.NewTreasuryRepository(treasuryClient)
	paymentSvc := payments.NewPaymentService(paymentsRepo, treasuryClient, log)
	paymentMethodSvc := payments.NewPaymentMethodService(paymentsRepo, log)
	paymentWebhookSvc := payments.NewWebhookService(paymentsRepo, cfg.Treasury.WebhookSecret, log)

	// Wire payment poller so stale pending-payment orders are auto-resolved
	paymentSvc.SetOrderingRepo(orderingRepo)
	// Shared confirmation handler: used by BOTH the event-driven treasury.payment.succeeded consumer
	// (instant) and the poller fallback. Idempotent (UpdatePaymentStatus no-ops if already paid).
	onPaymentSuccess := func(ctx context.Context, tenantID, orderID uuid.UUID) error {
		_, err := orderSvc.UpdatePaymentStatus(ctx, tenantID, orderID, ordering.PaymentStatusPaid, nil)
		return err
	}
	paymentSvc.SetPaymentSuccessCallback(onPaymentSuccess)
	paymentSvc.SetPaymentFailedCallback(func(ctx context.Context, tenantID, orderID uuid.UUID, errMsg string) error {
		_, err := orderSvc.CancelOrder(ctx, tenantID, orderID, "payment_failed: "+errMsg, nil, "system", "")
		return err
	})
	go paymentSvc.StartPaymentPolling(ctx)
	log.Info("app: payment polling started (2-minute interval, 5-minute staleness cutoff)")

	// Create payment handlers
	paymentHandler := paymentshandler.NewPaymentHandler(log, paymentSvc)
	paymentMethodHandler := paymentshandler.NewPaymentMethodHandler(log, paymentMethodSvc, treasuryClient)
	paymentWebhookHandler := paymentshandler.NewWebhookHandler(log, paymentWebhookSvc)

	// Initialize fulfilment module
	logisticsClient := logistics.NewClient(cfg.Logistics, log)
	orderSvc.SetLogisticsClient(logisticsClient)
	fulfilmentRepo := fulfilment.NewEntRepository(ormClient)
	taskSvc := fulfilment.NewTaskService(fulfilmentRepo, logisticsClient, log)
	orderHandler.SetTaskService(taskSvc)
	fulfilmentWebhookSvc := fulfilment.NewWebhookService(fulfilmentRepo, cfg.Logistics.WebhookSecret, log)

	// Create fulfilment handlers
	fulfilmentTaskHandler := fulfilmenthandler.NewTaskHandler(log, taskSvc, orderSvc)
	fulfilmentWebhookHandler := fulfilmenthandler.NewWebhookHandler(log, fulfilmentWebhookSvc)

	// Initialize external notifications client (for sending to notifications-service)
	notificationsClient := extnotifications.NewClient(cfg.Notifications, log)
	_ = notificationsClient // Available for use in services that need to send notifications

	// Initialize event publisher for NATS JetStream events
	var eventPublisher *events.Publisher
	var outboxPublisher *eventslib.OutboxPoller
	if natsConn != nil {
		// Ensure the "ordering" JetStream stream exists for durable event delivery
		if err := events.EnsureStream(ctx, natsConn, cfg.Events); err != nil {
			log.Warn("app: failed to ensure ordering JetStream stream (non-fatal)", zap.Error(err))
		} else {
			log.Info("app: ordering JetStream stream ensured", zap.String("stream", cfg.Events.StreamName))
		}

		// Initialize JetStream context for publishing
		js, jsErr := natsConn.JetStream()
		if jsErr != nil {
			log.Warn("app: failed to init JetStream context", zap.Error(jsErr))
		}

		if js != nil {
			eventPublisher = events.NewPublisher(js, log)
			log.Info("app: JetStream event publisher initialized")
		}

		// Wire event publisher to order service for publishing order events
		orderSvc.SetEventPublisher(eventPublisher)

		if js != nil {
			// Subscribe to auth-service events for user sync (JetStream durable consumers)
			authEventHandler := identity.NewEventHandler(identitySvc, log)
			if err := authEventHandler.SubscribeToAuthEvents(js); err != nil {
				log.Warn("app: failed to subscribe to auth events", zap.Error(err))
			}

			// Subscribe to order events for automatic delivery task creation
			fulfilmentEventHandler := fulfilment.NewEventHandler(taskSvc, orderSvc, orderingRepo, log)
			if err := fulfilmentEventHandler.SubscribeToOrderEvents(js); err != nil {
				log.Warn("app: failed to subscribe to order events for fulfilment", zap.Error(err))
			}

			// consumerFeatureGate restricts cross-service data sync (catalog projection,
			// treasury payment confirmation) to entitled tenants, mirroring HTTP-layer gating.
			// Cached per tenant; fails open on subscriptions-api outage.
			consumerFeatureGate := func(ctx context.Context, tenantID, feature string) bool {
				return subscriptionsClient.ConsumerHasFeature(ctx, tenantID, feature)
			}

			// Subscribe to inventory events for catalog projection sync
			inventoryEventHandler := catalog.NewInventoryEventHandler(ormClient, log)
			inventoryEventHandler.SetFeatureGate(consumerFeatureGate)
			if err := inventoryEventHandler.SubscribeToInventoryEvents(js); err != nil {
				log.Warn("app: failed to subscribe to inventory events for catalog sync", zap.Error(err))
			}

			// Subscribe to inventory stock-out and item-updated events
			stockEventHandler := catalog.NewStockEventHandler(ormClient, log)
			if err := stockEventHandler.SubscribeToStockEvents(js); err != nil {
				log.Warn("app: failed to subscribe to stock events", zap.Error(err))
			}

			// Subscribe to logistics task events for order auto-completion and assignment
			logisticsEventHandler := fulfilment.NewLogisticsEventHandler(fulfilmentRepo, orderSvc, orderingRepo, eventPublisher, log)
			// Wire treasury client so COD payments are settled when delivery is confirmed
			logisticsEventHandler.SetTreasuryClient(treasuryClient)
			if err := logisticsEventHandler.SubscribeToLogisticsEvents(js); err != nil {
				log.Warn("app: failed to subscribe to logistics events", zap.Error(err))
			}

			// Subscribe to treasury.payment.succeeded for INSTANT, event-driven order confirmation
			// (replaces reliance on the 5-minute poller; poller stays as a durable fallback).
			treasuryPaymentConsumer := payments.NewTreasuryPaymentConsumer(log, onPaymentSuccess)
			treasuryPaymentConsumer.SetFeatureGate(consumerFeatureGate)
			go func() {
				if err := treasuryPaymentConsumer.Start(ctx, js); err != nil {
					log.Error("app: treasury payment consumer stopped", zap.Error(err))
				}
			}()
			log.Info("app: treasury payment consumer started (treasury.payment.succeeded)")

			// Subscribe to pos-api events (shared-events "pos" stream): pos.kds.order.ready advances
			// the online order to ready; pos.online_order.collected completes it. Reuses the order
			// status-update service method so customer notifications fire automatically.
			posEventConsumer := ordering.NewPOSEventConsumer(log, orderSvc, orderingRepo)
			if err := posEventConsumer.Start(ctx, js); err != nil {
				log.Warn("app: failed to start pos event consumer", zap.Error(err))
			} else {
				log.Info("app: pos event consumer started (pos.kds.order.ready, pos.online_order.collected)")
			}

			// Subscribe to tenant.purge (subscriptions-api, "tenant" aggregate): on a
			// platform-owner-confirmed dormancy purge, IRREVERSIBLY delete all of the tenant's
			// ordering-backend data. The consumer is defensive: it acts only on a confirmed
			// dormancy event and scopes every delete strictly by tenant_id.
			tenantPurgeConsumer := tenant.NewPurgeConsumer(ormClient, log)
			go func() {
				if err := tenantPurgeConsumer.Start(ctx, js); err != nil {
					log.Error("app: tenant purge consumer stopped", zap.Error(err))
				}
			}()
			log.Info("app: tenant purge consumer started (tenant.purge)")
		}

		// Initialize outbox background publisher (Transactional Outbox Pattern)
		if cfg.Events.OutboxEnabled && js != nil {
			outboxRepo := eventslib.NewSQLOutboxRepository(sqlDB)
			outboxJSPublisher := eventslib.NewJetStreamAdapter(js, log)
			outboxConfig := eventslib.PollerConfig{
				BatchSize:  cfg.Events.OutboxBatchSize,
				PollPeriod: cfg.Events.OutboxPollPeriod,
			}
			outboxPublisher = eventslib.NewOutboxPoller(outboxRepo, outboxJSPublisher, log, outboxConfig)
			outboxPublisher.Start(ctx)
			log.Info("app: outbox background publisher started (JetStream)",
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
	rbacHandler := handlers.NewRBACHandler(log, rbacSvc, rbacRepo, identityRepo)

	// Pluggable backup destination (OneDrive/GDrive/S3/WebDAV/SFTP/SMB) — encrypted
	// at rest with a SECRET_KEY-derived key. The handler owns the destination Store;
	// its Uploader is attached to the backup service so every PVC backup is
	// additionally mirrored best-effort.
	backupDestHandler := handlers.NewBackupDestinationHandler(ormClient, log)

	// Tenant-scoped backups + daily 02:00 auto-backup scheduler + retention churn.
	backupSvc := backupmod.NewService(sqlDB, ormClient, cfg.Backup.Dir, log).
		WithMirrorer(backupDestHandler.Uploader())
	backupsHandler := handlers.NewBackupsHandler(log, backupSvc, cfg.Backup.RetentionDays)
	backupmod.NewScheduler(backupSvc, backupmod.SchedulerConfig{
		Enabled:       cfg.Backup.ScheduleEnabled,
		Hour:          cfg.Backup.ScheduleHour,
		RetentionDays: cfg.Backup.RetentionDays,
	}, log).Start(ctx)

	router := httprouter.New(log, healthHandler, cfg.Media.Root, configHandler, identityHandler, catalogHandler, cartHandler, orderHandler, promoHandler, loyaltyHandler, addressHandler, groupOrderHandler, paymentHandler, paymentMethodHandler, paymentWebhookHandler, fulfilmentTaskHandler, fulfilmentWebhookHandler, notificationsHandler, slaHandler, analyticsHandler, complianceHandler, zonesHandler, authenticator, authMiddleware, rateLimiter, auditLogger, cfg.Security, cfg.HTTP.AllowedOrigins, mediaHandler, rbacHandler, tenantSyncer, serviceConfigHandler, useCaseHandler, googleBusinessHandler, backupsHandler, backupDestHandler, encryptionKeyHandler)

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
