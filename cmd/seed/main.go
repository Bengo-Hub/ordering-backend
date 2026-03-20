package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/bengobox/ordering-backend/internal/config"
	"github.com/bengobox/ordering-backend/internal/ent"
	"github.com/bengobox/ordering-backend/internal/ent/catalogcategory"
	"github.com/bengobox/ordering-backend/internal/ent/catalogitem"
	"github.com/bengobox/ordering-backend/internal/ent/outlet"
	enttenant "github.com/bengobox/ordering-backend/internal/ent/tenant"
	"github.com/bengobox/ordering-backend/internal/ent/tenantsetting"
	"github.com/bengobox/ordering-backend/internal/ent/tenantsyncevent"
	"github.com/bengobox/ordering-backend/internal/ent/user"
	"github.com/bengobox/ordering-backend/internal/ent/userpreference"
	"github.com/bengobox/ordering-backend/internal/ent/userprofile"
	"github.com/bengobox/ordering-backend/internal/modules/tenant"
)

// tenantSyncDestinations lists all downstream services that must receive tenant provisioning events.
var tenantSyncDestinations = []string{
	"logistics-api",
	"inventory-api",
	"pos-api",
	"notifications-api",
	"treasury-api",
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

	syncer := tenant.NewSyncer(client)
	// Sync platform org (codevertex) so tenant row exists; seed continues if auth-api unavailable.
	var codevertexTenantID uuid.UUID
	if id, err := syncer.SyncTenant(ctx, "codevertex"); err != nil {
		log.Printf("  [SKIP] sync codevertex (platform org): %v", err)
	} else {
		codevertexTenantID = id
	}
	tenantUUID, err := syncer.SyncTenant(ctx, "urban-loft")
	if err != nil {
		log.Fatalf("sync tenant: %v", err)
	}

	if err := runSeed(ctx, client, tenantUUID); err != nil {
		log.Fatalf("seed data: %v", err)
	}

	// Seed platform admin user for codevertex (global admin) when SEED_PLATFORM_ADMIN_USER_ID is set.
	if codevertexTenantID != uuid.Nil {
		if err := seedPlatformAdminForCodevertex(ctx, client, codevertexTenantID); err != nil {
			log.Printf("  [SKIP] seed platform admin for codevertex: %v", err)
		}
	}

	log.Println("database seed completed successfully")
}

// runSeed seeds service-level data: tenant sync events, settings, roles, permissions, demo users, and catalog data.
func runSeed(ctx context.Context, client *ent.Client, tenantUUID uuid.UUID) (err error) {
	const superAdminID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

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

	if err = enqueueTenantSyncEvents(ctx, tx, tenantUUID, "urban-loft"); err != nil {
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

	if err = seedDemoUser(ctx, tx, tenantUUID, now); err != nil {
		return err
	}

	if err = seedCatalog(ctx, tx, tenantUUID); err != nil {
		return err
	}

	return nil
}

// seedDemoUser creates the demo admin user (idempotent).
func seedDemoUser(ctx context.Context, tx *ent.Tx, tenantID uuid.UUID, now time.Time) error {
	const (
		demoEmail    = "demo@theurbanloftcafe.com"
		demoFullName = "Demo Admin"
		status       = "active"
		locale       = "en"
		timezone     = "Africa/Nairobi"
		primaryRole  = "admin"
	)
	demoUserID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("bengobox:cafe:user:"+demoEmail))
	demoPasswordHash := "$2a$10$c1SpaELSb9xPUIoFQ8np.OYphHWIBxkPdm9Su.52eCeEet0VM8Q2G" // bcrypt("password123")
	demoRoleIDs := []string{"admin"}
	metadata := map[string]any{
		"timezone": timezone,
	}

	_, err := tx.User.Get(ctx, demoUserID)
	switch {
	case ent.IsNotFound(err):
		_, createErr := tx.User.Create().
			SetID(demoUserID).
			SetTenantID(tenantID).
			SetEmail(demoEmail).
			SetPasswordHash(demoPasswordHash).
			SetFullName(demoFullName).
			SetStatus(status).
			SetLocale(locale).
			SetTimezone(timezone).
			SetMetadata(metadata).
			SetPrimaryRole(primaryRole).
			SetCreatedAt(now).
			SetUpdatedAt(now).
			AddRoleIDs(demoRoleIDs...).
			Save(ctx)
		if createErr != nil {
			return fmt.Errorf("create demo admin: %w", createErr)
		}
	case err != nil:
		return fmt.Errorf("lookup demo admin: %w", err)
	default:
		_, updateErr := tx.User.UpdateOneID(demoUserID).
			SetTenantID(tenantID).
			SetEmail(demoEmail).
			SetPasswordHash(demoPasswordHash).
			SetFullName(demoFullName).
			SetStatus(status).
			SetLocale(locale).
			SetTimezone(timezone).
			SetMetadata(metadata).
			SetPrimaryRole(primaryRole).
			SetUpdatedAt(now).
			ClearRoles().
			AddRoleIDs(demoRoleIDs...).
			Save(ctx)
		if updateErr != nil {
			return fmt.Errorf("update demo admin: %w", updateErr)
		}
	}

	if err := upsertUserPreference(ctx, tx, demoUserID, timezone); err != nil {
		return fmt.Errorf("create demo user preference: %w", err)
	}
	if err := upsertUserProfile(ctx, tx, demoUserID); err != nil {
		return fmt.Errorf("create demo user profile: %w", err)
	}
	return nil
}

// seedPermissions seeds all service-level permissions following Django-style {entity}:{action} pattern.
// Actions: add, read, read_own, change, change_own, delete, delete_own, manage, manage_own.
// Plus service-specific actions where required.
func seedPermissions(ctx context.Context, tx *ent.Tx) (map[string]uuid.UUID, error) {
	type perm struct {
		code string
		name string
		mod  string
		desc string
	}
	permissions := []perm{
		// --- Orders ---
		{"orders:add", "Add orders", "orders", "Create new orders"},
		{"orders:read", "Read orders", "orders", "View any order in the tenant"},
		{"orders:read_own", "Read own orders", "orders", "View own orders only"},
		{"orders:change", "Change orders", "orders", "Update any order"},
		{"orders:change_own", "Change own orders", "orders", "Update own orders only"},
		{"orders:delete", "Delete orders", "orders", "Cancel or delete orders"},
		{"orders:manage", "Manage orders", "orders", "Full order management including status transitions"},
		{"orders:manage_own", "Manage own orders", "orders", "Manage own orders only"},
		{"orders:refund", "Refund orders", "orders", "Issue customer refunds"},

		// --- Catalog (projection — display/availability managed here; master data in inventory-api) ---
		{"catalog:add", "Add catalog items", "catalog", "Create catalog items and categories"},
		{"catalog:read", "Read catalog", "catalog", "View catalog items and categories"},
		{"catalog:read_own", "Read own catalog", "catalog", "View catalog scoped to own outlet"},
		{"catalog:change", "Change catalog", "catalog", "Edit catalog items and categories"},
		{"catalog:change_own", "Change own catalog", "catalog", "Edit catalog scoped to own outlet"},
		{"catalog:delete", "Delete catalog items", "catalog", "Remove catalog items and categories"},
		{"catalog:manage", "Manage catalog", "catalog", "Full catalog management"},
		{"catalog:manage_own", "Manage own catalog", "catalog", "Manage catalog scoped to own outlet"},

		// --- Payments (data owned by treasury-api; ordering stores payment_status reference) ---
		{"payments:read", "Read payments", "payments", "View payment and settlement data"},
		{"payments:manage", "Manage payments", "payments", "Capture and refund payments"},

		// --- Loyalty ---
		{"loyalty:read", "Read loyalty", "loyalty", "View loyalty points and history"},
		{"loyalty:read_own", "Read own loyalty", "loyalty", "View own loyalty balance"},
		{"loyalty:redeem", "Redeem loyalty", "loyalty", "Redeem loyalty points on orders"},
		{"loyalty:manage", "Manage loyalty", "loyalty", "Adjust loyalty accounts and tiers"},

		// --- Logistics (data owned by logistics-api; ordering stores rider_id reference) ---
		{"logistics:read", "Read logistics", "logistics", "View rider assignments and delivery tracking"},
		{"logistics:dispatch", "Dispatch orders", "logistics", "Assign riders and manage delivery flow"},
		{"logistics:manage", "Manage logistics", "logistics", "Full logistics management"},

		// --- Notifications (data owned by notifications-api) ---
		{"notifications:read", "Read notifications", "notifications", "View notification queue and state"},
		{"notifications:manage", "Manage notifications", "notifications", "Manage templates and retry notifications"},

		// --- Analytics ---
		{"analytics:read", "Read analytics", "analytics", "Access dashboards and reporting"},
		{"analytics:export", "Export analytics", "analytics", "Run and export analytic jobs"},
		{"analytics:manage", "Manage analytics", "analytics", "Configure analytic settings"},

		// --- Support ---
		{"support:read", "Read support", "support", "View support tickets"},
		{"support:manage", "Manage support", "support", "Respond to and close support cases"},

		// --- Promotions ---
		{"promotions:add", "Add promotions", "promotions", "Create promo codes and campaigns"},
		{"promotions:read", "Read promotions", "promotions", "View promotions and redemptions"},
		{"promotions:change", "Change promotions", "promotions", "Edit promo codes"},
		{"promotions:delete", "Delete promotions", "promotions", "Remove promo codes"},
		{"promotions:manage", "Manage promotions", "promotions", "Full promotions management"},

		// --- Delivery zones ---
		{"zones:add", "Add delivery zones", "zones", "Create delivery zones"},
		{"zones:read", "Read delivery zones", "zones", "View delivery zones"},
		{"zones:change", "Change delivery zones", "zones", "Edit delivery zones"},
		{"zones:delete", "Delete delivery zones", "zones", "Remove delivery zones"},
		{"zones:manage", "Manage delivery zones", "zones", "Full delivery zone management"},

		// --- Identity (users, roles, settings) ---
		{"profile:update", "Update profile", "identity", "Update personal profile and avatar"},
		{"preferences:update", "Update preferences", "identity", "Modify notification and UI preferences"},
		{"admin:manage", "Manage tenant settings", "identity", "Manage tenant configuration, users, and roles"},
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
		{"customer", "Customer", "End-customer placing delivery orders", "tenant"},
		{"rider", "Rider", "Delivery rider fulfilling orders", "tenant"},
		{"staff", "Staff", "Cafe/store staff: order prep, kitchen, counter", "tenant"},
		{"admin", "Admin", "Tenant administrator with full tenant management", "tenant"},
		{"superuser", "Super User", "Platform super administrator with global access", "global"},
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
			"orders:add",
			"orders:read_own",
			"orders:change_own",
			"orders:manage_own",
			"loyalty:read_own",
			"loyalty:redeem",
			"profile:update",
			"preferences:update",
		},
		"rider": {
			"orders:read",
			"logistics:read",
			"logistics:dispatch",
			"profile:update",
			"preferences:update",
		},
		"staff": {
			"orders:add",
			"orders:read",
			"orders:change",
			"orders:manage",
			"catalog:read",
			"catalog:add",
			"catalog:change",
			"catalog:manage",
			"loyalty:read",
			"notifications:read",
			"support:read",
			"profile:update",
			"preferences:update",
		},
		"admin": {
			"orders:add",
			"orders:read",
			"orders:change",
			"orders:delete",
			"orders:manage",
			"orders:refund",
			"catalog:add",
			"catalog:read",
			"catalog:change",
			"catalog:delete",
			"catalog:manage",
			"payments:read",
			"payments:manage",
			"loyalty:read",
			"loyalty:manage",
			"logistics:read",
			"logistics:dispatch",
			"logistics:manage",
			"notifications:read",
			"notifications:manage",
			"analytics:read",
			"analytics:export",
			"analytics:manage",
			"support:read",
			"support:manage",
			"promotions:add",
			"promotions:read",
			"promotions:change",
			"promotions:delete",
			"promotions:manage",
			"zones:add",
			"zones:read",
			"zones:change",
			"zones:delete",
			"zones:manage",
			"admin:manage",
			"profile:update",
			"preferences:update",
		},
		"superuser": {
			"orders:add",
			"orders:read",
			"orders:change",
			"orders:delete",
			"orders:manage",
			"orders:refund",
			"catalog:add",
			"catalog:read",
			"catalog:change",
			"catalog:delete",
			"catalog:manage",
			"payments:read",
			"payments:manage",
			"loyalty:read",
			"loyalty:manage",
			"loyalty:redeem",
			"logistics:read",
			"logistics:dispatch",
			"logistics:manage",
			"notifications:read",
			"notifications:manage",
			"analytics:read",
			"analytics:export",
			"analytics:manage",
			"support:read",
			"support:manage",
			"promotions:add",
			"promotions:read",
			"promotions:change",
			"promotions:delete",
			"promotions:manage",
			"zones:add",
			"zones:read",
			"zones:change",
			"zones:delete",
			"zones:manage",
			"admin:manage",
			"profile:update",
			"preferences:update",
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
		Where(tenantsetting.HasTenantWith(enttenant.IDEQ(tenantID))).
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
		existing, err := tx.TenantSyncEvent.Query().
			Where(
				tenantsyncevent.TenantIDEQ(tenantID),
				tenantsyncevent.DestinationServiceEQ(destination),
			).
			Only(ctx)
		if err == nil && existing != nil {
			_, updateErr := tx.TenantSyncEvent.UpdateOneID(existing.ID).
				SetPayload(payload).
				Save(ctx)
			if updateErr != nil {
				return fmt.Errorf("update tenant sync event for %s: %w", destination, updateErr)
			}
			continue
		}

		_, createErr := tx.TenantSyncEvent.
			Create().
			SetTenantID(tenantID).
			SetTenantSlug(tenantSlug).
			SetDestinationService(destination).
			SetPayload(payload).
			Save(ctx)
		if createErr != nil {
			if !strings.Contains(createErr.Error(), "duplicate key") && !strings.Contains(createErr.Error(), "UNIQUE constraint") {
				return fmt.Errorf("enqueue tenant sync event for %s: %w", destination, createErr)
			}
		}
	}
	return nil
}

func seedSuperAdmin(ctx context.Context, tx *ent.Tx, tenantID, userID uuid.UUID, passwordHash string, now time.Time) error {
	const (
		email       = "superuser@theurbanloftcafe.com"
		fullName    = "Super Admin"
		status      = "active"
		locale      = "en"
		timezone    = "Africa/Nairobi"
		primaryRole = "superuser"
	)
	roleIDs := []string{"superuser", "admin"}
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

// seedPlatformAdminForCodevertex creates or updates the platform org admin user so they have
// superuser + admin roles. Requires SEED_PLATFORM_ADMIN_USER_ID env var (UUID from auth-api).
func seedPlatformAdminForCodevertex(ctx context.Context, client *ent.Client, codevertexTenantID uuid.UUID) error {
	platformAdminIDStr := os.Getenv("SEED_PLATFORM_ADMIN_USER_ID")
	if platformAdminIDStr == "" {
		return fmt.Errorf("SEED_PLATFORM_ADMIN_USER_ID not set — skip platform admin seed")
	}
	platformAdminID, err := uuid.Parse(platformAdminIDStr)
	if err != nil {
		return fmt.Errorf("invalid SEED_PLATFORM_ADMIN_USER_ID: %w", err)
	}
	email := os.Getenv("SEED_PLATFORM_ADMIN_EMAIL")
	if email == "" {
		email = "admin@codevertexitsolutions.com"
	}

	tx, err := client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	now := time.Now().UTC()
	roleIDs := []string{"superuser", "admin"}
	_, err = tx.User.Get(ctx, platformAdminID)
	switch {
	case ent.IsNotFound(err):
		_, err = tx.User.Create().
			SetID(platformAdminID).
			SetTenantID(codevertexTenantID).
			SetEmail(email).
			SetPasswordHash("").
			SetFullName("Platform Admin").
			SetStatus("active").
			SetLocale("en").
			SetTimezone("Africa/Nairobi").
			SetPrimaryRole("superuser").
			SetCreatedAt(now).
			SetUpdatedAt(now).
			AddRoleIDs(roleIDs...).
			Save(ctx)
		if err != nil {
			return err
		}
		log.Printf("  ✓ Created platform admin user for codevertex: %s", email)
	case err != nil:
		return err
	default:
		_, err = tx.User.UpdateOneID(platformAdminID).
			SetTenantID(codevertexTenantID).
			SetEmail(email).
			SetFullName("Platform Admin").
			SetStatus("active").
			SetPrimaryRole("superuser").
			SetUpdatedAt(now).
			ClearRoles().
			AddRoleIDs(roleIDs...).
			Save(ctx)
		if err != nil {
			return err
		}
		log.Printf("  ✓ Updated platform admin user for codevertex: %s", email)
	}
	return tx.Commit()
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
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("bengobox:cafe:permission:"+code))
}

// --- Catalog Seeding ---

func seedCatalog(ctx context.Context, tx *ent.Tx, tenantID uuid.UUID) error {
	outletID, err := seedOutlet(ctx, tx, tenantID)
	if err != nil {
		return fmt.Errorf("seed outlet: %w", err)
	}

	categoryIDs, err := seedCategories(ctx, tx, tenantID, outletID)
	if err != nil {
		return fmt.Errorf("seed categories: %w", err)
	}

	if err := seedCatalogItems(ctx, tx, tenantID, outletID, categoryIDs); err != nil {
		return fmt.Errorf("seed catalog items: %w", err)
	}

	log.Println("  ✓ Catalog seeded (outlet, categories, menu items)")
	return nil
}

func seedOutlet(ctx context.Context, tx *ent.Tx, tenantID uuid.UUID) (uuid.UUID, error) {
	outletID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("bengobox:cafe:outlet:urban-loft:busia"))

	existing, err := tx.Outlet.Query().Where(outlet.ID(outletID)).Only(ctx)
	if err == nil {
		return existing.ID, nil
	}
	if !ent.IsNotFound(err) {
		return uuid.Nil, err
	}

	o, err := tx.Outlet.Create().
		SetID(outletID).
		SetTenantID(tenantID).
		SetName("Urban Loft Cafe Busia").
		SetSlug("busia").
		SetDescription("The Urban Loft Cafe, Busia branch — coffee, meals, and more.").
		SetAddress("Main Street, Busia, Kenya").
		SetPhone("+254700000000").
		SetEmail("busia@theurbanloftcafe.com").
		SetLocation("Busia, Kenya").
		SetImageURL("/media/images/outlets/urban-loft-busia.jpeg").
		SetUseCase("hospitality").
		SetStatus("active").
		Save(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	return o.ID, nil
}

// inventoryItemUUID computes the same deterministic UUID that inventory-api uses for items.
// This ensures inventory_item_id references match without needing to call the inventory API.
func inventoryItemUUID(tenantID uuid.UUID, sku string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(fmt.Sprintf("bengobox:inventory:item:%s:%s", tenantID, sku)))
}

// inventoryCategoryUUID computes the same deterministic UUID that inventory-api uses for categories.
func inventoryCategoryUUID(tenantID uuid.UUID, slug string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(fmt.Sprintf("bengobox:inventory:category:%s:%s", tenantID, slug)))
}

func seedCategories(ctx context.Context, tx *ent.Tx, tenantID, outletID uuid.UUID) (map[string]uuid.UUID, error) {
	type cat struct {
		name        string
		slug        string
		description string
		imageURL    string
		order       int
	}
	// Categories aligned with inventory-api's seeded categories
	categories := []cat{
		{"Hot Beverages", "hot-beverages", "Coffee, tea, and other hot drinks", "/media/icons/coffee-colored.svg", 1},
		{"Cold Beverages", "cold-beverages", "Juices, smoothies, and iced drinks", "/media/icons/juice-colored.svg", 2},
		{"Pastries & Bakery", "pastries", "Croissants, muffins, cakes, and baked goods", "/media/icons/cake-colored.svg", 3},
		{"Sandwiches & Wraps", "sandwiches", "Club sandwiches, panini, wraps", "/media/icons/sandwich-colored.svg", 4},
		{"Salads", "salads", "Fresh salads and bowls", "/media/icons/fresh-colored.svg", 5},
		{"Light Bites", "light-bites", "Samosas, spring rolls, and appetizers", "/media/icons/snack-colored.svg", 6},
		{"Main Courses", "main-courses", "Hearty lunch and dinner entrees", "/media/icons/drumstick-colored.svg", 7},
		{"Breakfast", "breakfast", "Morning meals and light bites", "/media/icons/breakfast-colored.svg", 8},
		{"Pizza", "pizza", "Freshly baked pizzas", "/media/icons/pizza-colored.svg", 9},
		{"Desserts", "desserts", "Sweet treats and pastries", "/media/icons/dessert-colored.svg", 10},
	}

	ids := make(map[string]uuid.UUID, len(categories))
	for _, c := range categories {
		catID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("bengobox:cafe:category:urban-loft:"+c.slug))

		existing, err := tx.CatalogCategory.Query().Where(catalogcategory.ID(catID)).Only(ctx)
		if err == nil {
			ids[c.slug] = existing.ID
			// Update in case fields changed
			_, _ = tx.CatalogCategory.UpdateOneID(catID).
				SetName(c.name).
				SetSlug(c.slug).
				SetDescription(c.description).
				SetImageURL(c.imageURL).
				SetDisplayOrder(c.order).
				Save(ctx)
			continue
		}
		if !ent.IsNotFound(err) {
			return nil, err
		}

		created, err := tx.CatalogCategory.Create().
			SetID(catID).
			SetNillableTenantID(&tenantID).
			SetNillableOutletID(&outletID).
			SetName(c.name).
			SetSlug(c.slug).
			SetDescription(c.description).
			SetImageURL(c.imageURL).
			SetDisplayOrder(c.order).
			SetIsActive(true).
			Save(ctx)
		if err != nil {
			return nil, fmt.Errorf("create category %s: %w", c.name, err)
		}
		ids[c.slug] = created.ID
	}
	return ids, nil
}

// seedCatalogItems creates catalog items as projections from inventory-api master data.
// Uses the same deterministic UUID formula as inventory-api so inventory_item_id matches.
// Prices are ordering-specific (inventory-api does not store sell prices).
func seedCatalogItems(ctx context.Context, tx *ent.Tx, tenantID, outletID uuid.UUID, categories map[string]uuid.UUID) error {
	// Media paths — relative to the media server root.
	// These match the paths used in inventory-api and the media folder layout.
	const (
		imgEspresso     = "/media/images/outlets/menu/espresso.jpg"
		imgCappuccino   = "/media/images/outlets/menu/cappuccino.jpg"
		imgHotCoffee    = "/media/images/outlets/menu/hot coffee.jpeg"
		imgIcedLatte    = "/media/images/outlets/menu/icedlatte.jpeg"
		imgMilkshake    = "/media/images/outlets/menu/milkshake.jpeg"
		imgCocktail     = "/media/images/outlets/menu/cocktail.jpeg"
		imgDessert      = "/media/images/outlets/menu/dessert.jpeg"
		imgLavaCake     = "/media/images/outlets/menu/chocolate-lava-cake.jpg"
		imgMain1        = "/media/images/outlets/menu/main-course-1.jpg"
		imgMain2        = "/media/images/outlets/menu/main-course-2.jpg"
		imgChicken      = "/media/images/outlets/menu/chicken.jpeg"
		imgChickenUgali = "/media/images/outlets/menu/chicken_ugali.jpeg"
		imgPilau        = "/media/images/outlets/menu/pilau.jpeg"
		imgFish         = "/media/images/outlets/menu/fish.jpeg"
		imgSalad        = "/media/images/outlets/menu/salad.jpg"
		imgBreakfast    = "/media/images/outlets/menu/breakfast.jpg"
		imgOats         = "/media/images/outlets/menu/oats.jpeg"
		imgPizza        = "/media/images/outlets/menu/margherita-pizza.jpg"
		imgBurger       = "/media/images/outlets/menu/burger.jpg"
	)

	type item struct {
		sku         string
		name        string
		description string
		price       float64 // KES — ordering-specific pricing
		category    string  // must match a seeded category slug
		imageURL    string
		featured    bool
	}
	// Items aligned with inventory-api catalogItemDefs (same SKUs, names, descriptions).
	// inventory_item_id is computed using the same deterministic UUID formula.
	// Image URLs match inventory-api's media paths.
	items := []item{
		// Hot Beverages (inventory: hot-beverages)
		{"BEV-ESP-001", "Espresso", "Single shot of rich espresso", 250, "hot-beverages", imgEspresso, false},
		{"BEV-ESP-002", "Double Espresso", "Double shot espresso", 300, "hot-beverages", imgEspresso, false},
		{"BEV-LAT-001", "Caffe Latte", "Espresso with steamed milk", 350, "hot-beverages", imgCappuccino, false},
		{"BEV-CAP-001", "Cappuccino", "Espresso with frothed milk and cocoa", 350, "hot-beverages", imgCappuccino, true},
		{"BEV-AME-001", "Americano", "Espresso with hot water", 280, "hot-beverages", imgHotCoffee, false},
		{"BEV-MOC-001", "Mocha", "Espresso, chocolate, steamed milk, whipped cream", 400, "hot-beverages", imgHotCoffee, false},
		{"BEV-MAC-001", "Macchiato", "Espresso with a dash of milk foam", 300, "hot-beverages", imgEspresso, false},
		{"BEV-TEA-001", "Kenya AA Black Tea", "Premium Kenyan black tea", 200, "hot-beverages", imgHotCoffee, false},
		{"BEV-TEA-002", "Masala Chai", "Spiced tea latte with cardamom and ginger", 250, "hot-beverages", imgHotCoffee, true},
		{"BEV-HOT-001", "Hot Chocolate", "Rich hot chocolate with whipped cream", 400, "hot-beverages", imgHotCoffee, false},

		// Cold Beverages (inventory: cold-beverages)
		{"BEV-ICE-001", "Iced Latte", "Chilled espresso with cold milk over ice", 350, "cold-beverages", imgIcedLatte, false},
		{"BEV-ICE-002", "Iced Americano", "Espresso over ice with cold water", 300, "cold-beverages", imgIcedLatte, false},
		{"BEV-FRP-001", "Caramel Frappe", "Blended iced coffee with caramel drizzle", 450, "cold-beverages", imgMilkshake, true},
		{"BEV-FRP-002", "Vanilla Frappe", "Blended iced coffee with vanilla", 450, "cold-beverages", imgMilkshake, false},
		{"BEV-SMO-001", "Mango Smoothie", "Fresh mango blended with yoghurt", 400, "cold-beverages", imgCocktail, true},
		{"BEV-SMO-002", "Mixed Berry Smoothie", "Strawberry, blueberry, and banana blend", 400, "cold-beverages", imgCocktail, false},
		{"BEV-JCE-001", "Fresh Orange Juice", "Freshly squeezed orange juice", 350, "cold-beverages", imgCocktail, false},

		// Pastries & Bakery (inventory: pastries)
		{"PST-CRO-001", "Butter Croissant", "Flaky French butter croissant", 200, "pastries", imgDessert, false},
		{"PST-CRO-002", "Chocolate Croissant", "Croissant filled with dark chocolate", 250, "pastries", imgDessert, true},
		{"PST-MUF-001", "Blueberry Muffin", "Moist muffin loaded with blueberries", 220, "pastries", imgDessert, false},
		{"PST-MUF-002", "Banana Walnut Muffin", "Banana muffin with crunchy walnuts", 220, "pastries", imgDessert, false},
		{"PST-CKE-001", "Carrot Cake Slice", "Spiced carrot cake with cream cheese frosting", 350, "pastries", imgLavaCake, false},
		{"PST-CKE-002", "Red Velvet Cake Slice", "Classic red velvet with vanilla cream cheese", 380, "pastries", imgLavaCake, false},
		{"PST-CKE-003", "Chocolate Fudge Cake Slice", "Rich chocolate fudge layer cake", 380, "pastries", imgLavaCake, false},
		{"PST-DAN-001", "Danish Pastry", "Flaky pastry with custard and fruit", 250, "pastries", imgDessert, false},
		{"PST-SCO-001", "Classic Scone", "Buttermilk scone with clotted cream and jam", 200, "pastries", imgDessert, false},

		// Sandwiches & Wraps (inventory: sandwiches)
		{"SND-CLB-001", "Club Sandwich", "Triple-decker with chicken, bacon, lettuce, tomato", 650, "sandwiches", imgMain1, false},
		{"SND-GRL-001", "Grilled Chicken Panini", "Grilled chicken, pesto, mozzarella on ciabatta", 600, "sandwiches", imgChicken, true},
		{"SND-VEG-001", "Veggie Wrap", "Hummus, avocado, roasted vegetables in tortilla", 500, "sandwiches", imgSalad, false},
		{"SND-BLT-001", "BLT Sandwich", "Bacon, lettuce, tomato on toasted sourdough", 550, "sandwiches", imgMain1, false},
		{"SND-TUN-001", "Tuna Melt", "Tuna salad with melted cheddar on rye bread", 550, "sandwiches", imgMain1, false},

		// Salads (inventory: salads)
		{"SAL-CES-001", "Caesar Salad", "Romaine, croutons, parmesan, caesar dressing", 500, "salads", imgSalad, false},
		{"SAL-GRK-001", "Greek Salad", "Cucumber, tomato, olives, feta, olive oil", 500, "salads", imgSalad, false},

		// Light Bites (inventory: light-bites)
		{"BTE-SAM-001", "Samosa (3pc)", "Crispy vegetable samosas with tamarind chutney", 300, "light-bites", imgMain2, false},
		{"BTE-SPR-001", "Spring Rolls (4pc)", "Crispy vegetable spring rolls with sweet chilli sauce", 350, "light-bites", imgMain2, false},

		// Main Courses (inventory: main-courses)
		{"MIN-GRL-001", "Grilled Beef Fillet", "250g beef fillet with pepper sauce, mash and seasonal veg", 1200, "main-courses", imgMain1, true},
		{"MIN-GRL-002", "Grilled Chicken Breast", "Herb-marinated chicken with gravy, rice and vegetables", 950, "main-courses", imgChickenUgali, true},
		{"MIN-CUR-001", "Chicken Curry", "Spiced chicken curry with basmati rice and naan", 850, "main-courses", imgChicken, false},
		{"MIN-CUR-002", "Beef Stew", "Tender beef stew with potatoes and carrots, served with ugali or rice", 800, "main-courses", imgPilau, false},
		{"MIN-SEA-001", "Fish and Chips", "Beer-battered fish with chips and tartar sauce", 850, "main-courses", imgFish, false},
		{"MIN-PAS-001", "Spaghetti Bolognese", "Classic beef bolognese with parmesan and garlic bread", 750, "main-courses", imgMain1, false},
		{"MIN-RIC-001", "Pilau Rice Bowl", "Spiced pilau rice with choice of beef, chicken or veg", 700, "main-courses", imgPilau, false},

		// Breakfast (inventory: breakfast)
		{"BRK-FUL-001", "Full English Breakfast", "Eggs, bacon, sausage, beans, toast, tomato", 850, "breakfast", imgBreakfast, true},
		{"BRK-PAN-001", "Pancake Stack", "Fluffy pancakes with maple syrup and berries", 650, "breakfast", imgBreakfast, false},
		{"BRK-AVT-001", "Avocado Toast", "Smashed avocado on sourdough with poached egg", 550, "breakfast", imgBreakfast, false},
		{"BRK-OAT-001", "Overnight Oats", "Oats soaked in almond milk with fresh fruits and honey", 400, "breakfast", imgOats, false},

		// Pizza (inventory: pizza)
		{"PIZ-MAR-001", "Margherita Pizza", "Fresh mozzarella, tomato sauce, and basil", 750, "pizza", imgPizza, false},
		{"PIZ-PEP-001", "Pepperoni Pizza", "Classic pepperoni with mozzarella and tomato sauce", 850, "pizza", imgPizza, true},
	}

	for i, it := range items {
		catID, ok := categories[it.category]
		if !ok {
			log.Printf("  [SKIP] category %s not found for item %s — skipping", it.category, it.name)
			continue
		}

		// Use deterministic UUID for catalog item
		itemID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("bengobox:cafe:item:urban-loft:"+it.sku))
		// Compute inventory_item_id using inventory-api's UUID formula
		invItemID := inventoryItemUUID(tenantID, it.sku)

		existing, err := tx.CatalogItem.Query().Where(catalogitem.ID(itemID)).Only(ctx)
		if err == nil {
			_, _ = tx.CatalogItem.UpdateOneID(existing.ID).
				SetName(it.name).
				SetDescription(it.description).
				SetBasePrice(it.price).
				SetCategoryID(catID).
				SetImageURL(it.imageURL).
				SetIsFeatured(it.featured).
				SetInventoryItemID(invItemID).
				Save(ctx)
			continue
		}
		if !ent.IsNotFound(err) {
			return err
		}

		_, err = tx.CatalogItem.Create().
			SetID(itemID).
			SetTenantID(tenantID).
			SetOutletID(outletID).
			SetCategoryID(catID).
			SetInventoryItemID(invItemID).
			SetName(it.name).
			SetDescription(it.description).
			SetBasePrice(it.price).
			SetCurrency("KES").
			SetImageURL(it.imageURL).
			SetSku(it.sku).
			SetIsAvailable(true).
			SetIsFeatured(it.featured).
			SetDisplayOrder(i + 1).
			Save(ctx)
		if err != nil {
			return fmt.Errorf("create item %s: %w", it.name, err)
		}
	}
	return nil
}
