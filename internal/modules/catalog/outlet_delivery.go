package catalog

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/ent"
	"github.com/bengobox/ordering-backend/internal/platform/logistics"
)

// DeliveryInfoSource is logistics-api's delivery pricing as seen by the catalog.
type DeliveryInfoSource interface {
	Quote(ctx context.Context, tenantID uuid.UUID, req logistics.QuoteRequest) (*logistics.DeliveryQuote, error)
	Coverage(ctx context.Context, tenantID uuid.UUID) (*logistics.Coverage, error)
}

// SetDeliverySource wires delivery pricing for outlet listings.
func (s *ProxyService) SetDeliverySource(d DeliveryInfoSource) { s.delivery = d }

type outletDelivery struct {
	Fee        float64
	EtaMinutes int
	DistanceKm float64
	// Deliverable is known only when the customer's pin is; without it Fee is the
	// cheapest area fee ("from"), not a quote.
	Deliverable *bool
}

// maxQuoteFanout bounds concurrent quote calls for one listing.
const maxQuoteFanout = 6

// deliveryForOutlets prices delivery from each outlet to the customer's pin. Without a
// pin every outlet shows the tenant's cheapest area fee. Failures leave the outlet
// without delivery details rather than failing the listing.
func (s *ProxyService) deliveryForOutlets(ctx context.Context, tenantID uuid.UUID, outlets []*ent.Outlet, lat, lng *float64) map[uuid.UUID]outletDelivery {
	out := make(map[uuid.UUID]outletDelivery, len(outlets))
	if s.delivery == nil {
		return out
	}
	if lat == nil || lng == nil {
		cov, err := s.delivery.Coverage(ctx, tenantID)
		if err != nil {
			s.logger.Debug("delivery coverage unavailable for listing", zap.Error(err))
			return out
		}
		if len(cov.Zones) == 0 {
			return out
		}
		for _, o := range outlets {
			out[o.ID] = outletDelivery{Fee: cov.MinFee}
		}
		return out
	}

	qctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxQuoteFanout)
	for _, o := range outlets {
		oid := o.ID
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			q, err := s.delivery.Quote(qctx, tenantID, logistics.QuoteRequest{Lat: *lat, Lng: *lng, OutletID: &oid})
			if err != nil {
				return
			}
			mu.Lock()
			serviceable := q.Serviceable
			out[oid] = outletDelivery{Fee: q.Fee, EtaMinutes: q.EtaMinutes, DistanceKm: q.DistanceKm, Deliverable: &serviceable}
			mu.Unlock()
		}()
	}
	wg.Wait()
	return out
}
