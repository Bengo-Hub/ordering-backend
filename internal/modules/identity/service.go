package identity

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/config"
	"github.com/bengobox/ordering-backend/internal/modules/tenant"
)

// DefaultTenantSlug is the default tenant slug for the ordering service (e.g. urban-loft).
// This matches config.DefaultTenantSlug but is defined here to avoid circular dependencies.
const DefaultTenantSlug = config.DefaultTenantSlug

// Service coordinates identity workflows across persistence and token services.
// Authentication is handled entirely by the SSO (auth-service) via OIDC.
// This service manages local user records synced from auth-service via NATS events.
type Service struct {
	repo         Repository
	authCfg      config.AuthConfig
	tokenSigner  *TokenSigner
	logger       *zap.Logger
	now          func() time.Time
	tenantSyncer *tenant.Syncer
}

// NewService constructs the identity service with provided dependencies.
func NewService(repo Repository, authCfg config.AuthConfig, logger *zap.Logger, tenantSyncer *tenant.Syncer) (*Service, error) {
	tokenSigner, err := NewTokenSigner(authCfg)
	if err != nil {
		return nil, fmt.Errorf("identity: token signer: %w", err)
	}

	svc := &Service{
		repo:         repo,
		authCfg:      authCfg,
		tokenSigner:  tokenSigner,
		logger:       logger.Named("identity.Service"),
		now:          time.Now,
		tenantSyncer: tenantSyncer,
	}

	return svc, nil
}

// SyncUserFromAuthService syncs user data from auth-service to local database.
// This is exported for use by event handlers.
func (s *Service) SyncUserFromAuthService(ctx context.Context, authServiceUserID uuid.UUID, tenantID string, authUserData map[string]interface{}, accessToken string) (*User, error) {
	return s.syncUserFromAuthService(ctx, authServiceUserID, tenantID, authUserData, accessToken)
}

// EnsureUserFromToken (JIT provisioning): returns the local user for the given auth-service user ID,
// creating a minimal user from token claims if not found. Used when a valid JWT is presented but
// the user does not yet exist locally (e.g. NATS sync delayed). tenantIDOrSlug is either a tenant UUID
// string or a tenant slug (e.g. "urban-loft"); it is resolved to tenant ID for user creation.
// authUserData may be nil to use defaults.
func (s *Service) EnsureUserFromToken(ctx context.Context, authServiceUserID uuid.UUID, tenantIDOrSlug string, authUserData map[string]interface{}) (*User, error) {
	user, err := s.repo.FindUserByAuthServiceID(ctx, authServiceUserID)
	if err == nil && user != nil {
		return user, nil
	}
	tenantID := tenantIDOrSlug
	if tenantIDOrSlug != "" {
		if _, err := uuid.Parse(tenantIDOrSlug); err != nil {
			// JIT Sync Tenant before we hit DB for it.
			if s.tenantSyncer != nil {
				realTenantID, syncErr := s.tenantSyncer.SyncTenant(ctx, tenantIDOrSlug)
				if syncErr != nil {
					s.logger.Warn("tenant sync failed during JIT user provisioning", zap.Error(syncErr))
				} else {
					tenantID = realTenantID.String()
				}
			}

			// If tenantID is still the slug, resolving it from local DB
			if tenantID == tenantIDOrSlug {
				t, err := s.repo.FindTenantBySlug(ctx, tenantIDOrSlug)
				if err != nil {
					return nil, fmt.Errorf("identity: resolve tenant for JIT: %w", err)
				}
				tenantID = t.ID.String()
			}
		}
	}
	if authUserData == nil {
		authUserData = map[string]interface{}{}
	}
	return s.createUserFromAuthService(ctx, authServiceUserID, tenantID, authUserData)
}

// syncUserFromAuthService syncs user data from auth-service to local database.
func (s *Service) syncUserFromAuthService(ctx context.Context, authServiceUserID uuid.UUID, tenantID string, authUserData map[string]interface{}, accessToken string) (*User, error) {
	// Try to find existing user by auth_service_user_id
	user, err := s.repo.FindUserByAuthServiceID(ctx, authServiceUserID)
	if err == nil && user != nil {
		// Update existing user
		return s.updateUserFromAuthService(ctx, user, authUserData)
	}

	// Try to find by email
	email, _ := authUserData["email"].(string)
	if email != "" {
		user, err = s.repo.FindUserByEmail(ctx, email)
		if err == nil && user != nil {
			// Link auth_service_user_id to existing user
			user.AuthServiceUserID = &authServiceUserID
			now := s.now()
			user.SyncAt = &now
			user.SyncStatus = "synced"
			if err := s.repo.UpdateUser(ctx, user); err != nil {
				return nil, fmt.Errorf("identity: update user with auth-service ID: %w", err)
			}
			return s.updateUserFromAuthService(ctx, user, authUserData)
		}
	}

	// Create new user
	return s.createUserFromAuthService(ctx, authServiceUserID, tenantID, authUserData)
}

// updateUserFromAuthService updates local user with data from auth-service.
func (s *Service) updateUserFromAuthService(ctx context.Context, user *User, authUserData map[string]interface{}) (*User, error) {
	// Update identity fields from auth-service
	if email, ok := authUserData["email"].(string); ok && email != "" {
		user.Email = email
	}
	if fullName, ok := authUserData["full_name"].(string); ok && fullName != "" {
		user.FullName = fullName
	}
	if phone, ok := authUserData["phone"].(string); ok {
		user.Phone = phone
	}
	if status, ok := authUserData["status"].(string); ok {
		user.Status = status
	}

	now := s.now()
	user.SyncAt = &now
	user.SyncStatus = "synced"
	user.UpdatedAt = now
	user.LastLoginAt = &now

	// Extract and merge roles and permissions from auth-service
	rolesFromAuth := extractRolesFromAuthServiceUser(authUserData, user.Email)
	permissionsFromAuth := extractPermissionsFromAuthServiceUser(authUserData)
	if len(rolesFromAuth) > 0 {
		user.Roles = mergeRoles(user.Roles, rolesFromAuth)
	}
	if len(permissionsFromAuth) > 0 {
		user.Permissions = mergePermissions(user.Permissions, permissionsFromAuth)
	} else if len(rolesFromAuth) > 0 {
		// Fallback to role-based permission consolidation only if no explicit permissions were provided
		user.Permissions = ConsolidatePermissions(user.Roles)
	}

	if err := s.repo.UpdateUser(ctx, user); err != nil {
		return nil, fmt.Errorf("identity: update user from auth-service: %w", err)
	}

	return user, nil
}

// createUserFromAuthService creates a new local user from auth-service data.
func (s *Service) createUserFromAuthService(ctx context.Context, authServiceUserID uuid.UUID, tenantID string, authUserData map[string]interface{}) (*User, error) {
	// Use default tenant if not provided
	if tenantID == "" {
		tenantID = DefaultTenantSlug
	}

	email, _ := authUserData["email"].(string)
	fullName, _ := authUserData["full_name"].(string)
	if fullName == "" {
		// Try alternative field names
		if name, ok := authUserData["name"].(string); ok {
			fullName = name
		}
	}
	if fullName == "" {
		fullName = email // Fallback to email
	}

	phone, _ := authUserData["phone"].(string)
	status, _ := authUserData["status"].(string)
	if status == "" {
		status = "active"
	}

	// Determine roles and permissions
	roles := []Role{RoleCustomer} // Default role
	rolesFromAuth := extractRolesFromAuthServiceUser(authUserData, email)
	if len(rolesFromAuth) > 0 {
		roles = mergeRoles(roles, rolesFromAuth)
	}
	
	permissionsFromAuth := extractPermissionsFromAuthServiceUser(authUserData)
	var permissions []Permission
	if len(permissionsFromAuth) > 0 {
		permissions = permissionsFromAuth
	} else {
		permissions = ConsolidatePermissions(roles)
	}

	now := s.now()
	user := &User{
		ID:                authServiceUserID,
		TenantID:          tenantID,
		AuthServiceUserID: &authServiceUserID,
		Email:             email,
		FullName:          fullName,
		Phone:             phone,
		Status:            status,
		Roles:             roles,
		Permissions:       permissions,
		SyncStatus:        "synced",
		SyncAt:            &now,
		Preferences: Preferences{
			Theme:    "system",
			Language: "en",
			Notifications: NotificationPreferences{
				Email: true,
				SMS:   false,
				Push:  true,
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.repo.CreateUser(ctx, user); err != nil {
		return nil, fmt.Errorf("identity: create user from auth-service: %w", err)
	}

	return user, nil
}

// extractRolesFromAuthServiceUser extracts roles from auth-service user data.
func extractRolesFromAuthServiceUser(authUserData map[string]interface{}, email string) []Role {
	var roles []Role

	// Platform owner with superuser role gets full admin access
	isPlatformOwner, _ := authUserData["is_platform_owner"].(bool)
	if isPlatformOwner {
		return []Role{RoleSuperAdmin, RoleAdmin}
	}

	// Check for roles array
	if rolesData, ok := authUserData["roles"].([]interface{}); ok {
		for _, r := range rolesData {
			if roleStr, ok := r.(string); ok {
				switch roleStr {
				case "superuser":
					roles = append(roles, RoleSuperAdmin)
				case "admin":
					roles = append(roles, RoleAdmin)
				case "staff":
					roles = append(roles, RoleStaff)
				case "driver", "rider":
					roles = append(roles, RoleDriver)
				case "customer", "user":
					roles = append(roles, RoleCustomer)
				}
			}
		}
	}

	// Also check for string roles (not just array)
	if rolesData, ok := authUserData["roles"].([]string); ok {
		for _, roleStr := range rolesData {
			switch strings.ToLower(roleStr) {
			case "superuser":
				roles = append(roles, RoleSuperAdmin)
			case "admin":
				roles = append(roles, RoleAdmin)
			case "staff":
				roles = append(roles, RoleStaff)
			case "driver", "rider":
				roles = append(roles, RoleDriver)
			case "customer", "user":
				roles = append(roles, RoleCustomer)
			}
		}
	}

	return roles
}

// extractPermissionsFromAuthServiceUser extracts permissions from auth-service user data.
func extractPermissionsFromAuthServiceUser(authUserData map[string]interface{}) []Permission {
	var permissions []Permission

	// Check for permissions array
	if permsData, ok := authUserData["permissions"].([]interface{}); ok {
		for _, p := range permsData {
			if permStr, ok := p.(string); ok {
				permissions = append(permissions, Permission(permStr))
			}
		}
	}

	// Also check for string permissions (not just array)
	if permsData, ok := authUserData["permissions"].([]string); ok {
		for _, permStr := range permsData {
			permissions = append(permissions, Permission(permStr))
		}
	}

	return permissions
}

// mergePermissions merges two permission slices, removing duplicates.
func mergePermissions(existing []Permission, newPerms []Permission) []Permission {
	permMap := make(map[Permission]bool)
	for _, p := range existing {
		permMap[p] = true
	}
	for _, p := range newPerms {
		permMap[p] = true
	}

	var merged []Permission
	for p := range permMap {
		merged = append(merged, p)
	}
	return merged
}

// mergeRoles merges two role slices, removing duplicates.
func mergeRoles(existing []Role, newRoles []Role) []Role {
	roleMap := make(map[Role]bool)
	for _, r := range existing {
		roleMap[r] = true
	}
	for _, r := range newRoles {
		roleMap[r] = true
	}

	var merged []Role
	for r := range roleMap {
		merged = append(merged, r)
	}
	return merged
}

// GetUser returns a user by identifier.
func (s *Service) GetUser(ctx context.Context, id uuid.UUID) (*User, error) {
	return s.repo.FindUserByID(ctx, id)
}


// GetOrders returns order summaries.
func (s *Service) GetOrders(ctx context.Context, userID uuid.UUID) ([]*OrderSummary, error) {
	return s.repo.ListOrdersByUser(ctx, userID)
}

// UpdateProfile mutates user profile fields.
func (s *Service) UpdateProfile(ctx context.Context, id uuid.UUID, input ProfileUpdateInput) (*User, error) {
	user, err := s.repo.FindUserByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if input.FullName != nil {
		user.FullName = *input.FullName
	}
	if input.Phone != nil {
		user.Phone = *input.Phone
	}
	if input.AvatarURL != nil {
		user.AvatarURL = *input.AvatarURL
	}

	user.UpdatedAt = s.now()

	if err := s.repo.UpdateUser(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}

// UpdatePreferences mutates user preferences.
func (s *Service) UpdatePreferences(ctx context.Context, id uuid.UUID, input PreferencesUpdateInput) (*User, error) {
	user, err := s.repo.FindUserByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if input.Theme != nil {
		user.Preferences.Theme = *input.Theme
	}
	if input.Language != nil {
		user.Preferences.Language = *input.Language
	}
	if input.Notifications != nil {
		user.Preferences.Notifications = *input.Notifications
	}

	user.UpdatedAt = s.now()

	if err := s.repo.UpdateUser(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}


// RequestMeta captures HTTP metadata for session logging.
type RequestMeta struct {
	UserAgent string
	IP        string
}

// ProfileUpdateInput for updating profile fields.
type ProfileUpdateInput struct {
	FullName  *string
	Phone     *string
	AvatarURL *string
}

// PreferencesUpdateInput for updating preferences.
type PreferencesUpdateInput struct {
	Theme         *string
	Language      *string
	Notifications *NotificationPreferences
}

