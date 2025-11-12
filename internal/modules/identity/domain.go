package identity

import (
	"time"

	"github.com/google/uuid"
)

// Role represents the high-level access tier for a user.
type Role string

const (
	RoleCustomer   Role = "customer"
	RoleRider      Role = "rider"
	RoleStaff      Role = "staff"
	RoleAdmin      Role = "admin"
	RoleSuperAdmin Role = "superadmin"
)

// Permission captures fine-grained feature access across the platform.
type Permission string

const (
	PermissionOrdersView        Permission = "orders:view"
	PermissionOrdersManage      Permission = "orders:manage"
	PermissionProfileUpdate     Permission = "profile:update"
	PermissionPreferencesUpdate Permission = "preferences:update"
	PermissionLoyaltyView       Permission = "loyalty:view"
	PermissionLoyaltyRedeem     Permission = "loyalty:redeem"
	PermissionNotifications     Permission = "notifications:manage"
	PermissionRidersOnboard     Permission = "riders:onboard"
	PermissionStaffInvite       Permission = "staff:invite"
	PermissionAdminManage       Permission = "admin:manage"
)

// DefaultPermissions returns the permissions granted to the supplied role.
func DefaultPermissions(role Role) []Permission {
	switch role {
	case RoleCustomer:
		return []Permission{
			PermissionOrdersView,
			PermissionProfileUpdate,
			PermissionPreferencesUpdate,
			PermissionLoyaltyView,
			PermissionLoyaltyRedeem,
		}
	case RoleRider:
		return []Permission{
			PermissionOrdersView,
			PermissionProfileUpdate,
			PermissionPreferencesUpdate,
		}
	case RoleStaff:
		return []Permission{
			PermissionOrdersView,
			PermissionOrdersManage,
			PermissionProfileUpdate,
			PermissionPreferencesUpdate,
			PermissionNotifications,
			PermissionRidersOnboard,
		}
	case RoleAdmin, RoleSuperAdmin:
		return []Permission{
			PermissionOrdersView,
			PermissionOrdersManage,
			PermissionProfileUpdate,
			PermissionPreferencesUpdate,
			PermissionNotifications,
			PermissionRidersOnboard,
			PermissionStaffInvite,
			PermissionAdminManage,
		}
	default:
		return nil
	}
}

// NotificationPreferences captures messaging channels enabled by the user.
type NotificationPreferences struct {
	Email bool `json:"email"`
	SMS   bool `json:"sms"`
	Push  bool `json:"push"`
}

// Preferences contains personalisation options for a user.
type Preferences struct {
	Theme         string                  `json:"theme"`
	Language      string                  `json:"language"`
	Notifications NotificationPreferences `json:"notifications"`
}

// User model stored in persistence.
type User struct {
	ID                   uuid.UUID              `json:"id"`
	TenantID             string                 `json:"tenantId"`
	Email                string                 `json:"email"`
	PasswordHash         string                 `json:"-"`
	FullName             string                 `json:"fullName"`
	Phone                string                 `json:"phone"`
	AvatarURL            string                 `json:"avatarUrl"`
	Roles                []Role                 `json:"roles"`
	Permissions          []Permission           `json:"permissions"`
	LoyaltyPoints        int                    `json:"loyaltyPoints"`
	AvailableCoupons     int                    `json:"availableCoupons"`
	DefaultLocationLabel string                 `json:"defaultLocationLabel"`
	TwoFactorEnabled     bool                   `json:"twoFactorEnabled"`
	TwoFactorSecret      string                 `json:"-"`
	BackupCodes          []string               `json:"-"`
	Preferences          Preferences            `json:"preferences"`
	LastLoginAt          *time.Time             `json:"lastLoginAt"`
	CreatedAt            time.Time              `json:"createdAt"`
	UpdatedAt            time.Time              `json:"updatedAt"`
	DeletedAt            *time.Time             `json:"-"`
	Status               string                 `json:"status"`
	Metadata             map[string]interface{} `json:"metadata"`
}

// HasRole checks whether the user has the provided role.
func (u *User) HasRole(role Role) bool {
	for _, r := range u.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// HasPermission checks whether the user has the provided permission.
func (u *User) HasPermission(permission Permission) bool {
	for _, p := range u.Permissions {
		if p == permission {
			return true
		}
	}
	return false
}

// Session represents a refresh token backed session.
type Session struct {
	ID           uuid.UUID  `json:"id"`
	UserID       uuid.UUID  `json:"userId"`
	RefreshToken string     `json:"refreshToken"`
	UserAgent    string     `json:"userAgent"`
	IP           string     `json:"ip"`
	ExpiresAt    time.Time  `json:"expiresAt"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
	RevokedAt    *time.Time `json:"revokedAt"`
}

// OrderSummary provides a high-level view of a user's past orders.
type OrderSummary struct {
	ID       uuid.UUID  `json:"id"`
	UserID   uuid.UUID  `json:"userId"`
	Status   string     `json:"status"`
	Total    float64    `json:"total"`
	PlacedAt time.Time  `json:"placedAt"`
	ETA      *time.Time `json:"eta"`
}
