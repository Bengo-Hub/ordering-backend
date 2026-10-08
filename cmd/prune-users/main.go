// Command prune-users removes the ordering users created before auth.user events were gated by
// shared UserRelevance (2026-10-08). Back then every member of every tenant got an ordering user.
// Real customers live here, so it only touches tenants that have never used ordering (no outlet
// mirrored and no order), and there only users who never signed in to ordering and have no cart,
// address or loyalty account. Anyone removed by mistake comes back on first sign-in.
//
// It reports counts per tenant and changes nothing unless run with --apply. The delete runs in one
// transaction, so a row still referenced elsewhere rolls the whole run back.
package main

import (
	"context"
	"database/sql"
	"flag"
	"log"
	"os"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/bengobox/ordering-backend/internal/ent"
	"github.com/bengobox/ordering-backend/internal/ent/order"
	"github.com/bengobox/ordering-backend/internal/ent/outlet"
	enttenant "github.com/bengobox/ordering-backend/internal/ent/tenant"
	"github.com/bengobox/ordering-backend/internal/ent/user"
	"github.com/bengobox/ordering-backend/internal/ent/userpreference"
	"github.com/bengobox/ordering-backend/internal/ent/userprofile"
	"github.com/bengobox/ordering-backend/internal/ent/userroleassignment"
)

func main() {
	apply := flag.Bool("apply", false, "delete the rows (default reports only)")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	dsn := os.Getenv("POSTGRES_MIGRATE_URL")
	if dsn == "" {
		dsn = os.Getenv("POSTGRES_URL")
	}
	if dsn == "" {
		log.Fatal("POSTGRES_URL is not set")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	defer client.Close()

	candidates, err := client.User.Query().
		Where(
			user.LastLoginAtIsNil(),
			user.Not(user.HasOrders()),
			user.Not(user.HasCarts()),
			user.Not(user.HasAddresses()),
			user.Not(user.HasLoyaltyAccount()),
		).
		Select(user.FieldID, user.FieldTenantID).
		All(ctx)
	if err != nil {
		log.Fatalf("load users: %v", err)
	}

	// unused caches whether a tenant has never used ordering.
	unused := map[uuid.UUID]bool{}
	isUnused := func(tid uuid.UUID) bool {
		if v, ok := unused[tid]; ok {
			return v
		}
		hasOutlet, err := client.Outlet.Query().Where(outlet.TenantID(tid)).Exist(ctx)
		if err != nil {
			log.Fatalf("check outlets: %v", err)
		}
		hasOrder, err := client.Order.Query().Where(order.TenantID(tid)).Exist(ctx)
		if err != nil {
			log.Fatalf("check orders: %v", err)
		}
		unused[tid] = !hasOutlet && !hasOrder
		return unused[tid]
	}

	byTenant := map[uuid.UUID]int{}
	var ids []uuid.UUID
	for _, u := range candidates {
		if !isUnused(u.TenantID) {
			continue
		}
		byTenant[u.TenantID]++
		ids = append(ids, u.ID)
	}
	for tid, n := range byTenant {
		slug := "?"
		if t, err := client.Tenant.Query().Where(enttenant.ID(tid)).Only(ctx); err == nil {
			slug = t.Slug
		}
		log.Printf("tenant %-28s %s  removable: %d", slug, tid, n)
	}
	total, _ := client.User.Query().Count(ctx)
	log.Printf("users: %d total, %d removable, %d kept", total, len(ids), total-len(ids))

	if !*apply || len(ids) == 0 {
		log.Printf("dry run: nothing changed (pass --apply to delete)")
		return
	}

	tx, err := client.Tx(ctx)
	if err != nil {
		log.Fatalf("begin: %v", err)
	}
	fail := func(step string, err error) {
		_ = tx.Rollback()
		log.Fatalf("%s: %v (rolled back, nothing deleted)", step, err)
	}
	if _, err := tx.UserRoleAssignment.Delete().Where(userroleassignment.UserIDIn(ids...)).Exec(ctx); err != nil {
		fail("delete role assignments", err)
	}
	if _, err := tx.UserPreference.Delete().Where(userpreference.HasUserWith(user.IDIn(ids...))).Exec(ctx); err != nil {
		fail("delete preferences", err)
	}
	if _, err := tx.UserProfile.Delete().Where(userprofile.HasUserWith(user.IDIn(ids...))).Exec(ctx); err != nil {
		fail("delete profiles", err)
	}
	n, err := tx.User.Delete().Where(user.IDIn(ids...)).Exec(ctx)
	if err != nil {
		fail("delete users", err)
	}
	if err := tx.Commit(); err != nil {
		log.Fatalf("commit: %v", err)
	}
	log.Printf("deleted %d users", n)
}
