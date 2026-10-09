package ordering

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/platform/logistics"
)

// DeliveryQuoter prices a delivery. logistics.Client implements it; tests use a fake.
// Delivery areas, geofencing and distance pricing live in logistics-api only.
type DeliveryQuoter interface {
	Quote(ctx context.Context, tenantID uuid.UUID, req logistics.QuoteRequest) (*logistics.DeliveryQuote, error)
}

var (
	// ErrDeliveryLocationRequired means a delivery order arrived without a map pin.
	ErrDeliveryLocationRequired = errors.New("choose your delivery location on the map")
	// ErrDeliveryPricingUnavailable means the delivery could not be priced right now.
	ErrDeliveryPricingUnavailable = errors.New("delivery pricing is unavailable right now, please try again or choose pickup")
)

// DeliveryNotServiceableError explains why a pin cannot be delivered to. It matches
// ErrDeliveryNotServiceable with errors.Is so existing handlers keep working.
type DeliveryNotServiceableError struct {
	Reason        string  `json:"reason"`
	NearestArea   string  `json:"nearest_area,omitempty"`
	NearestAreaKm float64 `json:"nearest_area_km,omitempty"`
}

func (e *DeliveryNotServiceableError) Error() string {
	if e.NearestArea != "" {
		return fmt.Sprintf("we do not deliver to this location yet (nearest area: %s, %.1f km away)", e.NearestArea, e.NearestAreaKm)
	}
	return "we do not deliver to this location yet"
}

// Is lets errors.Is(err, ErrDeliveryNotServiceable) match.
func (e *DeliveryNotServiceableError) Is(target error) bool {
	return target == ErrDeliveryNotServiceable
}

// BelowDeliveryMinimumError means the basket is below the delivery area's minimum order.
type BelowDeliveryMinimumError struct {
	Area     string  `json:"area"`
	MinOrder float64 `json:"min_order"`
}

func (e *BelowDeliveryMinimumError) Error() string {
	return fmt.Sprintf("orders delivered to %s need a minimum of %.0f", e.Area, e.MinOrder)
}

// DeliveryPrice is what checkout charges for delivery and the quote it came from. The
// snapshot is stored on the order (metadata.delivery_quote) so reports and support can
// see which area and policy priced it.
type DeliveryPrice struct {
	Fee        float64
	Quote      *logistics.DeliveryQuote
	FreeReason string // "zone" (free area) | "minimum" (free_delivery_minimum) | ""
}

// Snapshot is the order metadata record of the delivery price.
func (p DeliveryPrice) Snapshot() map[string]interface{} {
	if p.Quote == nil {
		return nil
	}
	q := p.Quote
	snap := map[string]interface{}{
		"method":         q.Method,
		"quoted_fee":     q.Fee,
		"charged_fee":    p.Fee,
		"currency":       q.Currency,
		"distance_km":    q.DistanceKm,
		"distance_type":  q.DistanceType,
		"eta_minutes":    q.EtaMinutes,
		"policy_version": q.PolicyVersion,
	}
	if q.Zone != nil {
		snap["zone_id"], snap["zone_name"] = q.Zone.ID, q.Zone.Name
	}
	if q.NearestArea != nil {
		snap["nearest_area"] = q.NearestArea.Name
	}
	if p.FreeReason != "" {
		snap["free_reason"] = p.FreeReason
	}
	return snap
}

// ZoneID and ZoneName of the matched area, empty for distance-priced drop-offs.
func (p DeliveryPrice) ZoneID() string {
	if p.Quote != nil && p.Quote.Zone != nil {
		return p.Quote.Zone.ID
	}
	return ""
}

func (p DeliveryPrice) ZoneName() string {
	if p.Quote != nil && p.Quote.Zone != nil {
		return p.Quote.Zone.Name
	}
	return ""
}

// SetDeliveryQuoter wires delivery pricing (logistics-api).
func (s *FeeService) SetDeliveryQuoter(q DeliveryQuoter) { s.quoter = q }

// PriceDelivery is the only place ordering decides a delivery fee: it takes the
// logistics quote for the pin and applies the tenant's own ordering fees on top
// (free delivery over a basket minimum, delivery discount). Non-delivery orders cost 0.
func (s *FeeService) PriceDelivery(ctx context.Context, tenantID, outletID uuid.UUID, ft FulfillmentType, lat, lng *float64, itemsTotal float64) (DeliveryPrice, error) {
	if ft == FulfillmentTypePickup || ft == FulfillmentTypeDineIn {
		return DeliveryPrice{}, nil
	}
	if lat == nil || lng == nil || (*lat == 0 && *lng == 0) {
		return DeliveryPrice{}, ErrDeliveryLocationRequired
	}
	if s.quoter == nil {
		return DeliveryPrice{}, ErrDeliveryPricingUnavailable
	}
	req := logistics.QuoteRequest{Lat: *lat, Lng: *lng, OrderTotal: itemsTotal}
	if outletID != uuid.Nil {
		oid := outletID
		req.OutletID = &oid
	}
	q, err := s.quoter.Quote(ctx, tenantID, req)
	if err != nil {
		s.logger.Warn("delivery quote failed", zap.Error(err))
		return DeliveryPrice{}, ErrDeliveryPricingUnavailable
	}
	return applyDeliveryFees(q, s.LoadFeeConfig(ctx, tenantID), itemsTotal)
}

// priceDelivery delegates to the fee service.
func (s *OrderService) priceDelivery(ctx context.Context, tenantID, outletID uuid.UUID, ft FulfillmentType, lat, lng *float64, itemsTotal float64) (DeliveryPrice, error) {
	if s.feeSvc == nil {
		return DeliveryPrice{}, ErrDeliveryPricingUnavailable
	}
	return s.feeSvc.PriceDelivery(ctx, tenantID, outletID, ft, lat, lng, itemsTotal)
}

// applyDeliveryFees turns a quote into the charged fee. Pure, for tests.
func applyDeliveryFees(q *logistics.DeliveryQuote, cfg FeeConfig, itemsTotal float64) (DeliveryPrice, error) {
	if !q.Serviceable {
		e := &DeliveryNotServiceableError{Reason: q.Reason, NearestAreaKm: q.NearestAreaKm}
		if q.NearestArea != nil {
			e.NearestArea = q.NearestArea.Name
		}
		return DeliveryPrice{}, e
	}
	if q.MinOrder > 0 && itemsTotal < q.MinOrder {
		area := "this area"
		if q.Zone != nil {
			area = q.Zone.Name
		}
		return DeliveryPrice{}, &BelowDeliveryMinimumError{Area: area, MinOrder: q.MinOrder}
	}
	p := DeliveryPrice{Fee: q.Fee, Quote: q}
	switch {
	case q.Free || q.Fee == 0:
		p.Fee, p.FreeReason = 0, "zone"
	case cfg.FreeDeliveryMinimum > 0 && itemsTotal >= cfg.FreeDeliveryMinimum:
		p.Fee, p.FreeReason = 0, "minimum"
	case cfg.DeliveryDiscountPct > 0:
		p.Fee = math.Round(q.Fee*(1-math.Min(cfg.DeliveryDiscountPct, 1))*100) / 100
	}
	return p, nil
}

// withDeliveryQuote records the delivery price on order metadata (delivery orders only).
func withDeliveryQuote(md map[string]interface{}, p DeliveryPrice, placeName string) map[string]interface{} {
	snap := p.Snapshot()
	if snap == nil {
		return md
	}
	if placeName != "" {
		snap["place_name"] = placeName
	}
	if md == nil {
		md = map[string]interface{}{}
	}
	md[metaDeliveryQuote] = snap
	return md
}

// deliveryPin keeps the drop-off coordinates only for orders that are delivered.
func deliveryPin(ft FulfillmentType, v *float64) *float64 {
	if ft == FulfillmentTypePickup || ft == FulfillmentTypeDineIn {
		return nil
	}
	return v
}

// metaDeliveryQuote is the order metadata key holding the delivery price snapshot.
const metaDeliveryQuote = "delivery_quote"

// deliveryZoneOf reads the priced area back from an order (for order.ready).
func deliveryZoneOf(md map[string]interface{}) (zoneID, zoneName string, distanceKm float64) {
	snap, ok := md[metaDeliveryQuote].(map[string]interface{})
	if !ok {
		return "", "", 0
	}
	zoneID, _ = snap["zone_id"].(string)
	zoneName, _ = snap["zone_name"].(string)
	switch d := snap["distance_km"].(type) {
	case float64:
		distanceKm = d
	case int:
		distanceKm = float64(d)
	}
	return
}
