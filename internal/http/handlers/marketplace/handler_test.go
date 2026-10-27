package marketplace

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/modules/catalog"
	"github.com/bengobox/ordering-backend/internal/platform/marketplace"
	"github.com/bengobox/ordering-backend/internal/platform/posdiscounts"
)

type fakeDir struct {
	page marketplace.Page
	got  marketplace.Query
}

func (f *fakeDir) ListTenants(_ context.Context, q marketplace.Query) (marketplace.Page, error) {
	f.got = q
	return f.page, nil
}

type fakePromos struct{ list []posdiscounts.StorefrontPromotion }

func (f *fakePromos) Enabled() bool { return true }
func (f *fakePromos) ListStorefront(_ context.Context, _ []string) []posdiscounts.StorefrontPromotion {
	return f.list
}

type fakeCatalog struct{ items map[string][]catalog.MergedCatalogItem }

func (f *fakeCatalog) ListItems(_ context.Context, slug string, _ uuid.UUID, flt catalog.CatalogFilter) ([]catalog.MergedCatalogItem, int, error) {
	var out []catalog.MergedCatalogItem
	for _, it := range f.items[slug] {
		if flt.IsAvailable != nil && it.IsAvailable != *flt.IsAvailable {
			continue
		}
		fold := func(list []string, v string) bool {
			return slices.ContainsFunc(list, func(s string) bool { return strings.EqualFold(s, v) })
		}
		if len(flt.SKUs) > 0 && !fold(flt.SKUs, it.InventorySKU) {
			continue
		}
		if len(flt.CategoryNames) > 0 && !fold(flt.CategoryNames, it.CategoryName) {
			continue
		}
		out = append(out, it)
	}
	return out, len(out), nil
}

func promo(tenantID, id string, scope string, ids []string, discount string, banner bool) posdiscounts.StorefrontPromotion {
	end := time.Now().Add(time.Hour)
	p := posdiscounts.StorefrontPromotion{TenantID: tenantID}
	p.ID, p.Name, p.EndAt = id, id, &end
	p.Rule = &struct {
		ScopeType     string   `json:"scope_type"`
		ScopeIDs      []string `json:"scope_ids"`
		DiscountType  string   `json:"discount_type"`
		DiscountValue float64  `json:"discount_value"`
		MaxDiscount   float64  `json:"max_discount"`
	}{ScopeType: scope, ScopeIDs: ids, DiscountType: discount, DiscountValue: 20}
	p.Banner.ShowOnStorefront = banner
	return p
}

func TestListTenantsSendsEligibleUseCasesAndPages(t *testing.T) {
	dir := &fakeDir{page: marketplace.Page{Data: []marketplace.TenantSummary{{ID: "1", Slug: "a"}}, Total: 30}}
	h := New(zap.NewNop(), dir, nil, nil, nil)

	rec := httptest.NewRecorder()
	h.ListTenants(rec, httptest.NewRequest(http.MethodGet, "/marketplace/tenants?limit=24", nil))
	var resp ListTenantsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Total != 30 || !resp.HasMore || len(resp.Data) != 1 {
		t.Fatalf("unexpected page %+v", resp)
	}
	if !slices.Contains(dir.got.UseCases, "retail") || !slices.Contains(dir.got.UseCases, "hospitality") {
		t.Fatalf("eligible verticals not sent: %v", dir.got.UseCases)
	}

	// A vertical with no storefront never reaches auth-api.
	dir.got = marketplace.Query{}
	rec = httptest.NewRecorder()
	h.ListTenants(rec, httptest.NewRequest(http.MethodGet, "/marketplace/tenants?use_case=axle_load_enforcement", nil))
	if dir.got.Limit != 0 {
		t.Fatal("ineligible use_case should not query the directory")
	}
	if !strings.Contains(rec.Body.String(), `"data":[]`) {
		t.Fatalf("ineligible use_case gives an empty list: %s", rec.Body.String())
	}
}

func TestListDealsResolvesAndMixesStores(t *testing.T) {
	shopA, shopB := uuid.NewString(), uuid.NewString()
	dir := &fakeDir{page: marketplace.Page{Data: []marketplace.TenantSummary{
		{ID: shopA, Slug: "alpha", Name: "Alpha", UseCase: "retail"},
		{ID: shopB, Slug: "boi", Name: "BOI", UseCases: []string{"retail"}},
	}, Total: 2}}
	// Rules carry SKUs and category names, exactly as pos-api stores and matches them.
	promos := &fakePromos{list: []posdiscounts.StorefrontPromotion{
		promo(shopA, "a-items", "item", []string{"acm-00077", "ACM-SOLD"}, "percentage", true),
		promo(shopB, "b-cat", "category", []string{"bags"}, "fixed_amount", false),
		promo(shopB, "b-bogo", "item", []string{"BOI-CHG"}, "bogo", false),
		promo(uuid.NewString(), "stranger", "item", []string{"ACM-00077"}, "percentage", true),
	}}
	cat := &fakeCatalog{items: map[string][]catalog.MergedCatalogItem{
		"alpha": {
			{InventoryID: uuid.New(), InventorySKU: "ACM-00077", Name: "Pan", BasePrice: 3999, IsAvailable: true},
			{InventoryID: uuid.New(), InventorySKU: "ACM-SOLD", Name: "Sold out", BasePrice: 100, IsAvailable: false},
		},
		"boi": {
			{InventoryID: uuid.New(), InventorySKU: "BOI-TOTE", Name: "Tote", BasePrice: 1200, IsAvailable: true, CategoryName: "Bags"},
			{InventoryID: uuid.New(), InventorySKU: "BOI-CHG", Name: "Charger", BasePrice: 1400, IsAvailable: true},
		},
	}}
	h := New(zap.NewNop(), dir, promos, cat, nil)

	rec := httptest.NewRecorder()
	h.ListDeals(rec, httptest.NewRequest(http.MethodGet, "/marketplace/deals", nil))
	var resp DealsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}

	var names []string
	for _, d := range resp.Deals {
		names = append(names, d.Store.Slug+":"+d.Item.Name)
	}
	if !slices.Equal(names, []string{"alpha:Pan", "boi:Tote"}) {
		t.Fatalf("want one available priced deal per store, bogo and sold-out skipped; got %v", names)
	}
	if resp.Deals[0].Item.Price != 3999 || resp.Deals[0].Promotion.Rule.DiscountType != "percentage" {
		t.Fatalf("deal card carries the item price and its rule: %+v", resp.Deals[0])
	}
	if len(resp.Banners) != 1 || resp.Banners[0].Store.Slug != "alpha" {
		t.Fatalf("only listed stores' banners: %+v", resp.Banners)
	}
	if resp.Deals[1].Store.UseCase != "retail" {
		t.Fatalf("store use case falls back to use_cases[0]: %+v", resp.Deals[1].Store)
	}
}

func TestListDealsEmptyWithoutPromotions(t *testing.T) {
	dir := &fakeDir{page: marketplace.Page{Data: []marketplace.TenantSummary{{ID: uuid.NewString(), Slug: "a"}}, Total: 1}}
	h := New(zap.NewNop(), dir, &fakePromos{}, &fakeCatalog{}, nil)
	rec := httptest.NewRecorder()
	h.ListDeals(rec, httptest.NewRequest(http.MethodGet, "/marketplace/deals", nil))
	var resp DealsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Deals == nil || resp.Banners == nil || len(resp.Deals)+len(resp.Banners) != 0 {
		t.Fatalf("want empty (non-null) lists, got %s", rec.Body.String())
	}
}
