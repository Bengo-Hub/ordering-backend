package subscriptions

import (
	"encoding/json"
	"net/http"

	authclient "github.com/Bengo-Hub/shared-auth-client"
)

// RequireUseCase gates a route to requests whose JWT outlet_use_case claim matches
// one of the allowed values. Platform owners and HQ users bypass the check.
//
// Usage:
//
//	r.With(subscriptions.RequireUseCase("hospitality")).Route("/tables", ...)
//	r.With(subscriptions.RequireUseCase("hospitality", "quick_service")).Route("/kds", ...)
func RequireUseCase(allowed ...string) func(http.Handler) http.Handler {
	set := make(map[string]bool, len(allowed))
	for _, uc := range allowed {
		set[uc] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := authclient.ClaimsFromContext(r.Context())
			if !ok {
				// Unauthenticated — pass through; auth middleware handles rejection
				next.ServeHTTP(w, r)
				return
			}

			// Platform owners and HQ users bypass use-case gating
			if claims.IsPlatformOwner || claims.CanAccessAllOutlets() {
				next.ServeHTTP(w, r)
				return
			}

			useCase := claims.OutletUseCase
			if useCase == "" || set[useCase] {
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":    "feature_not_available",
				"message":  "this feature is not available for your outlet type",
				"use_case": useCase,
			})
		})
	}
}
