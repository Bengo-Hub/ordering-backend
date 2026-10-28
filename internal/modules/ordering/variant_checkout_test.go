package ordering

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/modules/catalog"
	"github.com/bengobox/ordering-backend/internal/platform/posdiscounts"
)

// stubCatalog serves merged items by SKU, the way catalog.ProxyService.GetItem does after it has
// resolved a variant SKU.
type stubCatalog map[string]*catalog.MergedCatalogItem

func (s stubCatalog) GetItem(_ context.Context, _ string, _ uuid.UUID, sku string, _ *uuid.UUID) (*catalog.MergedCatalogItem, error) {
	if it, ok := s[sku]; ok {
		cp := *it
		return &cp, nil
	}
	return nil, catalog.ErrItemNotFound
}

var (
	redMID  = uuid.MustParse("11111111-1111-1111-1111-111111111111")
	blueLID = uuid.MustParse("22222222-2222-2222-2222-222222222222")
	groupID = uuid.MustParse("33333333-3333-3333-3333-333333333333")
	giftID  = uuid.MustParse("44444444-4444-4444-4444-444444444444")
)

func variantCatalog() stubCatalog {
	red := catalog.CatalogVariant{ID: redMID, SKU: "TS-1-RED-M", Price: 1200, IsActive: true,
		Attributes: map[string]string{"colour": "Red", "size": "M"}}
	blue := catalog.CatalogVariant{ID: blueLID, SKU: "TS-1-BLU-L", Price: 1100, IsActive: false,
		Attributes: map[string]string{"colour": "Blue", "size": "L"}}
	mods := []catalog.CatalogModifierGroup{{ID: groupID.String(), Name: "Extras",
		Options: []catalog.CatalogModifierOption{{ID: giftID.String(), Name: "Gift wrap", PriceAdjustment: 50}}}}
	parent := &catalog.MergedCatalogItem{InventorySKU: "TS-1", InventoryID: uuid.New(), Name: "T-shirt",
		CategoryName: "Clothing", BasePrice: 1000, IsAvailable: true, HasVariants: true,
		Variants: []catalog.CatalogVariant{red, blue}, ModifierGroups: mods}
	redItem := *parent
	redItem.InventorySKU, redItem.InventoryID, redItem.ParentSKU = red.SKU, red.ID, "TS-1"
	redItem.Name, redItem.BasePrice, redItem.Variant = "T-shirt, Red / M", 1200, &red
	blueItem := *parent
	blueItem.InventorySKU, blueItem.InventoryID, blueItem.ParentSKU = blue.SKU, blue.ID, "TS-1"
	blueItem.Name, blueItem.BasePrice, blueItem.Variant, blueItem.IsAvailable = "T-shirt, Blue / L", 1100, &blue, false
	return stubCatalog{"TS-1": parent, red.SKU: &redItem, blue.SKU: &blueItem}
}

func TestValidateAndPriceItems_VariantLine(t *testing.T) {
	s := &OrderService{catalogSvc: variantCatalog(), logger: zap.NewNop()}
	items := []CreateOrderItemInput{{
		InventorySKU: "TS-1-RED-M", Name: "T-shirt", Quantity: 2, UnitPrice: 1,
		Metadata: map[string]interface{}{"variant_name": "spoofed", "parent_sku": "X"},
	}}
	got, err := s.validateAndPriceItems(context.Background(), "t", uuid.New(), items, "")
	if err != nil {
		t.Fatal(err)
	}
	line := got[0]
	if line.UnitPrice != 1200 || line.TotalPrice != 2400 {
		t.Errorf("price = %v / %v, want 1200 / 2400", line.UnitPrice, line.TotalPrice)
	}
	if line.Name != "T-shirt, Red / M" {
		t.Errorf("name = %q, must be the variant name, not the parent name", line.Name)
	}
	m := line.Metadata
	if m["variant_id"] != redMID.String() || m["variant_sku"] != "TS-1-RED-M" ||
		m["variant_name"] != "T-shirt, Red / M" || m["parent_sku"] != "TS-1" {
		t.Errorf("variant metadata not stamped from the catalog: %#v", m)
	}
	if attrs, _ := m["variant_attributes"].(map[string]interface{}); attrs["size"] != "M" || attrs["colour"] != "Red" {
		t.Errorf("variant_attributes = %#v", m["variant_attributes"])
	}
	if id := variantIDFromMetadata(m); id == nil || *id != redMID {
		t.Errorf("order item VariantID = %v, want %s", id, redMID)
	}
}

func TestValidateAndPriceItems_ParentSKUWithVariantID(t *testing.T) {
	s := &OrderService{catalogSvc: variantCatalog(), logger: zap.NewNop()}
	items := []CreateOrderItemInput{{InventorySKU: "TS-1", Quantity: 1,
		Metadata: map[string]interface{}{"variant_id": redMID.String()}}}
	got, err := s.validateAndPriceItems(context.Background(), "t", uuid.New(), items, "")
	if err != nil {
		t.Fatal(err)
	}
	if got[0].InventorySKU != "TS-1-RED-M" || got[0].UnitPrice != 1200 {
		t.Fatalf("line not moved to the variant SKU: %#v", got[0])
	}
}

func TestValidateAndPriceItems_RejectsBadVariant(t *testing.T) {
	s := &OrderService{catalogSvc: variantCatalog(), logger: zap.NewNop()}
	cases := map[string]CreateOrderItemInput{
		"inactive variant": {InventorySKU: "TS-1-BLU-L", Quantity: 1},
		"unknown variant":  {InventorySKU: "TS-1", Quantity: 1, Metadata: map[string]interface{}{"variant_id": uuid.NewString()}},
		"mismatched id":    {InventorySKU: "TS-1-RED-M", Quantity: 1, Metadata: map[string]interface{}{"variant_id": blueLID.String()}},
	}
	for name, it := range cases {
		_, err := s.validateAndPriceItems(context.Background(), "t", uuid.New(), []CreateOrderItemInput{it}, "")
		if !errors.Is(err, ErrVariantUnavailable) {
			t.Errorf("%s: err = %v, want ErrVariantUnavailable", name, err)
		}
	}
}

func TestValidateAndPriceItems_PlainLineDropsClientVariantKeys(t *testing.T) {
	s := &OrderService{catalogSvc: variantCatalog(), logger: zap.NewNop()}
	items := []CreateOrderItemInput{{InventorySKU: "TS-1", Quantity: 1,
		Metadata: map[string]interface{}{"parent_sku": "FAKE", "variant_name": "FAKE"}}}
	got, err := s.validateAndPriceItems(context.Background(), "t", uuid.New(), items, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range variantMetadataKeys {
		if _, ok := got[0].Metadata[k]; ok {
			t.Errorf("client key %s survived on a plain line", k)
		}
	}
}

func TestApplyActiveDealDiscount_MatchesParentSKU(t *testing.T) {
	item, _ := variantCatalog().GetItem(context.Background(), "", uuid.Nil, "TS-1-RED-M", nil)
	var deals []posdiscounts.Discount
	raw := `[{"id":"d1","rule":{"scope_type":"item","scope_ids":["TS-1"],"discount_type":"percentage","discount_value":10}}]`
	if err := json.Unmarshal([]byte(raw), &deals); err != nil {
		t.Fatal(err)
	}
	price, dealID := applyActiveDealDiscount(item, deals)
	if dealID != "d1" || price != 1080 {
		t.Fatalf("parent-scoped deal on variant: price %v deal %q", price, dealID)
	}
}

func TestOutletLinePayload_CarriesVariantKeys(t *testing.T) {
	meta := withCatalogSnapshot(nil, variantCatalog()["TS-1-RED-M"])
	line := outletLinePayload(OrderItem{InventorySKU: "TS-1-RED-M", NameSnapshot: "T-shirt, Red / M", Quantity: 1, Metadata: meta})
	if line["sku"] != "TS-1-RED-M" || line["name"] != "T-shirt, Red / M" {
		t.Errorf("line sku/name = %v / %v", line["sku"], line["name"])
	}
	lm, _ := line["metadata"].(map[string]interface{})
	for _, k := range variantMetadataKeys {
		if _, ok := lm[k]; !ok {
			t.Errorf("outlet line metadata missing %s: %#v", k, lm)
		}
	}
}

// cartRepo is an in-memory cart store with just what AddItem needs.
type cartRepo struct {
	Repository
	cart  *Cart
	items []CartItem
}

func (r *cartRepo) GetActiveCartBySession(context.Context, uuid.UUID, uuid.UUID, string) (*Cart, error) {
	return r.cart, nil
}
func (r *cartRepo) GetCart(context.Context, uuid.UUID, uuid.UUID) (*Cart, error) { return r.cart, nil }
func (r *cartRepo) UpdateCart(context.Context, *Cart) error                      { return nil }
func (r *cartRepo) ListCartItems(context.Context, uuid.UUID) ([]CartItem, error) {
	return r.items, nil
}
func (r *cartRepo) GetTenantByID(_ context.Context, id uuid.UUID) (*Tenant, error) {
	return &Tenant{ID: id, Slug: "t"}, nil
}
func (r *cartRepo) GetCartItemBySKU(context.Context, uuid.UUID, string, *uuid.UUID) (*CartItem, error) {
	return nil, errors.New("not found")
}
func (r *cartRepo) CreateCartItem(_ context.Context, it *CartItem) error {
	r.items = append(r.items, *it)
	return nil
}

func TestCartAddItem_UsesVariantPriceAndServerModifierPrice(t *testing.T) {
	repo := &cartRepo{cart: &Cart{ID: uuid.New(), Status: CartStatusActive}}
	svc := &CartService{repo: repo, catalogSvc: variantCatalog(), logger: zap.NewNop()}
	_, err := svc.AddItem(context.Background(), AddItemRequest{
		TenantID: uuid.New(), OutletID: uuid.New(), SessionID: "s", InventorySKU: "TS-1-RED-M", Quantity: 1,
		Modifiers: []CartItemModifier{
			{GroupID: groupID, OptionID: giftID, PriceAdjustment: -900},  // tampered price
			{GroupID: groupID, OptionID: uuid.New(), PriceAdjustment: 5}, // unknown option
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(repo.items) != 1 {
		t.Fatalf("items = %d", len(repo.items))
	}
	it := repo.items[0]
	if it.UnitPrice != 1200 || it.ModifierTotal != 50 || it.TotalPrice != 1250 {
		t.Errorf("unit %v modifiers %v total %v, want 1200 / 50 / 1250", it.UnitPrice, it.ModifierTotal, it.TotalPrice)
	}
	if it.InventorySKU != "TS-1-RED-M" || it.VariantID == nil || *it.VariantID != redMID {
		t.Errorf("sku %s variant %v", it.InventorySKU, it.VariantID)
	}
	if !strings.Contains(it.NameSnapshot, "Red / M") || it.Metadata["parent_sku"] != "TS-1" {
		t.Errorf("name %q metadata %#v", it.NameSnapshot, it.Metadata)
	}
}

func TestCartAddItem_RejectsInactiveVariant(t *testing.T) {
	repo := &cartRepo{cart: &Cart{ID: uuid.New(), Status: CartStatusActive}}
	svc := &CartService{repo: repo, catalogSvc: variantCatalog(), logger: zap.NewNop()}
	_, err := svc.AddItem(context.Background(), AddItemRequest{
		TenantID: uuid.New(), OutletID: uuid.New(), SessionID: "s", InventorySKU: "TS-1", VariantID: &blueLID, Quantity: 1,
	})
	if !errors.Is(err, ErrVariantUnavailable) {
		t.Fatalf("err = %v, want ErrVariantUnavailable", err)
	}
}
