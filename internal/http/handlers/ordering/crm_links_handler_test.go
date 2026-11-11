package orderinghandler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Bengo-Hub/httpware"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// fakeCRMLinkStore holds tenant slugs and the contact ids that orders (in any tenant) link.
type fakeCRMLinkStore struct {
	slugs map[string]uuid.UUID
	links map[uuid.UUID]bool
}

// LocalTenant knows only the tenants in slugs, by slug or by id, like the database store.
func (f *fakeCRMLinkStore) LocalTenant(_ context.Context, uuidOrSlug string) (uuid.UUID, bool, error) {
	if id, ok := f.slugs[uuidOrSlug]; ok {
		return id, true, nil
	}
	for _, id := range f.slugs {
		if id.String() == uuidOrSlug {
			return id, true, nil
		}
	}
	return uuid.Nil, false, nil
}

func (f *fakeCRMLinkStore) ContactReferenced(_ context.Context, contactID uuid.UUID) (bool, error) {
	return f.links[contactID], nil
}

// marketflow asks, before erasing a CRM contact, whether ordering still links it. An order carrying
// it answers yes whichever known tenant asks; no order answers no. {tenant} may be the UUID or the
// slug and must be known, and the route needs the service key.
func TestCRMLinksAnswersPerTenant(t *testing.T) {
	tenantA, tenantB := uuid.New(), uuid.New()
	linked, unlinked := uuid.New(), uuid.New()
	store := &fakeCRMLinkStore{
		slugs: map[string]uuid.UUID{"tenant-a": tenantA, "tenant-b": tenantB},
		links: map[uuid.UUID]bool{linked: true},
	}

	const key = "test-service-key"
	r := chi.NewRouter()
	r.With(httpware.RequireServiceKey(key)).
		Get("/api/v1/s2s/{tenant}/crm-links/{contactID}", NewCRMLinksHandler(store, zap.NewNop()).Referenced)

	ask := func(tenant, contact string, withKey bool) (int, map[string]any) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/s2s/"+tenant+"/crm-links/"+contact, nil)
		if withKey {
			req.Header.Set("X-API-Key", key)
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		var body map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		return rec.Code, body
	}

	if code, body := ask(tenantA.String(), linked.String(), true); code != http.StatusOK || body["referenced"] != true || len(body) != 1 {
		t.Fatalf("linked contact by tenant id: %d %v", code, body)
	}
	if code, body := ask("tenant-a", linked.String(), true); code != http.StatusOK || body["referenced"] != true {
		t.Fatalf("linked contact by slug: %d %v", code, body)
	}
	if code, body := ask(tenantB.String(), linked.String(), true); code != http.StatusOK || body["referenced"] != true {
		t.Fatalf("a link under another known tenant must count: %d %v", code, body)
	}
	if code, body := ask("tenant-b", linked.String(), true); code != http.StatusOK || body["referenced"] != true {
		t.Fatalf("a link under another known tenant (slug) must count: %d %v", code, body)
	}
	if code, body := ask(tenantA.String(), unlinked.String(), true); code != http.StatusOK || body["referenced"] != false {
		t.Fatalf("unlinked contact: %d %v", code, body)
	}
	if code, _ := ask(tenantA.String(), "not-a-uuid", true); code != http.StatusBadRequest {
		t.Fatalf("bad id: %d", code)
	}
	if code, _ := ask("no-such-tenant", linked.String(), true); code != http.StatusNotFound {
		t.Fatalf("unknown slug: %d", code)
	}
	// A tenant UUID ordering does not know (sync drift) cannot vouch for anything.
	if code, _ := ask(uuid.NewString(), linked.String(), true); code != http.StatusNotFound {
		t.Fatalf("unknown tenant id: %d", code)
	}
	if code, _ := ask(tenantA.String(), linked.String(), false); code != http.StatusUnauthorized {
		t.Fatalf("no key: %d", code)
	}
}
