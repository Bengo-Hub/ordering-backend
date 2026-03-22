package rbac

import (
	"time"

	"github.com/google/uuid"
)

// OrderingUser represents an ordering service user reference.
type OrderingUser struct {
	ID                uuid.UUID
	TenantID          uuid.UUID
	AuthServiceUserID uuid.UUID
	Email             string
	Status            string
	SyncStatus        string
	LastSyncAt        *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// OrderingRole represents an ordering service role.
type OrderingRole struct {
	ID           uuid.UUID
	TenantID     uuid.UUID
	RoleCode     string
	Name         string
	Description  *string
	IsSystemRole bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// OrderingPermission represents an ordering service permission.
type OrderingPermission struct {
	ID             uuid.UUID
	PermissionCode string
	Name           string
	Module         string
	Action         string
	Resource       *string
	Description    *string
	CreatedAt      time.Time
}

// UserRoleAssignment represents a user role assignment.
type UserRoleAssignment struct {
	ID         uuid.UUID
	TenantID   uuid.UUID
	UserID     uuid.UUID
	RoleID     uuid.UUID
	AssignedBy uuid.UUID
	AssignedAt time.Time
	ExpiresAt  *time.Time
}
