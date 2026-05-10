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
	"github.com/bengobox/ordering-backend/internal/ent/catalogoverride"
	"github.com/bengobox/ordering-backend/internal/ent/orderingpermission"
	"github.com/bengobox/ordering-backend/internal/ent/orderingrole"
	"github.com/bengobox/ordering-backend/internal/ent/outlet"
	"github.com/bengobox/ordering-backend/internal/ent/ratelimitconfig"
	"github.com/bengobox/ordering-backend/internal/ent/rolepermission"
	"github.com/bengobox/ordering-backend/internal/ent/serviceconfig"
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
	db.SetMaxIdleConns(2)
	db.SetMaxOpenConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(1 * time.Minute)
	driver := entsql.OpenDB(dialect.Postgres, db)

	client := ent.NewClient(ent.Driver(driver))
	defer client.Close()

	syncer := tenant.NewSyncer(client, cfg.Auth.ServiceURL)
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

// runSeed seeds service-level data: tenant sync events, settings, roles, permissions, and catalog data.
// Users are NOT seeded here — they are synced from auth-api via JIT provisioning (EnsureUserFromToken)
// and NATS events (SyncUserFromAuthService). All user management is centralized in auth-api.
func runSeed(ctx context.Context, client *ent.Client, tenantUUID uuid.UUID) (err error) {
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

	// --- New RBAC system (OrderingPermission, OrderingRole, RolePermission, RateLimitConfig, ServiceConfig) ---
	orderingPermMap, err := seedOrderingPermissions(ctx, tx)
	if err != nil {
		return err
	}

	if err = seedOrderingRoles(ctx, tx, tenantUUID, orderingPermMap); err != nil {
		return err
	}

	if err = seedRateLimitConfigs(ctx, tx); err != nil {
		return err
	}

	if err = seedServiceConfigs(ctx, tx); err != nil {
		return err
	}

	if err = seedCatalog(ctx, tx, tenantUUID); err != nil {
		return err
	}

	return nil
}

// seedDemoUser is DEPRECATED — users are now synced from auth-api via JIT provisioning.
// This function is retained but no longer called from runSeed.
func seedDemoUser(ctx context.Context, tx *ent.Tx, tenantID uuid.UUID, now time.Time) error {
	const (
		demoEmail    = "info@theurbanloftcafe.com"
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

	// Resolve email conflicts: if another user has this email, delete them.
	_, err := tx.User.Query().
		Where(user.EmailEQ(demoEmail), user.TenantIDEQ(tenantID), user.IDNEQ(demoUserID)).
		Only(ctx)
	if err == nil {
		if _, delErr := tx.User.Delete().
			Where(user.EmailEQ(demoEmail), user.TenantIDEQ(tenantID), user.IDNEQ(demoUserID)).
			Exec(ctx); delErr != nil {
			return fmt.Errorf("resolve email conflict for demo user: %w", delErr)
		}
	}

	_, err = tx.User.Get(ctx, demoUserID)
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

// seedPermissions seeds all service-level permissions following Django-style ordering.{module}.{action} pattern.
// Standard actions: add, view, view_own, change, change_own, delete, delete_own, manage, manage_own.
func seedPermissions(ctx context.Context, tx *ent.Tx) (map[string]uuid.UUID, error) {
	type perm struct {
		code string
		name string
		mod  string
		desc string
	}
	permissions := []perm{
		// --- Orders ---
		{"ordering.orders.add", "Add orders", "orders", "Create new orders"},
		{"ordering.orders.view", "View orders", "orders", "View any order in the tenant"},
		{"ordering.orders.view_own", "View own orders", "orders", "View own orders only"},
		{"ordering.orders.change", "Change orders", "orders", "Update any order"},
		{"ordering.orders.change_own", "Change own orders", "orders", "Update own orders only"},
		{"ordering.orders.delete", "Delete orders", "orders", "Cancel or delete orders"},
		{"ordering.orders.delete_own", "Delete own orders", "orders", "Cancel own orders only"},
		{"ordering.orders.manage", "Manage orders", "orders", "Full order management including status transitions"},
		{"ordering.orders.manage_own", "Manage own orders", "orders", "Manage own orders only"},

		// --- Catalog ---
		{"ordering.catalog.add", "Add catalog items", "catalog", "Create catalog items and categories"},
		{"ordering.catalog.view", "View catalog", "catalog", "View catalog items and categories"},
		{"ordering.catalog.view_own", "View own catalog", "catalog", "View catalog scoped to own outlet"},
		{"ordering.catalog.change", "Change catalog", "catalog", "Edit catalog items and categories"},
		{"ordering.catalog.change_own", "Change own catalog", "catalog", "Edit catalog scoped to own outlet"},
		{"ordering.catalog.delete", "Delete catalog items", "catalog", "Remove catalog items and categories"},
		{"ordering.catalog.delete_own", "Delete own catalog items", "catalog", "Remove catalog scoped to own outlet"},
		{"ordering.catalog.manage", "Manage catalog", "catalog", "Full catalog management"},
		{"ordering.catalog.manage_own", "Manage own catalog", "catalog", "Manage catalog scoped to own outlet"},

		// --- Loyalty ---
		{"ordering.loyalty.add", "Add loyalty programs", "loyalty", "Create loyalty programs"},
		{"ordering.loyalty.view", "View loyalty", "loyalty", "View loyalty points and history"},
		{"ordering.loyalty.view_own", "View own loyalty", "loyalty", "View own loyalty balance"},
		{"ordering.loyalty.change", "Change loyalty", "loyalty", "Edit loyalty programs"},
		{"ordering.loyalty.change_own", "Change own loyalty", "loyalty", "Edit own loyalty"},
		{"ordering.loyalty.delete", "Delete loyalty programs", "loyalty", "Remove loyalty programs"},
		{"ordering.loyalty.delete_own", "Delete own loyalty programs", "loyalty", "Remove own loyalty programs"},
		{"ordering.loyalty.manage", "Manage loyalty", "loyalty", "Full loyalty management"},
		{"ordering.loyalty.manage_own", "Manage own loyalty", "loyalty", "Manage own loyalty"},

		// --- Promotions ---
		{"ordering.promotions.add", "Add promotions", "promotions", "Create promo codes and campaigns"},
		{"ordering.promotions.view", "View promotions", "promotions", "View promotions and redemptions"},
		{"ordering.promotions.view_own", "View own promotions", "promotions", "View own promotions"},
		{"ordering.promotions.change", "Change promotions", "promotions", "Edit promo codes"},
		{"ordering.promotions.change_own", "Change own promotions", "promotions", "Edit own promo codes"},
		{"ordering.promotions.delete", "Delete promotions", "promotions", "Remove promo codes"},
		{"ordering.promotions.delete_own", "Delete own promotions", "promotions", "Remove own promo codes"},
		{"ordering.promotions.manage", "Manage promotions", "promotions", "Full promotions management"},
		{"ordering.promotions.manage_own", "Manage own promotions", "promotions", "Manage own promotions"},

		// --- Delivery zones ---
		{"ordering.delivery_zones.add", "Add delivery zones", "delivery_zones", "Create delivery zones"},
		{"ordering.delivery_zones.view", "View delivery zones", "delivery_zones", "View delivery zones"},
		{"ordering.delivery_zones.view_own", "View own delivery zones", "delivery_zones", "View own delivery zones"},
		{"ordering.delivery_zones.change", "Change delivery zones", "delivery_zones", "Edit delivery zones"},
		{"ordering.delivery_zones.change_own", "Change own delivery zones", "delivery_zones", "Edit own delivery zones"},
		{"ordering.delivery_zones.delete", "Delete delivery zones", "delivery_zones", "Remove delivery zones"},
		{"ordering.delivery_zones.delete_own", "Delete own delivery zones", "delivery_zones", "Remove own delivery zones"},
		{"ordering.delivery_zones.manage", "Manage delivery zones", "delivery_zones", "Full delivery zone management"},
		{"ordering.delivery_zones.manage_own", "Manage own delivery zones", "delivery_zones", "Manage own delivery zones"},

		// --- Analytics ---
		{"ordering.analytics.add", "Add analytics reports", "analytics", "Create analytics reports"},
		{"ordering.analytics.view", "View analytics", "analytics", "Access dashboards and reporting"},
		{"ordering.analytics.view_own", "View own analytics", "analytics", "View own analytics"},
		{"ordering.analytics.change", "Change analytics", "analytics", "Edit analytics settings"},
		{"ordering.analytics.change_own", "Change own analytics", "analytics", "Edit own analytics"},
		{"ordering.analytics.delete", "Delete analytics reports", "analytics", "Remove analytics reports"},
		{"ordering.analytics.delete_own", "Delete own analytics reports", "analytics", "Remove own analytics reports"},
		{"ordering.analytics.manage", "Manage analytics", "analytics", "Full analytics management"},
		{"ordering.analytics.manage_own", "Manage own analytics", "analytics", "Manage own analytics"},

		// --- Config ---
		{"ordering.config.add", "Add config", "config", "Create configuration entries"},
		{"ordering.config.view", "View config", "config", "View service configuration"},
		{"ordering.config.view_own", "View own config", "config", "View own configuration"},
		{"ordering.config.change", "Change config", "config", "Edit configuration"},
		{"ordering.config.change_own", "Change own config", "config", "Edit own configuration"},
		{"ordering.config.delete", "Delete config", "config", "Remove configuration entries"},
		{"ordering.config.delete_own", "Delete own config", "config", "Remove own configuration entries"},
		{"ordering.config.manage", "Manage config", "config", "Full configuration management"},
		{"ordering.config.manage_own", "Manage own config", "config", "Manage own configuration"},

		// --- Users ---
		{"ordering.users.add", "Add users", "users", "Create new users"},
		{"ordering.users.view", "View users", "users", "View user profiles"},
		{"ordering.users.view_own", "View own profile", "users", "View own profile"},
		{"ordering.users.change", "Change users", "users", "Edit user details"},
		{"ordering.users.change_own", "Change own profile", "users", "Edit own profile"},
		{"ordering.users.delete", "Delete users", "users", "Remove users"},
		{"ordering.users.delete_own", "Delete own profile", "users", "Remove own profile"},
		{"ordering.users.manage", "Manage users", "users", "Full user management including roles"},
		{"ordering.users.manage_own", "Manage own profile", "users", "Manage own profile"},
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
			"ordering.orders.add",
			"ordering.orders.view_own",
			"ordering.orders.change_own",
			"ordering.orders.manage_own",
			"ordering.loyalty.view_own",
			"ordering.loyalty.change",
			"ordering.users.change_own",
			"ordering.config.change_own",
		},
		"rider": {
			"ordering.orders.view",
			"ordering.orders.change",
			"ordering.users.change_own",
			"ordering.config.change_own",
		},
		"staff": {
			"ordering.orders.add",
			"ordering.orders.view",
			"ordering.orders.change",
			"ordering.orders.manage",
			"ordering.catalog.view",
			"ordering.catalog.add",
			"ordering.catalog.change",
			"ordering.catalog.manage",
			"ordering.loyalty.view",
			"ordering.config.view",
			"ordering.orders.view_own",
			"ordering.users.change_own",
			"ordering.config.change_own",
		},
		"admin": {
			"ordering.orders.add",
			"ordering.orders.view",
			"ordering.orders.change",
			"ordering.orders.delete",
			"ordering.orders.manage",
			"ordering.catalog.add",
			"ordering.catalog.view",
			"ordering.catalog.change",
			"ordering.catalog.delete",
			"ordering.catalog.manage",
			"ordering.loyalty.view",
			"ordering.loyalty.manage",
			"ordering.analytics.view",
			"ordering.analytics.manage",
			"ordering.promotions.add",
			"ordering.promotions.view",
			"ordering.promotions.change",
			"ordering.promotions.delete",
			"ordering.promotions.manage",
			"ordering.delivery_zones.add",
			"ordering.delivery_zones.view",
			"ordering.delivery_zones.change",
			"ordering.delivery_zones.delete",
			"ordering.delivery_zones.manage",
			"ordering.config.view",
			"ordering.config.manage",
			"ordering.users.add",
			"ordering.users.view",
			"ordering.users.manage",
			"ordering.users.change_own",
			"ordering.config.change_own",
		},
		"superuser": {
			"ordering.orders.add",
			"ordering.orders.view",
			"ordering.orders.change",
			"ordering.orders.delete",
			"ordering.orders.manage",
			"ordering.catalog.add",
			"ordering.catalog.view",
			"ordering.catalog.change",
			"ordering.catalog.delete",
			"ordering.catalog.manage",
			"ordering.loyalty.view",
			"ordering.loyalty.change",
			"ordering.loyalty.manage",
			"ordering.analytics.view",
			"ordering.analytics.manage",
			"ordering.promotions.add",
			"ordering.promotions.view",
			"ordering.promotions.change",
			"ordering.promotions.delete",
			"ordering.promotions.manage",
			"ordering.delivery_zones.add",
			"ordering.delivery_zones.view",
			"ordering.delivery_zones.change",
			"ordering.delivery_zones.delete",
			"ordering.delivery_zones.manage",
			"ordering.config.view",
			"ordering.config.manage",
			"ordering.users.add",
			"ordering.users.view",
			"ordering.users.manage",
			"ordering.users.change_own",
			"ordering.config.change_own",
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

		update := tx.Role.UpdateOneID(code).ClearLegacyPermissions()
		if len(permIDs) > 0 {
			update = update.AddLegacyPermissionIDs(permIDs...)
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

// seedSuperAdmin is DEPRECATED — users are now synced from auth-api via JIT provisioning.
// This function is retained but no longer called from runSeed.
func seedSuperAdmin(ctx context.Context, tx *ent.Tx, tenantID, userID uuid.UUID, passwordHash string, now time.Time) error {
	const (
		email       = "admin@theurbanloftcafe.com"
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

	// Resolve email conflicts: if another user has this email, delete them.
	// This handles manual email swaps between runs.
	_, err := tx.User.Query().
		Where(user.EmailEQ(email), user.TenantIDEQ(tenantID), user.IDNEQ(userID)).
		Only(ctx)
	if err == nil {
		if _, delErr := tx.User.Delete().
			Where(user.EmailEQ(email), user.TenantIDEQ(tenantID), user.IDNEQ(userID)).
			Exec(ctx); delErr != nil {
			return fmt.Errorf("resolve email conflict for super admin: %w", delErr)
		}
	}

	_, err = tx.User.Get(ctx, userID)
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

func orderingPermissionUUID(code string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("bengobox:ordering:permission:"+code))
}

func orderingRoleUUID(tenantID uuid.UUID, code string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(fmt.Sprintf("bengobox:ordering:role:%s:%s", tenantID, code)))
}

// seedOrderingPermissions seeds all ordering permissions using the ordering.{module}.{action} code pattern.
func seedOrderingPermissions(ctx context.Context, tx *ent.Tx) (map[string]uuid.UUID, error) {
	type perm struct {
		code     string
		name     string
		mod      string
		action   string
		resource string
		desc     string
	}

	modules := []struct {
		name     string
		resource string
		actions  []struct{ code, name, desc string }
	}{
		{"orders", "orders", []struct{ code, name, desc string }{
			{"add", "Add orders", "Create new orders"},
			{"view", "View orders", "View any order in the tenant"},
			{"view_own", "View own orders", "View own orders only"},
			{"change", "Change orders", "Update any order"},
			{"change_own", "Change own orders", "Update own orders only"},
			{"delete", "Delete orders", "Cancel or delete orders"},
			{"delete_own", "Delete own orders", "Cancel own orders only"},
			{"manage", "Manage orders", "Full order management including status transitions"},
			{"manage_own", "Manage own orders", "Manage own orders only"},
		}},
		{"catalog", "catalog", []struct{ code, name, desc string }{
			{"add", "Add catalog items", "Create catalog items and categories"},
			{"view", "View catalog", "View catalog items and categories"},
			{"view_own", "View own catalog", "View catalog scoped to own outlet"},
			{"change", "Change catalog", "Edit catalog items and categories"},
			{"change_own", "Change own catalog", "Edit catalog scoped to own outlet"},
			{"delete", "Delete catalog items", "Remove catalog items and categories"},
			{"delete_own", "Delete own catalog items", "Remove catalog items scoped to own outlet"},
			{"manage", "Manage catalog", "Full catalog management"},
			{"manage_own", "Manage own catalog", "Manage catalog scoped to own outlet"},
		}},
		{"outlets", "outlets", []struct{ code, name, desc string }{
			{"add", "Add outlets", "Create new outlets"},
			{"view", "View outlets", "View outlets"},
			{"view_own", "View own outlets", "View outlets assigned to self"},
			{"change", "Change outlets", "Edit outlet details"},
			{"change_own", "Change own outlets", "Edit own outlet details"},
			{"delete", "Delete outlets", "Remove outlets"},
			{"delete_own", "Delete own outlets", "Remove own outlets"},
			{"manage", "Manage outlets", "Full outlet management"},
			{"manage_own", "Manage own outlets", "Manage own outlets"},
		}},
		{"promotions", "promotions", []struct{ code, name, desc string }{
			{"add", "Add promotions", "Create promo codes and campaigns"},
			{"view", "View promotions", "View promotions and redemptions"},
			{"view_own", "View own promotions", "View own promotions"},
			{"change", "Change promotions", "Edit promo codes"},
			{"change_own", "Change own promotions", "Edit own promo codes"},
			{"delete", "Delete promotions", "Remove promo codes"},
			{"delete_own", "Delete own promotions", "Remove own promo codes"},
			{"manage", "Manage promotions", "Full promotions management"},
			{"manage_own", "Manage own promotions", "Manage own promotions"},
		}},
		{"delivery_zones", "delivery_zones", []struct{ code, name, desc string }{
			{"add", "Add delivery zones", "Create delivery zones"},
			{"view", "View delivery zones", "View delivery zones"},
			{"view_own", "View own delivery zones", "View own delivery zones"},
			{"change", "Change delivery zones", "Edit delivery zones"},
			{"change_own", "Change own delivery zones", "Edit own delivery zones"},
			{"delete", "Delete delivery zones", "Remove delivery zones"},
			{"delete_own", "Delete own delivery zones", "Remove own delivery zones"},
			{"manage", "Manage delivery zones", "Full delivery zone management"},
			{"manage_own", "Manage own delivery zones", "Manage own delivery zones"},
		}},
		{"delivery_windows", "delivery_windows", []struct{ code, name, desc string }{
			{"add", "Add delivery windows", "Create delivery windows"},
			{"view", "View delivery windows", "View delivery windows"},
			{"view_own", "View own delivery windows", "View own delivery windows"},
			{"change", "Change delivery windows", "Edit delivery windows"},
			{"change_own", "Change own delivery windows", "Edit own delivery windows"},
			{"delete", "Delete delivery windows", "Remove delivery windows"},
			{"delete_own", "Delete own delivery windows", "Remove own delivery windows"},
			{"manage", "Manage delivery windows", "Full delivery window management"},
			{"manage_own", "Manage own delivery windows", "Manage own delivery windows"},
		}},
		{"loyalty", "loyalty", []struct{ code, name, desc string }{
			{"add", "Add loyalty programs", "Create loyalty programs"},
			{"view", "View loyalty", "View loyalty points and history"},
			{"view_own", "View own loyalty", "View own loyalty balance"},
			{"change", "Change loyalty", "Edit loyalty programs"},
			{"change_own", "Change own loyalty", "Edit own loyalty"},
			{"delete", "Delete loyalty programs", "Remove loyalty programs"},
			{"delete_own", "Delete own loyalty programs", "Remove own loyalty programs"},
			{"manage", "Manage loyalty", "Full loyalty management"},
			{"manage_own", "Manage own loyalty", "Manage own loyalty"},
		}},
		{"analytics", "analytics", []struct{ code, name, desc string }{
			{"add", "Add analytics reports", "Create analytics reports"},
			{"view", "View analytics", "Access dashboards and reporting"},
			{"view_own", "View own analytics", "View own analytics"},
			{"change", "Change analytics", "Edit analytics settings"},
			{"change_own", "Change own analytics", "Edit own analytics"},
			{"delete", "Delete analytics reports", "Remove analytics reports"},
			{"delete_own", "Delete own analytics reports", "Remove own analytics reports"},
			{"manage", "Manage analytics", "Full analytics management"},
			{"manage_own", "Manage own analytics", "Manage own analytics"},
		}},
		{"config", "config", []struct{ code, name, desc string }{
			{"add", "Add config", "Create configuration entries"},
			{"view", "View config", "View service configuration"},
			{"view_own", "View own config", "View own configuration"},
			{"change", "Change config", "Edit configuration"},
			{"change_own", "Change own config", "Edit own configuration"},
			{"delete", "Delete config", "Remove configuration entries"},
			{"delete_own", "Delete own config", "Remove own configuration entries"},
			{"manage", "Manage config", "Full configuration management"},
			{"manage_own", "Manage own config", "Manage own configuration"},
		}},
		{"users", "users", []struct{ code, name, desc string }{
			{"add", "Add users", "Create new users"},
			{"view", "View users", "View user profiles"},
			{"view_own", "View own profile", "View own profile"},
			{"change", "Change users", "Edit user details"},
			{"change_own", "Change own profile", "Edit own profile"},
			{"delete", "Delete users", "Remove users"},
			{"delete_own", "Delete own profile", "Remove own profile"},
			{"manage", "Manage users", "Full user management including roles"},
			{"manage_own", "Manage own profile", "Manage own profile"},
		}},
	}

	permIDs := make(map[string]uuid.UUID, 100)
	for _, m := range modules {
		for _, a := range m.actions {
			code := fmt.Sprintf("ordering.%s.%s", m.name, a.code)
			id := orderingPermissionUUID(code)
			permIDs[code] = id

			exists, err := tx.OrderingPermission.Query().
				Where(orderingpermission.PermissionCode(code)).
				Exist(ctx)
			if err != nil {
				return nil, fmt.Errorf("check ordering permission %s: %w", code, err)
			}
			if exists {
				_, updateErr := tx.OrderingPermission.Update().
					Where(orderingpermission.PermissionCode(code)).
					SetName(a.name).
					SetModule(m.name).
					SetAction(a.code).
					SetResource(m.resource).
					SetDescription(a.desc).
					Save(ctx)
				if updateErr != nil {
					return nil, fmt.Errorf("update ordering permission %s: %w", code, updateErr)
				}
			} else {
				_, createErr := tx.OrderingPermission.Create().
					SetID(id).
					SetPermissionCode(code).
					SetName(a.name).
					SetModule(m.name).
					SetAction(a.code).
					SetResource(m.resource).
					SetDescription(a.desc).
					Save(ctx)
				if createErr != nil {
					return nil, fmt.Errorf("seed ordering permission %s: %w", code, createErr)
				}
			}
		}
	}

	log.Printf("  + Seeded %d ordering permissions", len(permIDs))
	return permIDs, nil
}

// seedOrderingRoles seeds the ordering RBAC roles and assigns permissions via the RolePermission junction table.
func seedOrderingRoles(ctx context.Context, tx *ent.Tx, tenantID uuid.UUID, permMap map[string]uuid.UUID) error {
	type roleSpec struct {
		code  string
		name  string
		desc  string
		perms []string
	}

	allPerms := func(modules ...string) []string {
		var perms []string
		actions := []string{"add", "view", "view_own", "change", "change_own", "delete", "delete_own", "manage", "manage_own"}
		for _, mod := range modules {
			for _, act := range actions {
				perms = append(perms, fmt.Sprintf("ordering.%s.%s", mod, act))
			}
		}
		return perms
	}

	allModules := []string{"orders", "catalog", "outlets", "promotions", "delivery_zones", "delivery_windows", "loyalty", "analytics", "config", "users"}

	roles := []roleSpec{
		{
			code:  "admin",
			name:  "Ordering Admin",
			desc:  "Full tenant administrator with access to all ordering modules",
			perms: allPerms(allModules...),
		},
		{
			code: "manager",
			name: "Store Manager",
			desc: "Manages a store/outlet including orders, catalog, and staff",
			perms: append(
				allPerms("orders", "catalog", "outlets", "promotions", "delivery_zones", "delivery_windows", "loyalty"),
				[]string{
					"ordering.analytics.view", "ordering.analytics.view_own",
					"ordering.users.view", "ordering.users.view_own", "ordering.users.change_own",
					"ordering.config.view",
				}...),
		},
		{
			code: "kitchen",
			name: "Kitchen Staff",
			desc: "Kitchen/preparation staff who manage order preparation",
			perms: []string{
				"ordering.orders.view", "ordering.orders.change", "ordering.orders.manage",
				"ordering.catalog.view",
				"ordering.users.view_own", "ordering.users.change_own",
			},
		},
		{
			code: "cashier",
			name: "Cashier",
			desc: "Point of sale operator handling orders and payments",
			perms: []string{
				"ordering.orders.add", "ordering.orders.view", "ordering.orders.change", "ordering.orders.manage",
				"ordering.catalog.view",
				"ordering.promotions.view",
				"ordering.loyalty.view", "ordering.loyalty.manage",
				"ordering.users.view_own", "ordering.users.change_own",
			},
		},
		{
			code: "delivery_coordinator",
			name: "Delivery Coordinator",
			desc: "Manages delivery zones, windows, and order dispatch",
			perms: []string{
				"ordering.orders.view", "ordering.orders.change",
				"ordering.delivery_zones.view", "ordering.delivery_zones.change", "ordering.delivery_zones.manage",
				"ordering.delivery_windows.view", "ordering.delivery_windows.change", "ordering.delivery_windows.manage",
				"ordering.outlets.view",
				"ordering.users.view_own", "ordering.users.change_own",
			},
		},
		{
			code: "viewer",
			name: "Viewer",
			desc: "Read-only access to ordering data",
			perms: []string{
				"ordering.orders.view",
				"ordering.catalog.view",
				"ordering.outlets.view",
				"ordering.promotions.view",
				"ordering.delivery_zones.view",
				"ordering.delivery_windows.view",
				"ordering.loyalty.view",
				"ordering.analytics.view",
				"ordering.config.view",
				"ordering.users.view_own",
			},
		},
	}

	for _, r := range roles {
		roleID := orderingRoleUUID(tenantID, r.code)

		exists, err := tx.OrderingRole.Query().
			Where(
				orderingrole.TenantID(tenantID),
				orderingrole.RoleCode(r.code),
			).Exist(ctx)
		if err != nil {
			return fmt.Errorf("check ordering role %s: %w", r.code, err)
		}

		if !exists {
			_, createErr := tx.OrderingRole.Create().
				SetID(roleID).
				SetTenantID(tenantID).
				SetRoleCode(r.code).
				SetName(r.name).
				SetDescription(r.desc).
				SetIsSystemRole(true).
				Save(ctx)
			if createErr != nil {
				return fmt.Errorf("seed ordering role %s: %w", r.code, createErr)
			}
		} else {
			// Update existing role
			_, updateErr := tx.OrderingRole.Update().
				Where(
					orderingrole.TenantID(tenantID),
					orderingrole.RoleCode(r.code),
				).
				SetName(r.name).
				SetDescription(r.desc).
				SetIsSystemRole(true).
				Save(ctx)
			if updateErr != nil {
				return fmt.Errorf("update ordering role %s: %w", r.code, updateErr)
			}
			// Re-fetch the actual ID for permission assignment
			existingRole, getErr := tx.OrderingRole.Query().
				Where(
					orderingrole.TenantID(tenantID),
					orderingrole.RoleCode(r.code),
				).Only(ctx)
			if getErr != nil {
				return fmt.Errorf("get ordering role %s: %w", r.code, getErr)
			}
			roleID = existingRole.ID
		}

		// Clear existing role-permission mappings and re-assign
		_, _ = tx.RolePermission.Delete().
			Where(rolepermission.RoleID(roleID)).
			Exec(ctx)

		for _, permCode := range r.perms {
			permID, ok := permMap[permCode]
			if !ok {
				log.Printf("  [WARN] permission %s not found for role %s, skipping", permCode, r.code)
				continue
			}
			_, err := tx.RolePermission.Create().
				SetRoleID(roleID).
				SetPermissionID(permID).
				Save(ctx)
			if err != nil {
				if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "UNIQUE constraint") {
					continue
				}
				return fmt.Errorf("assign permission %s to role %s: %w", permCode, r.code, err)
			}
		}
	}

	log.Printf("  + Seeded %d ordering roles with permission assignments", len(roles))
	return nil
}

// seedRateLimitConfigs seeds rate limiting configurations.
func seedRateLimitConfigs(ctx context.Context, tx *ent.Tx) error {
	type rlConfig struct {
		serviceName     string
		keyType         string
		endpointPattern string
		requestsPerWin  int
		windowSecs      int
		burstMultiplier float64
		desc            string
	}

	configs := []rlConfig{
		{"ordering-backend", "ip", "*", 120, 60, 1.5, "Default IP rate limit: 120 req/min"},
		{"ordering-backend", "tenant", "*", 600, 60, 2.0, "Default tenant rate limit: 600 req/min"},
		{"ordering-backend", "user", "*", 60, 60, 1.5, "Default user rate limit: 60 req/min"},
		{"ordering-backend", "endpoint", "/api/v1/*/orders", 30, 60, 1.0, "Order creation endpoint rate limit: 30 req/min"},
		{"ordering-backend", "endpoint", "/api/v1/*/auth/*", 10, 60, 1.0, "Auth endpoint rate limit: 10 req/min"},
		{"ordering-backend", "endpoint", "/api/v1/*/checkout/*", 20, 60, 1.0, "Checkout endpoint rate limit: 20 req/min"},
		{"ordering-backend", "global", "*", 5000, 60, 2.0, "Global rate limit: 5000 req/min"},
	}

	for _, c := range configs {
		exists, err := tx.RateLimitConfig.Query().
			Where(
				ratelimitconfig.ServiceName(c.serviceName),
				ratelimitconfig.KeyType(c.keyType),
				ratelimitconfig.EndpointPattern(c.endpointPattern),
			).Exist(ctx)
		if err != nil {
			return fmt.Errorf("check rate limit config: %w", err)
		}
		if exists {
			_, updateErr := tx.RateLimitConfig.Update().
				Where(
					ratelimitconfig.ServiceName(c.serviceName),
					ratelimitconfig.KeyType(c.keyType),
					ratelimitconfig.EndpointPattern(c.endpointPattern),
				).
				SetRequestsPerWindow(c.requestsPerWin).
				SetWindowSeconds(c.windowSecs).
				SetBurstMultiplier(c.burstMultiplier).
				SetDescription(c.desc).
				SetIsActive(true).
				Save(ctx)
			if updateErr != nil {
				return fmt.Errorf("update rate limit config: %w", updateErr)
			}
		} else {
			_, createErr := tx.RateLimitConfig.Create().
				SetServiceName(c.serviceName).
				SetKeyType(c.keyType).
				SetEndpointPattern(c.endpointPattern).
				SetRequestsPerWindow(c.requestsPerWin).
				SetWindowSeconds(c.windowSecs).
				SetBurstMultiplier(c.burstMultiplier).
				SetDescription(c.desc).
				SetIsActive(true).
				Save(ctx)
			if createErr != nil {
				return fmt.Errorf("seed rate limit config: %w", createErr)
			}
		}
	}

	log.Printf("  + Seeded %d rate limit configs", len(configs))
	return nil
}

// seedServiceConfigs seeds service-level configuration entries.
func seedServiceConfigs(ctx context.Context, tx *ent.Tx) error {
	type svcConfig struct {
		key        string
		value      string
		configType string
		desc       string
		isSecret   bool
	}

	configs := []svcConfig{
		{"ordering.max_order_amount", "500000", "int", "Maximum order amount in cents", false},
		{"ordering.min_order_amount", "500", "int", "Minimum order amount in cents", false},
		{"ordering.default_currency", "KES", "string", "Default currency for orders", false},
		{"ordering.order_expiry_minutes", "30", "int", "Minutes before an unpaid order expires", false},
		{"ordering.max_cart_items", "50", "int", "Maximum number of items in a cart", false},
		{"ordering.loyalty_points_per_unit", "1", "int", "Loyalty points earned per currency unit spent", false},
		{"ordering.loyalty_redemption_rate", "0.01", "float", "Currency value per loyalty point", false},
		{"ordering.catalog_sync_interval_minutes", "15", "int", "Interval for syncing catalog from inventory-api", false},
		{"ordering.default_delivery_radius_km", "10", "float", "Default delivery radius in kilometers", false},
		{"ordering.max_delivery_radius_km", "50", "float", "Maximum delivery radius in kilometers", false},
		{"ordering.enable_guest_checkout", "true", "bool", "Allow checkout without authentication", false},
		{"ordering.enable_promo_stacking", "false", "bool", "Allow multiple promo codes per order", false},
	}

	for _, c := range configs {
		exists, err := tx.ServiceConfig.Query().
			Where(
				serviceconfig.ConfigKey(c.key),
				serviceconfig.TenantIDIsNil(),
			).Exist(ctx)
		if err != nil {
			return fmt.Errorf("check service config %s: %w", c.key, err)
		}
		if exists {
			_, updateErr := tx.ServiceConfig.Update().
				Where(
					serviceconfig.ConfigKey(c.key),
					serviceconfig.TenantIDIsNil(),
				).
				SetConfigValue(c.value).
				SetConfigType(c.configType).
				SetDescription(c.desc).
				SetIsSecret(c.isSecret).
				Save(ctx)
			if updateErr != nil {
				return fmt.Errorf("update service config %s: %w", c.key, updateErr)
			}
		} else {
			_, createErr := tx.ServiceConfig.Create().
				SetConfigKey(c.key).
				SetConfigValue(c.value).
				SetConfigType(c.configType).
				SetDescription(c.desc).
				SetIsSecret(c.isSecret).
				Save(ctx)
			if createErr != nil {
				return fmt.Errorf("seed service config %s: %w", c.key, createErr)
			}
		}
	}

	log.Printf("  + Seeded %d service configs", len(configs))
	return nil
}

// --- Catalog Seeding ---

func seedCatalog(ctx context.Context, tx *ent.Tx, tenantID uuid.UUID) error {
	outletID, err := seedOutlet(ctx, tx, tenantID)
	if err != nil {
		return fmt.Errorf("seed outlet: %w", err)
	}

	if err := seedCatalogOverrides(ctx, tx, tenantID, outletID); err != nil {
		return fmt.Errorf("seed catalog overrides: %w", err)
	}

	log.Println("  ✓ Catalog seeded (outlet, catalog overrides)")
	return nil
}

func seedOutlet(ctx context.Context, tx *ent.Tx, tenantID uuid.UUID) (uuid.UUID, error) {
	outletID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("bengobox:cafe:outlet:urban-loft:busia"))

	existing, err := tx.Outlet.Query().Where(outlet.ID(outletID)).Only(ctx)
	if err == nil {
		// Update existing outlet with coordinates and pickup support
		_, updErr := tx.Outlet.UpdateOneID(existing.ID).
			SetLatitude(0.4612).
			SetLongitude(34.1109).
			SetSupportsPickup(true).
			Save(ctx)
		if updErr != nil {
			return uuid.Nil, updErr
		}
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
		SetEmail("info@theurbanloftcafe.com").
		SetLocation("Busia, Kenya").
		SetImageURL("/media/images/outlets/urban-loft-busia.jpeg").
		SetUseCase("hospitality").
		SetLatitude(0.4612).
		SetLongitude(34.1109).
		SetSupportsPickup(true).
		SetStatus("active").
		SetOpeningHours(map[string]any{
			"monday":    map[string]any{"open": "07:00", "close": "22:00"},
			"tuesday":   map[string]any{"open": "07:00", "close": "22:00"},
			"wednesday": map[string]any{"open": "07:00", "close": "22:00"},
			"thursday":  map[string]any{"open": "07:00", "close": "22:00"},
			"friday":    map[string]any{"open": "07:00", "close": "23:00"},
			"saturday":  map[string]any{"open": "08:00", "close": "23:00"},
			"sunday":    map[string]any{"open": "08:00", "close": "21:00"},
		}).
		Save(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	return o.ID, nil
}

// seedCatalogOverrides creates per-outlet pricing overrides for inventory-api items.
// Master data (name, description, image, category) comes from inventory-api.
// This seeds ONLY ordering-specific fields: pricing, availability, featured status, etc.
// Uses the same SKUs as inventory-api so items are linked via inventory_sku.
func seedCatalogOverrides(ctx context.Context, tx *ent.Tx, tenantID, outletID uuid.UUID) error {
	type override struct {
		sku            string
		price          float64 // KES — ordering-specific pricing
		featured       bool
		leadTime       int    // prep time in minutes
		displaySection string // featured, new, top_rated, stores, default
		imageOverride  string // optional image override (empty = use inventory image)
	}
	// Overrides aligned with inventory-api item SKUs.
	// Name, description, category, and image come from inventory-api master data.
	overrides := []override{
		// Hot Beverages
		{"BEV-ESP-001", 250, false, 3, "default", ""},
		{"BEV-ESP-002", 300, false, 3, "default", ""},
		{"BEV-LAT-001", 350, false, 4, "default", ""},
		{"BEV-CAP-001", 350, true, 4, "featured", ""},
		{"BEV-AME-001", 280, false, 3, "default", ""},
		{"BEV-MOC-001", 400, false, 5, "default", ""},
		{"BEV-MAC-001", 300, false, 3, "default", ""},
		{"BEV-TEA-001", 200, false, 4, "default", ""},
		{"BEV-TEA-002", 250, true, 5, "featured", ""},
		{"BEV-HOT-001", 400, false, 5, "default", ""},

		// Cold Beverages
		{"BEV-ICE-001", 350, false, 4, "default", ""},
		{"BEV-ICE-002", 300, false, 3, "default", ""},
		{"BEV-FRP-001", 450, true, 5, "featured", ""},
		{"BEV-FRP-002", 450, false, 5, "default", ""},
		{"BEV-SMO-001", 400, true, 5, "featured", ""},
		{"BEV-SMO-002", 400, false, 5, "default", ""},
		{"BEV-JCE-001", 350, false, 5, "default", ""},

		// Pastries & Bakery
		{"PST-CRO-001", 200, false, 2, "default", ""},
		{"PST-CRO-002", 250, true, 2, "new", ""},
		{"PST-MUF-001", 220, false, 2, "default", ""},
		{"PST-MUF-002", 220, false, 2, "default", ""},
		{"PST-CKE-001", 350, false, 2, "default", ""},
		{"PST-CKE-002", 380, false, 2, "default", ""},
		{"PST-CKE-003", 380, false, 2, "default", ""},
		{"PST-DAN-001", 250, false, 2, "default", ""},
		{"PST-SCO-001", 200, false, 2, "default", ""},

		// Sandwiches & Wraps
		{"SND-CLB-001", 650, false, 12, "default", ""},
		{"SND-GRL-001", 600, true, 10, "top_rated", ""},
		{"SND-VEG-001", 500, false, 8, "default", ""},
		{"SND-BLT-001", 550, false, 10, "default", ""},
		{"SND-TUN-001", 550, false, 10, "default", ""},

		// Salads
		{"SAL-CES-001", 500, false, 8, "default", ""},
		{"SAL-GRK-001", 500, false, 8, "default", ""},

		// Light Bites
		{"BTE-SAM-001", 300, false, 10, "default", ""},
		{"BTE-SPR-001", 350, false, 10, "default", ""},

		// Main Courses
		{"MIN-GRL-001", 1200, true, 30, "featured", ""},
		{"MIN-GRL-002", 950, true, 25, "featured", ""},
		{"MIN-CUR-001", 850, false, 25, "default", ""},
		{"MIN-CUR-002", 800, false, 30, "default", ""},
		{"MIN-SEA-001", 850, false, 20, "default", ""},
		{"MIN-PAS-001", 750, false, 20, "default", ""},
		{"MIN-RIC-001", 700, false, 20, "default", ""},

		// Breakfast
		{"BRK-FUL-001", 850, true, 15, "featured", ""},
		{"BRK-PAN-001", 650, false, 12, "default", ""},
		{"BRK-AVT-001", 550, false, 10, "new", ""},
		{"BRK-OAT-001", 400, false, 2, "default", ""},

		// Pizza
		{"PIZ-MAR-001", 750, false, 20, "default", ""},
		{"PIZ-PEP-001", 850, true, 20, "top_rated", ""},
	}

	for i, o := range overrides {
		// Use deterministic UUID for the catalog override
		overrideID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("bengobox:cafe:override:urban-loft:"+o.sku))

		existing, err := tx.CatalogOverride.Query().Where(catalogoverride.ID(overrideID)).Only(ctx)
		if err == nil {
			// Update in case fields changed
			upd := tx.CatalogOverride.UpdateOneID(existing.ID).
				SetBasePrice(o.price).
				SetIsFeatured(o.featured).
				SetLeadTimeMinutes(o.leadTime).
				SetDisplayOrder(i + 1).
				SetDisplaySection(o.displaySection)
			if o.imageOverride != "" {
				upd = upd.SetImageURLOverride(o.imageOverride)
			}
			_, _ = upd.Save(ctx)
			continue
		}
		if !ent.IsNotFound(err) {
			return err
		}

		cr := tx.CatalogOverride.Create().
			SetID(overrideID).
			SetTenantID(tenantID).
			SetOutletID(outletID).
			SetInventorySku(o.sku).
			SetBasePrice(o.price).
			SetCurrency("KES").
			SetIsAvailable(true).
			SetIsFeatured(o.featured).
			SetLeadTimeMinutes(o.leadTime).
			SetDisplayOrder(i + 1).
			SetDisplaySection(o.displaySection)
		if o.imageOverride != "" {
			cr = cr.SetImageURLOverride(o.imageOverride)
		}
		_, err = cr.Save(ctx)
		if err != nil {
			return fmt.Errorf("create catalog override for %s: %w", o.sku, err)
		}
	}
	return nil
}
