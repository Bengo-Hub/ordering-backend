package subscriptions

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/Bengo-Hub/httpware"
	authclient "github.com/Bengo-Hub/shared-auth-client"
)

// guestEntitlement answers whether a tenant's plan covers ordering (SetGuestEntitlement).
var guestEntitlement atomic.Pointer[func(ctx context.Context, tenantID string) bool]

// SetGuestEntitlement wires the plan check RequireGuestEntitlement uses.
func SetGuestEntitlement(fn func(ctx context.Context, tenantID string) bool) {
	guestEntitlement.Store(&fn)
}

// RequireGuestEntitlement closes the storefront to shoppers when the tenant's plan has no ordering.
// RequireServiceAccess only reads a token's claims, so public routes pass through it. Signed-in and
// service callers are left to that gate. The check caches per tenant, so shopper traffic stays cheap.
func RequireGuestEntitlement() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fn := guestEntitlement.Load()
			tenantID := httpware.GetTenantID(r.Context())
			_, signedIn := authclient.ClaimsFromContext(r.Context())
			if fn != nil && tenantID != "" && !signedIn && r.Header.Get("X-API-Key") == "" && !(*fn)(r.Context(), tenantID) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"code":    "service_not_in_plan",
					"error":   "service_not_in_plan",
					"message": "This store is not available.",
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

const (
	gracePeriodDays = 7
	upgradeURL      = "/subscription/upgrade"
)

// SubscriptionGate returns middleware that enforces subscription status on every request.
//
// Pass-through conditions (in priority order):
//   - Gating-exempt (platform owner, subscription-exempt tenant, demo, service-charge) → allowed
//   - ACTIVE or TRIAL → allowed
//   - EXPIRED within gracePeriodDays → allowed, sets X-Sub-Grace-Days-Left header
//
// Blocked: EXPIRED beyond grace, CANCELLED, SUSPENDED → 402.
// Routes without JWT claims (public routes) pass through unchanged.
//
// SEC-3 (auth-client v0.10.0): a tenant superuser is NOT exempt — only IsPlatformOwner,
// explicitly subscription-exempt tenants, demo and service-charge tenants bypass, all of
// which funnel through claims.IsGatingExempt().
func SubscriptionGate() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := authclient.ClaimsFromContext(r.Context())
			if !ok {
				next.ServeHTTP(w, r)
				return
			}

			if claims.IsGatingExempt() {
				next.ServeHTTP(w, r)
				return
			}

			switch claims.SubscriptionStatus {
			case "ACTIVE", "TRIAL", "":
				next.ServeHTTP(w, r)
				return
			case "EXPIRED":
				if claims.SubscriptionExpires != nil {
					expAt := claims.ExpiresAt()
					if expAt != nil {
						deadline := expAt.Add(gracePeriodDays * 24 * time.Hour)
						if time.Now().Before(deadline) {
							daysLeft := int(time.Until(deadline).Hours()/24) + 1
							w.Header().Set("X-Sub-Grace-Days-Left", fmt.Sprintf("%d", daysLeft))
							next.ServeHTTP(w, r)
							return
						}
					}
				}
				writeSubscriptionError(w, true)
				return
			default:
				// CANCELLED, SUSPENDED
				writeSubscriptionError(w, false)
				return
			}
		})
	}
}

// RequireFeature returns middleware that blocks requests when the subscription does not
// include featureCode. Gating-exempt tokens (platform owner, subscription-exempt tenant,
// demo, service-charge) bypass; a tenant superuser does NOT (SEC-3 / auth-client v0.10.0).
// Returns 403 with a structured JSON body on failure.
func RequireFeature(featureCode string) func(http.Handler) http.Handler {
	return authclient.RequireFeatureCode(featureCode)
}

// CheckLimit returns true when currentValue is within the limit set for limitKey in the
// subscription claims. Returns true (within limit) when:
//   - No claims are present in the context
//   - limitKey is absent from sub_limits (treat as unlimited)
//   - limit is 0 (treat as unlimited)
func CheckLimit(r *http.Request, limitKey string, currentValue int) bool {
	claims, ok := authclient.ClaimsFromContext(r.Context())
	if !ok {
		return true
	}
	if claims.IsGatingExempt() { // SEC-3: superuser is NOT exempt; platform owner / exempt / demo / service-charge are
		return true
	}
	limit := claims.GetLimit(limitKey)
	if limit <= 0 || currentValue < limit { // 0 = absent/unlimited, -1 = explicit unlimited
		return true
	}
	// Ask the live plan before refusing: an upgrade since the token was issued must count.
	if live, ok := liveLimit(r.Context(), claims.TenantID, limitKey); ok {
		return live <= 0 || currentValue < live
	}
	return false
}

// liveLimitLookup answers the tenant's current plan limit (SetLiveLimitLookup).
var liveLimitLookup atomic.Pointer[func(ctx context.Context, tenantID, limitKey string) (int, bool)]

// SetLiveLimitLookup wires the live plan limit consulted before a token-based refusal. Tokens can be
// hours old, so an upgrade would otherwise stay blocked until the next sign-in.
func SetLiveLimitLookup(fn func(ctx context.Context, tenantID, limitKey string) (int, bool)) {
	liveLimitLookup.Store(&fn)
}

func liveLimit(ctx context.Context, tenantID, limitKey string) (int, bool) {
	fn := liveLimitLookup.Load()
	if fn == nil || tenantID == "" {
		return 0, false
	}
	return (*fn)(ctx, tenantID, limitKey)
}

// AssertLimit writes a structured 402 response if currentValue has reached or exceeded the
// plan limit for limitKey. Returns true when the limit is exceeded (caller stops). The
// `metric` is the UI-facing name (e.g. "orders"); the body matches the shared limit-reached
// modal contract.
func AssertLimit(w http.ResponseWriter, r *http.Request, metric, limitKey string, currentValue int) bool {
	if CheckLimit(r, limitKey, currentValue) {
		return false
	}
	limit := 0
	if claims, ok := authclient.ClaimsFromContext(r.Context()); ok {
		limit = claims.GetLimit(limitKey)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusPaymentRequired)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"code":             "usage_limit_exceeded",
		"error":            "usage_limit_exceeded",
		"message":          "You've reached your plan's " + metric + " limit.",
		"metric":           metric,
		"limit":            limit,
		"used":             currentValue,
		"overage_eligible": false,
		"limit_key":        limitKey,
		"upgrade_url":      upgradeURL,
	})
	return true
}

func writeSubscriptionError(w http.ResponseWriter, gracePeriodEnded bool) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusPaymentRequired)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"code":               "subscription_expired", // canonical discriminator read by frontends
		"error":              "subscription_expired",
		"grace_period_ended": gracePeriodEnded,
		"upgrade":            true,
		"upgrade_url":        upgradeURL,
	})
}
