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
	_ "github.com/jackc/pgx/v5/stdlib"
	"go.uber.org/zap"

	authclient "github.com/Bengo-Hub/shared-auth-client"
	"github.com/bengobox/ordering-backend/internal/config"
	"github.com/bengobox/ordering-backend/internal/ent"
	handlers "github.com/bengobox/ordering-backend/internal/http/handlers"
	cataloghandler "github.com/bengobox/ordering-backend/internal/http/handlers/catalog"
	fulfilmenthandler "github.com/bengobox/ordering-backend/internal/http/handlers/fulfilment"
	identityhandler "github.com/bengobox/ordering-backend/internal/http/handlers/identity"
	notificationshandler "github.com/bengobox/ordering-backend/internal/http/handlers/notifications"
	orderinghandler "github.com/bengobox/ordering-backend/internal/http/handlers/ordering"
	paymentshandler "github.com/bengobox/ordering-backend/internal/http/handlers/payments"
	slahandler "github.com/bengobox/ordering-backend/internal/http/handlers/sla"
	httprouter "github.com/bengobox/ordering-backend/internal/http/router"
	"github.com/bengobox/ordering-backend/internal/modules/catalog"
	"github.com/bengobox/ordering-backend/internal/modules/fulfilment"
	"github.com/bengobox/ordering-backend/internal/modules/identity"
	"github.com/bengobox/ordering-backend/internal/modules/notifications"
	"github.com/bengobox/ordering-backend/internal/modules/ordering"
	"github.com/bengobox/ordering-backend/internal/modules/payments"
	"github.com/bengobox/ordering-backend/internal/modules/sla"
	"github.com/bengobox/ordering-backend/internal/platform/cache"
	"github.com/bengobox/ordering-backend/internal/platform/database"
	"github.com/bengobox/ordering-backend/internal/platform/events"
	"github.com/bengobox/ordering-backend/internal/platform/logistics"
	"github.com/bengobox/ordering-backend/internal/platform/treasury"
	"github.com/bengobox/ordering-backend/internal/shared/logger"
)

type App struct {
	cfg        *config.Config
	log        *zap.Logger
	httpServer *http.Server
	db         dbCloser
	cache      cacheCloser
	events     eventCloser
	orm        *ent.Client
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
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetConnMaxIdleTime(5 * time.Minute)

	drv := entsql.OpenDB(dialect.Postgres, sqlDB)
	ormClient := ent.NewClient(ent.Driver(drv))
	if err := ormClient.Schema.Create(ctx); err != nil {
		return nil, fmt.Errorf("app: ent schema create: %w", err)
	}

	// Note: Seeding should be done manually via 'go run cmd/seed/main.go' after migrations
	// This ensures roles, permissions, tenant, and demo user are always available
	// The seed command is idempotent and can be run multiple times safely
	log.Info("app: migrations completed - run 'go run cmd/seed/main.go' to seed initial data (idempotent)")

	identityRepo := identity.NewEntRepository(ormClient)
	identitySvc, err := identity.NewService(identityRepo, cfg.Auth, log)
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
		}
	}

	identityHandler := identityhandler.New(log, identitySvc)
	authenticator := identityhandler.NewAuthenticator(log, identitySvc, validator)

	// Initialize catalog module
	catalogRepo := catalog.NewEntRepository(ormClient)
	catalogSvc := catalog.NewService(catalogRepo, log)
	catalogHandler := cataloghandler.New(log, catalogSvc)

	// Initialize ordering module
	orderingRepo := ordering.NewEntRepository(ormClient)
	cartSvc := ordering.NewCartService(orderingRepo, catalogSvc, log)
	promoSvc := ordering.NewPromoService(orderingRepo, log)
	loyaltySvc := ordering.NewLoyaltyService(orderingRepo, log)
	addressSvc := ordering.NewAddressService(orderingRepo, log)
	orderSvc := ordering.NewOrderService(orderingRepo, cartSvc, promoSvc, loyaltySvc, log)

	// Create ordering handlers
	cartHandler := orderinghandler.NewCartHandler(log, cartSvc)
	orderHandler := orderinghandler.NewOrderHandler(log, orderSvc)
	promoHandler := orderinghandler.NewPromoHandler(log, promoSvc, cartSvc)
	loyaltyHandler := orderinghandler.NewLoyaltyHandler(log, loyaltySvc)
	addressHandler := orderinghandler.NewAddressHandler(log, addressSvc)

	// Initialize payments module
	treasuryClient := treasury.NewClient(cfg.Treasury, log)
	paymentsRepo := payments.NewEntRepository(ormClient)
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

	// Initialize notifications module
	notificationsRepo := notifications.NewEntRepository(ormClient)
	notificationsSvc := notifications.NewService(notificationsRepo, log)
	notificationsHandler := notificationshandler.NewHandler(log, notificationsSvc)

	// Initialize SLA module
	slaRepo := sla.NewEntRepository(ormClient)
	slaSvc := sla.NewService(slaRepo, log)
	slaHandler := slahandler.NewHandler(log, slaSvc)

	router := httprouter.New(log, healthHandler, identityHandler, catalogHandler, cartHandler, orderHandler, promoHandler, loyaltyHandler, addressHandler, paymentHandler, paymentMethodHandler, paymentWebhookHandler, fulfilmentTaskHandler, fulfilmentWebhookHandler, notificationsHandler, slaHandler, authenticator, authMiddleware, cfg.HTTP.AllowedOrigins)

	httpServer := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", cfg.HTTP.Host, cfg.HTTP.Port),
		Handler:           router,
		ReadTimeout:       cfg.HTTP.ReadTimeout,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      cfg.HTTP.WriteTimeout,
		IdleTimeout:       cfg.HTTP.IdleTimeout,
	}

	return &App{
		cfg:        cfg,
		log:        log,
		httpServer: httpServer,
		db:         dbPool,
		cache:      redisClient,
		events:     natsConn,
		orm:        ormClient,
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
