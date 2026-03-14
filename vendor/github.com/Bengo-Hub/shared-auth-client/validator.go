package authclient

import (
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

// Config holds validator configuration.
type Config struct {
	JWKSUrl         string
	Issuer          string
	Audience        string
	CacheTTL        time.Duration // How long to cache JWKS
	RefreshInterval time.Duration // How often to refresh JWKS in background
	HTTPClient      *http.Client
	RedisClient     *redis.Client // Optional: Redis client for session caching
	SessionCacheTTL time.Duration // Duration to cache validated sessions
}

// DefaultConfig returns a config with sensible defaults.
func DefaultConfig(jwksURL, issuer, audience string) Config {
	return Config{
		JWKSUrl:         jwksURL,
		Issuer:          issuer,
		Audience:        audience,
		CacheTTL:        1 * time.Hour,
		RefreshInterval: 5 * time.Minute,
		HTTPClient:      &http.Client{Timeout: 10 * time.Second},
		SessionCacheTTL: 5 * time.Minute,
	}
}

// Validator validates JWT tokens using JWKS from auth-service.
type Validator struct {
	config      Config
	keys        map[string]*rsa.PublicKey
	keysMu      sync.RWMutex
	lastFetch   time.Time
	fetchGroup  singleflight.Group
	parser      *jwt.Parser
	stopRefresh chan struct{}
}

// NewValidator creates a new JWT validator.
func NewValidator(config Config) (*Validator, error) {
	v := &Validator{
		config:      config,
		keys:        make(map[string]*rsa.PublicKey),
		parser:      jwt.NewParser(jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()})),
		stopRefresh: make(chan struct{}),
	}

	// Initial fetch
	if err := v.fetchJWKS(context.Background()); err != nil {
		return nil, fmt.Errorf("initial JWKS fetch: %w", err)
	}

	// Start background refresh
	go v.refreshLoop()

	return v, nil
}

// ValidateToken validates a JWT token string and returns claims.
func (v *Validator) ValidateToken(tokenString string) (*Claims, error) {
	// 1. Check Redis cache if configured
	if v.config.RedisClient != nil {
		claims, err := v.getCachedClaims(tokenString)
		if err == nil && claims != nil {
			return claims, nil
		}
	}

	// 2. Parse and validate token (CPU bound)
	token, err := v.parser.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		kid, ok := token.Header["kid"].(string)
		if !ok {
			return nil, fmt.Errorf("missing kid in token header")
		}

		key := v.getKey(kid)
		if key == nil {
			// Try to refresh JWKS
			if err := v.fetchJWKS(context.Background()); err != nil {
				return nil, fmt.Errorf("key not found and JWKS refresh failed: %w", err)
			}
			key = v.getKey(kid)
			if key == nil {
				return nil, fmt.Errorf("key %s not found in JWKS", kid)
			}
		}

		return key, nil
	})

	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("token invalid")
	}

	claims, ok := token.Claims.(*Claims)
	if !ok {
		return nil, fmt.Errorf("invalid claims type")
	}

	// Validate issuer
	if v.config.Issuer != "" && claims.Issuer != v.config.Issuer {
		return nil, fmt.Errorf("invalid issuer: expected %s, got %s", v.config.Issuer, claims.Issuer)
	}

	// Validate audience
	if v.config.Audience != "" {
		found := false
		for _, aud := range claims.Audience {
			if aud == v.config.Audience {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("invalid audience: expected %s", v.config.Audience)
		}
	}

	// 3. Cache the validated claims if Redis is configured
	if v.config.RedisClient != nil {
		_ = v.cacheClaims(tokenString, claims)
	}

	return claims, nil
}

func (v *Validator) getCachedClaims(tokenString string) (*Claims, error) {
	tokenHash := sha256.Sum256([]byte(tokenString))
	key := fmt.Sprintf("session:%s", hex.EncodeToString(tokenHash[:]))

	data, err := v.config.RedisClient.Get(context.Background(), key).Bytes()
	if err != nil {
		return nil, err
	}

	var claims Claims
	if err := json.Unmarshal(data, &claims); err != nil {
		return nil, err
	}

	return &claims, nil
}

func (v *Validator) cacheClaims(tokenString string, claims *Claims) error {
	tokenHash := sha256.Sum256([]byte(tokenString))
	key := fmt.Sprintf("session:%s", hex.EncodeToString(tokenHash[:]))

	data, err := json.Marshal(claims)
	if err != nil {
		return err
	}

	// Use Configured TTL or default
	ttl := v.config.SessionCacheTTL
	if ttl == 0 {
		ttl = 5 * time.Minute
	}

	return v.config.RedisClient.Set(context.Background(), key, data, ttl).Err()
}

func (v *Validator) getKey(kid string) *rsa.PublicKey {
	v.keysMu.RLock()
	defer v.keysMu.RUnlock()
	return v.keys[kid]
}

func (v *Validator) fetchJWKS(ctx context.Context) error {
	// Use singleflight to prevent concurrent fetches
	_, err, _ := v.fetchGroup.Do("jwks", func() (interface{}, error) {
		req, err := http.NewRequestWithContext(ctx, "GET", v.config.JWKSUrl, nil)
		if err != nil {
			return nil, err
		}

		resp, err := v.config.HTTPClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("JWKS fetch failed: status %d", resp.StatusCode)
		}

		var jwks struct {
			Keys []struct {
				Kty string `json:"kty"`
				Kid string `json:"kid"`
				Use string `json:"use"`
				Alg string `json:"alg"`
				N   string `json:"n"`
				E   string `json:"e"`
			} `json:"keys"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
			return nil, err
		}

		newKeys := make(map[string]*rsa.PublicKey)
		for _, jwk := range jwks.Keys {
			if jwk.Kty != "RSA" || jwk.Use != "sig" || jwk.Alg != "RS256" {
				continue
			}

			nBytes, err := base64.RawURLEncoding.DecodeString(jwk.N)
			if err != nil {
				continue
			}

			eBytes, err := base64.RawURLEncoding.DecodeString(jwk.E)
			if err != nil {
				continue
			}

			var eInt int64
			for _, b := range eBytes {
				eInt = eInt<<8 | int64(b)
			}

			pubKey := &rsa.PublicKey{
				N: new(big.Int).SetBytes(nBytes),
				E: int(eInt),
			}

			newKeys[jwk.Kid] = pubKey
		}

		v.keysMu.Lock()
		v.keys = newKeys
		v.lastFetch = time.Now()
		v.keysMu.Unlock()

		return nil, nil
	})

	return err
}

func (v *Validator) refreshLoop() {
	ticker := time.NewTicker(v.config.RefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			_ = v.fetchJWKS(ctx)
			cancel()
		case <-v.stopRefresh:
			return
		}
	}
}

// Stop stops the background refresh loop.
func (v *Validator) Stop() {
	close(v.stopRefresh)
}
