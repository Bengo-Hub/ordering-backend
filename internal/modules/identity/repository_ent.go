package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/bengobox/food-delivery-backend/internal/ent"
	"github.com/bengobox/food-delivery-backend/internal/ent/session"
	"github.com/bengobox/food-delivery-backend/internal/ent/twofactorsetting"
	"github.com/bengobox/food-delivery-backend/internal/ent/user"
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
	"logistics-service",
	"inventory-service",
	"pos-service",
	"notifications-app",
	"treasury-app",
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

// CreateSession persists a new refresh session.
func (r *EntRepository) CreateSession(ctx context.Context, s *Session) error {
	if s == nil {
		return errors.New("identity: nil session")
	}

	tenantID, userTenantErr := r.lookupTenantForUser(ctx, s.UserID)
	if userTenantErr != nil {
		return userTenantErr
	}

	builder := r.client.Session.
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
		SetNillableRevokedAt(s.RevokedAt)

	if err := builder.
		OnConflict().
		UpdateNewValues().
		Exec(ctx); err != nil {
		return fmt.Errorf("identity: create session: %w", err)
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

	return r.client.UserPreference.
		Create().
		SetUserID(userID).
		SetTheme(theme).
		SetLanguage(lang).
		SetNotifyEmail(prefs.Notifications.Email).
		SetNotifySms(prefs.Notifications.SMS).
		SetNotifyPush(prefs.Notifications.Push).
		SetTimezone(tz).
		OnConflict().
		UpdateNewValues().
		Exec(ctx)
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

	return r.client.TwoFactorSetting.
		Create().
		SetUserID(userID).
		SetEnabled(usr.TwoFactorEnabled).
		SetMethod("totp").
		SetSecret(usr.TwoFactorSecret).
		OnConflict().
		UpdateNewValues().
		Exec(ctx)
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

	return &User{
		ID:                   u.ID,
		TenantID:             u.TenantID.String(),
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
	if tenantID == "" {
		return errors.New("identity: tenant id required")
	}
	id, err := uuid.Parse(tenantID)
	if err != nil {
		return fmt.Errorf("identity: invalid tenant id %q: %w", tenantID, err)
	}
	if err := tx.Tenant.
		Create().
		SetID(id).
		SetSlug(tenantID).
		SetName("Tenant " + tenantID).
		OnConflict().
		DoNothing().
		Exec(ctx); err != nil {
		return err
	}
	return enqueueTenantSyncEvents(ctx, tx, id, tenantID)
}

func upsertRoles(ctx context.Context, tx *ent.Tx, roles []Role) error {
	for _, role := range roles {
		if err := tx.Role.
			Create().
			SetID(string(role)).
			SetName(string(role)).
			SetDescription("").
			OnConflict().
			UpdateNewValues().
			Exec(ctx); err != nil {
			return fmt.Errorf("identity: upsert role %s: %w", role, err)
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

	roleIDs := make([]string, 0, len(usr.Roles))
	for _, role := range usr.Roles {
		roleIDs = append(roleIDs, string(role))
	}
	builder.AddRoleIDs(roleIDs...)

	if err := builder.OnConflict().
		UpdateNewValues().
		Exec(ctx); err != nil {
		return fmt.Errorf("identity: upsert user: %w", err)
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
		if err := client.UserProfile.
			Create().
			SetUserID(usr.ID).
			SetAvatarURL(usr.AvatarURL).
			OnConflict().
			UpdateNewValues().
			Exec(ctx); err != nil {
			return fmt.Errorf("identity: upsert profile: %w", err)
		}
	}

	for _, code := range usr.BackupCodes {
		if err := client.BackupCode.
			Create().
			SetUserID(usr.ID).
			SetCodeHash(code).
			OnConflict().
			DoNothing().
			Exec(ctx); err != nil {
			return fmt.Errorf("identity: upsert backup code: %w", err)
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
		if err := tx.TenantSyncEvent.
			Create().
			SetTenantID(tenantID).
			SetTenantSlug(tenantSlug).
			SetDestinationService(destination).
			SetPayload(payload).
			OnConflictColumns("tenant_id", "destination_service").
			UpdateNewValues().
			Exec(ctx); err != nil {
			return fmt.Errorf("identity: enqueue tenant sync event for %s: %w", destination, err)
		}
	}
	return nil
}
