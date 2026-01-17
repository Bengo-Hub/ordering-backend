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
	App       AppConfig
	HTTP      HTTPConfig
	Postgres  PostgresConfig
	Redis     RedisConfig
	Events    EventsConfig
	Telemetry TelemetryConfig
	Auth      AuthConfig
	Treasury  TreasuryConfig
	Logistics LogisticsConfig
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

// Load reads configuration from environment variables and optional .env files.
func Load() (*Config, error) {
	_ = godotenv.Load()

	var cfg Config
	if err := envconfig.Process(namespace, &cfg); err != nil {
		return nil, fmt.Errorf("config: failed to load environment variables: %w", err)
	}

	return &cfg, nil
}
