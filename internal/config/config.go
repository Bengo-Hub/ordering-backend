package config

import (
	"fmt"
	"time"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

const namespace = ""

// DefaultTenantSlug is the default tenant slug for the ordering service (e.g. fallback for UI or default org).
// App context default is urban-loft; codevertex is the platform owner tenant.
const DefaultTenantSlug = "urban-loft"

// Config captures environment configuration for the Ordering service backend.
// Note: Empty envconfig tags on nested structs prevent field names from being added to the key prefix.
// Without empty tags, envconfig adds field names to prefix, e.g., Postgres.URL becomes ORDERING_POSTGRES_POSTGRES_URL.
// With empty tags, it becomes ORDERING_POSTGRES_URL as expected.
type Config struct {
	App           AppConfig           `envconfig:""`
	HTTP          HTTPConfig          `envconfig:""`
	Postgres      PostgresConfig      `envconfig:""`
	Redis         RedisConfig         `envconfig:""`
	Events        EventsConfig        `envconfig:""`
	Telemetry     TelemetryConfig     `envconfig:""`
	Auth          AuthConfig          `envconfig:""`
	Treasury      TreasuryConfig      `envconfig:""`
	Logistics     LogisticsConfig     `envconfig:""`
	Inventory     InventoryConfig     `envconfig:""`
	Notifications  NotificationsConfig  `envconfig:""`
	Subscriptions  SubscriptionsConfig  `envconfig:""`
	Superset       SupersetConfig       `envconfig:""`
	Security      SecurityConfig      `envconfig:""`
	Media         MediaConfig         `envconfig:""`
}

// MediaConfig configures local media storage for menu item images and uploads.
// Production: set MEDIA_ROOT to a persistent volume path (e.g. /media). Local: ./media or empty to disable.
type MediaConfig struct {
	// Root is the filesystem path for uploaded and local media (menu images, avatars). Empty disables local file serving.
	Root string `envconfig:"MEDIA_ROOT" default:"./media"`
	// URLBase is the base URL for serving media (e.g. https://orderingapi.codevertexitsolutions.com/media). Empty means relative /media.
	URLBase string `envconfig:"MEDIA_URL_BASE"`
}

type AppConfig struct {
	Name    string `envconfig:"APP_NAME" default:"ordering-backend"`
	Env     string `envconfig:"APP_ENV" default:"development"`
	Version string `envconfig:"APP_VERSION" default:"0.1.0"`
}

type HTTPConfig struct {
	Host         string        `envconfig:"HTTP_HOST" default:"0.0.0.0"`
	Port         int           `envconfig:"HTTP_PORT" default:"4005"`
	ReadTimeout  time.Duration `envconfig:"HTTP_READ_TIMEOUT" default:"15s"`
	WriteTimeout time.Duration `envconfig:"HTTP_WRITE_TIMEOUT" default:"15s"`
	IdleTimeout  time.Duration `envconfig:"HTTP_IDLE_TIMEOUT" default:"60s"`
	// Browser origins that may call this API (frontends). Not the API host itself.
	AllowedOrigins []string `envconfig:"HTTP_ALLOWED_ORIGINS" default:"https://ordersapp.codevertexitsolutions.com,https://theurbanloftcafe.com,https://pos.codevertexitsolutions.com,https://accounts.codevertexitsolutions.com,https://notifications.codevertexitsolutions.com,http://localhost:3000,http://127.0.0.1:3000,http://localhost:3001,http://127.0.0.1:3001"`
}

type PostgresConfig struct {
	URL                      string        `envconfig:"POSTGRES_URL" default:"postgres://postgres:postgres@localhost:5432/ordering?sslmode=disable"`
	MaxOpenConns             int           `envconfig:"POSTGRES_MAX_OPEN_CONNS" default:"5"`
	MaxIdleConns             int           `envconfig:"POSTGRES_MAX_IDLE_CONNS" default:"3"`
	ConnMaxLifetime          time.Duration `envconfig:"POSTGRES_CONN_MAX_LIFETIME" default:"5m"`
	StatementTimeout         time.Duration `envconfig:"POSTGRES_STATEMENT_TIMEOUT" default:"30s"`
	IdleInTransactionTimeout time.Duration `envconfig:"POSTGRES_IDLE_IN_TRANSACTION_TIMEOUT" default:"60s"`
	RunMigrations            bool          `envconfig:"POSTGRES_RUN_MIGRATIONS" default:"false"`
}

type RedisConfig struct {
	Addr     string `envconfig:"REDIS_ADDR" default:"localhost:6379"`
	Username string `envconfig:"REDIS_USERNAME"`
	Password string `envconfig:"REDIS_PASSWORD"`
	DB       int    `envconfig:"REDIS_DB" default:"0"`
}

type EventsConfig struct {
	// EVENTS_NATS_URL standardized across all Go services (ordering loads Events via Process("", &cfg.Events) so tag is full key)
	NATSURL    string `envconfig:"EVENTS_NATS_URL" default:"nats://127.0.0.1:4222"`
	StreamName string `envconfig:"NATS_STREAM" default:"ordering"`

	// Outbox publisher settings
	OutboxEnabled    bool          `envconfig:"OUTBOX_ENABLED" default:"true"`
	OutboxPollPeriod time.Duration `envconfig:"OUTBOX_POLL_PERIOD" default:"2s"`
	OutboxBatchSize  int           `envconfig:"OUTBOX_BATCH_SIZE" default:"100"`
}

type TelemetryConfig struct {
	OTLPEndpoint string `envconfig:"OTLP_ENDPOINT"`
}

type AuthConfig struct {
	// Auth-service SSO integration (Production: https://sso.codevertexitsolutions.com/)
	ServiceURL          string        `envconfig:"AUTH_SERVICE_URL" default:"https://sso.codevertexitsolutions.com"`
	Issuer              string        `envconfig:"AUTH_ISSUER" default:"https://sso.codevertexitsolutions.com"`
	Audience            string        `envconfig:"AUTH_AUDIENCE" default:"codevertex"`
	JWKSUrl             string        `envconfig:"AUTH_JWKS_URL" default:"https://sso.codevertexitsolutions.com/api/v1/.well-known/jwks.json"`
	JWKSCacheTTL        time.Duration `envconfig:"AUTH_JWKS_CACHE_TTL" default:"3600s"`
	JWKSRefreshInterval time.Duration `envconfig:"AUTH_JWKS_REFRESH_INTERVAL" default:"300s"`
	EnableAPIKeyAuth    bool          `envconfig:"AUTH_ENABLE_API_KEY_AUTH" default:"true"`
	AuthServiceAPIKey   string        `envconfig:"INTERNAL_SERVICE_KEY"`

	// Legacy config (deprecated - use auth-service instead)
	AccessTokenSecret  string        `envconfig:"AUTH_ACCESS_TOKEN_SECRET" default:"dev-access-secret-change-me"`
	RefreshTokenSecret string        `envconfig:"AUTH_REFRESH_TOKEN_SECRET" default:"dev-refresh-secret-change-me"`
	AccessTokenTTL     time.Duration `envconfig:"AUTH_ACCESS_TOKEN_TTL" default:"15m"`
	RefreshTokenTTL    time.Duration `envconfig:"AUTH_REFRESH_TOKEN_TTL" default:"720h"`
	GoogleClientID     string        `envconfig:"AUTH_GOOGLE_CLIENT_ID"`
	GoogleClientSecret string        `envconfig:"AUTH_GOOGLE_CLIENT_SECRET"`
	GoogleRedirectBase string        `envconfig:"AUTH_GOOGLE_REDIRECT_BASE" default:"https://ordersapp.codevertexitsolutions.com/auth/callback"`
	TwoFactorIssuer    string        `envconfig:"AUTH_TWO_FACTOR_ISSUER" default:"Ordering Platform"`
}

type TreasuryConfig struct {
	// Treasury service URL
	ServiceURL     string        `envconfig:"TREASURY_API_URL" default:"http://localhost:4010"`
	APIKey         string        `envconfig:"INTERNAL_SERVICE_KEY"`
	WebhookSecret  string        `envconfig:"TREASURY_WEBHOOK_SECRET"`
	RequestTimeout time.Duration `envconfig:"TREASURY_REQUEST_TIMEOUT" default:"30s"`

	// M-Pesa configuration (via treasury service)
	MpesaEnabled         bool   `envconfig:"MPESA_ENABLED" default:"true"`
	MpesaCallbackBaseURL string `envconfig:"MPESA_CALLBACK_BASE_URL" default:"http://localhost:4000/api/v1/webhooks/mpesa"`
}

type LogisticsConfig struct {
	// Logistics service URL
	ServiceURL     string        `envconfig:"LOGISTICS_SERVICE_URL" default:"http://localhost:4003"`
	APIKey         string        `envconfig:"INTERNAL_SERVICE_KEY"`
	WebhookSecret  string        `envconfig:"LOGISTICS_WEBHOOK_SECRET"`
	RequestTimeout time.Duration `envconfig:"LOGISTICS_REQUEST_TIMEOUT" default:"30s"`

	// WebSocket configuration for live tracking
	WebSocketURL string `envconfig:"LOGISTICS_WEBSOCKET_URL" default:"ws://localhost:4005/ws"`
}

type InventoryConfig struct {
	// Inventory service URL
	ServiceURL     string        `envconfig:"INVENTORY_SERVICE_URL" default:"http://localhost:4001"`
	APIKey         string        `envconfig:"INTERNAL_SERVICE_KEY"`
	RequestTimeout time.Duration `envconfig:"INVENTORY_REQUEST_TIMEOUT" default:"10s"`
}

type NotificationsConfig struct {
	// Notifications service URL
	ServiceURL     string        `envconfig:"NOTIFICATIONS_SERVICE_URL" default:"http://localhost:4004"`
	APIKey         string        `envconfig:"INTERNAL_SERVICE_KEY"`
	RequestTimeout time.Duration `envconfig:"NOTIFICATIONS_REQUEST_TIMEOUT" default:"10s"`
}

type SubscriptionsConfig struct {
	// Subscriptions service URL
	ServiceURL     string        `envconfig:"SUBSCRIPTIONS_SERVICE_URL" default:"http://localhost:4008"`
	APIKey         string        `envconfig:"INTERNAL_SERVICE_KEY"`
	RequestTimeout time.Duration `envconfig:"SUBSCRIPTIONS_REQUEST_TIMEOUT" default:"10s"`
}

type SupersetConfig struct {
	// Superset service URL
	BaseURL        string        `envconfig:"SUPERSET_BASE_URL" default:"https://superset.codevertexitsolutions.com"`
	AdminUsername  string        `envconfig:"SUPERSET_ADMIN_USERNAME" default:"admin"`
	AdminPassword  string        `envconfig:"SUPERSET_ADMIN_PASSWORD"`
	APIVersion     string        `envconfig:"SUPERSET_API_VERSION" default:"v1"`
	RequestTimeout time.Duration `envconfig:"SUPERSET_REQUEST_TIMEOUT" default:"30s"`

	// Guest token settings
	GuestTokenTTLMinutes int `envconfig:"SUPERSET_GUEST_TOKEN_TTL_MINUTES" default:"5"`

	// Dashboard IDs (configured per module)
	OrderAnalyticsDashboardID    int `envconfig:"SUPERSET_ORDER_ANALYTICS_DASHBOARD_ID"`
	RevenueDashboardID           int `envconfig:"SUPERSET_REVENUE_DASHBOARD_ID"`
	CustomerAnalyticsDashboardID int `envconfig:"SUPERSET_CUSTOMER_ANALYTICS_DASHBOARD_ID"`
	OperationsDashboardID        int `envconfig:"SUPERSET_OPERATIONS_DASHBOARD_ID"`
	SubscriptionDashboardID      int `envconfig:"SUPERSET_SUBSCRIPTION_DASHBOARD_ID"`
}

type SecurityConfig struct {
	// Rate limiting
	RateLimitEnabled         bool    `envconfig:"RATE_LIMIT_ENABLED" default:"true"`
	RateLimitRequestsPerMin  int     `envconfig:"RATE_LIMIT_REQUESTS_PER_MINUTE" default:"200"`
	RateLimitRequestsPerHour int     `envconfig:"RATE_LIMIT_REQUESTS_PER_HOUR" default:"5000"`
	RateLimitAuthPerMin      int     `envconfig:"RATE_LIMIT_AUTH_PER_MINUTE" default:"10"`
	RateLimitPaymentPerMin   int     `envconfig:"RATE_LIMIT_PAYMENT_PER_MINUTE" default:"20"`
	RateLimitBurstMultiplier float64 `envconfig:"RATE_LIMIT_BURST_MULTIPLIER" default:"1.5"`
	RateLimitKeyPrefix       string  `envconfig:"RATE_LIMIT_KEY_PREFIX" default:"rl:ordering:"`

	// Request limits
	MaxRequestBodySize int64 `envconfig:"MAX_REQUEST_BODY_SIZE" default:"10485760"` // 10MB

	// Input validation
	InputValidationEnabled bool `envconfig:"INPUT_VALIDATION_ENABLED" default:"true"`

	// Security headers
	SecurityHeadersEnabled bool `envconfig:"SECURITY_HEADERS_ENABLED" default:"true"`
}

// Load reads configuration from environment variables and optional .env files.
// Each nested config struct is processed separately to avoid double-prefix issues
// (e.g., ORDERING_POSTGRES_URL instead of ORDERING_POSTGRES_POSTGRES_URL).
func Load() (*Config, error) {
	_ = godotenv.Load()

	var cfg Config

	// Process each nested config struct separately to match env var names
	// This avoids the envconfig behavior of adding field names to the prefix
	if err := envconfig.Process(namespace, &cfg.App); err != nil {
		return nil, fmt.Errorf("config: failed to load app config: %w", err)
	}
	if err := envconfig.Process(namespace, &cfg.HTTP); err != nil {
		return nil, fmt.Errorf("config: failed to load http config: %w", err)
	}
	if err := envconfig.Process(namespace, &cfg.Postgres); err != nil {
		return nil, fmt.Errorf("config: failed to load postgres config: %w", err)
	}
	if err := envconfig.Process(namespace, &cfg.Redis); err != nil {
		return nil, fmt.Errorf("config: failed to load redis config: %w", err)
	}
	if err := envconfig.Process(namespace, &cfg.Events); err != nil {
		return nil, fmt.Errorf("config: failed to load events config: %w", err)
	}
	if err := envconfig.Process(namespace, &cfg.Telemetry); err != nil {
		return nil, fmt.Errorf("config: failed to load telemetry config: %w", err)
	}
	if err := envconfig.Process(namespace, &cfg.Auth); err != nil {
		return nil, fmt.Errorf("config: failed to load auth config: %w", err)
	}
	if err := envconfig.Process(namespace, &cfg.Treasury); err != nil {
		return nil, fmt.Errorf("config: failed to load treasury config: %w", err)
	}
	if err := envconfig.Process(namespace, &cfg.Logistics); err != nil {
		return nil, fmt.Errorf("config: failed to load logistics config: %w", err)
	}
	if err := envconfig.Process(namespace, &cfg.Inventory); err != nil {
		return nil, fmt.Errorf("config: failed to load inventory config: %w", err)
	}
	if err := envconfig.Process(namespace, &cfg.Notifications); err != nil {
		return nil, fmt.Errorf("config: failed to load notifications config: %w", err)
	}
	if err := envconfig.Process(namespace, &cfg.Subscriptions); err != nil {
		return nil, fmt.Errorf("config: failed to load subscriptions config: %w", err)
	}
	if err := envconfig.Process(namespace, &cfg.Superset); err != nil {
		return nil, fmt.Errorf("config: failed to load superset config: %w", err)
	}
	if err := envconfig.Process(namespace, &cfg.Security); err != nil {
		return nil, fmt.Errorf("config: failed to load security config: %w", err)
	}
	if err := envconfig.Process(namespace, &cfg.Media); err != nil {
		return nil, fmt.Errorf("config: failed to load media config: %w", err)
	}

	return &cfg, nil
}

// LoadDatabaseOnly loads only database configuration for migration/setup-db commands.
// This avoids validation errors for OAuth, secrets, etc. that aren't needed for migrations.
func LoadDatabaseOnly() (PostgresConfig, error) {
	_ = godotenv.Load()

	// Process PostgresConfig directly with the namespace prefix
	// envconfig will look for ORDERING_POSTGRES_URL, ORDERING_POSTGRES_MAX_OPEN_CONNS, etc.
	var cfg PostgresConfig
	if err := envconfig.Process(namespace, &cfg); err != nil {
		return PostgresConfig{}, fmt.Errorf("config: failed to load database config: %w", err)
	}

	if cfg.URL == "" {
		return PostgresConfig{}, fmt.Errorf("ORDERING_POSTGRES_URL is required")
	}

	return cfg, nil
}
