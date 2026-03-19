package identity

import (
	"context"

	"github.com/google/uuid"
)

type contextKey string

const (
	contextUserKey    contextKey = "identityUser"
)

// ContextWithUser attaches a user to context.
func ContextWithUser(ctx context.Context, user *User) context.Context {
	return context.WithValue(ctx, contextUserKey, user)
}

// UserFromContext retrieves a user from context.
func UserFromContext(ctx context.Context) (*User, bool) {
	user, ok := ctx.Value(contextUserKey).(*User)
	return user, ok
}

// MustUserID extracts user ID from context (via User object or Claims).
func MustUserID(ctx context.Context) uuid.UUID {
	if user, ok := UserFromContext(ctx); ok {
		return user.ID
	}
	return uuid.Nil
}
