// Package marketplace serves the platform-level (no tenant slug) marketplace landing page data —
// GET /api/v1/marketplace/tenants. This route lives OUTSIDE the /{tenant} route group: it has no
// tenant context by design (it lists across tenants), so it must not go through TenantV2,
// tenant-sync, the per-tenant auth-skip wrapper, or SubscriptionGate.
package marketplace

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/http/handlers"
	"github.com/bengobox/ordering-backend/internal/platform/cache"
	"github.com/bengobox/ordering-backend/internal/platform/marketplace"
)

// storefrontEligibleUseCases mirrors ordering-frontend's normalizeOrderingUseCase (the set of
// raw use_case values that resolve to a real per-vertical homepage) — a tenant whose use_case is
// empty, "other", or anything unrecognized has no real customer-facing storefront experience and
// must not appear on the platform's public marketplace landing page (e.g. a government body or an
// individual/professional-services tenant registered for back-office use only).
var storefrontEligibleUseCases = map[string]bool{
	"hospitality": true, "restaurant": true, "cafe": true, "bar": true, "food": true, "food_delivery": true,
	"quick_service": true, "qsr": true, "fast_food": true,
	"pharmacy": true, "chemist": true,
	"retail": true, "e_commerce": true, "ecommerce": true, "electronics": true, "hardware": true,
	"services": true, "salon": true, "spa": true, "wellness": true, "beauty": true,
	"ticketing": true, "events": true, "event": true,
	"warehousing": true, "warehouse": true, "wholesale": true, "manufacturing": true,
}

// isStorefrontEligible reports whether a tenant's use_case (or any of its use_cases) resolves to
// a real storefront profile.
func isStorefrontEligible(t marketplace.TenantSummary) bool {
	if storefrontEligibleUseCases[strings.ToLower(strings.TrimSpace(t.UseCase))] {
		return true
	}
	for _, uc := range t.UseCases {
		if storefrontEligibleUseCases[strings.ToLower(strings.TrimSpace(uc))] {
			return true
		}
	}
	return false
}

// listCacheTTL is short — this is public, anonymous, potentially very high-traffic (it's the
// platform's front door), and must never hammer auth-api on every page load, while still
// reflecting new/changed tenants reasonably quickly.
const listCacheTTL = 60 * time.Second

// Handler serves the public marketplace tenant directory.
type Handler struct {
	log    *zap.Logger
	client *marketplace.Client
	cache  *cache.Service
}

// New constructs the marketplace handler. cache may be nil (every request then hits auth-api
// directly — fine for local dev, not recommended in production).
func New(log *zap.Logger, client *marketplace.Client, c *cache.Service) *Handler {
	return &Handler{log: log.Named("marketplace.Handler"), client: client, cache: c}
}

// ListTenantsResponse is the JSON body of GET /api/v1/marketplace/tenants.
type ListTenantsResponse struct {
	Data []marketplace.TenantSummary `json:"data"`
}

// ListTenants returns marketplace-visible tenants (auth-api already filters out inactive/demo/
// platform-internal tenants and ranks by subscription tier), optionally filtered by `?use_case=`.
// Always 200s with a (possibly empty) list — a directory hiccup must degrade the landing page to
// an empty/skeleton state, never a hard error.
// @Summary List marketplace tenants
// @Description Active, non-demo tenants ranked by subscription tier, for the platform landing page
// @Tags Marketplace
// @Produce json
// @Param use_case query string false "Filter by tenant use_case"
// @Param page query int false "Page number (default 1)"
// @Param limit query int false "Page size (default 20, max 50)"
// @Success 200 {object} marketplace.ListTenantsResponse
// @Router /marketplace/tenants [get]
func (h *Handler) ListTenants(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	useCase := q.Get("use_case")
	page := parsePositiveInt(q.Get("page"), 1)
	limit := parsePositiveInt(q.Get("limit"), 20)
	if limit > 50 {
		limit = 50
	}

	// Storefront eligibility is filtered here (not passed to auth-api), so it must happen
	// BEFORE this handler's own pagination — filtering an already-paginated page would starve
	// page 2+ of eligible tenants sitting behind ineligible ones on page 1 (the exact
	// "client-side filter ran after pagination → empty page" bug class already fixed once in
	// this codebase's catalog proxy). So: fetch a large-enough pool from auth-api unpaginated,
	// filter, then paginate the filtered set ourselves.
	const fetchPoolLimit = 200
	cacheKey := "marketplace-tenants:" + useCase + ":pool"
	var pool []marketplace.TenantSummary
	fetch := func() (interface{}, error) {
		return h.client.ListTenants(ctx, useCase, 1, fetchPoolLimit), nil
	}
	if h.cache != nil {
		if err := h.cache.GetOrSet(ctx, cacheKey, &pool, listCacheTTL, fetch); err != nil {
			h.log.Warn("marketplace: cache/fetch failed, falling back to direct call", zap.Error(err))
			pool = h.client.ListTenants(ctx, useCase, 1, fetchPoolLimit)
		}
	} else {
		pool = h.client.ListTenants(ctx, useCase, 1, fetchPoolLimit)
	}

	eligible := make([]marketplace.TenantSummary, 0, len(pool))
	for _, t := range pool {
		if isStorefrontEligible(t) {
			eligible = append(eligible, t)
		}
	}

	start := (page - 1) * limit
	tenants := []marketplace.TenantSummary{}
	if start < len(eligible) {
		end := start + limit
		if end > len(eligible) {
			end = len(eligible)
		}
		tenants = eligible[start:end]
	}
	handlers.RespondJSON(w, http.StatusOK, ListTenantsResponse{Data: tenants})
}

func parsePositiveInt(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}
