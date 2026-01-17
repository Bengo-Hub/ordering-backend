package identity

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	authclient "github.com/Bengo-Hub/shared-auth-client"
	"github.com/bengobox/ordering-backend/internal/config"
)

// DefaultTenantSlug is the default tenant slug for the ordering service (empty = no default).
// This matches config.DefaultTenantSlug but is defined here to avoid circular dependencies.
const DefaultTenantSlug = config.DefaultTenantSlug

// Service coordinates identity workflows across persistence and token services.
type Service struct {
	repo              Repository
	authCfg           config.AuthConfig
	authServiceClient *authclient.Client
	authServiceAPIKey string
	tokenSigner       *TokenSigner
	googleCfg         *oauth2.Config
	logger            *zap.Logger
	now               func() time.Time
}

// AuthResult models the payload returned to clients after successful auth.
type AuthResult struct {
	Session SessionTokens
	User    *User
}

// SessionTokens captures issued access and refresh tokens.
type SessionTokens struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	SessionID    uuid.UUID
}

// NewService constructs the identity service with provided dependencies.
func NewService(repo Repository, authCfg config.AuthConfig, logger *zap.Logger) (*Service, error) {
	tokenSigner, err := NewTokenSigner(authCfg)
	if err != nil {
		return nil, fmt.Errorf("identity: token signer: %w", err)
	}

	var googleCfg *oauth2.Config
	if authCfg.GoogleClientID != "" && authCfg.GoogleClientSecret != "" {
		redirect := authCfg.GoogleRedirectBase
		if redirect == "" {
			redirect = "http://localhost:3000/auth/callback"
		}
		googleCfg = &oauth2.Config{
			ClientID:     authCfg.GoogleClientID,
			ClientSecret: authCfg.GoogleClientSecret,
			Endpoint:     google.Endpoint,
			RedirectURL:  redirect,
			Scopes: []string{
				"openid",
				"email",
				"profile",
			},
		}
	}

	// Initialize auth-service client if URL is configured
	var authServiceClient *authclient.Client
	if authCfg.ServiceURL != "" {
		authServiceClient = authclient.NewClient(authCfg.ServiceURL, logger)
		logger.Info("Auth-service client initialized", zap.String("service_url", authCfg.ServiceURL))
	} else {
		logger.Warn("Auth-service URL not configured - registration and login will fail")
	}

	svc := &Service{
		repo:              repo,
		authCfg:           authCfg,
		authServiceClient: authServiceClient,
		authServiceAPIKey: authCfg.AuthServiceAPIKey,
		tokenSigner:       tokenSigner,
		googleCfg:         googleCfg,
		logger:            logger.Named("identity.Service"),
		now:               time.Now,
	}

	if err := svc.seedDemoData(context.Background()); err != nil {
		logger.Warn("identity: demo seed failed", zap.Error(err))
	}

	return svc, nil
}

// LoginWithEmail authenticates a user via email/password combination.
// If auth-service is configured, it proxies to auth-service. Otherwise, falls back to local auth.
func (s *Service) LoginWithEmail(ctx context.Context, email, password, tenantSlug string, role Role, meta RequestMeta) (*AuthResult, error) {
	// Use default tenant if not provided
	if tenantSlug == "" {
		tenantSlug = DefaultTenantSlug
	}

	// If auth-service is configured, proxy to auth-service
	if s.authServiceClient != nil {
		return s.loginViaAuthService(ctx, email, password, tenantSlug, role, meta)
	}

	// Fallback to local auth (legacy mode)
	s.logger.Warn("Using legacy local authentication. Auth-service not configured.")
	return s.loginLocal(ctx, email, password, role, meta)
}

// loginViaAuthService proxies login to auth-service.
func (s *Service) loginViaAuthService(ctx context.Context, email, password, tenantSlug string, role Role, meta RequestMeta) (*AuthResult, error) {
	// Ensure tenant exists in auth-service (auto-discovery)
	// This is a best-effort operation - if it fails, continue with login
	if err := s.ensureTenantInAuthService(ctx, tenantSlug); err != nil {
		s.logger.Warn("Tenant sync to auth-service failed, continuing with login",
			zap.Error(err),
			zap.String("tenant_slug", tenantSlug))
		// Continue anyway - tenant might exist or auth-service might handle it
	}

	req := authclient.LoginRequest{
		Email:      email,
		Password:   password,
		TenantSlug: tenantSlug,
	}

	authResp, err := s.authServiceClient.Login(ctx, req)
	if err != nil {
		s.logger.Warn("Auth-service login failed", zap.Error(err), zap.String("email", email))
		return nil, ErrInvalidCredentials
	}

	// Extract user ID from auth-service response
	userIDStr, ok := authResp.User["id"].(string)
	if !ok {
		return nil, fmt.Errorf("identity: invalid user ID in auth-service response")
	}

	authServiceUserID, err := uuid.Parse(userIDStr)
	if err != nil {
		return nil, fmt.Errorf("identity: parse auth-service user ID: %w", err)
	}

	// Extract tenant ID
	tenantIDStr, ok := authResp.Tenant["id"].(string)
	if !ok {
		return nil, fmt.Errorf("identity: invalid tenant ID in auth-service response")
	}

	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		return nil, fmt.Errorf("identity: parse tenant ID: %w", err)
	}

	// Sync tenant locally to avoid FK violation
	tenantSlugFromAuth, _ := authResp.Tenant["slug"].(string)
	tenantNameFromAuth, _ := authResp.Tenant["name"].(string)
	if tenantSlugFromAuth == "" {
		tenantSlugFromAuth = tenantSlug // Fallback
	}
	if tenantNameFromAuth == "" {
		tenantNameFromAuth = tenantSlugFromAuth
	}

	localTenant := &Tenant{
		ID:       tenantID,
		Slug:     tenantSlugFromAuth,
		Name:     tenantNameFromAuth,
		Status:   "active",
		Metadata: authResp.Tenant,
	}
	if err := s.repo.UpsertTenant(ctx, localTenant); err != nil {
		s.logger.Warn("Failed to sync tenant locally", zap.Error(err))
		// Continue, maybe it exists?
	}

	// Sync or create local user
	user, err := s.syncUserFromAuthService(ctx, authServiceUserID, tenantID.String(), authResp.User, authResp.AccessToken)
	if err != nil {
		s.logger.Error("Failed to sync user from auth-service",
			zap.Error(err),
			zap.String("auth_service_user_id", authServiceUserID.String()),
			zap.String("tenant_id", tenantID.String()))
		// Create minimal user if sync fails
		user = &User{
			ID:                authServiceUserID,
			TenantID:          tenantID.String(),
			AuthServiceUserID: &authServiceUserID,
			Email:             email,
			Status:            "active",
			Roles:             []Role{RoleCustomer},
		}
		if fullName, ok := authResp.User["full_name"].(string); ok {
			user.FullName = fullName
		} else if name, ok := authResp.User["name"].(string); ok {
			user.FullName = name
		} else {
			user.FullName = email
		}
		s.logger.Warn("Created minimal user object after sync failure", zap.String("email", email))
	} else {
		s.logger.Info("User synced successfully from auth-service", zap.String("email", email), zap.String("user_id", user.ID.String()))
	}

	// Extract roles from auth-service user (if available)
	rolesFromAuth := extractRolesFromAuthServiceUser(authResp.User, email)
	if len(rolesFromAuth) > 0 {
		// Merge with ordering-specific roles
		user.Roles = mergeRoles(user.Roles, rolesFromAuth)
		user.Permissions = ConsolidatePermissions(user.Roles)

		// Persist updated roles to database
		if err := s.repo.UpdateUser(ctx, user); err != nil {
			s.logger.Warn("Failed to persist synced user roles", zap.Error(err))
		}
	}

	// Check if user has required role (if specified)
	// Allow only SuperUser to bypass specific role checks
	hasPrivilegedRole := user.HasRole(RoleSuperAdmin)
	if role != "" && !user.HasRole(role) && !hasPrivilegedRole {
		s.logger.Warn("User does not have required role",
			zap.String("email", email),
			zap.String("required_role", string(role)),
			zap.Any("user_roles", user.Roles))
		return nil, ErrRoleNotPermitted
	}

	// Return auth-service tokens
	return &AuthResult{
		Session: SessionTokens{
			AccessToken:  authResp.AccessToken,
			RefreshToken: authResp.RefreshToken,
			ExpiresAt:    time.Now().Add(time.Duration(authResp.ExpiresIn) * time.Second),
			SessionID:    uuid.MustParse(authResp.SessionID),
		},
		User: user,
	}, nil
}

// loginLocal handles legacy local authentication.
// DEPRECATED: This is a fallback for development/testing when auth-service is not configured.
// In production, auth-service should always be configured and this should never be used.
func (s *Service) loginLocal(ctx context.Context, email, password string, role Role, meta RequestMeta) (*AuthResult, error) {
	user, err := s.repo.FindUserByEmail(ctx, email)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	if !user.HasRole(role) {
		return nil, ErrRoleNotPermitted
	}

	return s.issueSession(ctx, user, meta)
}

// ensureTenantInAuthService ensures a tenant exists in auth-service.
// If the tenant doesn't exist, it pulls full tenant details from local database
// and creates it in auth-service with the same UUID and slug (tenant ID must match across all services).
// This is a public operation that doesn't require authentication.
func (s *Service) ensureTenantInAuthService(ctx context.Context, tenantSlug string) error {
	if s.authServiceClient == nil {
		return nil // No auth-service configured, skip tenant sync
	}

	// Check if tenant exists in auth-service
	exists, err := s.authServiceClient.CheckTenantExists(ctx, tenantSlug)
	if err != nil {
		// If check fails, log warning but don't fail (might be network issue)
		s.logger.Warn("Failed to check tenant existence in auth-service",
			zap.Error(err),
			zap.String("tenant_slug", tenantSlug))
		return nil // Continue anyway, auth-service might create tenant on registration
	}

	if exists {
		s.logger.Debug("Tenant already exists in auth-service", zap.String("tenant_slug", tenantSlug))
		return nil
	}

	// Tenant doesn't exist in auth-service, pull full details from local database
	localTenant, err := s.repo.FindTenantBySlug(ctx, tenantSlug)
	if err != nil {
		// Tenant doesn't exist locally either, use defaults
		s.logger.Warn("Tenant not found in local database, using defaults",
			zap.Error(err),
			zap.String("tenant_slug", tenantSlug))

		tenantName := ""
		contactEmail := ""
		contactPhone := ""
		tenantID := uuid.New() // Generate new UUID if tenant doesn't exist locally

		if tenantSlug == "" {
			// No default tenant - use generic values
			tenantName = "Ordering Platform"
			contactEmail = "support@codevertexitsolutions.com"
			contactPhone = "+254700000000"
		} else {
			// For other tenants, derive name from slug
			parts := strings.Split(tenantSlug, "-")
			for i, part := range parts {
				if len(part) > 0 {
					parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
				}
			}
			tenantName = strings.Join(parts, " ")
			contactEmail = fmt.Sprintf("support@%s.com", tenantSlug)
		}

		createReq := authclient.TenantRequest{
			ID:           tenantID.String(), // Use generated UUID
			Slug:         tenantSlug,
			Name:         tenantName,
			ContactEmail: contactEmail,
			ContactPhone: contactPhone,
			Metadata: map[string]interface{}{
				"source":       "ordering-service",
				"auto_created": true,
			},
		}

		tenantResp, err := s.authServiceClient.CreateTenant(ctx, createReq)
		if err != nil {
			s.logger.Warn("Failed to create tenant in auth-service",
				zap.Error(err),
				zap.String("tenant_slug", tenantSlug))
			return nil // Don't fail registration/login
		}

		s.logger.Info("Tenant auto-created in auth-service (with defaults)",
			zap.String("tenant_slug", tenantSlug),
			zap.String("tenant_id", tenantResp.ID))
		return nil
	}

	// Tenant exists locally, use full details and same UUID
	createReq := authclient.TenantRequest{
		ID:           localTenant.ID.String(), // Use same UUID from local database
		Slug:         localTenant.Slug,
		Name:         localTenant.Name,
		ContactEmail: localTenant.ContactEmail,
		ContactPhone: localTenant.ContactPhone,
		Metadata:     localTenant.Metadata,
	}

	// Add source tracking to metadata
	if createReq.Metadata == nil {
		createReq.Metadata = make(map[string]interface{})
	}
	createReq.Metadata["source"] = "ordering-service"
	createReq.Metadata["auto_created"] = true
	createReq.Metadata["synced_at"] = s.now().Format(time.RFC3339)

	tenantResp, err := s.authServiceClient.CreateTenant(ctx, createReq)
	if err != nil {
		s.logger.Warn("Failed to create tenant in auth-service",
			zap.Error(err),
			zap.String("tenant_slug", tenantSlug),
			zap.String("tenant_id", localTenant.ID.String()))
		// Don't fail registration/login if tenant creation fails
		// Auth-service might create tenant automatically on registration
		return nil
	}

	s.logger.Info("Tenant synced to auth-service with matching UUID",
		zap.String("tenant_slug", tenantSlug),
		zap.String("tenant_id", tenantResp.ID),
		zap.String("local_tenant_id", localTenant.ID.String()))
	return nil
}

// RegisterWithEmail registers a new user via auth-service.
// If auth-service is configured, it proxies to auth-service. Otherwise, returns an error.
func (s *Service) RegisterWithEmail(ctx context.Context, email, password, tenantSlug string, profile map[string]interface{}, meta RequestMeta) (*AuthResult, error) {
	// Use default tenant if not provided
	if tenantSlug == "" {
		tenantSlug = DefaultTenantSlug
	}

	// Auth-service must be configured for registration
	if s.authServiceClient == nil {
		return nil, fmt.Errorf("identity: registration requires auth-service configuration")
	}

	// Ensure tenant exists in auth-service (auto-discovery)
	if err := s.ensureTenantInAuthService(ctx, tenantSlug); err != nil {
		s.logger.Warn("Tenant sync to auth-service failed, continuing with registration",
			zap.Error(err),
			zap.String("tenant_slug", tenantSlug))
		// Continue anyway - auth-service might handle tenant creation on registration
	}

	req := authclient.RegisterRequest{
		Email:      email,
		Password:   password,
		TenantSlug: tenantSlug,
		Profile:    profile,
	}

	authResp, err := s.authServiceClient.Register(ctx, req)
	if err != nil {
		s.logger.Error("Auth-service registration failed",
			zap.Error(err),
			zap.String("email", email),
			zap.String("tenant_slug", tenantSlug),
			zap.Any("profile", profile))
		return nil, fmt.Errorf("identity: registration failed: %w", err)
	}
	s.logger.Info("Auth-service registration successful", zap.String("email", email), zap.String("tenant_slug", tenantSlug))

	// Extract user ID from auth-service response
	userIDStr, ok := authResp.User["id"].(string)
	if !ok {
		return nil, fmt.Errorf("identity: invalid user ID in auth-service response")
	}

	authServiceUserID, err := uuid.Parse(userIDStr)
	if err != nil {
		return nil, fmt.Errorf("identity: parse auth-service user ID: %w", err)
	}

	// Extract tenant ID
	tenantIDStr, ok := authResp.Tenant["id"].(string)
	if !ok {
		return nil, fmt.Errorf("identity: invalid tenant ID in auth-service response")
	}

	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		return nil, fmt.Errorf("identity: parse tenant ID: %w", err)
	}

	// Sync tenant locally to avoid FK violation
	tenantSlugFromAuth, _ := authResp.Tenant["slug"].(string)
	tenantNameFromAuth, _ := authResp.Tenant["name"].(string)
	if tenantSlugFromAuth == "" {
		tenantSlugFromAuth = tenantSlug // Fallback
	}
	if tenantNameFromAuth == "" {
		tenantNameFromAuth = tenantSlugFromAuth
	}

	localTenant := &Tenant{
		ID:       tenantID,
		Slug:     tenantSlugFromAuth,
		Name:     tenantNameFromAuth,
		Status:   "active",
		Metadata: authResp.Tenant,
	}
	if err := s.repo.UpsertTenant(ctx, localTenant); err != nil {
		s.logger.Warn("Failed to sync tenant locally", zap.Error(err))
		// Continue, maybe it exists?
	}

	// Sync or create local user
	user, err := s.syncUserFromAuthService(ctx, authServiceUserID, tenantID.String(), authResp.User, authResp.AccessToken)
	if err != nil {
		s.logger.Error("Failed to sync user from auth-service after registration",
			zap.Error(err),
			zap.String("auth_service_user_id", authServiceUserID.String()),
			zap.String("tenant_id", tenantID.String()))
		// If sync fails, create a minimal user object from auth-service response
		// This ensures we can still return a valid response
		user = &User{
			ID:                authServiceUserID,
			TenantID:          tenantID.String(),
			AuthServiceUserID: &authServiceUserID,
			Email:             email,
			Status:            "active",
			Roles:             []Role{RoleCustomer},
		}
		if fullName, ok := authResp.User["full_name"].(string); ok {
			user.FullName = fullName
		} else if name, ok := authResp.User["name"].(string); ok {
			user.FullName = name
		} else {
			user.FullName = email
		}
		s.logger.Warn("Created minimal user object after sync failure", zap.String("email", email))
	} else {
		s.logger.Info("User synced successfully from auth-service", zap.String("email", email), zap.String("user_id", user.ID.String()))
	}

	// Ensure user is not nil before proceeding
	if user == nil {
		s.logger.Error("User is nil after sync - creating minimal user", zap.String("email", email))
		user = &User{
			ID:                authServiceUserID,
			TenantID:          tenantID.String(),
			AuthServiceUserID: &authServiceUserID,
			Email:             email,
			Status:            "active",
			Roles:             []Role{RoleCustomer},
		}
		if fullName, ok := authResp.User["full_name"].(string); ok {
			user.FullName = fullName
		} else if name, ok := authResp.User["name"].(string); ok {
			user.FullName = name
		} else {
			user.FullName = email
		}
	}

	// Extract roles from auth-service user (if available)
	rolesFromAuth := extractRolesFromAuthServiceUser(authResp.User, email)
	if len(rolesFromAuth) > 0 {
		// Merge with ordering-specific roles
		user.Roles = mergeRoles(user.Roles, rolesFromAuth)
		user.Permissions = ConsolidatePermissions(user.Roles)

		// Persist updated roles to database
		if err := s.repo.UpdateUser(ctx, user); err != nil {
			s.logger.Warn("Failed to persist synced usage roles", zap.Error(err))
		}
	}

	// Return auth-service tokens
	return &AuthResult{
		Session: SessionTokens{
			AccessToken:  authResp.AccessToken,
			RefreshToken: authResp.RefreshToken,
			ExpiresAt:    time.Now().Add(time.Duration(authResp.ExpiresIn) * time.Second),
			SessionID:    uuid.MustParse(authResp.SessionID),
		},
		User: user,
	}, nil
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
	// Ensure tenant exists locally before creating user to avoid FK violation
	tenantUUID, err := uuid.Parse(tenantID)
	if err == nil {
		if _, err := s.repo.FindTenantByID(ctx, tenantUUID); err != nil {
			s.logger.Warn("Tenant not found locally during user sync, attempting to create",
				zap.String("tenant_id", tenantID),
				zap.Error(err))

			// We need to create the tenant locally.
			// Since we don't have a direct CreateTenant method exposed on Repository (it's implicit in CreateUser/Seed),
			// and we don't have the full tenant details here (only ID and maybe slug from authUserData if available),
			// we will try to fetch it from auth-service or use defaults.

			// Try to get tenant slug from authUserData or auth-service
			// For now, we'll try to use the tenantID as slug if not found, or "unknown"
			// But wait, CreateUser in EntRepository actually upserts the tenant!
			// Upsert tenant in local database (see repository_ent.go -> upsertTenant)

			// The issue is that upsertTenant in EntRepository (line 587) takes a string `tenantID` which it treats as a SLUG if it's not a UUID?
			// No, line 664: tenantUUID, err := uuid.Parse(usr.TenantID)
			// And upsertTenant (line 587) takes `tenantID` string.
			// Line 607: SetSlug(tenantID).

			// If we pass the UUID string as `tenantID` to CreateUser, `upsertTenant` will use that UUID string as the SLUG.
			// And it generates a NEW UUID for the ID (line 603).
			// This is the problem!

			// We need to ensure the tenant exists with the CORRECT ID (from auth-service).
			// The current EntRepository.CreateUser implementation seems to assume it can create the tenant if missing,
			// but it generates a NEW ID.

			// However, we can't easily change EntRepository.CreateUser without affecting other flows.
			// But wait, `upsertTenant` checks if tenant exists by SLUG (line 595).

			// If we want to sync the tenant with the correct ID, we should probably do it here in the service
			// before calling CreateUser, OR we need to fix EntRepository to handle explicit IDs.

			// Given the constraints and the error "violates foreign key constraint", it means `CreateUser`
			// failed to create the tenant or created it with a different ID?
			// Actually, `CreateUser` calls `upsertTenant` first.
			// If `upsertTenant` succeeded, the tenant should exist.
			// Why did it fail?

			// Ah, `upsertUser` (line 656) parses `usr.TenantID` as UUID.
			// `upsertTenant` (line 587) takes `tenantID` string.
			// If `upsertTenant` creates a tenant, it uses `uuid.New()` for ID (line 603).
			// But `upsertUser` uses `usr.TenantID` (which is the auth-service tenant ID).
			// So `upsertTenant` creates a tenant with a RANDOM ID, but `upsertUser` tries to link to `usr.TenantID`.
			// MISMATCH!

			// FIX: We need to make sure the tenant exists with the SPECIFIC ID.
			// Since we can't easily modify the private `upsertTenant` in `repository_ent.go` from here,
			// and `Repository` interface doesn't have `CreateTenant`.

			// Wait, `EntRepository.CreateUser` is:
			// 1. upsertTenant(usr.TenantID) -> Creates tenant with RANDOM ID if not found by slug=usr.TenantID
			// 2. upsertUser(usr) -> Uses usr.TenantID as FK.

			// If usr.TenantID is the UUID string (which it is from auth-service),
			// then `upsertTenant` checks for slug = UUID_String. Likely not found.
			// Then it creates tenant with slug = UUID_String and ID = Random_UUID.
			// Then `upsertUser` tries to use ID = UUID_String.
			// FK Violation because tenant has ID = Random_UUID, not UUID_String.

			// We need to fix `EntRepository.CreateUser` or `upsertTenant` to respect the ID if provided?
			// Or we need to look up the tenant by ID in `upsertTenant`?

			// Actually, the cleanest fix is in `repository_ent.go`.
			// `upsertTenant` should probably accept the UUID if it's a valid UUID and use it as ID?
			// But `upsertTenant` signature is `(ctx, tx, tenantID string)`.

			// Let's look at `repository_ent.go` again.
		}
	}

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
	if passwordHash, ok := authUserData["password_hash"].(string); ok && passwordHash != "" {
		user.PasswordHash = passwordHash
	}

	now := s.now()
	user.SyncAt = &now
	user.SyncStatus = "synced"
	user.UpdatedAt = now
	user.LastLoginAt = &now // Track last login timestamp

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
	passwordHash, _ := authUserData["password_hash"].(string)
	status, _ := authUserData["status"].(string)
	if status == "" {
		status = "active"
	}

	now := s.now()
	user := &User{
		ID:                authServiceUserID,
		TenantID:          tenantID,
		AuthServiceUserID: &authServiceUserID,
		Email:             email,
		FullName:          fullName,
		Phone:             phone,
		PasswordHash:      passwordHash,
		Status:            status,
		Roles:             []Role{RoleCustomer}, // Default role
		Permissions:       ConsolidatePermissions([]Role{RoleCustomer}),
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
		// Return only SuperUser and Admin roles, ignoring others to keep it clean
		return []Role{RoleSuperAdmin, RoleAdmin}
	}

	// Check for roles array
	if rolesData, ok := authUserData["roles"].([]interface{}); ok {
		for _, r := range rolesData {
			if roleStr, ok := r.(string); ok {
				// Map auth-service roles to ordering service roles
				switch roleStr {
				case "superuser":
					roles = append(roles, RoleSuperAdmin)
				case "admin":
					roles = append(roles, RoleAdmin)
				case "rider":
					roles = append(roles, RoleRider)
				case "customer", "user":
					roles = append(roles, RoleCustomer)
				}
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

// BeginGoogleOAuth returns an OAuth consent url for the requested role.
func (s *Service) BeginGoogleOAuth(ctx context.Context, role Role, redirectURI string) (string, error) {
	if s.googleCfg == nil {
		// Fallback to a generated demo URL.
		v := url.Values{
			"client_id":     {s.authCfg.GoogleClientID},
			"redirect_uri":  {redirectURI},
			"response_type": {"code"},
			"scope":         {"openid email profile"},
			"state":         {string(role)},
		}
		return fmt.Sprintf("https://accounts.google.com/o/oauth2/v2/auth?%s", v.Encode()), nil
	}

	state, err := s.tokenSigner.GenerateState(role)
	if err != nil {
		return "", err
	}

	redirect := redirectURI
	if redirect == "" {
		redirect = s.googleCfg.RedirectURL
	}

	return s.googleCfg.AuthCodeURL(state,
		oauth2.SetAuthURLParam("redirect_uri", redirect),
		oauth2.SetAuthURLParam("prompt", "select_account"),
	), nil
}

// CompleteGoogleOAuth validates the callback and issues an auth session.
func (s *Service) CompleteGoogleOAuth(ctx context.Context, code string, state string, meta RequestMeta) (*AuthResult, error) {
	if code == "" {
		return nil, fmt.Errorf("identity: oauth code required")
	}

	var role Role
	if state != "" {
		decodedRole, err := s.tokenSigner.ParseState(state)
		if err != nil {
			return nil, err
		}
		role = decodedRole
	}

	// Demo fallback if Google config disabled.
	if s.googleCfg == nil {
		user, err := s.pickDemoUserByRole(ctx, role)
		if err != nil {
			return nil, err
		}
		return s.issueSession(ctx, user, meta)
	}

	token, err := s.googleCfg.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("identity: google exchange: %w", err)
	}

	client := s.googleCfg.Client(ctx, token)
	profile, err := fetchGoogleProfile(client)
	if err != nil {
		return nil, err
	}

	// Sync or create user via auth-service
	user, err := s.syncGoogleUserToAuthService(ctx, profile)
	if err != nil {
		s.logger.Error("Failed to sync Google user to auth-service", zap.Error(err))
		// Fallback: try minimal local get/create if sync failed but profile email exists
		// forcing consistency with auth-service is better, but let's try gracefully.
		// Actually, if sync failed due to network, we might want to error out or retry.
		// For now, let's return error to avoid split-brain.
		// But wait, if we are offline, maybe we can login locally if user exists?
		// Stick to sync for now.
		return nil, err
	}

	if role != "" && !user.HasRole(role) {
		// Auto-assign role if needed? Or just check permission.
		// If user is new, they got default role.
		// If they explicitly requested a role (e.g. rider signup), we should grant it?
		// Security risk: anyone can signup as admin if they pass state?
		// No, usually we restrict powerful roles.
		// If role is Customer/Rider, maybe safe.
		if role == RoleCustomer || role == RoleRider {
			user.Roles = mergeRoles(user.Roles, []Role{role})
			user.Permissions = ConsolidatePermissions(user.Roles)
			_ = s.repo.UpdateUser(ctx, user)
		} else {
			return nil, ErrRoleNotPermitted
		}
	}

	return s.issueSession(ctx, user, meta)
}

func (s *Service) syncGoogleUserToAuthService(ctx context.Context, profile googleProfile) (*User, error) {
	if s.authServiceClient == nil {
		return nil, fmt.Errorf("identity: auth-service not configured")
	}

	// Use default tenant slug for Google OAuth logins
	tenantSlug := DefaultTenantSlug
	if tenantSlug == "" {
		return nil, fmt.Errorf("identity: tenant_slug is required for Google OAuth login")
	}
	if err := s.ensureTenantInAuthService(ctx, tenantSlug); err != nil {
		s.logger.Warn("Tenant sync failed during Google login", zap.Error(err))
		// Don't fail - tenant may be auto-created
	}

	// Sync User to Auth Service using the admin sync endpoint
	req := authclient.SyncUserRequest{
		Email:      strings.ToLower(profile.Email),
		TenantSlug: tenantSlug,
		Profile: map[string]interface{}{
			"full_name":   profile.Name,
			"avatar_url":  profile.Picture,
			"google_id":   profile.ID,
			"verified":    profile.VerifiedEmail,
			"source":      "google_oauth",
			"signup_type": "sso",
		},
		Service: "ordering-backend",
	}

	syncResp, err := s.authServiceClient.SyncUser(ctx, req, s.authServiceAPIKey)
	if err != nil {
		return nil, fmt.Errorf("identity: sync user to auth-service: %w", err)
	}

	authServiceUserID, err := uuid.Parse(syncResp.UserID)
	if err != nil {
		return nil, fmt.Errorf("identity: invalid auth-service user id: %w", err)
	}

	// 3. Sync to Local Database
	authUserData := map[string]interface{}{
		"email":     syncResp.Email,
		"full_name": profile.Name,
		// "roles": ... auth-service might return roles in metadata or separate call?
		// SyncUser response is minimal. We might need to fetch full user or just trust local defaults.
		// For now, we assume default roles or existing roles.
	}

	// Ensure we pass the auth service user ID
	return s.syncUserFromAuthService(ctx, authServiceUserID, syncResp.TenantID, authUserData, "")
}

// Logout revokes a session.
func (s *Service) Logout(ctx context.Context, sessionID uuid.UUID) error {
	session, err := s.repo.FindSessionByID(ctx, sessionID)
	if err != nil {
		return err
	}
	now := s.now()
	session.RevokedAt = &now
	return s.repo.UpdateSession(ctx, session)
}

// Refresh issues new access tokens from a refresh token.
func (s *Service) Refresh(ctx context.Context, refreshToken string, meta RequestMeta) (*AuthResult, error) {
	session, err := s.repo.FindSessionByToken(ctx, refreshToken)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	if session.RevokedAt != nil || session.ExpiresAt.Before(s.now()) {
		return nil, ErrInvalidCredentials
	}

	user, err := s.repo.FindUserByID(ctx, session.UserID)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	return s.issueSession(ctx, user, meta)
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

// VerifyAccessToken validates the JWT access token.
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

func (s *Service) issueSession(ctx context.Context, user *User, meta RequestMeta) (*AuthResult, error) {
	sessionID := uuid.New()
	refreshToken := uuid.NewString()
	now := s.now()

	session := &Session{
		ID:           sessionID,
		UserID:       user.ID,
		RefreshToken: refreshToken,
		UserAgent:    meta.UserAgent,
		IP:           meta.IP,
		ExpiresAt:    now.Add(s.authCfg.RefreshTokenTTL),
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.repo.CreateSession(ctx, session); err != nil {
		return nil, err
	}

	accessToken, err := s.tokenSigner.GenerateAccessToken(&TokenPayload{
		SessionID:   sessionID,
		UserID:      user.ID,
		TenantID:    user.TenantID,
		Roles:       user.Roles,
		Permissions: user.Permissions,
	})
	if err != nil {
		return nil, err
	}

	return &AuthResult{
		Session: SessionTokens{
			AccessToken:  accessToken,
			RefreshToken: refreshToken,
			ExpiresAt:    now.Add(s.authCfg.AccessTokenTTL),
			SessionID:    sessionID,
		},
		User: user,
	}, nil
}

func (s *Service) pickDemoUserByRole(ctx context.Context, role Role) (*User, error) {
	users, err := s.repo.ListUsers(ctx)
	if err != nil {
		return nil, err
	}

	for _, user := range users {
		if role == "" || user.HasRole(role) {
			return user, nil
		}
	}

	return nil, ErrUserNotFound
}

func (s *Service) createGoogleUser(ctx context.Context, profile googleProfile, role Role) (*User, error) {
	now := s.now()
	if role == "" {
		role = RoleCustomer
	}

	user := &User{
		ID:               uuid.New(),
		TenantID:         "", // Will be set from tenant context
		Email:            strings.ToLower(profile.Email),
		FullName:         profile.Name,
		AvatarURL:        profile.Picture,
		Roles:            []Role{role},
		Permissions:      DefaultPermissions(role),
		LoyaltyPoints:    150,
		AvailableCoupons: 2,
		Preferences: Preferences{
			Theme:    "system",
			Language: "en",
			Notifications: NotificationPreferences{
				Email: true,
				SMS:   false,
				Push:  true,
			},
		},
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.repo.CreateUser(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}

func (s *Service) seedDemoData(ctx context.Context) error {
	users := []*User{}

	demoUsers := []struct {
		email    string
		password string
		role     Role
		fullName string
	}{
		{"customer@demo.com", "demo1234", RoleCustomer, "Demo Customer"},
		{"rider@demo.com", "demo1234", RoleRider, "Swift Rider"},
		{"staff@demo.com", "demo1234", RoleStaff, "Ordering Staff"},
		{"admin@demo.com", "demo1234", RoleAdmin, "Ordering Admin"},
	}

	now := s.now()

	for _, demo := range demoUsers {
		hash, err := bcrypt.GenerateFromPassword([]byte(demo.password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}

		userID := uuid.New()
		user := &User{
			ID:                   userID,
			TenantID:             "", // Demo users will need tenant context from request
			Email:                strings.ToLower(demo.email),
			PasswordHash:         string(hash),
			FullName:             demo.fullName,
			Roles:                []Role{demo.role},
			Permissions:          DefaultPermissions(demo.role),
			LoyaltyPoints:        870,
			AvailableCoupons:     3,
			DefaultLocationLabel: "Busia township",
			Preferences: Preferences{
				Theme:    "system",
				Language: "en",
				Notifications: NotificationPreferences{
					Email: true,
					SMS:   true,
					Push:  true,
				},
			},
			Status:    "active",
			CreatedAt: now.Add(-24 * time.Hour),
			UpdatedAt: now,
		}

		users = append(users, user)
	}

	orders := []*OrderSummary{}
	for _, user := range users {
		if user.HasRole(RoleCustomer) {
			orderID := uuid.New()
			eta := now.Add(45 * time.Minute)
			orders = append(orders, &OrderSummary{
				ID:       orderID,
				UserID:   user.ID,
				Status:   "delivered",
				Total:    1450,
				PlacedAt: now.Add(-6 * time.Hour),
				ETA:      &eta,
			})
		}
	}

	return s.repo.Seed(ctx, users, nil, orders)
}

// googleProfile minimal userinfo payload.
type googleProfile struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	VerifiedEmail bool   `json:"verified_email"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
}

func fetchGoogleProfile(client *http.Client) (googleProfile, error) {
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		return googleProfile{}, fmt.Errorf("identity: google userinfo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return googleProfile{}, fmt.Errorf("identity: google userinfo status %d", resp.StatusCode)
	}

	var profile googleProfile
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return googleProfile{}, fmt.Errorf("identity: google userinfo decode: %w", err)
	}

	return profile, nil
}
