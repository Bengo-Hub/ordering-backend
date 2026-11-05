package subscriptions

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	authclient "github.com/Bengo-Hub/shared-auth-client"
)

// A token minted before an upgrade must not keep refusing: the refusal asks the live plan first, and a
// failed lookup keeps the token's answer.
func TestCheckLimitConsultsTheLivePlan(t *testing.T) {
	t.Cleanup(func() { liveLimitLookup.Store(nil) })
	live, ok := 0, false
	SetLiveLimitLookup(func(context.Context, string, string) (int, bool) { return live, ok })
	r := httptest.NewRequest(http.MethodPost, "/outlets", nil)
	r = r.WithContext(authclient.ContextWithClaims(r.Context(), &authclient.Claims{TenantID: "t1", SubscriptionLimits: map[string]int{"max_outlets": 1}}))
	if CheckLimit(r, "max_outlets", 1) {
		t.Fatal("a failed lookup must keep the token's refusal")
	}
	live, ok = 3, true
	if !CheckLimit(r, "max_outlets", 1) {
		t.Fatal("an upgrade to 3 outlets must allow the second on a stale token")
	}
	if CheckLimit(r, "max_outlets", 3) {
		t.Fatal("the live cap still applies")
	}
}
