package security

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// RateLimiter provides rate limiting functionality using Redis.
type RateLimiter struct {
	redis  *redis.Client
	logger *zap.Logger
	config RateLimitConfig
}

// RateLimitConfig configures rate limiting behavior.
type RateLimitConfig struct {
	// Default rate limits
	RequestsPerMinute int `envconfig:"RATE_LIMIT_REQUESTS_PER_MINUTE" default:"60"`
	RequestsPerHour   int `envconfig:"RATE_LIMIT_REQUESTS_PER_HOUR" default:"1000"`

	// Per-endpoint rate limits (stricter)
	AuthRequestsPerMinute    int `envconfig:"RATE_LIMIT_AUTH_PER_MINUTE" default:"10"`
	PaymentRequestsPerMinute int `envconfig:"RATE_LIMIT_PAYMENT_PER_MINUTE" default:"20"`

	// Burst allowance
	BurstMultiplier float64 `envconfig:"RATE_LIMIT_BURST_MULTIPLIER" default:"1.5"`

	// Key prefix for Redis
	KeyPrefix string `envconfig:"RATE_LIMIT_KEY_PREFIX" default:"rl:ordering:"`

	// Enable/disable rate limiting
	Enabled bool `envconfig:"RATE_LIMIT_ENABLED" default:"true"`
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter(redis *redis.Client, config RateLimitConfig, logger *zap.Logger) *RateLimiter {
	return &RateLimiter{
		redis:  redis,
		logger: logger.Named("security.rate_limiter"),
		config: config,
	}
}

// RateLimitResult contains the result of a rate limit check.
type RateLimitResult struct {
	Allowed   bool
	Remaining int
	ResetAt   time.Time
	RetryAt   *time.Time
}

// Check performs a rate limit check for the given key.
// Uses sliding window algorithm with Redis.
func (rl *RateLimiter) Check(ctx context.Context, key string, limit int, window time.Duration) (*RateLimitResult, error) {
	if !rl.config.Enabled {
		return &RateLimitResult{Allowed: true, Remaining: limit}, nil
	}

	// Create a short-lived context for the Redis operation to fail fast
	redisCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()

	now := time.Now()
	windowStart := now.Add(-window)
	redisKey := rl.config.KeyPrefix + key

	// Use Redis ZREMRANGEBYSCORE + ZCARD + ZADD pattern for sliding window
	pipe := rl.redis.Pipeline()

	// Remove old entries outside the window
	pipe.ZRemRangeByScore(redisCtx, redisKey, "0", strconv.FormatInt(windowStart.UnixMilli(), 10))

	// Count current entries in window
	countCmd := pipe.ZCard(redisCtx, redisKey)

	// Execute pipeline
	_, err := pipe.Exec(redisCtx)
	if err != nil && err != redis.Nil {
		rl.logger.Error("Rate limiter pipeline error", zap.Error(err), zap.String("key", key))
		// On Redis error, allow the request (fail open)
		return &RateLimitResult{Allowed: true, Remaining: limit}, nil
	}

	count := countCmd.Val()
	remaining := limit - int(count)

	if remaining <= 0 {
		// Rate limited - find when the oldest entry expires
		oldest, err := rl.redis.ZRangeWithScores(redisCtx, redisKey, 0, 0).Result()
		if err == nil && len(oldest) > 0 {
			oldestTime := time.UnixMilli(int64(oldest[0].Score))
			retryAt := oldestTime.Add(window)
			return &RateLimitResult{
				Allowed:   false,
				Remaining: 0,
				ResetAt:   now.Add(window),
				RetryAt:   &retryAt,
			}, nil
		}
		return &RateLimitResult{
			Allowed:   false,
			Remaining: 0,
			ResetAt:   now.Add(window),
		}, nil
	}

	// Add current request to the window
	err = rl.redis.ZAdd(redisCtx, redisKey, redis.Z{
		Score:  float64(now.UnixMilli()),
		Member: fmt.Sprintf("%d:%d", now.UnixNano(), now.Nanosecond()),
	}).Err()
	if err != nil {
		rl.logger.Warn("Failed to record rate limit entry", zap.Error(err), zap.String("key", key))
	}

	// Set TTL on the key
	rl.redis.Expire(redisCtx, redisKey, window+time.Minute)

	return &RateLimitResult{
		Allowed:   true,
		Remaining: remaining - 1,
		ResetAt:   now.Add(window),
	}, nil
}

// Middleware returns an HTTP middleware that enforces rate limiting.
func (rl *RateLimiter) Middleware(keyFunc func(r *http.Request) string, limit int, window time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !rl.config.Enabled {
				next.ServeHTTP(w, r)
				return
			}

			key := keyFunc(r)
			result, err := rl.Check(r.Context(), key, limit, window)
			if err != nil {
				rl.logger.Error("Rate limit check failed", zap.Error(err))
				next.ServeHTTP(w, r)
				return
			}

			// Set rate limit headers
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(limit))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(result.Remaining))
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(result.ResetAt.Unix(), 10))

			if !result.Allowed {
				if result.RetryAt != nil {
					w.Header().Set("Retry-After", strconv.FormatInt(int64(time.Until(*result.RetryAt).Seconds()), 10))
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"error":"rate limit exceeded","message":"too many requests, please try again later"}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// IPRateLimiter returns middleware that rate limits by IP address.
func (rl *RateLimiter) IPRateLimiter(limit int, window time.Duration) func(http.Handler) http.Handler {
	return rl.Middleware(func(r *http.Request) string {
		return "ip:" + getRealIP(r)
	}, limit, window)
}

// TenantRateLimiter returns middleware that rate limits by tenant.
func (rl *RateLimiter) TenantRateLimiter(limit int, window time.Duration) func(http.Handler) http.Handler {
	return rl.Middleware(func(r *http.Request) string {
		tenant := r.Header.Get("X-Tenant-ID")
		if tenant == "" {
			// Fall back to IP if no tenant
			return "ip:" + getRealIP(r)
		}
		return "tenant:" + tenant
	}, limit, window)
}

// UserRateLimiter returns middleware that rate limits by user ID.
func (rl *RateLimiter) UserRateLimiter(limit int, window time.Duration) func(http.Handler) http.Handler {
	return rl.Middleware(func(r *http.Request) string {
		// Get user from context (set by auth middleware)
		userID := r.Header.Get("X-User-ID")
		if userID == "" {
			// Fall back to IP if no user
			return "ip:" + getRealIP(r)
		}
		return "user:" + userID
	}, limit, window)
}

// EndpointRateLimiter returns middleware that rate limits by endpoint + IP.
func (rl *RateLimiter) EndpointRateLimiter(endpoint string, limit int, window time.Duration) func(http.Handler) http.Handler {
	return rl.Middleware(func(r *http.Request) string {
		return fmt.Sprintf("endpoint:%s:ip:%s", endpoint, getRealIP(r))
	}, limit, window)
}

// getRealIP extracts the real client IP from the request.
func getRealIP(r *http.Request) string {
	// Check X-Forwarded-For header first
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the chain
		for i := 0; i < len(xff); i++ {
			if xff[i] == ',' {
				return xff[:i]
			}
		}
		return xff
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	ip := r.RemoteAddr
	for i := len(ip) - 1; i >= 0; i-- {
		if ip[i] == ':' {
			return ip[:i]
		}
	}
	return ip
}
