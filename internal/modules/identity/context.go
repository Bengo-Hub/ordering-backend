package identity

import (
	"context"

	"github.com/google/uuid"
)

type contextKey string

const (
	contextClaimsKey  contextKey = "identityClaims"
	contextUserKey    contextKey = "identityUser"
	contextSessionKey contextKey = "identitySession"
)

// ContextWithClaims attaches claims to context.
func ContextWithClaims(ctx context.Context, claims *Claims) context.Context {
	return context.WithValue(ctx, contextClaimsKey, claims)
}

// ClaimsFromContext extracts claims from context.
func ClaimsFromContext(ctx context.Context) (*Claims, bool) {
	claims, ok := ctx.Value(contextClaimsKey).(*Claims)
	return claims, ok
}

// ContextWithUser attaches a user to context.
func ContextWithUser(ctx context.Context, user *User) context.Context {
	return context.WithValue(ctx, contextUserKey, user)
}

// UserFromContext retrieves a user from context.
func UserFromContext(ctx context.Context) (*User, bool) {
	user, ok := ctx.Value(contextUserKey).(*User)
	return user, ok
}

// ContextWithSession attaches a session to context.
func ContextWithSession(ctx context.Context, session *Session) context.Context {
	return context.WithValue(ctx, contextSessionKey, session)
}

// SessionFromContext retrieves a session from context.
func SessionFromContext(ctx context.Context) (*Session, bool) {
	session, ok := ctx.Value(contextSessionKey).(*Session)
	return session, ok
}

// MustUserID extracts user ID from context (via User object or Claims).
func MustUserID(ctx context.Context) uuid.UUID {
	if user, ok := UserFromContext(ctx); ok {
		return user.ID
	}

	claims, _ := ClaimsFromContext(ctx)
	if claims == nil {
		return uuid.Nil
	}
	id, err := uuid.Parse(claims.UserID)
	if err != nil {
		return uuid.Nil
	}
	return id
}
