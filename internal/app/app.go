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

	"github.com/bengobox/food-delivery-backend/internal/config"
	"github.com/bengobox/food-delivery-backend/internal/ent"
	handlers "github.com/bengobox/food-delivery-backend/internal/http/handlers"
	identityhandler "github.com/bengobox/food-delivery-backend/internal/http/handlers/identity"
	httprouter "github.com/bengobox/food-delivery-backend/internal/http/router"
	"github.com/bengobox/food-delivery-backend/internal/modules/identity"
	"github.com/bengobox/food-delivery-backend/internal/platform/cache"
	"github.com/bengobox/food-delivery-backend/internal/platform/database"
	"github.com/bengobox/food-delivery-backend/internal/platform/events"
	"github.com/bengobox/food-delivery-backend/internal/shared/logger"
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

	identityRepo := identity.NewEntRepository(ormClient)
	identitySvc, err := identity.NewService(identityRepo, cfg.Auth, log)
	if err != nil {
		return nil, fmt.Errorf("app: identity service init: %w", err)
	}
	identityHandler := identityhandler.New(log, identitySvc)
	authenticator := identityhandler.NewAuthenticator(log, identitySvc)

	router := httprouter.New(log, healthHandler, identityHandler, authenticator)

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
