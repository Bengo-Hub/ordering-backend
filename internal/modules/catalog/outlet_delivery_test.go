package catalog

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/ent"
	"github.com/bengobox/ordering-backend/internal/platform/logistics"
)

type fakeDelivery struct {
	quotes map[uuid.UUID]*logistics.DeliveryQuote
	cov    *logistics.Coverage
}

func (f fakeDelivery) Quote(_ context.Context, _ uuid.UUID, req logistics.QuoteRequest) (*logistics.DeliveryQuote, error) {
	if q, ok := f.quotes[*req.OutletID]; ok {
		return q, nil
	}
	return nil, errors.New("down")
}

func (f fakeDelivery) Coverage(context.Context, uuid.UUID) (*logistics.Coverage, error) {
	if f.cov == nil {
		return nil, errors.New("down")
	}
	return f.cov, nil
}

func TestDeliveryForOutlets(t *testing.T) {
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	outlets := []*ent.Outlet{{ID: a}, {ID: b}, {ID: c}}
	src := fakeDelivery{
		quotes: map[uuid.UUID]*logistics.DeliveryQuote{
			a: {Serviceable: true, Fee: 150, EtaMinutes: 32, DistanceKm: 4.2},
			b: {Serviceable: false, Reason: "outside_delivery_area"},
		},
		cov: &logistics.Coverage{MinFee: 0, Zones: []logistics.CoverageZone{{Name: "Busia Town", Free: true}, {Name: "Alupe", Fee: 100}}},
	}
	s := &ProxyService{logger: zap.NewNop()}
	s.SetDeliverySource(src)
	lat, lng := 0.47, 34.16

	got := s.deliveryForOutlets(context.Background(), uuid.New(), outlets, &lat, &lng)
	if d := got[a]; d.Fee != 150 || d.EtaMinutes != 32 || d.Deliverable == nil || !*d.Deliverable {
		t.Fatalf("outlet a = %+v", d)
	}
	if d := got[b]; d.Deliverable == nil || *d.Deliverable {
		t.Fatalf("outlet b should be known not to deliver: %+v", d)
	}
	if _, ok := got[c]; ok {
		t.Fatal("a failed quote leaves the outlet without delivery details")
	}

	// Without a pin: cheapest area fee for every outlet, deliverability unknown.
	got = s.deliveryForOutlets(context.Background(), uuid.New(), outlets, nil, nil)
	for _, id := range []uuid.UUID{a, b, c} {
		if d := got[id]; d.Fee != 0 || d.Deliverable != nil {
			t.Fatalf("no-pin listing for %s = %+v", id, d)
		}
	}

	// No delivery source configured: nothing.
	if got := (&ProxyService{logger: zap.NewNop()}).deliveryForOutlets(context.Background(), uuid.New(), outlets, &lat, &lng); len(got) != 0 {
		t.Fatal("without logistics the listing has no delivery details")
	}
}
