// Package promobanner exposes the storefront-facing promotions endpoints. They are thin
// read-through proxies over pos-api's Promotion records and sales aggregates (the platform's
// discount source of truth); this package owns NO discount data of its own.
package promobanner

import (
	"context"
	"net/http"
	"time"

	sharedcache "github.com/Bengo-Hub/cache"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/ent"
	"github.com/bengobox/ordering-backend/internal/ent/tenant"
	"github.com/bengobox/ordering-backend/internal/http/handlers"
	"github.com/bengobox/ordering-backend/internal/platform/posdiscounts"
	"github.com/bengobox/ordering-backend/internal/platform/posreports"
)

// cacheTTL is short: these are public, anonymous, high-traffic endpoints that must never hit
// pos-api on every storefront page load, while promotion edits still show up quickly.
const cacheTTL = 2 * time.Minute

// topSellersWindowDays is the trailing sales window used to rank "best sellers" for the
// storefront's best-sellers rail. 90 days is a "currently trending" horizon.
const topSellersWindowDays = 90

// topSellersLimit is how many best sellers the rail can use; pos-api ranks and cuts them in SQL.
const topSellersLimit = 40

// Handler serves GET /{tenant}/promotions/{banners,deals,top-sellers} (public, no auth: the same
// class of route as /config and /outlets).
type Handler struct {
	log     *zap.Logger
	db      *ent.Client
	client  *posdiscounts.Client
	reports *posreports.Client
	cache   *sharedcache.Aside
}

// New constructs the promotions handler. cache may be nil (every request then hits pos-api
// directly). reports may be nil (ListTopSellers then always returns empty).
func New(log *zap.Logger, db *ent.Client, client *posdiscounts.Client, reports *posreports.Client, c *sharedcache.Aside) *Handler {
	return &Handler{log: log.Named("promobanner.Handler"), db: db, client: client, reports: reports, cache: c}
}

// ListBanners returns the active storefront-flagged promotions for the tenant in the URL,
// optionally filtered by `?use_case=` (the browsed outlet's vertical; empty returns every
// use_case-unscoped banner). Always 200s with a (possibly empty) array: a promotions
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
	if !h.client.Enabled() {
		handlers.RespondJSON(w, http.StatusOK, []posdiscounts.Banner{})
		return
	}
	useCase := r.URL.Query().Get("use_case")
	banners := cachedBySlug(r.Context(), h, "ordering:promo-banners:"+slug+":"+useCase, slug, func(ctx context.Context, tenantID uuid.UUID) []posdiscounts.Banner {
		return h.client.ListBanners(ctx, tenantID, useCase)
	})
	handlers.RespondJSON(w, http.StatusOK, banners)
}

// ListDeals returns active, currently-in-window discounts for the tenant's deals grid. Always
// 200s with a (possibly empty) array, same posture as ListBanners.
// @Summary List storefront deals
// @Description Active, in-window discounts for the ordering storefront's deals grid
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
	if !h.client.Enabled() {
		handlers.RespondJSON(w, http.StatusOK, []posdiscounts.Discount{})
		return
	}
	deals := cachedBySlug(r.Context(), h, "ordering:promo-deals:"+slug, slug, h.client.ListDeals)
	handlers.RespondJSON(w, http.StatusOK, deals)
}

// ListTopSellers returns units sold per SKU from the tenant's real completed sales over the
// trailing topSellersWindowDays, best first and at most topSellersLimit rows (ranked by pos-api).
// The storefront matches them against its loaded catalog items to build a real best-sellers rail.
// Always 200s with a (possibly empty) array.
// @Summary List real best-selling SKUs
// @Description Top units sold per SKU over the trailing 90 days, for the storefront's best-sellers rail
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
	if h.reports == nil || !h.reports.Enabled() {
		handlers.RespondJSON(w, http.StatusOK, []posreports.SKURow{})
		return
	}
	rows := cachedBySlug(r.Context(), h, "ordering:promo-top-sellers:"+slug, slug, func(ctx context.Context, tenantID uuid.UUID) []posreports.SKURow {
		now := time.Now()
		from := now.AddDate(0, 0, -topSellersWindowDays).Format("2006-01-02")
		return h.reports.SalesBySKU(ctx, tenantID, from, now.Format("2006-01-02"), topSellersLimit)
	})
	handlers.RespondJSON(w, http.StatusOK, rows)
}

// cachedBySlug serves a per-tenant promotions read from the cache keyed by slug, so a hit costs
// neither a tenant lookup nor a pos-api call. On a miss it resolves the slug to the local tenant
// (one that has not synced yet simply has nothing to show) and runs fetch. Never returns nil, so
// the JSON is [] not null.
func cachedBySlug[T any](ctx context.Context, h *Handler, key, slug string, fetch func(context.Context, uuid.UUID) []T) []T {
	load := func(ctx context.Context) ([]T, error) {
		t, err := h.db.Tenant.Query().Where(tenant.SlugEQ(slug)).Only(ctx)
		if err != nil {
			return []T{}, nil
		}
		out := fetch(ctx, t.ID)
		if out == nil {
			out = []T{}
		}
		return out, nil
	}
	if h.cache == nil {
		out, _ := load(ctx)
		return out
	}
	out, err := sharedcache.GetOrSet(ctx, h.cache, key, cacheTTL, load)
	if err != nil {
		h.log.Warn("promobanner: cache read failed, loading directly", zap.String("key", key), zap.Error(err))
		out, _ = load(ctx)
	}
	if out == nil {
		out = []T{}
	}
	return out
}
