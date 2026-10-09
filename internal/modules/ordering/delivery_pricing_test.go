package ordering

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/platform/logistics"
)

type fakeQuoter struct {
	q    *logistics.DeliveryQuote
	err  error
	last logistics.QuoteRequest
}

func (f *fakeQuoter) Quote(_ context.Context, _ uuid.UUID, req logistics.QuoteRequest) (*logistics.DeliveryQuote, error) {
	f.last = req
	return f.q, f.err
}

// feeRepo answers only the fee_config lookup the fee service needs.
type feeRepo struct {
	Repository
	cfg string
}

func (r feeRepo) GetServiceConfigValue(_ context.Context, _ uuid.UUID, key string) (string, bool, error) {
	if key == FeeConfigKey && r.cfg != "" {
		return r.cfg, true, nil
	}
	return "", false, nil
}

func newFeeSvc(cfg string, q DeliveryQuoter) *FeeService {
	s := NewFeeService(feeRepo{cfg: cfg}, zap.NewNop())
	if q != nil {
		s.SetDeliveryQuoter(q)
	}
	return s
}

func f64(v float64) *float64 { return &v }

func zoneQuote(name string, fee float64) *logistics.DeliveryQuote {
	return &logistics.DeliveryQuote{Serviceable: true, Method: "zone", Fee: fee, Currency: "KES", Zone: &logistics.ZoneRef{ID: "z-" + name, Name: name}, DistanceKm: 4.2, PolicyVersion: "v1"}
}

func TestPriceDeliveryUsesQuoteFee(t *testing.T) {
	q := &fakeQuoter{q: zoneQuote("Bugengi", 150)}
	outlet := uuid.New()
	p, err := newFeeSvc("", q).PriceDelivery(context.Background(), uuid.New(), outlet, FulfillmentTypeDelivery, f64(0.47), f64(34.16), 800)
	if err != nil || p.Fee != 150 || p.ZoneName() != "Bugengi" {
		t.Fatalf("price = %+v, %v", p, err)
	}
	if q.last.OutletID == nil || *q.last.OutletID != outlet || q.last.Lat != 0.47 || q.last.OrderTotal != 800 {
		t.Fatalf("quote request = %+v", q.last)
	}
	snap := p.Snapshot()
	if snap["zone_name"] != "Bugengi" || snap["charged_fee"] != 150.0 || snap["method"] != "zone" {
		t.Fatalf("snapshot = %v", snap)
	}
}

func TestPriceDeliveryPickupAndDineInAreFree(t *testing.T) {
	svc := newFeeSvc("", &fakeQuoter{err: errors.New("must not be called")})
	for _, ft := range []FulfillmentType{FulfillmentTypePickup, FulfillmentTypeDineIn} {
		p, err := svc.PriceDelivery(context.Background(), uuid.New(), uuid.New(), ft, nil, nil, 500)
		if err != nil || p.Fee != 0 || p.Quote != nil {
			t.Fatalf("%s: %+v %v", ft, p, err)
		}
	}
}

func TestPriceDeliveryErrors(t *testing.T) {
	ctx := context.Background()
	// No pin: refuse instead of delivering for free.
	if _, err := newFeeSvc("", &fakeQuoter{}).PriceDelivery(ctx, uuid.New(), uuid.Nil, FulfillmentTypeDelivery, nil, nil, 100); !errors.Is(err, ErrDeliveryLocationRequired) {
		t.Fatalf("missing pin: %v", err)
	}
	if _, err := newFeeSvc("", &fakeQuoter{}).PriceDelivery(ctx, uuid.New(), uuid.Nil, FulfillmentTypeScheduled, f64(0), f64(0), 100); !errors.Is(err, ErrDeliveryLocationRequired) {
		t.Fatalf("0,0 pin: %v", err)
	}
	// Logistics down: refuse, never guess a fee.
	if _, err := newFeeSvc("", &fakeQuoter{err: logistics.ErrQuoteUnavailable}).PriceDelivery(ctx, uuid.New(), uuid.Nil, FulfillmentTypeDelivery, f64(1), f64(34), 100); !errors.Is(err, ErrDeliveryPricingUnavailable) {
		t.Fatalf("outage: %v", err)
	}
	if _, err := newFeeSvc("", nil).PriceDelivery(ctx, uuid.New(), uuid.Nil, FulfillmentTypeDelivery, f64(1), f64(34), 100); !errors.Is(err, ErrDeliveryPricingUnavailable) {
		t.Fatalf("no quoter: %v", err)
	}
	// Outside the coverage.
	out := &logistics.DeliveryQuote{Serviceable: false, Reason: "outside_delivery_area", NearestArea: &logistics.ZoneRef{Name: "Malaba"}, NearestAreaKm: 6.1}
	_, err := newFeeSvc("", &fakeQuoter{q: out}).PriceDelivery(ctx, uuid.New(), uuid.Nil, FulfillmentTypeDelivery, f64(1), f64(34), 100)
	var ns *DeliveryNotServiceableError
	if !errors.As(err, &ns) || !errors.Is(err, ErrDeliveryNotServiceable) || ns.NearestArea != "Malaba" || ns.Reason != "outside_delivery_area" {
		t.Fatalf("not serviceable: %v", err)
	}
	// Below the area minimum.
	q := zoneQuote("Malaba", 1050)
	q.MinOrder = 1500
	_, err = newFeeSvc("", &fakeQuoter{q: q}).PriceDelivery(ctx, uuid.New(), uuid.Nil, FulfillmentTypeDelivery, f64(1), f64(34), 900)
	var bm *BelowDeliveryMinimumError
	if !errors.As(err, &bm) || bm.MinOrder != 1500 || bm.Area != "Malaba" {
		t.Fatalf("below minimum: %v", err)
	}
}

func TestApplyDeliveryFeesTenantRules(t *testing.T) {
	// Free area stays free.
	free := zoneQuote("Busia Town", 0)
	free.Free = true
	if p, _ := applyDeliveryFees(free, FeeConfig{}, 100); p.Fee != 0 || p.FreeReason != "zone" {
		t.Fatalf("free zone: %+v", p)
	}
	// free_delivery_minimum waives the fee only at or above the threshold.
	cfg := FeeConfig{FreeDeliveryMinimum: 2000}
	if p, _ := applyDeliveryFees(zoneQuote("Alupe", 100), cfg, 1999); p.Fee != 100 {
		t.Fatalf("below threshold: %+v", p)
	}
	if p, _ := applyDeliveryFees(zoneQuote("Alupe", 100), cfg, 2000); p.Fee != 0 || p.FreeReason != "minimum" {
		t.Fatalf("at threshold: %+v", p)
	}
	// Default config (all zero) never waives or discounts.
	if p, _ := applyDeliveryFees(zoneQuote("Alupe", 100), DefaultFeeConfig(), 100000); p.Fee != 100 {
		t.Fatalf("defaults must not change the quote: %+v", p)
	}
	// Delivery discount.
	if p, _ := applyDeliveryFees(zoneQuote("Lukolis", 600), FeeConfig{DeliveryDiscountPct: 0.25}, 100); p.Fee != 450 {
		t.Fatalf("discount: %+v", p)
	}
	// Per-km quotes keep the quoted fee.
	pk := &logistics.DeliveryQuote{Serviceable: true, Method: "per_km", Fee: 230, DistanceKm: 4.6}
	if p, _ := applyDeliveryFees(pk, FeeConfig{}, 100); p.Fee != 230 || p.ZoneName() != "" || p.Snapshot()["method"] != "per_km" {
		t.Fatalf("per km: %+v", p)
	}
}

func TestPriceDeliveryReadsFeeConfig(t *testing.T) {
	svc := newFeeSvc(`{"free_delivery_minimum": 1000}`, &fakeQuoter{q: zoneQuote("Alupe", 100)})
	p, err := svc.PriceDelivery(context.Background(), uuid.New(), uuid.Nil, FulfillmentTypeDelivery, f64(0.49), f64(34.13), 1200)
	if err != nil || p.Fee != 0 || p.FreeReason != "minimum" {
		t.Fatalf("fee_config not applied: %+v %v", p, err)
	}
	// Broken JSON falls back to defaults rather than failing checkout.
	svc = newFeeSvc(`{not json`, &fakeQuoter{q: zoneQuote("Alupe", 100)})
	if p, err := svc.PriceDelivery(context.Background(), uuid.New(), uuid.Nil, FulfillmentTypeDelivery, f64(0.49), f64(34.13), 5000); err != nil || p.Fee != 100 {
		t.Fatalf("bad config: %+v %v", p, err)
	}
}

func TestComputeFees(t *testing.T) {
	delivery := DeliveryPrice{Fee: 75, Quote: &logistics.DeliveryQuote{Fee: 100}}
	cfg := FeeConfig{PackagingFeeFlat: 20, SmallOrderFee: 50, SmallOrderThreshold: 500, ServiceFeePercent: 0.05}
	f := computeFees(cfg, 400, 0, delivery, 40)
	// subtotal = 400 - 40 + 20 = 380; small order 50; service 19; delivery 75.
	if f.Subtotal != 380 || f.SmallOrderFee != 50 || f.ServiceFee != 19 || f.DeliveryFee != 75 || f.DeliveryDiscount != 25 || f.GrandTotal != 524 {
		t.Fatalf("fees = %+v", f)
	}
	plain := computeFees(DefaultFeeConfig(), 1000, 0, DeliveryPrice{Fee: 150}, 0)
	if plain.GrandTotal != 1150 || plain.SmallOrderFee != 0 || plain.ServiceFee != 0 {
		t.Fatalf("defaults add nothing: %+v", plain)
	}
	if neg := computeFees(DefaultFeeConfig(), 100, 0, DeliveryPrice{}, 500); neg.Subtotal != 0 || neg.GrandTotal != 0 {
		t.Fatalf("discount above total floors at 0: %+v", neg)
	}
}

func TestDeliveryZoneOfAndWithDeliveryQuote(t *testing.T) {
	p := DeliveryPrice{Fee: 150, Quote: zoneQuote("Bugengi", 150)}
	md := withDeliveryQuote(map[string]interface{}{"guest": true}, p, "Bugengi Primary")
	id, name, km := deliveryZoneOf(md)
	if id != "z-Bugengi" || name != "Bugengi" || km != 4.2 || md["guest"] != true {
		t.Fatalf("round trip = %s %s %v %v", id, name, km, md)
	}
	if snap := md[metaDeliveryQuote].(map[string]interface{}); snap["place_name"] != "Bugengi Primary" {
		t.Fatalf("place name missing: %v", snap)
	}
	if got := withDeliveryQuote(nil, DeliveryPrice{}, "x"); got != nil {
		t.Fatal("no quote must leave metadata untouched")
	}
	if deliveryPin(FulfillmentTypePickup, f64(1)) != nil || *deliveryPin(FulfillmentTypeDelivery, f64(1)) != 1 {
		t.Fatal("deliveryPin wrong")
	}
}
