package identity

import (
	"context"

	"github.com/google/uuid"
)

// Repository abstracts persistence for identity entities.
type Repository interface {
	CreateUser(ctx context.Context, user *User) error
	UpdateUser(ctx context.Context, user *User) error
	FindUserByEmail(ctx context.Context, email string) (*User, error)
	FindUserByID(ctx context.Context, id uuid.UUID) (*User, error)
	ListUsers(ctx context.Context) ([]*User, error)

	CreateSession(ctx context.Context, session *Session) error
	UpdateSession(ctx context.Context, session *Session) error
	FindSessionByID(ctx context.Context, id uuid.UUID) (*Session, error)
	FindSessionByToken(ctx context.Context, refreshToken string) (*Session, error)
	DeleteSession(ctx context.Context, id uuid.UUID) error
	DeleteSessionsByUser(ctx context.Context, userID uuid.UUID) error

	ListOrdersByUser(ctx context.Context, userID uuid.UUID) ([]*OrderSummary, error)
	Seed(ctx context.Context, users []*User, sessions []*Session, orders []*OrderSummary) error
}
