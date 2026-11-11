package orderinghandler

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/bengobox/ordering-backend/internal/ent"
)

// The database store behind the crm-links check: the tenant must have a local row (by id or slug)
// and an order carrying the contact id counts in any tenant. Runs against a local Postgres in its own schema; skipped
// when none is reachable (ordering has no other database test helper).
func TestEntCRMLinkStore(t *testing.T) {
	if os.Getenv("SKIP_POSTGRES_TESTS") != "" {
		t.Skip("SKIP_POSTGRES_TESTS set")
	}
	url := os.Getenv("POSTGRES_URL")
	if url == "" {
		url = "postgres://postgres:postgres@localhost:5432/ordering?sslmode=disable"
	}
	admin, err := sql.Open("pgx", url)
	if err != nil || admin.Ping() != nil {
		t.Skip("local postgres unavailable")
	}
	schema := "crmlinks_it_" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	if _, err := admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP SCHEMA " + schema + " CASCADE"); _ = admin.Close() })
	sep := "?"
	if strings.Contains(url, "?") {
		sep = "&"
	}
	db, err := sql.Open("pgx", url+sep+"search_path="+schema)
	if err != nil {
		t.Fatal(err)
	}
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("schema: %v", err)
	}

	tenantA, tenantB := uuid.New(), uuid.New()
	for _, id := range []uuid.UUID{tenantA, tenantB} {
		client.Tenant.Create().SetID(id).SetName("T " + id.String()[:6]).SetSlug("t-" + id.String()[:8]).ExecX(ctx)
	}
	outlet := client.Outlet.Create().SetTenantID(tenantA).SetName("Main").SetSlug("main").SaveX(ctx)
	linked := uuid.New()
	client.Order.Create().SetTenantID(tenantA).SetOutletID(outlet.ID).SetOrderNumber("ORD-1").
		SetSubtotal(100).SetGrandTotal(100).SetCrmContactID(linked).ExecX(ctx)

	store := NewEntCRMLinkStore(client)
	for _, in := range []string{tenantA.String(), "t-" + tenantA.String()[:8]} {
		if id, ok, err := store.LocalTenant(ctx, in); err != nil || !ok || id != tenantA {
			t.Fatalf("local tenant %q: %v %v %v", in, id, ok, err)
		}
	}
	for _, in := range []string{uuid.NewString(), "no-such-tenant"} {
		if _, ok, err := store.LocalTenant(ctx, in); err != nil || ok {
			t.Fatalf("unknown tenant %q must not be found: %v %v", in, ok, err)
		}
	}
	// The order sits under tenant A; the match is on the contact id in any tenant, so it counts
	// whichever known tenant (A or B) is asking.
	if ok, err := store.ContactReferenced(ctx, linked); err != nil || !ok {
		t.Fatalf("linked: %v %v", ok, err)
	}
	if id, ok, err := store.LocalTenant(ctx, tenantB.String()); err != nil || !ok || id != tenantB {
		t.Fatalf("tenant B must be known: %v %v %v", id, ok, err)
	}
	if ok, err := store.ContactReferenced(ctx, uuid.New()); err != nil || ok {
		t.Fatalf("unlinked: %v %v", ok, err)
	}
}
