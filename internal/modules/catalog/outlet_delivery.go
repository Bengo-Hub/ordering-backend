package catalog

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/ent"
	"github.com/bengobox/ordering-backend/internal/platform/logistics"

	sharedcache "github.com/Bengo-Hub/cache"
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
	// AtOutlet: the pin is at the outlet itself (logistics reason "at_outlet", inside the
	// pickup-only radius). Not a refusal: the customer orders and collects at the counter, so
	// Deliverable stays unknown.
	AtOutlet bool
}

// quoteCacheTTL keeps an outlet's quote for a rounded pin, so browsing pages and rails do not
// re-ask logistics for the same place.
const quoteCacheTTL = 5 * time.Minute

// quoteCacheKey rounds the pin to 3 decimals (about 110 m), close enough that the zone and fee
// do not change between neighbouring pins.
func quoteCacheKey(tenantID, outletID uuid.UUID, lat, lng float64) string {
	r := func(v float64) float64 { return math.Round(v*1000) / 1000 }
	return fmt.Sprintf("ordering:quote:%s:%s:%.3f:%.3f", tenantID, outletID, r(lat), r(lng))
}

// listingDelivery turns a logistics quote into the listing's delivery details.
func listingDelivery(q *logistics.DeliveryQuote) outletDelivery {
	d := outletDelivery{Fee: q.Fee, EtaMinutes: q.EtaMinutes, DistanceKm: q.DistanceKm}
	if q.Reason == "at_outlet" {
		d.AtOutlet = true
		return d
	}
	serviceable := q.Serviceable
	d.Deliverable = &serviceable
	return d
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
			q, err := s.quoteCached(qctx, tenantID, oid, *lat, *lng)
			if err != nil {
				return
			}
			mu.Lock()
			out[oid] = listingDelivery(q)
			mu.Unlock()
		}()
	}
	wg.Wait()
	return out
}

// quoteCached asks logistics for the outlet's quote to the pin, cached per rounded pin.
// Failures are not cached.
func (s *ProxyService) quoteCached(ctx context.Context, tenantID, outletID uuid.UUID, lat, lng float64) (*logistics.DeliveryQuote, error) {
	fetch := func(ctx context.Context) (*logistics.DeliveryQuote, error) {
		return s.delivery.Quote(ctx, tenantID, logistics.QuoteRequest{Lat: lat, Lng: lng, OutletID: &outletID})
	}
	if s.cache == nil {
		return fetch(ctx)
	}
	return sharedcache.GetOrSet(ctx, s.cache, quoteCacheKey(tenantID, outletID, lat, lng), quoteCacheTTL, fetch)
}
