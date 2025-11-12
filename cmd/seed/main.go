package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/bengobox/food-delivery-backend/internal/config"
	"github.com/bengobox/food-delivery-backend/internal/ent"
	"github.com/bengobox/food-delivery-backend/internal/ent/tenant"
	"github.com/bengobox/food-delivery-backend/internal/ent/tenantsetting"
	"github.com/bengobox/food-delivery-backend/internal/ent/user"
	"github.com/bengobox/food-delivery-backend/internal/ent/userpreference"
	"github.com/bengobox/food-delivery-backend/internal/ent/userprofile"
)

var tenantSyncDestinations = []string{
	"logistics-service",
	"inventory-service",
	"pos-service",
	"notifications-app",
	"treasury-app",
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	db, err := sql.Open("pgx", cfg.Postgres.URL)
	if err != nil {
		log.Fatalf("open ent driver: %v", err)
	}
	db.SetMaxIdleConns(10)
	db.SetMaxOpenConns(25)
	db.SetConnMaxIdleTime(5 * time.Minute)
	driver := entsql.OpenDB(dialect.Postgres, db)

	client := ent.NewClient(ent.Driver(driver))
	defer client.Close()

	if err := runSeed(ctx, client); err != nil {
		log.Fatalf("seed data: %v", err)
	}

	log.Println("database seed completed successfully")
}

func runSeed(ctx context.Context, client *ent.Client) (err error) {
	const (
		superAdminID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
		tenantID     = "11111111-2222-3333-4444-555555555555"
	)

	tenantUUID := uuid.MustParse(tenantID)
	superAdminUUID := uuid.MustParse(superAdminID)

	tx, err := client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
			return
		}
		err = tx.Commit()
	}()

	now := time.Now().UTC()

	if err = upsertTenant(ctx, tx, tenantUUID); err != nil {
		return err
	}

	if err = upsertTenantSettings(ctx, tx, tenantUUID); err != nil {
		return err
	}

	permMap, err := seedPermissions(ctx, tx)
	if err != nil {
		return err
	}

	if err = seedRoles(ctx, tx); err != nil {
		return err
	}

	if err = seedRolePermissions(ctx, tx, permMap); err != nil {
		return err
	}

	hashedPassword := "$2b$12$Yw8cGZRD4imkA6zQwO4k4O9SFeNkn8MS9KqSXB25tOfvtTDHkJNNy" // bcrypt("ChangeMe123!")
	if err = seedSuperAdmin(ctx, tx, tenantUUID, superAdminUUID, hashedPassword, now); err != nil {
		return err
	}

	return nil
}

func seedPermissions(ctx context.Context, tx *ent.Tx) (map[string]uuid.UUID, error) {
	type perm struct {
		code string
		name string
		mod  string
		desc string
	}
	permissions := []perm{
		{"profile:update", "Update profile", "identity", "Update personal profile information"},
		{"preferences:update", "Update preferences", "identity", "Modify notification and UI preferences"},
		{"loyalty:view", "View loyalty balance", "loyalty", "View loyalty points and history"},
		{"loyalty:redeem", "Redeem loyalty points", "loyalty", "Redeem loyalty points"},
		{"orders:view", "View orders", "orders", "View personal or tenant orders"},
		{"orders:manage", "Manage orders", "orders", "Create/update/cancel orders"},
		{"orders:refund", "Refund orders", "orders", "Issue customer refunds"},
		{"catalog:view", "View catalog", "catalog", "View menu catalog data"},
		{"catalog:manage", "Manage catalog", "catalog", "Create or edit menu items and categories"},
		{"payments:view", "View payments", "payments", "View payment and settlement data"},
		{"payments:manage", "Manage payments", "payments", "Capture/refund payments"},
		{"logistics:view", "View logistics data", "logistics", "View rider assignments and tracking"},
		{"logistics:dispatch", "Dispatch orders", "logistics", "Assign riders and manage delivery flow"},
		{"operations:kitchen", "Manage kitchen tickets", "operations", "Process kitchen tickets"},
		{"operations:inventory", "Manage inventory", "operations", "Adjust stock and inventory"},
		{"notifications:view", "View notifications", "notifications", "Review notification queue/state"},
		{"notifications:manage", "Manage notifications", "notifications", "Modify templates and retry notifications"},
		{"analytics:view", "View analytics", "analytics", "Access dashboards and insights"},
		{"analytics:export", "Export analytics", "analytics", "Run/export analytic jobs"},
		{"support:view", "View support tickets", "support", "View support tickets"},
		{"support:manage", "Manage support tickets", "support", "Respond and close support cases"},
		{"admin:manage", "Manage tenant settings", "identity", "Manage tenant configuration and users"},
	}

	permIDs := make(map[string]uuid.UUID, len(permissions))
	for _, p := range permissions {
		id := permissionUUID(p.code)
		permIDs[p.code] = id

		_, err := tx.Permission.Get(ctx, id)
		switch {
		case ent.IsNotFound(err):
			if _, createErr := tx.Permission.Create().
				SetID(id).
				SetName(p.name).
				SetModule(p.mod).
				SetDescription(p.desc).
				Save(ctx); createErr != nil {
				return nil, fmt.Errorf("seed permission %s: %w", p.code, createErr)
			}
		case err != nil:
			return nil, fmt.Errorf("lookup permission %s: %w", p.code, err)
		default:
			if _, updateErr := tx.Permission.UpdateOneID(id).
				SetName(p.name).
				SetModule(p.mod).
				SetDescription(p.desc).
				Save(ctx); updateErr != nil {
				return nil, fmt.Errorf("update permission %s: %w", p.code, updateErr)
			}
		}
	}
	return permIDs, nil
}

func seedRoles(ctx context.Context, tx *ent.Tx) error {
	type role struct {
		code  string
		name  string
		desc  string
		scope string
	}
	roles := []role{
		{"customer", "Customer", "End-customer placing orders", "tenant"},
		{"rider", "Rider", "Delivery rider", "tenant"},
		{"staff", "Staff", "Cafe staff/admin user", "tenant"},
		{"admin", "Admin", "Tenant administrator", "tenant"},
		{"superadmin", "Super Admin", "Platform super administrator", "global"},
	}

	for _, r := range roles {
		_, err := tx.Role.Get(ctx, r.code)
		switch {
		case ent.IsNotFound(err):
			if _, createErr := tx.Role.Create().
				SetID(r.code).
				SetName(r.name).
				SetDescription(r.desc).
				SetScope(r.scope).
				SetSystemRole(true).
				Save(ctx); createErr != nil {
				return fmt.Errorf("seed role %s: %w", r.code, createErr)
			}
		case err != nil:
			return fmt.Errorf("lookup role %s: %w", r.code, err)
		default:
			if _, updateErr := tx.Role.UpdateOneID(r.code).
				SetName(r.name).
				SetDescription(r.desc).
				SetScope(r.scope).
				SetSystemRole(true).
				Save(ctx); updateErr != nil {
				return fmt.Errorf("update role %s: %w", r.code, updateErr)
			}
		}
	}
	return nil
}

func seedRolePermissions(ctx context.Context, tx *ent.Tx, permMap map[string]uuid.UUID) error {
	rolePerms := map[string][]string{
		"customer": {
			"orders:view",
			"profile:update",
			"preferences:update",
			"loyalty:view",
			"loyalty:redeem",
		},
		"rider": {
			"orders:view",
			"profile:update",
			"preferences:update",
			"logistics:view",
			"logistics:dispatch",
		},
		"staff": {
			"orders:view",
			"orders:manage",
			"profile:update",
			"preferences:update",
			"catalog:view",
			"catalog:manage",
			"operations:kitchen",
			"operations:inventory",
			"notifications:view",
			"support:view",
		},
		"admin": {
			"orders:view",
			"orders:manage",
			"orders:refund",
			"catalog:view",
			"catalog:manage",
			"payments:view",
			"payments:manage",
			"logistics:view",
			"logistics:dispatch",
			"operations:kitchen",
			"operations:inventory",
			"notifications:view",
			"notifications:manage",
			"analytics:view",
			"analytics:export",
			"support:view",
			"support:manage",
			"admin:manage",
		},
		"superadmin": {
			"orders:view",
			"orders:manage",
			"orders:refund",
			"catalog:view",
			"catalog:manage",
			"payments:view",
			"payments:manage",
			"logistics:view",
			"logistics:dispatch",
			"operations:kitchen",
			"operations:inventory",
			"notifications:view",
			"notifications:manage",
			"analytics:view",
			"analytics:export",
			"support:view",
			"support:manage",
			"admin:manage",
			"profile:update",
			"preferences:update",
			"loyalty:view",
			"loyalty:redeem",
		},
	}

	for code, perms := range rolePerms {
		var permIDs []uuid.UUID
		if len(perms) > 0 {
			permIDs = make([]uuid.UUID, 0, len(perms))
		}
		for _, permCode := range perms {
			id, ok := permMap[permCode]
			if !ok {
				return fmt.Errorf("permission %s not seeded", permCode)
			}
			permIDs = append(permIDs, id)
		}

		update := tx.Role.UpdateOneID(code).ClearPermissions()
		if len(permIDs) > 0 {
			update = update.AddPermissionIDs(permIDs...)
		}
		if err := update.Exec(ctx); err != nil {
			return fmt.Errorf("assign permissions to role %s: %w", code, err)
		}
	}
	return nil
}

func upsertTenant(ctx context.Context, tx *ent.Tx, tenantID uuid.UUID) error {
	const (
		slug   = "urban-cafe"
		name   = "Urban Café"
		status = "active"
		email  = "support@urbancafe.com"
		phone  = "+254700000000"
	)

	_, err := tx.Tenant.Get(ctx, tenantID)
	switch {
	case ent.IsNotFound(err):
		_, createErr := tx.Tenant.Create().
			SetID(tenantID).
			SetSlug(slug).
			SetName(name).
			SetStatus(status).
			SetContactEmail(email).
			SetContactPhone(phone).
			Save(ctx)
		if createErr != nil {
			return fmt.Errorf("create tenant %s: %w", tenantID, createErr)
		}
	case err != nil:
		return fmt.Errorf("lookup tenant %s: %w", tenantID, err)
	default:
		_, updateErr := tx.Tenant.UpdateOneID(tenantID).
			SetSlug(slug).
			SetName(name).
			SetStatus(status).
			SetContactEmail(email).
			SetContactPhone(phone).
			Save(ctx)
		if updateErr != nil {
			return fmt.Errorf("update tenant %s: %w", tenantID, updateErr)
		}
	}
	return enqueueTenantSyncEvents(ctx, tx, tenantID, slug)
}

func upsertTenantSettings(ctx context.Context, tx *ent.Tx, tenantID uuid.UUID) error {
	brandPalette := map[string]any{
		"primary":   "#f97316",
		"secondary": "#1f2937",
	}
	locales := []string{"en", "sw"}
	features := map[string]any{
		"loyalty":      true,
		"multiOutlets": true,
		"dispatch":     true,
	}

	setting, err := tx.TenantSetting.Query().
		Where(tenantsetting.HasTenantWith(tenant.IDEQ(tenantID))).
		Only(ctx)
	switch {
	case ent.IsNotFound(err):
		_, createErr := tx.TenantSetting.Create().
			SetTenantID(tenantID).
			SetBrandPalette(brandPalette).
			SetLocales(locales).
			SetFeatures(features).
			Save(ctx)
		if createErr != nil {
			return fmt.Errorf("create tenant setting for %s: %w", tenantID, createErr)
		}
	case err != nil:
		return fmt.Errorf("lookup tenant setting for %s: %w", tenantID, err)
	default:
		_, updateErr := tx.TenantSetting.UpdateOneID(setting.ID).
			SetBrandPalette(brandPalette).
			SetLocales(locales).
			SetFeatures(features).
			Save(ctx)
		if updateErr != nil {
			return fmt.Errorf("update tenant setting for %s: %w", tenantID, updateErr)
		}
	}
	return nil
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
			return fmt.Errorf("enqueue tenant sync event for %s: %w", destination, err)
		}
	}
	return nil
}

func seedSuperAdmin(ctx context.Context, tx *ent.Tx, tenantID, userID uuid.UUID, passwordHash string, now time.Time) error {
	const (
		email       = "superadmin@urbancafe.com"
		fullName    = "Super Admin"
		status      = "active"
		locale      = "en"
		timezone    = "Africa/Nairobi"
		primaryRole = "superadmin"
	)
	roleIDs := []string{"superadmin", "admin"}
	metadata := map[string]any{
		"timezone": timezone,
	}

	_, err := tx.User.Get(ctx, userID)
	switch {
	case ent.IsNotFound(err):
		_, createErr := tx.User.Create().
			SetID(userID).
			SetTenantID(tenantID).
			SetEmail(email).
			SetPasswordHash(passwordHash).
			SetFullName(fullName).
			SetStatus(status).
			SetLocale(locale).
			SetTimezone(timezone).
			SetMetadata(metadata).
			SetPrimaryRole(primaryRole).
			SetCreatedAt(now).
			SetUpdatedAt(now).
			AddRoleIDs(roleIDs...).
			Save(ctx)
		if createErr != nil {
			return fmt.Errorf("create super admin: %w", createErr)
		}
	case err != nil:
		return fmt.Errorf("lookup super admin: %w", err)
	default:
		_, updateErr := tx.User.UpdateOneID(userID).
			SetTenantID(tenantID).
			SetEmail(email).
			SetPasswordHash(passwordHash).
			SetFullName(fullName).
			SetStatus(status).
			SetLocale(locale).
			SetTimezone(timezone).
			SetMetadata(metadata).
			SetPrimaryRole(primaryRole).
			SetUpdatedAt(now).
			ClearRoles().
			AddRoleIDs(roleIDs...).
			Save(ctx)
		if updateErr != nil {
			return fmt.Errorf("update super admin: %w", updateErr)
		}
	}

	if err := upsertUserPreference(ctx, tx, userID, timezone); err != nil {
		return err
	}
	if err := upsertUserProfile(ctx, tx, userID); err != nil {
		return err
	}
	return nil
}

func upsertUserPreference(ctx context.Context, tx *ent.Tx, userID uuid.UUID, timezone string) error {
	const (
		theme    = "system"
		language = "en"
	)

	pref, err := tx.UserPreference.Query().
		Where(userpreference.HasUserWith(user.IDEQ(userID))).
		Only(ctx)
	switch {
	case ent.IsNotFound(err):
		_, createErr := tx.UserPreference.Create().
			SetUserID(userID).
			SetTheme(theme).
			SetLanguage(language).
			SetNotifyEmail(true).
			SetNotifySms(true).
			SetNotifyPush(true).
			SetTimezone(timezone).
			Save(ctx)
		if createErr != nil {
			return fmt.Errorf("create user preference: %w", createErr)
		}
	case err != nil:
		return fmt.Errorf("lookup user preference: %w", err)
	default:
		_, updateErr := tx.UserPreference.UpdateOneID(pref.ID).
			SetTheme(theme).
			SetLanguage(language).
			SetNotifyEmail(true).
			SetNotifySms(true).
			SetNotifyPush(true).
			SetTimezone(timezone).
			Save(ctx)
		if updateErr != nil {
			return fmt.Errorf("update user preference: %w", updateErr)
		}
	}
	return nil
}

func upsertUserProfile(ctx context.Context, tx *ent.Tx, userID uuid.UUID) error {
	profile, err := tx.UserProfile.Query().
		Where(userprofile.HasUserWith(user.IDEQ(userID))).
		Only(ctx)
	switch {
	case ent.IsNotFound(err):
		_, createErr := tx.UserProfile.Create().
			SetUserID(userID).
			SetAvatarURL("").
			Save(ctx)
		if createErr != nil {
			return fmt.Errorf("create user profile: %w", createErr)
		}
	case err != nil:
		return fmt.Errorf("lookup user profile: %w", err)
	default:
		_, updateErr := tx.UserProfile.UpdateOneID(profile.ID).
			SetAvatarURL("").
			Save(ctx)
		if updateErr != nil {
			return fmt.Errorf("update user profile: %w", updateErr)
		}
	}
	return nil
}

func permissionUUID(code string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("bengobox:food-delivery:permission:"+code))
}
