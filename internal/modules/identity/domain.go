package identity

import (
	"time"

	"github.com/google/uuid"
)

// Role represents the high-level access tier for a user.
type Role string

const (
	RoleCustomer            Role = "customer"
	RoleDriver              Role = "driver" // Universal role for delivery/courier/taxi use cases
	RoleRider               Role = "rider"  // Deprecated: use RoleDriver; kept for backward compatibility
	RoleStaff               Role = "staff"
	RoleManager             Role = "manager"
	RoleKitchen             Role = "kitchen"
	RoleCashier             Role = "cashier"
	RoleDeliveryCoordinator Role = "delivery_coordinator"
	RoleViewer              Role = "viewer"
	RoleAdmin               Role = "admin"
	RoleSuperAdmin          Role = "superuser"
)

// Permission captures fine-grained feature access across the platform.
type Permission string

const (
	PermissionOrdersView          Permission = "ordering.orders.view"
	PermissionOrdersManage        Permission = "ordering.orders.manage"
	PermissionOrdersRefund        Permission = "ordering.orders.delete" // refund mapped to delete (destructive action)
	PermissionProfileUpdate       Permission = "ordering.users.change_own"
	PermissionPreferencesUpdate   Permission = "ordering.config.change_own"
	PermissionLoyaltyView         Permission = "ordering.loyalty.view"
	PermissionLoyaltyRedeem       Permission = "ordering.loyalty.change"
	PermissionCatalogView         Permission = "ordering.catalog.view"
	PermissionCatalogManage       Permission = "ordering.catalog.manage"
	PermissionPaymentsView        Permission = "ordering.orders.view" // payments projection — same as orders view
	PermissionPaymentsManage      Permission = "ordering.orders.manage"
	PermissionLogisticsView       Permission = "ordering.orders.view" // logistics projection — same as orders view
	PermissionLogisticsDispatch   Permission = "ordering.orders.change"
	PermissionOperationsKitchen   Permission = "ordering.orders.change"
	PermissionOperationsInventory Permission = "ordering.catalog.view"
	PermissionNotificationsView   Permission = "ordering.config.view"
	PermissionNotificationsManage Permission = "ordering.config.manage"
	PermissionAnalyticsView       Permission = "ordering.analytics.view"
	PermissionAnalyticsExport     Permission = "ordering.analytics.manage"
	PermissionSupportView         Permission = "ordering.orders.view_own"
	PermissionSupportManage       Permission = "ordering.orders.manage"
	PermissionZonesView           Permission = "ordering.delivery_zones.view"
	PermissionZonesManage         Permission = "ordering.delivery_zones.manage"
	PermissionRidersOnboard       Permission = "ordering.users.add"
	PermissionStaffInvite         Permission = "ordering.users.add"
	PermissionAdminManage         Permission = "ordering.config.manage"
	PermissionRbacManage          Permission = "ordering.users.manage"
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
			PermissionRbacManage,
			PermissionRidersOnboard,
			PermissionStaffInvite,
		}
	case RoleManager:
		// Mirrors seedOrderingRoles "manager" grants in cmd/seed/main.go.
		return []Permission{
			// orders, catalog, outlets, promotions, delivery_zones, delivery_windows, loyalty (all actions)
			Permission("ordering.orders.add"), Permission("ordering.orders.view"), Permission("ordering.orders.view_own"),
			Permission("ordering.orders.change"), Permission("ordering.orders.change_own"), Permission("ordering.orders.delete"),
			Permission("ordering.orders.delete_own"), Permission("ordering.orders.manage"), Permission("ordering.orders.manage_own"),
			Permission("ordering.catalog.add"), Permission("ordering.catalog.view"), Permission("ordering.catalog.view_own"),
			Permission("ordering.catalog.change"), Permission("ordering.catalog.change_own"), Permission("ordering.catalog.delete"),
			Permission("ordering.catalog.delete_own"), Permission("ordering.catalog.manage"), Permission("ordering.catalog.manage_own"),
			Permission("ordering.outlets.add"), Permission("ordering.outlets.view"), Permission("ordering.outlets.view_own"),
			Permission("ordering.outlets.change"), Permission("ordering.outlets.change_own"), Permission("ordering.outlets.delete"),
			Permission("ordering.outlets.delete_own"), Permission("ordering.outlets.manage"), Permission("ordering.outlets.manage_own"),
			Permission("ordering.promotions.add"), Permission("ordering.promotions.view"), Permission("ordering.promotions.view_own"),
			Permission("ordering.promotions.change"), Permission("ordering.promotions.change_own"), Permission("ordering.promotions.delete"),
			Permission("ordering.promotions.delete_own"), Permission("ordering.promotions.manage"), Permission("ordering.promotions.manage_own"),
			Permission("ordering.delivery_zones.add"), Permission("ordering.delivery_zones.view"), Permission("ordering.delivery_zones.view_own"),
			Permission("ordering.delivery_zones.change"), Permission("ordering.delivery_zones.change_own"), Permission("ordering.delivery_zones.delete"),
			Permission("ordering.delivery_zones.delete_own"), Permission("ordering.delivery_zones.manage"), Permission("ordering.delivery_zones.manage_own"),
			Permission("ordering.delivery_windows.add"), Permission("ordering.delivery_windows.view"), Permission("ordering.delivery_windows.view_own"),
			Permission("ordering.delivery_windows.change"), Permission("ordering.delivery_windows.change_own"), Permission("ordering.delivery_windows.delete"),
			Permission("ordering.delivery_windows.delete_own"), Permission("ordering.delivery_windows.manage"), Permission("ordering.delivery_windows.manage_own"),
			Permission("ordering.loyalty.add"), Permission("ordering.loyalty.view"), Permission("ordering.loyalty.view_own"),
			Permission("ordering.loyalty.change"), Permission("ordering.loyalty.change_own"), Permission("ordering.loyalty.delete"),
			Permission("ordering.loyalty.delete_own"), Permission("ordering.loyalty.manage"), Permission("ordering.loyalty.manage_own"),
			// extras
			Permission("ordering.analytics.view"), Permission("ordering.analytics.view_own"),
			Permission("ordering.users.view"), Permission("ordering.users.view_own"), Permission("ordering.users.change_own"),
			Permission("ordering.config.view"),
		}
	case RoleKitchen:
		// Mirrors seedOrderingRoles "kitchen" grants in cmd/seed/main.go.
		return []Permission{
			Permission("ordering.orders.view"), Permission("ordering.orders.change"), Permission("ordering.orders.manage"),
			Permission("ordering.catalog.view"),
			Permission("ordering.users.view_own"), Permission("ordering.users.change_own"),
		}
	case RoleCashier:
		// Mirrors seedOrderingRoles "cashier" grants in cmd/seed/main.go.
		return []Permission{
			Permission("ordering.orders.add"), Permission("ordering.orders.view"), Permission("ordering.orders.change"), Permission("ordering.orders.manage"),
			Permission("ordering.catalog.view"),
			Permission("ordering.promotions.view"),
			Permission("ordering.loyalty.view"), Permission("ordering.loyalty.manage"),
			Permission("ordering.users.view_own"), Permission("ordering.users.change_own"),
		}
	case RoleDeliveryCoordinator:
		// Mirrors seedOrderingRoles "delivery_coordinator" grants in cmd/seed/main.go.
		return []Permission{
			Permission("ordering.orders.view"), Permission("ordering.orders.change"),
			Permission("ordering.delivery_zones.view"), Permission("ordering.delivery_zones.change"), Permission("ordering.delivery_zones.manage"),
			Permission("ordering.delivery_windows.view"), Permission("ordering.delivery_windows.change"), Permission("ordering.delivery_windows.manage"),
			Permission("ordering.outlets.view"),
			Permission("ordering.users.view_own"), Permission("ordering.users.change_own"),
		}
	case RoleViewer:
		// Mirrors seedOrderingRoles "viewer" grants in cmd/seed/main.go.
		return []Permission{
			Permission("ordering.orders.view"),
			Permission("ordering.catalog.view"),
			Permission("ordering.outlets.view"),
			Permission("ordering.promotions.view"),
			Permission("ordering.delivery_zones.view"),
			Permission("ordering.delivery_windows.view"),
			Permission("ordering.loyalty.view"),
			Permission("ordering.analytics.view"),
			Permission("ordering.config.view"),
			Permission("ordering.users.view_own"),
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
