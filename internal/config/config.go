package config

import (
	"fmt"
	"time"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

const namespace = "ORDERING"

// DefaultTenantSlug is the default tenant slug for the ordering service (empty = no default).
const DefaultTenantSlug = ""

// Config captures environment configuration for the Ordering service backend.
type Config struct {
	App           AppConfig
	HTTP          HTTPConfig
	Postgres      PostgresConfig
	Redis         RedisConfig
	Events        EventsConfig
	Telemetry     TelemetryConfig
	Auth          AuthConfig
	Treasury      TreasuryConfig
	Logistics     LogisticsConfig
	Inventory     InventoryConfig
	Notifications NotificationsConfig
	Superset      SupersetConfig
	Security      SecurityConfig
}

type AppConfig struct {
	Name    string `envconfig:"APP_NAME" default:"ordering-backend"`
	Env     string `envconfig:"APP_ENV" default:"development"`
	Version string `envconfig:"APP_VERSION" default:"0.1.0"`
}

type HTTPConfig struct {
	Host           string        `envconfig:"HTTP_HOST" default:"0.0.0.0"`
	Port           int           `envconfig:"HTTP_PORT" default:"4000"`
	ReadTimeout    time.Duration `envconfig:"HTTP_READ_TIMEOUT" default:"15s"`
	WriteTimeout   time.Duration `envconfig:"HTTP_WRITE_TIMEOUT" default:"15s"`
	IdleTimeout    time.Duration `envconfig:"HTTP_IDLE_TIMEOUT" default:"60s"`
	AllowedOrigins []string      `envconfig:"HTTP_ALLOWED_ORIGINS" default:"http://localhost:3000"`
}

type PostgresConfig struct {
	URL             string        `envconfig:"POSTGRES_URL" default:"postgres://postgres:postgres@localhost:5432/ordering-service?sslmode=disable"`
	MaxOpenConns    int           `envconfig:"POSTGRES_MAX_OPEN_CONNS" default:"20"`
	MaxIdleConns    int           `envconfig:"POSTGRES_MAX_IDLE_CONNS" default:"10"`
	ConnMaxLifetime time.Duration `envconfig:"POSTGRES_CONN_MAX_LIFETIME" default:"30m"`
}

type RedisConfig struct {
	Addr     string `envconfig:"REDIS_ADDR" default:"localhost:6379"`
	Username string `envconfig:"REDIS_USERNAME"`
	Password string `envconfig:"REDIS_PASSWORD"`
	DB       int    `envconfig:"REDIS_DB" default:"0"`
}

type EventsConfig struct {
	NATSURL    string `envconfig:"NATS_URL" default:"nats://127.0.0.1:4222"`
	StreamName string `envconfig:"NATS_STREAM" default:"ordering"`
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
	AuthServiceAPIKey   string        `envconfig:"AUTH_SERVICE_API_KEY"`

	// Legacy config (deprecated - use auth-service instead)
	AccessTokenSecret  string        `envconfig:"AUTH_ACCESS_TOKEN_SECRET" default:"dev-access-secret-change-me"`
	RefreshTokenSecret string        `envconfig:"AUTH_REFRESH_TOKEN_SECRET" default:"dev-refresh-secret-change-me"`
	AccessTokenTTL     time.Duration `envconfig:"AUTH_ACCESS_TOKEN_TTL" default:"15m"`
	RefreshTokenTTL    time.Duration `envconfig:"AUTH_REFRESH_TOKEN_TTL" default:"720h"`
	GoogleClientID     string        `envconfig:"AUTH_GOOGLE_CLIENT_ID"`
	GoogleClientSecret string        `envconfig:"AUTH_GOOGLE_CLIENT_SECRET"`
	GoogleRedirectBase string        `envconfig:"AUTH_GOOGLE_REDIRECT_BASE" default:"http://localhost:3000/auth/callback"`
	TwoFactorIssuer    string        `envconfig:"AUTH_TWO_FACTOR_ISSUER" default:"Ordering Platform"`
}

type TreasuryConfig struct {
	// Treasury service URL
	ServiceURL     string        `envconfig:"TREASURY_SERVICE_URL" default:"http://localhost:4001"`
	APIKey         string        `envconfig:"TREASURY_API_KEY"`
	WebhookSecret  string        `envconfig:"TREASURY_WEBHOOK_SECRET"`
	RequestTimeout time.Duration `envconfig:"TREASURY_REQUEST_TIMEOUT" default:"30s"`

	// M-Pesa configuration (via treasury service)
	MpesaEnabled         bool   `envconfig:"MPESA_ENABLED" default:"true"`
	MpesaCallbackBaseURL string `envconfig:"MPESA_CALLBACK_BASE_URL" default:"http://localhost:4000/api/v1/webhooks/mpesa"`
}

type LogisticsConfig struct {
	// Logistics service URL
	ServiceURL     string        `envconfig:"LOGISTICS_SERVICE_URL" default:"http://localhost:4005"`
	APIKey         string        `envconfig:"LOGISTICS_API_KEY"`
	WebhookSecret  string        `envconfig:"LOGISTICS_WEBHOOK_SECRET"`
	RequestTimeout time.Duration `envconfig:"LOGISTICS_REQUEST_TIMEOUT" default:"30s"`

	// WebSocket configuration for live tracking
	WebSocketURL string `envconfig:"LOGISTICS_WEBSOCKET_URL" default:"ws://localhost:4005/ws"`
}

type InventoryConfig struct {
	// Inventory service URL
	ServiceURL     string        `envconfig:"INVENTORY_SERVICE_URL" default:"http://localhost:4003"`
	APIKey         string        `envconfig:"INVENTORY_API_KEY"`
	RequestTimeout time.Duration `envconfig:"INVENTORY_REQUEST_TIMEOUT" default:"10s"`
}

type NotificationsConfig struct {
	// Notifications service URL
	ServiceURL     string        `envconfig:"NOTIFICATIONS_SERVICE_URL" default:"http://localhost:4002"`
	APIKey         string        `envconfig:"NOTIFICATIONS_API_KEY"`
	RequestTimeout time.Duration `envconfig:"NOTIFICATIONS_REQUEST_TIMEOUT" default:"10s"`
}

type SupersetConfig struct {
	// Superset service URL
	BaseURL        string        `envconfig:"SUPERSET_BASE_URL" default:"https://superset.codevertexitsolutions.co.ke"`
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
	RateLimitEnabled          bool    `envconfig:"RATE_LIMIT_ENABLED" default:"true"`
	RateLimitRequestsPerMin   int     `envconfig:"RATE_LIMIT_REQUESTS_PER_MINUTE" default:"60"`
	RateLimitRequestsPerHour  int     `envconfig:"RATE_LIMIT_REQUESTS_PER_HOUR" default:"1000"`
	RateLimitAuthPerMin       int     `envconfig:"RATE_LIMIT_AUTH_PER_MINUTE" default:"10"`
	RateLimitPaymentPerMin    int     `envconfig:"RATE_LIMIT_PAYMENT_PER_MINUTE" default:"20"`
	RateLimitBurstMultiplier  float64 `envconfig:"RATE_LIMIT_BURST_MULTIPLIER" default:"1.5"`
	RateLimitKeyPrefix        string  `envconfig:"RATE_LIMIT_KEY_PREFIX" default:"rl:ordering:"`

	// Request limits
	MaxRequestBodySize int64 `envconfig:"MAX_REQUEST_BODY_SIZE" default:"10485760"` // 10MB

	// Input validation
	InputValidationEnabled bool `envconfig:"INPUT_VALIDATION_ENABLED" default:"true"`

	// Security headers
	SecurityHeadersEnabled bool `envconfig:"SECURITY_HEADERS_ENABLED" default:"true"`
}

// Load reads configuration from environment variables and optional .env files.
func Load() (*Config, error) {
	_ = godotenv.Load()

	var cfg Config
	if err := envconfig.Process(namespace, &cfg); err != nil {
		return nil, fmt.Errorf("config: failed to load environment variables: %w", err)
	}

	return &cfg, nil
}
