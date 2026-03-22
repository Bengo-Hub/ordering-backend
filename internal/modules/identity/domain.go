package identity

import (
	"time"

	"github.com/google/uuid"
)

// Role represents the high-level access tier for a user.
type Role string

const (
	RoleCustomer   Role = "customer"
	RoleDriver     Role = "driver" // Universal role for delivery/courier/taxi use cases
	RoleRider      Role = "rider"  // Deprecated: use RoleDriver; kept for backward compatibility
	RoleStaff      Role = "staff"
	RoleAdmin      Role = "admin"
	RoleSuperAdmin Role = "superuser"
)

// Permission captures fine-grained feature access across the platform.
type Permission string

const (
	PermissionOrdersView          Permission = "orders:read"
	PermissionOrdersManage        Permission = "orders:manage"
	PermissionOrdersRefund        Permission = "orders:refund"
	PermissionProfileUpdate       Permission = "profile:update"
	PermissionPreferencesUpdate   Permission = "preferences:update"
	PermissionLoyaltyView         Permission = "loyalty:read"
	PermissionLoyaltyRedeem       Permission = "loyalty:redeem"
	PermissionCatalogView         Permission = "catalog:read"
	PermissionCatalogManage       Permission = "catalog:manage"
	PermissionPaymentsView        Permission = "payments:read"
	PermissionPaymentsManage      Permission = "payments:manage"
	PermissionLogisticsView       Permission = "logistics:read"
	PermissionLogisticsDispatch   Permission = "logistics:dispatch"
	PermissionOperationsKitchen   Permission = "operations:kitchen"
	PermissionOperationsInventory Permission = "operations:inventory"
	PermissionNotificationsView   Permission = "notifications:read"
	PermissionNotificationsManage Permission = "notifications:manage"
	PermissionAnalyticsView       Permission = "analytics:read"
	PermissionAnalyticsExport     Permission = "analytics:export"
	PermissionSupportView         Permission = "support:read"
	PermissionSupportManage       Permission = "support:manage"
	PermissionZonesView           Permission = "zones:read"
	PermissionZonesManage         Permission = "zones:manage"
	PermissionRidersOnboard       Permission = "riders:onboard" // Kept for backward compatibility if used elsewhere
	PermissionStaffInvite         Permission = "staff:invite"   // Kept for backward compatibility if used elsewhere
	PermissionAdminManage         Permission = "admin:manage"
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
	case RoleDriver, RoleRider:
		return []Permission{
			PermissionOrdersView,
			PermissionProfileUpdate,
			PermissionPreferencesUpdate,
		}
	case RoleStaff:
		return []Permission{
			PermissionOrdersView,
			PermissionOrdersManage,
			PermissionCatalogView,
			PermissionCatalogManage,
			PermissionProfileUpdate,
			PermissionPreferencesUpdate,
			PermissionNotificationsView,
			PermissionRidersOnboard,
		}
	case RoleSuperAdmin:
		return []Permission{
			PermissionOrdersView,
			PermissionOrdersManage,
			PermissionOrdersRefund,
			PermissionCatalogView,
			PermissionCatalogManage,
			PermissionPaymentsView,
			PermissionPaymentsManage,
			PermissionLogisticsView,
			PermissionLogisticsDispatch,
			PermissionOperationsKitchen,
			PermissionOperationsInventory,
			PermissionNotificationsView,
			PermissionNotificationsManage,
			PermissionAnalyticsView,
			PermissionAnalyticsExport,
			PermissionSupportView,
			PermissionSupportManage,
			PermissionAdminManage,
			PermissionProfileUpdate,
			PermissionPreferencesUpdate,
			PermissionLoyaltyView,
			PermissionLoyaltyRedeem,
			PermissionRidersOnboard,
			PermissionStaffInvite,
		}
	case RoleAdmin:
		return []Permission{
			PermissionOrdersView,
			PermissionOrdersManage,
			PermissionOrdersRefund,
			PermissionCatalogView,
			PermissionCatalogManage,
			PermissionPaymentsView,
			PermissionPaymentsManage,
			PermissionLogisticsView,
			PermissionLogisticsDispatch,
			PermissionOperationsKitchen,
			PermissionOperationsInventory,
			PermissionNotificationsView,
			PermissionNotificationsManage,
			PermissionAnalyticsView,
			PermissionAnalyticsExport,
			PermissionSupportView,
			PermissionSupportManage,
			PermissionAdminManage,
			PermissionRidersOnboard,
			PermissionStaffInvite,
		}
	default:
		return []Permission{}
	}
}

// ConsolidatePermissions calculates the union of permissions for a list of roles.
func ConsolidatePermissions(roles []Role) []Permission {
	permMap := make(map[Permission]bool)
	for _, r := range roles {
		for _, p := range DefaultPermissions(r) {
			permMap[p] = true
		}
	}
	var perms []Permission
	for p := range permMap {
		perms = append(perms, p)
	}
	if perms == nil {
		return []Permission{}
	}
	return perms
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
	AuthServiceUserID    *uuid.UUID             `json:"authServiceUserId,omitempty"` // Reference to auth-service user
	Email                string                 `json:"email"`
	PasswordHash         string                 `json:"-"` // Deprecated: Password handled by auth-service
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
	SyncStatus           string                 `json:"syncStatus,omitempty"` // pending, synced, failed
	SyncAt               *time.Time             `json:"syncAt,omitempty"`
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
