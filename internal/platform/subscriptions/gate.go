package subscriptions

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	authclient "github.com/Bengo-Hub/shared-auth-client"
)

const (
	gracePeriodDays = 7
	upgradeURL      = "/subscription/upgrade"
)

// SubscriptionGate returns middleware that enforces subscription status on every request.
//
// Pass-through conditions (in priority order):
//   - Platform owner (is_platform_owner) → always allowed
//   - ACTIVE or TRIAL → allowed
//   - EXPIRED within gracePeriodDays → allowed, sets X-Sub-Grace-Days-Left header
//
// Blocked: EXPIRED beyond grace, CANCELLED, SUSPENDED → 402.
// Routes without JWT claims (public routes) pass through unchanged.
func SubscriptionGate() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := authclient.ClaimsFromContext(r.Context())
			if !ok {
				next.ServeHTTP(w, r)
				return
			}

			if claims.IsPlatformOwner || claims.IsSuperuser() {
				next.ServeHTTP(w, r)
				return
			}

			// Service-charge tenants pay per transaction; demo tenants bypass for demos.
			if claims.BillingMode == "service_charge" || claims.IsDemo {
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
// include featureCode. Platform owners and superusers bypass the check.
// Returns 403 with a structured JSON body on failure.
func RequireFeature(featureCode string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := authclient.ClaimsFromContext(r.Context())
			if !ok {
				next.ServeHTTP(w, r)
				return
			}

			if claims.IsPlatformOwner || claims.IsSuperuser() {
				next.ServeHTTP(w, r)
				return
			}

			// Demo and service-charge tenants get all features.
			if claims.IsDemo || claims.BillingMode == "service_charge" {
				next.ServeHTTP(w, r)
				return
			}

			if !claims.HasFeature(featureCode) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error":            "feature_not_available",
					"required_feature": featureCode,
					"upgrade_url":      upgradeURL,
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
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
	limit := claims.GetLimit(limitKey)
	if limit == 0 {
		return true
	}
	return currentValue < limit
}

// AssertLimit writes a 402 response if currentValue has reached or exceeded the limit
// for limitKey. Returns true when the limit is exceeded (caller should stop processing).
func AssertLimit(w http.ResponseWriter, r *http.Request, limitKey string, currentValue int) bool {
	if CheckLimit(r, limitKey, currentValue) {
		return false
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusPaymentRequired)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":       "limit_exceeded",
		"limit_key":   limitKey,
		"upgrade_url": upgradeURL,
	})
	return true
}

func writeSubscriptionError(w http.ResponseWriter, gracePeriodEnded bool) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusPaymentRequired)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":              "subscription_expired",
		"grace_period_ended": gracePeriodEnded,
		"upgrade_url":        upgradeURL,
	})
}
