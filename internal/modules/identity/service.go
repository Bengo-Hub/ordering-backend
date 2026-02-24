package identity

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/config"
)

// DefaultTenantSlug is the default tenant slug for the ordering service (empty = no default).
// This matches config.DefaultTenantSlug but is defined here to avoid circular dependencies.
const DefaultTenantSlug = config.DefaultTenantSlug

// Service coordinates identity workflows across persistence and token services.
// Authentication is handled entirely by the SSO (auth-service) via OIDC.
// This service manages local user records synced from auth-service via NATS events.
type Service struct {
	repo        Repository
	authCfg     config.AuthConfig
	tokenSigner *TokenSigner
	logger      *zap.Logger
	now         func() time.Time
}

// NewService constructs the identity service with provided dependencies.
func NewService(repo Repository, authCfg config.AuthConfig, logger *zap.Logger) (*Service, error) {
	tokenSigner, err := NewTokenSigner(authCfg)
	if err != nil {
		return nil, fmt.Errorf("identity: token signer: %w", err)
	}

	svc := &Service{
		repo:        repo,
		authCfg:     authCfg,
		tokenSigner: tokenSigner,
		logger:      logger.Named("identity.Service"),
		now:         time.Now,
	}

	return svc, nil
}

// SyncUserFromAuthService syncs user data from auth-service to local database.
// This is exported for use by event handlers.
func (s *Service) SyncUserFromAuthService(ctx context.Context, authServiceUserID uuid.UUID, tenantID string, authUserData map[string]interface{}, accessToken string) (*User, error) {
	return s.syncUserFromAuthService(ctx, authServiceUserID, tenantID, authUserData, accessToken)
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

	// Extract and merge roles from auth-service
	rolesFromAuth := extractRolesFromAuthServiceUser(authUserData, user.Email)
	if len(rolesFromAuth) > 0 {
		user.Roles = mergeRoles(user.Roles, rolesFromAuth)
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

	// Determine roles
	roles := []Role{RoleCustomer} // Default role
	rolesFromAuth := extractRolesFromAuthServiceUser(authUserData, email)
	if len(rolesFromAuth) > 0 {
		roles = mergeRoles(roles, rolesFromAuth)
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
		Permissions:       ConsolidatePermissions(roles),
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

	// Super user bypass
	if email == "admin@codevertexitsolutions.com" {
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
				case "rider":
					roles = append(roles, RoleRider)
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
			case "rider":
				roles = append(roles, RoleRider)
			case "customer", "user":
				roles = append(roles, RoleCustomer)
			}
		}
	}

	return roles
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

// GetSession returns a session by identifier.
func (s *Service) GetSession(ctx context.Context, id uuid.UUID) (*Session, error) {
	return s.repo.FindSessionByID(ctx, id)
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

// UpdateSecurity toggles MFA configuration.
func (s *Service) UpdateSecurity(ctx context.Context, id uuid.UUID, input SecurityUpdateInput) (*User, error) {
	user, err := s.repo.FindUserByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if input.EnableTwoFactor && input.DisableTwoFactor {
		return nil, ErrTwoFactorConflict
	}

	if input.EnableTwoFactor {
		user.TwoFactorEnabled = true
	} else if input.DisableTwoFactor {
		user.TwoFactorEnabled = false
	}

	user.UpdatedAt = s.now()

	if err := s.repo.UpdateUser(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

// VerifyAccessToken validates the JWT access token (legacy path for backward compatibility).
func (s *Service) VerifyAccessToken(ctx context.Context, token string) (*Claims, error) {
	claims, err := s.tokenSigner.VerifyAccessToken(token)
	if err != nil {
		return nil, err
	}

	// Confirm session still valid.
	sessionID, err := uuid.Parse(claims.SessionID)
	if err != nil {
		return nil, ErrInvalidCredentials
	}
	session, err := s.repo.FindSessionByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	if session.RevokedAt != nil || session.ExpiresAt.Before(s.now()) {
		return nil, ErrInvalidCredentials
	}

	return claims, nil
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

// SecurityUpdateInput toggles 2FA configuration.
type SecurityUpdateInput struct {
	EnableTwoFactor  bool
	DisableTwoFactor bool
}
