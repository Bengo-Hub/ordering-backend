package identity

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/bengobox/food-delivery-backend/internal/config"
)

// Claims represents the JWT payload embedded in access tokens.
type Claims struct {
	SessionID   string       `json:"sid"`
	UserID      string       `json:"uid"`
	TenantID    string       `json:"tid"`
	Roles       []Role       `json:"roles"`
	Permissions []Permission `json:"permissions"`
	jwt.RegisteredClaims
}

// TokenPayload captures inputs required to build a JWT.
type TokenPayload struct {
	SessionID   uuid.UUID
	UserID      uuid.UUID
	TenantID    string
	Roles       []Role
	Permissions []Permission
}

// TokenSigner encapsulates JWT signing and validation logic.
type TokenSigner struct {
	accessSecret []byte
	stateSecret  []byte
	accessTTL    time.Duration
}

// NewTokenSigner constructs a TokenSigner from the auth config.
func NewTokenSigner(cfg config.AuthConfig) (*TokenSigner, error) {
	if cfg.AccessTokenSecret == "" {
		return nil, errors.New("identity: access token secret required")
	}

	stateSecret := cfg.AccessTokenSecret
	if cfg.RefreshTokenSecret != "" {
		stateSecret = cfg.RefreshTokenSecret
	}

	return &TokenSigner{
		accessSecret: []byte(cfg.AccessTokenSecret),
		stateSecret:  []byte(stateSecret),
		accessTTL:    cfg.AccessTokenTTL,
	}, nil
}

// GenerateAccessToken signs a JWT representing the supplied payload.
func (s *TokenSigner) GenerateAccessToken(payload *TokenPayload) (string, error) {
	now := time.Now()
	claims := &Claims{
		SessionID:   payload.SessionID.String(),
		UserID:      payload.UserID.String(),
		TenantID:    payload.TenantID,
		Roles:       payload.Roles,
		Permissions: payload.Permissions,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   payload.UserID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.accessTTL)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.accessSecret)
}

// VerifyAccessToken validates a JWT and returns the embedded claims.
func (s *TokenSigner) VerifyAccessToken(token string) (*Claims, error) {
	parsed, err := jwt.ParseWithClaims(token, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		return s.accessSecret, nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := parsed.Claims.(*Claims)
	if !ok || !parsed.Valid {
		return nil, errors.New("identity: invalid access token")
	}
	return claims, nil
}

type stateClaims struct {
	Role Role `json:"role"`
	jwt.RegisteredClaims
}

// GenerateState produces a signed short-lived JWT storing the OAuth role.
func (s *TokenSigner) GenerateState(role Role) (string, error) {
	now := time.Now()
	claims := &stateClaims{
		Role: role,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(10 * time.Minute)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.stateSecret)
}

// ParseState validates the OAuth state token.
func (s *TokenSigner) ParseState(token string) (Role, error) {
	parsed, err := jwt.ParseWithClaims(token, &stateClaims{}, func(t *jwt.Token) (interface{}, error) {
		return s.stateSecret, nil
	})
	if err != nil {
		return "", err
	}

	claims, ok := parsed.Claims.(*stateClaims)
	if !ok || !parsed.Valid {
		return "", ErrStateMismatch
	}
	return claims.Role, nil
}
