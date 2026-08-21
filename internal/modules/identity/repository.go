package identity

import (
	"context"

	"github.com/google/uuid"
)

// Tenant represents a tenant entity from the database.
type Tenant struct {
	ID     uuid.UUID
	Slug   string
	Name   string
	Status string
}

// Repository abstracts persistence for identity entities.
type Repository interface {
	CreateUser(ctx context.Context, user *User) error
	UpdateUser(ctx context.Context, user *User) error
	FindUserByEmail(ctx context.Context, email string) (*User, error)
	FindUserByID(ctx context.Context, id uuid.UUID) (*User, error)
	FindUserByAuthServiceID(ctx context.Context, authServiceUserID uuid.UUID) (*User, error)
	ListUsers(ctx context.Context) ([]*User, error)
	// ListTenantUsers returns the users belonging to a tenant, optionally filtered
	// by a case-insensitive name/email substring (q). A non-positive limit applies
	// a sane default cap.
	ListTenantUsers(ctx context.Context, tenantID uuid.UUID, q string, limit int) ([]*User, error)

	FindTenantBySlug(ctx context.Context, slug string) (*Tenant, error)
	FindTenantByID(ctx context.Context, id uuid.UUID) (*Tenant, error)
	UpsertTenant(ctx context.Context, tenant *Tenant) error

	ListOrdersByUser(ctx context.Context, userID uuid.UUID) ([]*OrderSummary, error)
	Seed(ctx context.Context, users []*User, _ []*OrderSummary) error

	// HardDeleteUser reacts to auth-api's real hard-delete (AdminPurgeUser, published
	// as auth.user.deleted) of a platform user. Returns false (no error) when the user
	// was never synced locally. See EntRepository.HardDeleteUser's doc comment for the
	// full per-table policy (identity data hard-deleted; Order/OrderItem/OrderEvent/
	// OrderAssignment preserved with their customer_id/delivery_address_id auto-nulled
	// by the DB's ON DELETE SET NULL).
	HardDeleteUser(ctx context.Context, authServiceUserID uuid.UUID) (bool, error)
}
