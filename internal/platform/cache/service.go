package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Service provides high-level caching operations for application data.
type Service struct {
	client *redis.Client
	logger *zap.Logger
	config CacheConfig
}

// CacheConfig holds caching configuration settings.
type CacheConfig struct {
	DefaultTTL      time.Duration
	MenuItemTTL     time.Duration
	CategoryTTL     time.Duration
	UserProfileTTL  time.Duration
	LoyaltyTTL      time.Duration
	PromoCodeTTL    time.Duration
	KeyPrefix       string
	Enabled         bool
}

// DefaultCacheConfig returns sensible defaults for cache configuration.
func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		DefaultTTL:     15 * time.Minute,
		MenuItemTTL:    30 * time.Minute,
		CategoryTTL:    1 * time.Hour,
		UserProfileTTL: 10 * time.Minute,
		LoyaltyTTL:     5 * time.Minute,
		PromoCodeTTL:   15 * time.Minute,
		KeyPrefix:      "ordering:",
		Enabled:        true,
	}
}

// NewService creates a new cache service instance.
func NewService(client *redis.Client, config CacheConfig, logger *zap.Logger) *Service {
	return &Service{
		client: client,
		logger: logger.Named("cache"),
		config: config,
	}
}

// Get retrieves a value from cache and unmarshals it into the target.
func (s *Service) Get(ctx context.Context, key string, target interface{}) error {
	if !s.config.Enabled {
		return redis.Nil
	}

	fullKey := s.config.KeyPrefix + key
	data, err := s.client.Get(ctx, fullKey).Bytes()
	if err != nil {
		if err == redis.Nil {
			s.logger.Debug("cache miss", zap.String("key", key))
		} else {
			s.logger.Warn("cache get error", zap.String("key", key), zap.Error(err))
		}
		return err
	}

	if err := json.Unmarshal(data, target); err != nil {
		s.logger.Error("cache unmarshal error", zap.String("key", key), zap.Error(err))
		return fmt.Errorf("cache unmarshal: %w", err)
	}

	s.logger.Debug("cache hit", zap.String("key", key))
	return nil
}

// Set stores a value in cache with the specified TTL.
func (s *Service) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	if !s.config.Enabled {
		return nil
	}

	data, err := json.Marshal(value)
	if err != nil {
		s.logger.Error("cache marshal error", zap.String("key", key), zap.Error(err))
		return fmt.Errorf("cache marshal: %w", err)
	}

	fullKey := s.config.KeyPrefix + key
	if err := s.client.Set(ctx, fullKey, data, ttl).Err(); err != nil {
		s.logger.Warn("cache set error", zap.String("key", key), zap.Error(err))
		return err
	}

	s.logger.Debug("cache set", zap.String("key", key), zap.Duration("ttl", ttl))
	return nil
}

// Delete removes a value from cache.
func (s *Service) Delete(ctx context.Context, keys ...string) error {
	if !s.config.Enabled {
		return nil
	}

	fullKeys := make([]string, len(keys))
	for i, key := range keys {
		fullKeys[i] = s.config.KeyPrefix + key
	}

	if err := s.client.Del(ctx, fullKeys...).Err(); err != nil {
		s.logger.Warn("cache delete error", zap.Strings("keys", keys), zap.Error(err))
		return err
	}

	s.logger.Debug("cache delete", zap.Strings("keys", keys))
	return nil
}

// DeletePattern removes all keys matching a pattern.
func (s *Service) DeletePattern(ctx context.Context, pattern string) error {
	if !s.config.Enabled {
		return nil
	}

	fullPattern := s.config.KeyPrefix + pattern
	iter := s.client.Scan(ctx, 0, fullPattern, 0).Iterator()

	keys := make([]string, 0)
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}

	if err := iter.Err(); err != nil {
		s.logger.Warn("cache scan error", zap.String("pattern", pattern), zap.Error(err))
		return err
	}

	if len(keys) > 0 {
		if err := s.client.Del(ctx, keys...).Err(); err != nil {
			s.logger.Warn("cache delete pattern error", zap.String("pattern", pattern), zap.Error(err))
			return err
		}
		s.logger.Debug("cache delete pattern", zap.String("pattern", pattern), zap.Int("count", len(keys)))
	}

	return nil
}

// GetOrSet retrieves a value from cache, or computes and stores it if missing.
func (s *Service) GetOrSet(ctx context.Context, key string, target interface{}, ttl time.Duration, compute func() (interface{}, error)) error {
	// Try to get from cache first
	err := s.Get(ctx, key, target)
	if err == nil {
		return nil // Cache hit
	}

	if err != redis.Nil {
		// Cache error, but we can still compute
		s.logger.Warn("cache error, computing value", zap.String("key", key), zap.Error(err))
	}

	// Cache miss or error - compute the value
	computed, err := compute()
	if err != nil {
		return err
	}

	// Store in cache (best effort, don't fail if caching fails)
	if err := s.Set(ctx, key, computed, ttl); err != nil {
		s.logger.Warn("failed to cache computed value", zap.String("key", key), zap.Error(err))
	}

	// Marshal the computed value to target
	data, err := json.Marshal(computed)
	if err != nil {
		return fmt.Errorf("marshal computed: %w", err)
	}

	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("unmarshal computed: %w", err)
	}

	return nil
}

// Increment increments a counter in cache.
func (s *Service) Increment(ctx context.Context, key string) (int64, error) {
	if !s.config.Enabled {
		return 0, nil
	}

	fullKey := s.config.KeyPrefix + key
	val, err := s.client.Incr(ctx, fullKey).Result()
	if err != nil {
		s.logger.Warn("cache increment error", zap.String("key", key), zap.Error(err))
		return 0, err
	}

	return val, nil
}

// SetNX sets a key only if it doesn't exist (for distributed locking).
func (s *Service) SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error) {
	if !s.config.Enabled {
		return true, nil
	}

	data, err := json.Marshal(value)
	if err != nil {
		return false, fmt.Errorf("marshal: %w", err)
	}

	fullKey := s.config.KeyPrefix + key
	return s.client.SetNX(ctx, fullKey, data, ttl).Result()
}

// CacheKeys provides constants for common cache key patterns.
type CacheKeys struct{}

// MenuItem returns a cache key for a menu item.
func (CacheKeys) MenuItem(tenantID, itemID string) string {
	return fmt.Sprintf("menu:item:%s:%s", tenantID, itemID)
}

// MenuCategory returns a cache key for a menu category.
func (CacheKeys) MenuCategory(tenantID, categoryID string) string {
	return fmt.Sprintf("menu:category:%s:%s", tenantID, categoryID)
}

// MenuItemsByCafe returns a cache key for all menu items in a cafe.
func (CacheKeys) MenuItemsByCafe(tenantID, cafeID string) string {
	return fmt.Sprintf("menu:cafe:%s:%s:items", tenantID, cafeID)
}

// UserProfile returns a cache key for a user profile.
func (CacheKeys) UserProfile(tenantID, userID string) string {
	return fmt.Sprintf("user:profile:%s:%s", tenantID, userID)
}

// LoyaltyAccount returns a cache key for a loyalty account.
func (CacheKeys) LoyaltyAccount(tenantID, userID string) string {
	return fmt.Sprintf("loyalty:account:%s:%s", tenantID, userID)
}

// PromoCode returns a cache key for a promo code.
func (CacheKeys) PromoCode(tenantID, code string) string {
	return fmt.Sprintf("promo:code:%s:%s", tenantID, code)
}

// Cart returns a cache key for a cart.
func (CacheKeys) Cart(tenantID, cartID string) string {
	return fmt.Sprintf("cart:%s:%s", tenantID, cartID)
}

// ActiveCart returns a cache key for a user's active cart.
func (CacheKeys) ActiveCart(tenantID, userID string) string {
	return fmt.Sprintf("cart:active:%s:%s", tenantID, userID)
}
