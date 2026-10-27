// Package marketplace serves the platform-level (no tenant slug) marketplace landing page data:
// GET /api/v1/marketplace/tenants and GET /api/v1/marketplace/deals. These routes live OUTSIDE
// the /{tenant} route group: they have no tenant context by design (they list across tenants),
// so they must not go through TenantV2, tenant-sync, the per-tenant auth-skip wrapper, or
// SubscriptionGate.
package marketplace

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	sharedcache "github.com/Bengo-Hub/cache"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/http/handlers"
	"github.com/bengobox/ordering-backend/internal/modules/catalog"
	"github.com/bengobox/ordering-backend/internal/platform/marketplace"
	"github.com/bengobox/ordering-backend/internal/platform/posdiscounts"
)

// storefrontEligibleUseCases mirrors ordering-frontend's normalizeOrderingUseCase (the set of
// raw use_case values that resolve to a real per-vertical homepage). A tenant whose use_case is
// empty, "other", or anything unrecognized has no customer-facing storefront and must not appear
// on the public marketplace page (e.g. a government body or a back-office-only tenant). The list
// is sent to auth-api, which filters in SQL, so totals and pages count only these tenants.
var storefrontEligibleUseCases = []string{
	"hospitality", "restaurant", "cafe", "bar", "food", "food_delivery",
	"quick_service", "qsr", "fast_food",
	"pharmacy", "chemist",
	"retail", "e_commerce", "ecommerce", "electronics", "hardware", "grocery", "supermarket",
	"services", "salon", "spa", "wellness", "beauty",
	"ticketing", "events", "event",
	"warehousing", "warehouse", "wholesale", "manufacturing",
}

func isEligibleUseCase(uc string) bool {
	return slices.Contains(storefrontEligibleUseCases, strings.ToLower(strings.TrimSpace(uc)))
}

const (
	// listCacheTTL is short: this is the platform's public front door, and must never hit
	// auth-api on every page load while still showing new tenants quickly.
	listCacheTTL = 60 * time.Second
	// dealsCacheTTL matches the per-tenant promotions endpoints.
	dealsCacheTTL = 2 * time.Minute
	// dealTenants is how many top-ranked stores the deals feed looks at (pos-api's cap).
	dealTenants = 50
	// maxDeals and maxDealsPerStore keep the rail varied: no single store fills it.
	maxDeals         = 12
	maxDealsPerStore = 4
	maxBanners       = 4
	// resolveConcurrency bounds the parallel per-store catalogue reads.
	resolveConcurrency = 4
)

// storefrontPromotions is the pos-api side the deals feed needs (posdiscounts.Client).
type storefrontPromotions interface {
	Enabled() bool
	ListStorefront(ctx context.Context, tenantIDs []string) []posdiscounts.StorefrontPromotion
}

// catalogItems is the catalogue side the deals feed needs (catalog.ProxyService).
type catalogItems interface {
	ListItems(ctx context.Context, tenantSlug string, tenantID uuid.UUID, filter catalog.CatalogFilter) ([]catalog.MergedCatalogItem, int, error)
}

// tenantDirectory is the auth-api side (marketplace.Client).
type tenantDirectory interface {
	ListTenants(ctx context.Context, q marketplace.Query) (marketplace.Page, error)
}

// Handler serves the public marketplace directory and its deals feed.
type Handler struct {
	log     *zap.Logger
	dir     tenantDirectory
	promos  storefrontPromotions
	catalog catalogItems
	cache   *sharedcache.Aside
}

// New constructs the marketplace handler. cache may be nil (every request then hits the
// upstream services directly; fine for local dev, not for production). promos or items may be
// nil, which turns the deals feed into an empty list.
func New(log *zap.Logger, dir tenantDirectory, promos storefrontPromotions, items catalogItems, c *sharedcache.Aside) *Handler {
	return &Handler{log: log.Named("marketplace.Handler"), dir: dir, promos: promos, catalog: items, cache: c}
}

// ListTenantsResponse is the JSON body of GET /api/v1/marketplace/tenants.
type ListTenantsResponse struct {
	Data    []marketplace.TenantSummary `json:"data"`
	Total   int                         `json:"total"`
	Page    int                         `json:"page"`
	Limit   int                         `json:"limit"`
	HasMore bool                        `json:"hasMore"`
}

// ListTenants returns marketplace-visible tenants that have a real storefront, ranked by plan
// tier by auth-api, optionally narrowed to one `?use_case=`. Filtering and pagination happen in
// auth-api's SQL, so `total` is exact. Always 200s: a directory hiccup degrades the landing page
// to an empty list, never a hard error.
// @Summary List marketplace tenants
// @Description Active, non-demo tenants with a storefront, ranked by subscription tier
// @Tags Marketplace
// @Produce json
// @Param use_case query string false "Filter by tenant use_case"
// @Param page query int false "Page number (default 1)"
// @Param limit query int false "Page size (default 24, max 50)"
// @Success 200 {object} marketplace.ListTenantsResponse
// @Router /marketplace/tenants [get]
func (h *Handler) ListTenants(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	useCase := strings.ToLower(strings.TrimSpace(q.Get("use_case")))
	page := parsePositiveInt(q.Get("page"), 1)
	limit := min(parsePositiveInt(q.Get("limit"), 24), 50)

	resp := ListTenantsResponse{Data: []marketplace.TenantSummary{}, Page: page, Limit: limit}
	if useCase != "" && !isEligibleUseCase(useCase) {
		handlers.RespondJSON(w, http.StatusOK, resp)
		return
	}
	got, err := h.tenantPage(r.Context(), useCase, page, limit)
	if err != nil {
		h.log.Warn("marketplace: list tenants failed", zap.Error(err))
	}
	if got.Data != nil {
		resp.Data = got.Data
	}
	resp.Total = got.Total
	resp.HasMore = page*limit < got.Total
	handlers.RespondJSON(w, http.StatusOK, resp)
}

// tenantPage reads one directory page through the cache.
func (h *Handler) tenantPage(ctx context.Context, useCase string, page, limit int) (marketplace.Page, error) {
	query := marketplace.Query{UseCase: useCase, UseCases: storefrontEligibleUseCases, Page: page, Limit: limit}
	load := func(ctx context.Context) (marketplace.Page, error) { return h.dir.ListTenants(ctx, query) }
	if h.cache == nil {
		return load(ctx)
	}
	key := "ordering:marketplace-tenants:v2:" + useCase + ":" + strconv.Itoa(page) + ":" + strconv.Itoa(limit)
	return sharedcache.GetOrSet(ctx, h.cache, key, listCacheTTL, load)
}

// StoreRef identifies the store a deal or banner belongs to.
type StoreRef struct {
	Slug    string `json:"slug"`
	Name    string `json:"name"`
	LogoURL string `json:"logo_url,omitempty"`
	UseCase string `json:"use_case,omitempty"`
}

// DealItem is the catalogue side of a deal card. Price is the item's own price; the storefront
// applies the rule with the same applyDeal it uses on every tenant page.
type DealItem struct {
	InventoryID       uuid.UUID `json:"inventory_id"`
	SKU               string    `json:"sku"`
	Name              string    `json:"name"`
	ImageURL          string    `json:"image_url,omitempty"`
	Price             float64   `json:"price"`
	Currency          string    `json:"currency,omitempty"`
	AvailableQuantity *float64  `json:"available_quantity,omitempty"`
}

// DealCard is one product on a live promotion.
type DealCard struct {
	Store     StoreRef              `json:"store"`
	Promotion posdiscounts.Discount `json:"promotion"`
	Item      DealItem              `json:"item"`
}

// BannerTile is one store's storefront banner for the marketplace hero tiles.
type BannerTile struct {
	Store     StoreRef                        `json:"store"`
	PromoID   string                          `json:"promo_id"`
	Name      string                          `json:"name"`
	EndAt     *time.Time                      `json:"end_at,omitempty"`
	FlashSale bool                            `json:"is_flash_sale"`
	Banner    posdiscounts.StorefrontBannerUI `json:"banner"`
}

// DealsResponse is the JSON body of GET /api/v1/marketplace/deals.
type DealsResponse struct {
	Deals   []DealCard   `json:"deals"`
	Banners []BannerTile `json:"banners"`
}

// ListDeals returns the products on live promotions across the marketplace's stores, soonest
// ending first, plus the stores' storefront banners. Only real promotions appear: when no store
// runs one, both lists are empty and the page hides those sections. Cost per cache period: one
// directory read, one pos-api call for every store's promotions, and one catalogue read for each
// store that has a deal (each tenant's catalogue is itself cached). Always 200s.
// @Summary List marketplace deals
// @Description Products on live promotions across stores, and store banners
// @Tags Marketplace
// @Produce json
// @Success 200 {object} marketplace.DealsResponse
// @Router /marketplace/deals [get]
func (h *Handler) ListDeals(w http.ResponseWriter, r *http.Request) {
	if h.promos == nil || !h.promos.Enabled() || h.catalog == nil {
		handlers.RespondJSON(w, http.StatusOK, DealsResponse{Deals: []DealCard{}, Banners: []BannerTile{}})
		return
	}
	ctx := r.Context()
	if h.cache == nil {
		handlers.RespondJSON(w, http.StatusOK, h.buildDeals(ctx))
		return
	}
	resp, err := sharedcache.GetOrSet(ctx, h.cache, "ordering:marketplace-deals:v1", dealsCacheTTL,
		func(ctx context.Context) (DealsResponse, error) { return h.buildDeals(ctx), nil })
	if err != nil {
		h.log.Warn("marketplace: deals cache read failed, building directly", zap.Error(err))
		resp = h.buildDeals(ctx)
	}
	handlers.RespondJSON(w, http.StatusOK, resp)
}

// priceableDiscount reports whether a rule reduces to one price on one product card.
func priceableDiscount(d posdiscounts.Discount) bool {
	if d.Rule == nil || len(d.Rule.ScopeIDs) == 0 {
		return false
	}
	if d.Rule.ScopeType != "item" && d.Rule.ScopeType != "category" {
		return false
	}
	switch d.Rule.DiscountType {
	case "percentage", "fixed_amount", "fixed_price":
		return true
	}
	return false
}

func (h *Handler) buildDeals(ctx context.Context) DealsResponse {
	resp := DealsResponse{Deals: []DealCard{}, Banners: []BannerTile{}}
	dir, err := h.tenantPage(ctx, "", 1, dealTenants)
	if err != nil || len(dir.Data) == 0 {
		return resp
	}
	stores := make(map[string]marketplace.TenantSummary, len(dir.Data))
	ids := make([]string, 0, len(dir.Data))
	for _, t := range dir.Data {
		stores[t.ID] = t
		ids = append(ids, t.ID)
	}
	promos := h.promos.ListStorefront(ctx, ids)

	// Banners, and the promotions that can price a product card, grouped by store in the
	// order pos-api returned them (soonest ending first).
	byStore := map[string][]posdiscounts.StorefrontPromotion{}
	var storeOrder []string
	for _, p := range promos {
		st, ok := stores[p.TenantID]
		if !ok {
			continue
		}
		if p.Banner.ShowOnStorefront && len(resp.Banners) < maxBanners {
			resp.Banners = append(resp.Banners, BannerTile{
				Store: storeRef(st), PromoID: p.ID, Name: p.Name, EndAt: p.EndAt, FlashSale: p.IsFlashSale, Banner: p.Banner,
			})
		}
		if !priceableDiscount(p.Discount) {
			continue
		}
		if _, seen := byStore[p.TenantID]; !seen {
			storeOrder = append(storeOrder, p.TenantID)
		}
		byStore[p.TenantID] = append(byStore[p.TenantID], p)
	}

	// One catalogue read per store with deals, a few at a time.
	cards := make([][]DealCard, len(storeOrder))
	var wg sync.WaitGroup
	sem := make(chan struct{}, resolveConcurrency)
	for i, tid := range storeOrder {
		wg.Add(1)
		go func(i int, tid string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			cards[i] = h.storeDeals(ctx, stores[tid], byStore[tid])
		}(i, tid)
	}
	wg.Wait()

	// Interleave stores so the rail mixes shops, keeping each store's soonest-ending first.
	for round := 0; round < maxDealsPerStore && len(resp.Deals) < maxDeals; round++ {
		for _, c := range cards {
			if round < len(c) && len(resp.Deals) < maxDeals {
				resp.Deals = append(resp.Deals, c[round])
			}
		}
	}
	return resp
}

// storeDeals resolves one store's promotions to available products, at most maxDealsPerStore.
func (h *Handler) storeDeals(ctx context.Context, st marketplace.TenantSummary, promos []posdiscounts.StorefrontPromotion) []DealCard {
	tenantID, err := uuid.Parse(st.ID)
	if err != nil {
		return nil
	}
	var itemIDs, categoryIDs []uuid.UUID
	for _, p := range promos {
		for _, raw := range p.Rule.ScopeIDs {
			id, err := uuid.Parse(raw)
			if err != nil {
				continue
			}
			if p.Rule.ScopeType == "item" {
				itemIDs = append(itemIDs, id)
			} else {
				categoryIDs = append(categoryIDs, id)
			}
		}
	}
	available := true
	var items []catalog.MergedCatalogItem
	for _, f := range []catalog.CatalogFilter{
		{InventoryIDs: itemIDs, IsAvailable: &available, Limit: 50},
		{CategoryIDs: categoryIDs, IsAvailable: &available, Limit: 50},
	} {
		if len(f.InventoryIDs) == 0 && len(f.CategoryIDs) == 0 {
			continue
		}
		got, _, err := h.catalog.ListItems(ctx, st.Slug, tenantID, f)
		if err != nil {
			h.log.Warn("marketplace: deal items not resolved", zap.String("tenant", st.Slug), zap.Error(err))
			continue
		}
		items = append(items, got...)
	}

	out := []DealCard{}
	used := map[uuid.UUID]bool{}
	for _, p := range promos {
		for _, it := range items {
			if len(out) == maxDealsPerStore {
				return out
			}
			if used[it.InventoryID] || it.BasePrice <= 0 {
				continue
			}
			match := false
			if p.Rule.ScopeType == "item" {
				match = slices.Contains(p.Rule.ScopeIDs, it.InventoryID.String())
			} else if it.CategoryID != nil {
				match = slices.Contains(p.Rule.ScopeIDs, it.CategoryID.String())
			}
			if !match {
				continue
			}
			used[it.InventoryID] = true
			out = append(out, DealCard{
				Store:     storeRef(st),
				Promotion: p.Discount,
				Item: DealItem{
					InventoryID: it.InventoryID, SKU: it.InventorySKU, Name: it.Name, ImageURL: it.ImageURL,
					Price: it.BasePrice, Currency: it.Currency, AvailableQuantity: it.AvailableQuantity,
				},
			})
		}
	}
	return out
}

func storeRef(t marketplace.TenantSummary) StoreRef {
	uc := t.UseCase
	if uc == "" && len(t.UseCases) > 0 {
		uc = t.UseCases[0]
	}
	return StoreRef{Slug: t.Slug, Name: t.Name, LogoURL: t.LogoURL, UseCase: uc}
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
