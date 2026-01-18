package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/bengobox/ordering-backend/internal/config"
	"github.com/bengobox/ordering-backend/internal/ent"
	"github.com/bengobox/ordering-backend/internal/ent/session"
	"github.com/bengobox/ordering-backend/internal/ent/tenant"
	"github.com/bengobox/ordering-backend/internal/ent/tenantsyncevent"
	"github.com/bengobox/ordering-backend/internal/ent/twofactorsetting"
	"github.com/bengobox/ordering-backend/internal/ent/user"
	"github.com/bengobox/ordering-backend/internal/ent/userpreference"
	"github.com/bengobox/ordering-backend/internal/ent/userprofile"
)

// EntRepository implements the Repository interface using Ent as the persistence layer.
type EntRepository struct {
	client *ent.Client
}

const (
	defaultLocale   = "en"
	defaultTimezone = "Africa/Nairobi"
)

var tenantSyncDestinations = []string{
	"logistics-api",
	"inventory-api",
	"pos-api",
	"notifications-api",
	"treasury-api",
}

// NewEntRepository constructs an Ent-backed repository.
func NewEntRepository(client *ent.Client) *EntRepository {
	return &EntRepository{client: client}
}

// CreateUser persists a new user record.
func (r *EntRepository) CreateUser(ctx context.Context, usr *User) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("identity: begin tx: %w", err)
	}

	if err := upsertTenant(ctx, tx, usr.TenantID); err != nil {
		_ = tx.Rollback()
		return err
	}

	if err := upsertRoles(ctx, tx, usr.Roles); err != nil {
		_ = tx.Rollback()
		return err
	}

	if err := upsertUser(ctx, tx.Client(), usr); err != nil {
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
}

// UpdateUser mutates persisted user information.
func (r *EntRepository) UpdateUser(ctx context.Context, usr *User) error {
	if usr == nil {
		return errors.New("identity: nil user update")
	}
	metadata := usr.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}

	locale := usr.Preferences.Language
	if locale == "" {
		locale = defaultLocale
	}
	tz := determineTimezone(usr)
	metadata["timezone"] = tz

	// Store auth_service_user_id and sync fields in metadata temporarily
	// TODO: After Ent regeneration, use SetAuthServiceUserID, SetSyncStatus, SetSyncAt
	if usr.AuthServiceUserID != nil {
		metadata["auth_service_user_id"] = usr.AuthServiceUserID.String()
	}
	if usr.SyncStatus != "" {
		metadata["sync_status"] = usr.SyncStatus
	}
	if usr.SyncAt != nil {
		metadata["sync_at"] = usr.SyncAt.Format(time.RFC3339)
	}

	_, err := r.client.User.UpdateOneID(usr.ID).
		SetFullName(usr.FullName).
		SetStatus(usr.Status).
		SetMetadata(metadata).
		SetNillablePasswordHash(optionalString(usr.PasswordHash)).
		SetNillablePhone(optionalString(usr.Phone)).
		SetLocale(locale).
		SetTimezone(tz).
		SetNillableLastLoginAt(usr.LastLoginAt).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("identity: update user: %w", err)
	}

	if err := r.syncUserPreferences(ctx, usr.ID, usr.Preferences, tz); err != nil {
		return err
	}
	if err := r.syncUserRoles(ctx, usr.ID, usr.Roles); err != nil {
		return err
	}
	if err := r.syncTwoFactor(ctx, usr.ID, usr); err != nil {
		return err
	}
	return nil
}

// FindUserByEmail fetches a user by email (case-insensitive).
func (r *EntRepository) FindUserByEmail(ctx context.Context, email string) (*User, error) {
	u, err := r.client.User.
		Query().
		Where(user.EmailEqualFold(email)).
		WithTenant().
		WithRoles(func(q *ent.RoleQuery) {
			q.WithPermissions()
		}).
		WithPreferences().
		WithProfile().
		WithTwoFactorSettings().
		WithBackupCodes().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("identity: find user by email: %w", err)
	}
	return mapEntUser(u), nil
}

// FindUserByAuthServiceID fetches a user by auth-service user ID.
func (r *EntRepository) FindUserByAuthServiceID(ctx context.Context, authServiceUserID uuid.UUID) (*User, error) {
	u, err := r.client.User.
		Query().
		Where(user.AuthServiceUserIDEQ(authServiceUserID)).
		WithTenant().
		WithRoles().
		WithPreferences().
		WithProfile().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("identity: find user by auth-service ID: %w", err)
	}
	return mapEntUser(u), nil
}

// FindUserByID fetches a user by identifier.
func (r *EntRepository) FindUserByID(ctx context.Context, id uuid.UUID) (*User, error) {
	u, err := r.client.User.
		Query().
		Where(user.IDEQ(id)).
		WithTenant().
		WithRoles(func(q *ent.RoleQuery) {
			q.WithPermissions()
		}).
		WithPreferences().
		WithProfile().
		WithTwoFactorSettings().
		WithBackupCodes().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("identity: find user by id: %w", err)
	}
	return mapEntUser(u), nil
}

// ListUsers returns all users.
func (r *EntRepository) ListUsers(ctx context.Context) ([]*User, error) {
	records, err := r.client.User.
		Query().
		WithTenant().
		WithRoles(func(q *ent.RoleQuery) { q.WithPermissions() }).
		WithPreferences().
		WithProfile().
		WithTwoFactorSettings().
		WithBackupCodes().
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("identity: list users: %w", err)
	}
	out := make([]*User, 0, len(records))
	for _, u := range records {
		out = append(out, mapEntUser(u))
	}
	return out, nil
}

// FindTenantBySlug finds a tenant by its slug.
func (r *EntRepository) FindTenantBySlug(ctx context.Context, slug string) (*Tenant, error) {
	tenantEntity, err := r.client.Tenant.Query().
		Where(tenant.SlugEQ(slug)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("identity: tenant not found: %s", slug)
		}
		return nil, fmt.Errorf("identity: find tenant by slug: %w", err)
	}

	metadata := make(map[string]interface{})
	if tenantEntity.Metadata != nil {
		metadata = tenantEntity.Metadata
	}

	return &Tenant{
		ID:           tenantEntity.ID,
		Slug:         tenantEntity.Slug,
		Name:         tenantEntity.Name,
		Status:       tenantEntity.Status,
		ContactEmail: tenantEntity.ContactEmail,
		ContactPhone: tenantEntity.ContactPhone,
		Metadata:     metadata,
	}, nil
}

// FindTenantByID finds a tenant by its ID.
func (r *EntRepository) FindTenantByID(ctx context.Context, id uuid.UUID) (*Tenant, error) {
	tenantEntity, err := r.client.Tenant.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("identity: tenant not found: %s", id)
		}
		return nil, fmt.Errorf("identity: find tenant by id: %w", err)
	}

	metadata := make(map[string]interface{})
	if tenantEntity.Metadata != nil {
		metadata = tenantEntity.Metadata
	}

	return &Tenant{
		ID:           tenantEntity.ID,
		Slug:         tenantEntity.Slug,
		Name:         tenantEntity.Name,
		Status:       tenantEntity.Status,
		ContactEmail: tenantEntity.ContactEmail,
		ContactPhone: tenantEntity.ContactPhone,
		Metadata:     metadata,
	}, nil
}

// UpsertTenant creates or updates a tenant.
func (r *EntRepository) UpsertTenant(ctx context.Context, t *Tenant) error {
	if t == nil {
		return errors.New("identity: nil tenant upsert")
	}

	// Try to update existing tenant by ID
	err := r.client.Tenant.UpdateOneID(t.ID).
		SetSlug(t.Slug).
		SetName(t.Name).
		SetStatus(t.Status).
		SetContactEmail(t.ContactEmail).
		SetContactPhone(t.ContactPhone).
		SetMetadata(t.Metadata).
		Exec(ctx)

	if err != nil {
		if ent.IsNotFound(err) {
			// Create new tenant
			err = r.client.Tenant.Create().
				SetID(t.ID).
				SetSlug(t.Slug).
				SetName(t.Name).
				SetStatus(t.Status).
				SetContactEmail(t.ContactEmail).
				SetContactPhone(t.ContactPhone).
				SetMetadata(t.Metadata).
				Exec(ctx)
			if err != nil {
				return fmt.Errorf("identity: create tenant: %w", err)
			}
		} else {
			return fmt.Errorf("identity: update tenant: %w", err)
		}
	}
	return nil
}

// CreateSession persists a new refresh session.
func (r *EntRepository) CreateSession(ctx context.Context, s *Session) error {
	if s == nil {
		return errors.New("identity: nil session")
	}

	tenantID, userTenantErr := r.lookupTenantForUser(ctx, s.UserID)
	if userTenantErr != nil {
		return userTenantErr
	}

	// Upsert session: try update first, then create if not exists
	_, err := r.client.Session.UpdateOneID(s.ID).
		SetTenantID(tenantID).
		SetUserID(s.UserID).
		SetRefreshTokenHash(s.RefreshToken).
		SetNillableUserAgent(optionalString(s.UserAgent)).
		SetNillableIPAddress(optionalString(s.IP)).
		SetUpdatedAt(s.UpdatedAt).
		SetNillableRevokedAt(s.RevokedAt).
		Save(ctx)
	if err != nil {
		// If not found, create new
		if ent.IsNotFound(err) {
			if err := r.client.Session.
				Create().
				SetID(s.ID).
				SetTenantID(tenantID).
				SetUserID(s.UserID).
				SetRefreshTokenHash(s.RefreshToken).
				SetNillableUserAgent(optionalString(s.UserAgent)).
				SetNillableIPAddress(optionalString(s.IP)).
				SetExpiresAt(s.ExpiresAt).
				SetCreatedAt(s.CreatedAt).
				SetUpdatedAt(s.UpdatedAt).
				SetNillableRevokedAt(s.RevokedAt).
				Exec(ctx); err != nil {
				return fmt.Errorf("identity: create session: %w", err)
			}
		} else {
			return fmt.Errorf("identity: update session: %w", err)
		}
	}

	return nil
}

// UpdateSession persists changes to an existing session.
func (r *EntRepository) UpdateSession(ctx context.Context, s *Session) error {
	if s == nil {
		return errors.New("identity: nil session update")
	}
	if err := r.client.Session.UpdateOneID(s.ID).
		SetRefreshTokenHash(s.RefreshToken).
		SetNillableUserAgent(optionalString(s.UserAgent)).
		SetNillableIPAddress(optionalString(s.IP)).
		SetNillableRevokedAt(s.RevokedAt).
		SetUpdatedAt(s.UpdatedAt).
		Exec(ctx); err != nil {
		return fmt.Errorf("identity: update session: %w", err)
	}
	return nil
}

// FindSessionByID fetches a session by identifier.
func (r *EntRepository) FindSessionByID(ctx context.Context, id uuid.UUID) (*Session, error) {
	s, err := r.client.Session.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrSessionNotFound
		}
		return nil, fmt.Errorf("identity: find session: %w", err)
	}
	return mapEntSession(s), nil
}

// FindSessionByToken fetches a session by refresh token hash.
func (r *EntRepository) FindSessionByToken(ctx context.Context, refreshToken string) (*Session, error) {
	s, err := r.client.Session.
		Query().
		Where(session.RefreshTokenHashEQ(refreshToken)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrSessionNotFound
		}
		return nil, fmt.Errorf("identity: find session by token: %w", err)
	}
	return mapEntSession(s), nil
}

// DeleteSession removes a session by ID.
func (r *EntRepository) DeleteSession(ctx context.Context, id uuid.UUID) error {
	if err := r.client.Session.DeleteOneID(id).Exec(ctx); err != nil && !ent.IsNotFound(err) {
		return fmt.Errorf("identity: delete session: %w", err)
	}
	return nil
}

// DeleteSessionsByUser removes all sessions for a user.
func (r *EntRepository) DeleteSessionsByUser(ctx context.Context, userID uuid.UUID) error {
	if _, err := r.client.Session.Delete().Where(session.UserIDEQ(userID)).Exec(ctx); err != nil {
		return fmt.Errorf("identity: delete user sessions: %w", err)
	}
	return nil
}

// ListOrdersByUser returns aggregated order summaries (placeholder until order service integration).
func (r *EntRepository) ListOrdersByUser(context.Context, uuid.UUID) ([]*OrderSummary, error) {
	return []*OrderSummary{}, nil
}

// Seed inserts bootstrap data using the Ent client.
func (r *EntRepository) Seed(ctx context.Context, users []*User, sessions []*Session, _ []*OrderSummary) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("identity: seed tx: %w", err)
	}

	for _, usr := range users {
		if err := upsertTenant(ctx, tx, usr.TenantID); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := upsertRoles(ctx, tx, usr.Roles); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := upsertUser(ctx, tx.Client(), usr); err != nil {
			_ = tx.Rollback()
			return err
		}
	}

	for _, s := range sessions {
		if err := r.CreateSession(ctx, s); err != nil {
			_ = tx.Rollback()
			return err
		}
	}

	return tx.Commit()
}

func (r *EntRepository) syncUserRoles(ctx context.Context, userID uuid.UUID, roles []Role) error {
	roleIDs := make([]string, 0, len(roles))
	for _, role := range roles {
		roleIDs = append(roleIDs, string(role))
	}
	return r.client.User.UpdateOneID(userID).ClearRoles().AddRoleIDs(roleIDs...).Exec(ctx)
}

func (r *EntRepository) syncUserPreferences(ctx context.Context, userID uuid.UUID, prefs Preferences, timezone string) error {
	theme := prefs.Theme
	if theme == "" {
		theme = "system"
	}
	lang := prefs.Language
	if lang == "" {
		lang = defaultLocale
	}
	tz := timezone
	if tz == "" {
		tz = defaultTimezone
	}

	// Upsert user preference: try to find and update, then create if not exists
	pref, err := r.client.UserPreference.Query().
		Where(userpreference.HasUserWith(user.IDEQ(userID))).
		Only(ctx)
	if err == nil && pref != nil {
		// Update existing
		_, err = r.client.UserPreference.UpdateOneID(pref.ID).
			SetTheme(theme).
			SetLanguage(lang).
			SetNotifyEmail(prefs.Notifications.Email).
			SetNotifySms(prefs.Notifications.SMS).
			SetNotifyPush(prefs.Notifications.Push).
			SetTimezone(tz).
			Save(ctx)
		if err != nil {
			return err
		}
	} else {
		// Create new
		if err := r.client.UserPreference.
			Create().
			SetUserID(userID).
			SetTheme(theme).
			SetLanguage(lang).
			SetNotifyEmail(prefs.Notifications.Email).
			SetNotifySms(prefs.Notifications.SMS).
			SetNotifyPush(prefs.Notifications.Push).
			SetTimezone(tz).
			Exec(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (r *EntRepository) syncTwoFactor(ctx context.Context, userID uuid.UUID, usr *User) error {
	if !usr.TwoFactorEnabled && usr.TwoFactorSecret == "" {
		_, err := r.client.TwoFactorSetting.Delete().
			Where(twofactorsetting.HasUserWith(user.IDEQ(userID))).
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("identity: clear two-factor: %w", err)
		}
		return nil
	}

	// Upsert two-factor setting: try update first, then create if not exists
	_, err := r.client.TwoFactorSetting.Update().
		Where(twofactorsetting.HasUserWith(user.IDEQ(userID))).
		SetEnabled(usr.TwoFactorEnabled).
		SetMethod("totp").
		SetSecret(usr.TwoFactorSecret).
		Save(ctx)
	if err != nil {
		// If not found, create new
		if ent.IsNotFound(err) {
			if err := r.client.TwoFactorSetting.
				Create().
				SetUserID(userID).
				SetEnabled(usr.TwoFactorEnabled).
				SetMethod("totp").
				SetSecret(usr.TwoFactorSecret).
				Exec(ctx); err != nil {
				return err
			}
		} else {
			return err
		}
	}
	return nil
}

func (r *EntRepository) lookupTenantForUser(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	u, err := r.client.User.Get(ctx, userID)
	if err != nil {
		if ent.IsNotFound(err) {
			return uuid.Nil, ErrUserNotFound
		}
		return uuid.Nil, fmt.Errorf("identity: lookup tenant for user: %w", err)
	}
	return u.TenantID, nil
}

func mapEntUser(u *ent.User) *User {
	var (
		roles       []Role
		permissions = make(map[Permission]struct{})
	)
	for _, r := range u.Edges.Roles {
		roles = append(roles, Role(r.ID))
		for _, perm := range r.Edges.Permissions {
			permissions[Permission(perm.Name)] = struct{}{}
		}
	}
	var perms []Permission
	for p := range permissions {
		perms = append(perms, p)
	}

	var prefs Preferences
	if pref := u.Edges.Preferences; pref != nil {
		prefs = Preferences{
			Theme:    pref.Theme,
			Language: pref.Language,
			Notifications: NotificationPreferences{
				Email: pref.NotifyEmail,
				SMS:   pref.NotifySms,
				Push:  pref.NotifyPush,
			},
		}
	}

	var avatarURL string
	if profile := u.Edges.Profile; profile != nil {
		avatarURL = profile.AvatarURL
	}

	var backupCodes []string
	for _, bc := range u.Edges.BackupCodes {
		backupCodes = append(backupCodes, bc.CodeHash)
	}

	var twoFactorEnabled bool
	var twoFactorSecret string
	if tf := u.Edges.TwoFactorSettings; tf != nil {
		twoFactorEnabled = tf.Enabled
		twoFactorSecret = tf.Secret
	}

	// Use direct field access from regenerated Ent code
	var authServiceUserID *uuid.UUID
	if u.AuthServiceUserID != uuid.Nil {
		authServiceUserID = &u.AuthServiceUserID
	}
	syncStatus := u.SyncStatus
	var syncAt *time.Time
	if !u.SyncAt.IsZero() {
		syncAt = &u.SyncAt
	}

	return &User{
		ID:                   u.ID,
		TenantID:             u.TenantID.String(),
		AuthServiceUserID:    authServiceUserID,
		Email:                u.Email,
		PasswordHash:         u.PasswordHash,
		FullName:             u.FullName,
		Phone:                u.Phone,
		AvatarURL:            avatarURL,
		Roles:                roles,
		Permissions:          perms,
		LoyaltyPoints:        0,
		AvailableCoupons:     0,
		DefaultLocationLabel: "",
		TwoFactorEnabled:     twoFactorEnabled,
		TwoFactorSecret:      twoFactorSecret,
		BackupCodes:          backupCodes,
		Preferences:          prefs,
		SyncStatus:           syncStatus,
		SyncAt:               syncAt,
		LastLoginAt:          optionalTime(u.LastLoginAt),
		CreatedAt:            u.CreatedAt,
		UpdatedAt:            u.UpdatedAt,
		Status:               u.Status,
		Metadata:             u.Metadata,
	}
}

func mapEntSession(s *ent.Session) *Session {
	return &Session{
		ID:           s.ID,
		UserID:       s.UserID,
		RefreshToken: s.RefreshTokenHash,
		UserAgent:    s.UserAgent,
		IP:           s.IPAddress,
		ExpiresAt:    s.ExpiresAt,
		CreatedAt:    s.CreatedAt,
		UpdatedAt:    s.UpdatedAt,
		RevokedAt:    optionalTime(s.RevokedAt),
	}
}

func upsertTenant(ctx context.Context, tx *ent.Tx, tenantID string) error {
	// Use default tenant slug if not provided
	if tenantID == "" {
		tenantID = config.DefaultTenantSlug
	}

	// Try to find tenant by slug first
	tenantEntity, err := tx.Tenant.Query().
		Where(tenant.SlugEQ(tenantID)).
		Only(ctx)
	if err == nil && tenantEntity != nil {
		// Tenant exists, use its ID
		return enqueueTenantSyncEvents(ctx, tx, tenantEntity.ID, tenantID)
	}

	// Tenant doesn't exist, create it
	id := uuid.New()
	err = tx.Tenant.
		Create().
		SetID(id).
		SetSlug(tenantID).
		SetName("Ordering Platform").
		SetStatus("active").
		SetContactEmail("support@codevertexitsolutions.com").
		SetContactPhone("+254700000000").
		Exec(ctx)
	if err != nil {
		// Check if it's a unique constraint violation (tenant already exists)
		if !strings.Contains(err.Error(), "duplicate key") && !strings.Contains(err.Error(), "UNIQUE constraint") {
			return err
		}
		// Tenant already exists, that's fine - try to get it
		tenantEntity, err := tx.Tenant.Query().
			Where(tenant.SlugEQ(tenantID)).
			Only(ctx)
		if err != nil {
			return fmt.Errorf("identity: get tenant by slug: %w", err)
		}
		return enqueueTenantSyncEvents(ctx, tx, tenantEntity.ID, tenantID)
	}
	return enqueueTenantSyncEvents(ctx, tx, id, tenantID)
}

func upsertRoles(ctx context.Context, tx *ent.Tx, roles []Role) error {
	for _, role := range roles {
		// Upsert role: try update first, then create if not exists
		_, err := tx.Role.UpdateOneID(string(role)).
			SetName(string(role)).
			SetDescription("").
			Save(ctx)
		if err != nil {
			// If not found, create new
			if ent.IsNotFound(err) {
				if err := tx.Role.
					Create().
					SetID(string(role)).
					SetName(string(role)).
					SetDescription("").
					Exec(ctx); err != nil {
					return fmt.Errorf("identity: create role %s: %w", role, err)
				}
			} else {
				return fmt.Errorf("identity: upsert role %s: %w", role, err)
			}
		}
	}
	return nil
}

func upsertUser(ctx context.Context, client *ent.Client, usr *User) error {
	if usr == nil {
		return errors.New("identity: nil user upsert")
	}
	if usr.ID == uuid.Nil {
		usr.ID = uuid.New()
	}

	tenantUUID, err := uuid.Parse(usr.TenantID)
	if err != nil {
		return fmt.Errorf("identity: invalid tenant id %q: %w", usr.TenantID, err)
	}

	metadata := usr.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}

	locale := usr.Preferences.Language
	if locale == "" {
		locale = defaultLocale
	}
	tz := determineTimezone(usr)
	metadata["timezone"] = tz

	builder := client.User.
		Create().
		SetID(usr.ID).
		SetTenantID(tenantUUID).
		SetEmail(usr.Email).
		SetNillablePasswordHash(optionalString(usr.PasswordHash)).
		SetFullName(usr.FullName).
		SetNillablePhone(optionalString(usr.Phone)).
		SetStatus(usr.Status).
		SetLocale(locale).
		SetTimezone(tz).
		SetMetadata(metadata).
		SetCreatedAt(usr.CreatedAt).
		SetUpdatedAt(usr.UpdatedAt).
		SetNillableLastLoginAt(usr.LastLoginAt)

	// Set auth-service fields directly (after Ent regeneration)
	if usr.AuthServiceUserID != nil {
		builder.SetAuthServiceUserID(*usr.AuthServiceUserID)
	}
	if usr.SyncStatus != "" {
		builder.SetSyncStatus(usr.SyncStatus)
	}
	if usr.SyncAt != nil {
		builder.SetSyncAt(*usr.SyncAt)
	}

	roleIDs := make([]string, 0, len(usr.Roles))
	for _, role := range usr.Roles {
		roleIDs = append(roleIDs, string(role))
	}
	builder.AddRoleIDs(roleIDs...)

	// Upsert user: try update first, then create if not exists
	_, err = client.User.UpdateOneID(usr.ID).
		SetTenantID(tenantUUID).
		SetEmail(usr.Email).
		SetNillablePasswordHash(optionalString(usr.PasswordHash)).
		SetFullName(usr.FullName).
		SetNillablePhone(optionalString(usr.Phone)).
		SetStatus(usr.Status).
		SetLocale(locale).
		SetTimezone(tz).
		SetMetadata(metadata).
		SetUpdatedAt(usr.UpdatedAt).
		SetNillableLastLoginAt(usr.LastLoginAt).
		Save(ctx)
	if err != nil {
		// If not found, create new
		if ent.IsNotFound(err) {
			// Set auth-service fields directly
			if usr.AuthServiceUserID != nil {
				builder.SetAuthServiceUserID(*usr.AuthServiceUserID)
			}
			if usr.SyncStatus != "" {
				builder.SetSyncStatus(usr.SyncStatus)
			}
			if usr.SyncAt != nil {
				builder.SetSyncAt(*usr.SyncAt)
			}
			if err := builder.Exec(ctx); err != nil {
				return fmt.Errorf("identity: create user: %w", err)
			}
		} else {
			return fmt.Errorf("identity: update user: %w", err)
		}
	} else {
		// Update auth-service fields if user exists
		updateBuilder := client.User.UpdateOneID(usr.ID)
		if usr.AuthServiceUserID != nil {
			updateBuilder.SetAuthServiceUserID(*usr.AuthServiceUserID)
		}
		if usr.SyncStatus != "" {
			updateBuilder.SetSyncStatus(usr.SyncStatus)
		}
		if usr.SyncAt != nil {
			updateBuilder.SetSyncAt(*usr.SyncAt)
		}
		if _, err := updateBuilder.Save(ctx); err != nil {
			return fmt.Errorf("identity: update user auth fields: %w", err)
		}
	}

	repo := NewEntRepository(client)

	if err := repo.syncUserPreferences(ctx, usr.ID, usr.Preferences, tz); err != nil {
		return err
	}
	if err := repo.syncTwoFactor(ctx, usr.ID, usr); err != nil {
		return err
	}

	hasNotifications := usr.Preferences.Notifications.Email ||
		usr.Preferences.Notifications.SMS ||
		usr.Preferences.Notifications.Push
	if usr.AvatarURL != "" || hasNotifications {
		// Upsert user profile: try to find and update, then create if not exists
		profile, err := client.UserProfile.Query().
			Where(userprofile.HasUserWith(user.IDEQ(usr.ID))).
			Only(ctx)
		if err == nil && profile != nil {
			// Update existing
			_, err = client.UserProfile.UpdateOneID(profile.ID).
				SetAvatarURL(usr.AvatarURL).
				Save(ctx)
			if err != nil {
				return fmt.Errorf("identity: update profile: %w", err)
			}
		} else {
			// Create new
			if err := client.UserProfile.
				Create().
				SetUserID(usr.ID).
				SetAvatarURL(usr.AvatarURL).
				Exec(ctx); err != nil {
				return fmt.Errorf("identity: create profile: %w", err)
			}
		}
	}

	for _, code := range usr.BackupCodes {
		// Try to create backup code, ignore if already exists
		err := client.BackupCode.
			Create().
			SetUserID(usr.ID).
			SetCodeHash(code).
			Exec(ctx)
		if err != nil {
			// Check if it's a unique constraint violation (code already exists)
			if !strings.Contains(err.Error(), "duplicate key") && !strings.Contains(err.Error(), "UNIQUE constraint") {
				return fmt.Errorf("identity: create backup code: %w", err)
			}
			// Code already exists, that's fine
		}
	}

	return nil
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func optionalTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	return &value
}

func determineTimezone(usr *User) string {
	if usr == nil {
		return defaultTimezone
	}
	if usr.Metadata != nil {
		if tz, ok := usr.Metadata["timezone"]; ok {
			if str, ok := tz.(string); ok {
				if trimmed := strings.TrimSpace(str); trimmed != "" {
					return trimmed
				}
			}
		}
		if tz, ok := usr.Metadata["tz"]; ok {
			if str, ok := tz.(string); ok {
				if trimmed := strings.TrimSpace(str); trimmed != "" {
					return trimmed
				}
			}
		}
	}
	return defaultTimezone
}

func enqueueTenantSyncEvents(ctx context.Context, tx *ent.Tx, tenantID uuid.UUID, tenantSlug string) error {
	payload := map[string]any{
		"tenant_slug": tenantSlug,
	}
	for _, destination := range tenantSyncDestinations {
		// Check if event already exists to avoid aborting the transaction on duplicate key error
		exists, err := tx.TenantSyncEvent.Query().
			Where(
				tenantsyncevent.TenantIDEQ(tenantID),
				tenantsyncevent.DestinationServiceEQ(destination),
			).
			Exist(ctx)
		if err != nil {
			return fmt.Errorf("identity: check existing tenant sync event: %w", err)
		}

		if exists {
			continue // Skip if already exists
		}

		// Create
		err = tx.TenantSyncEvent.
			Create().
			SetTenantID(tenantID).
			SetTenantSlug(tenantSlug).
			SetDestinationService(destination).
			SetPayload(payload).
			Exec(ctx)
		if err != nil {
			// Even with the check, there's a tiny race condition, but inside a transaction it might be safer?
			// Actually, if we are inside a serializable transaction, we are good.
			// But if Exec fails now, it WILL abort the transaction.
			// However, since we checked first, it's unlikely to fail unless parallel tx commits first.
			return fmt.Errorf("identity: enqueue tenant sync event for %s: %w", destination, err)
		}
	}
	return nil
}
