package subscriptions

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Bengo-Hub/httpware"
	authclient "github.com/Bengo-Hub/shared-auth-client"
)

// A shopper cannot reach the store of a tenant whose plan has no ordering. Signed-in and service
// callers are left to RequireServiceAccess, and a request with no tenant is not this gate's business.
func TestRequireGuestEntitlement(t *testing.T) {
	t.Cleanup(func() { guestEntitlement.Store(nil) })
	SetGuestEntitlement(func(_ context.Context, tenantID string) bool { return tenantID == "paid" })

	cases := []struct {
		name   string
		tenant string
		claims bool
		apiKey string
		want   int
	}{
		{"shopper, plan has ordering", "paid", false, "", http.StatusOK},
		{"shopper, plan has no ordering", "tier1", false, "", http.StatusForbidden},
		{"signed in user", "tier1", true, "", http.StatusOK},
		{"service caller", "tier1", false, "key", http.StatusOK},
		{"no tenant resolved", "", false, "", http.StatusOK},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/catalog", nil)
			ctx := r.Context()
			if c.tenant != "" {
				ctx = httpware.WithTenantID(ctx, c.tenant)
			}
			if c.claims {
				ctx = authclient.ContextWithClaims(ctx, &authclient.Claims{TenantID: c.tenant})
			}
			if c.apiKey != "" {
				r.Header.Set("X-API-Key", c.apiKey)
			}
			w := httptest.NewRecorder()
			RequireGuestEntitlement()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})).ServeHTTP(w, r.WithContext(ctx))
			if w.Code != c.want {
				t.Fatalf("got %d, want %d", w.Code, c.want)
			}
		})
	}
}
