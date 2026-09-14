// Package promobanner exposes the storefront-facing promotions banner endpoint. It is a thin
// read-through proxy over pos-api's Promotion records (the platform's discount source of truth) —
// this package owns NO discount data of its own.
package promobanner

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/ent"
	"github.com/bengobox/ordering-backend/internal/ent/tenant"
	"github.com/bengobox/ordering-backend/internal/http/handlers"
	"github.com/bengobox/ordering-backend/internal/platform/cache"
	"github.com/bengobox/ordering-backend/internal/platform/posdiscounts"
	"github.com/bengobox/ordering-backend/internal/platform/posreports"
)

// bannerCacheTTL is short — a public, anonymous, high-traffic endpoint should never hammer
// pos-api on every storefront page load, but banner edits should still show up quickly.
const bannerCacheTTL = 2 * time.Minute

// Handler serves GET /{tenant}/promotions/banners (public, no auth — same class of route as
// /config and /outlets).
type Handler struct {
	log     *zap.Logger
	db      *ent.Client
	client  *posdiscounts.Client
	reports *posreports.Client
	cache   *cache.Service
}

// New constructs the promotions banner handler. cache may be nil (caching then no-ops, every
// request hits pos-api directly). reports may be nil (ListTopSellers then always returns empty).
func New(log *zap.Logger, db *ent.Client, client *posdiscounts.Client, reports *posreports.Client, c *cache.Service) *Handler {
	return &Handler{log: log.Named("promobanner.Handler"), db: db, client: client, reports: reports, cache: c}
}

// ListBanners returns the active storefront-flagged promotions for the tenant in the URL,
// optionally filtered by `?use_case=` (the browsed outlet's vertical — empty/omitted returns
// every use_case-unscoped banner). Always 200s with a (possibly empty) array — a promotions
// integration hiccup must never break the storefront homepage.
// @Summary List storefront promotion banners
// @Description Active promotions flagged to appear on the ordering storefront homepage
// @Tags Promotions
// @Produce json
// @Param tenant path string true "Tenant slug"
// @Param use_case query string false "Outlet use_case filter"
// @Success 200 {array} posdiscounts.Banner
// @Router /{tenant}/promotions/banners [get]
func (h *Handler) ListBanners(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "tenant")
	if slug == "" {
		handlers.RespondError(w, http.StatusBadRequest, "tenant slug required")
		return
	}
	ctx := r.Context()

	t, err := h.db.Tenant.Query().Where(tenant.SlugEQ(slug)).Only(ctx)
	if err != nil {
		// A tenant that hasn't synced locally yet simply has no banners — not an error the
		// customer-facing homepage should surface.
		handlers.RespondJSON(w, http.StatusOK, []posdiscounts.Banner{})
		return
	}

	useCase := r.URL.Query().Get("use_case")
	if !h.client.Enabled() {
		handlers.RespondJSON(w, http.StatusOK, []posdiscounts.Banner{})
		return
	}

	cacheKey := "promo-banners:" + t.ID.String() + ":" + useCase
	var banners []posdiscounts.Banner
	fetch := func() (interface{}, error) {
		return h.client.ListBanners(context.WithoutCancel(ctx), t.ID, useCase), nil
	}
	if h.cache != nil {
		if err := h.cache.GetOrSet(ctx, cacheKey, &banners, bannerCacheTTL, fetch); err != nil {
			h.log.Warn("promobanner: cache/fetch failed, falling back to direct call", zap.Error(err))
			banners = h.client.ListBanners(ctx, t.ID, useCase)
		}
	} else {
		banners = h.client.ListBanners(ctx, t.ID, useCase)
	}
	if banners == nil {
		banners = []posdiscounts.Banner{}
	}
	handlers.RespondJSON(w, http.StatusOK, banners)
}

// dealsCacheTTL matches bannerCacheTTL — same class of public, high-traffic, edit-should-show-up
// -quickly endpoint.
const dealsCacheTTL = bannerCacheTTL

// ListDeals returns active, currently-in-window discounts for the tenant's "Top Deals" storefront
// grid. Always 200s with a (possibly empty) array, same posture as ListBanners.
// @Summary List storefront "Top Deals"
// @Description Active, in-window discounts for the ordering storefront's Top Deals grid
// @Tags Promotions
// @Produce json
// @Param tenant path string true "Tenant slug"
// @Success 200 {array} posdiscounts.Discount
// @Router /{tenant}/promotions/deals [get]
func (h *Handler) ListDeals(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "tenant")
	if slug == "" {
		handlers.RespondError(w, http.StatusBadRequest, "tenant slug required")
		return
	}
	ctx := r.Context()

	t, err := h.db.Tenant.Query().Where(tenant.SlugEQ(slug)).Only(ctx)
	if err != nil {
		handlers.RespondJSON(w, http.StatusOK, []posdiscounts.Discount{})
		return
	}

	if !h.client.Enabled() {
		handlers.RespondJSON(w, http.StatusOK, []posdiscounts.Discount{})
		return
	}

	cacheKey := "promo-deals:" + t.ID.String()
	var deals []posdiscounts.Discount
	fetch := func() (interface{}, error) {
		return h.client.ListDeals(context.WithoutCancel(ctx), t.ID), nil
	}
	if h.cache != nil {
		if err := h.cache.GetOrSet(ctx, cacheKey, &deals, dealsCacheTTL, fetch); err != nil {
			h.log.Warn("promobanner: deals cache/fetch failed, falling back to direct call", zap.Error(err))
			deals = h.client.ListDeals(ctx, t.ID)
		}
	} else {
		deals = h.client.ListDeals(ctx, t.ID)
	}
	if deals == nil {
		deals = []posdiscounts.Discount{}
	}
	handlers.RespondJSON(w, http.StatusOK, deals)
}

// topSellersCacheTTL matches bannerCacheTTL — a public, high-traffic endpoint should never
// hammer pos-api's order-scanning aggregate on every storefront page load.
const topSellersCacheTTL = bannerCacheTTL

// topSellersWindowDays is the trailing sales window used to rank "best sellers" for the
// storefront's real "Top Deals" rail (redefined 2026-09-14 to mean best-sellers, not discounts —
// see FlashSaleStrip for the discount-driven rail). 90 days is a reasonable "currently trending"
// horizon; easy to change later, it's the only place this constant lives.
const topSellersWindowDays = 90

// ListTopSellers returns units-sold-per-SKU for the tenant's real completed sales over the
// trailing topSellersWindowDays — the storefront merges this against its already-loaded catalog
// items client-side (same "fetch a bounded item page + cross-reference a pos-api aggregate"
// pattern ListDeals's caller already uses for discount matching) to build a genuine best-sellers
// rail. Always 200s with a (possibly empty) array, same posture as ListBanners/ListDeals.
// @Summary List real best-selling SKUs
// @Description Units sold per SKU over the trailing 90 days, for the storefront's Top Deals rail
// @Tags Promotions
// @Produce json
// @Param tenant path string true "Tenant slug"
// @Success 200 {array} posreports.SKURow
// @Router /{tenant}/promotions/top-sellers [get]
func (h *Handler) ListTopSellers(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "tenant")
	if slug == "" {
		handlers.RespondError(w, http.StatusBadRequest, "tenant slug required")
		return
	}
	ctx := r.Context()

	t, err := h.db.Tenant.Query().Where(tenant.SlugEQ(slug)).Only(ctx)
	if err != nil {
		handlers.RespondJSON(w, http.StatusOK, []posreports.SKURow{})
		return
	}

	if h.reports == nil || !h.reports.Enabled() {
		handlers.RespondJSON(w, http.StatusOK, []posreports.SKURow{})
		return
	}

	now := time.Now()
	from := now.AddDate(0, 0, -topSellersWindowDays).Format("2006-01-02")
	to := now.Format("2006-01-02")

	cacheKey := "promo-top-sellers:" + t.ID.String()
	var rows []posreports.SKURow
	fetch := func() (interface{}, error) {
		return h.reports.SalesBySKU(context.WithoutCancel(ctx), t.ID, from, to), nil
	}
	if h.cache != nil {
		if err := h.cache.GetOrSet(ctx, cacheKey, &rows, topSellersCacheTTL, fetch); err != nil {
			h.log.Warn("promobanner: top-sellers cache/fetch failed, falling back to direct call", zap.Error(err))
			rows = h.reports.SalesBySKU(ctx, t.ID, from, to)
		}
	} else {
		rows = h.reports.SalesBySKU(ctx, t.ID, from, to)
	}
	if rows == nil {
		rows = []posreports.SKURow{}
	}
	handlers.RespondJSON(w, http.StatusOK, rows)
}
