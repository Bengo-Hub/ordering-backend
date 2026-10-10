package logistics

import (
	"context"
	"errors"
	"fmt"
	"time"

	sharedcache "github.com/Bengo-Hub/cache"
	"github.com/google/uuid"
)

// ErrQuoteUnavailable means logistics-api could not be reached to price a delivery.
// Checkout refuses the delivery (pickup and dine-in still work) rather than guess a fee.
var ErrQuoteUnavailable = errors.New("delivery pricing is unavailable right now")

// ZoneRef names a delivery area.
type ZoneRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// QuoteBreakdown explains a distance-rate fee.
type QuoteBreakdown struct {
	BaseFee   float64 `json:"base_fee"`
	PerKmRate float64 `json:"per_km_rate"`
	Raw       float64 `json:"raw"`
	MinFee    float64 `json:"min_fee"`
	Rounding  float64 `json:"rounding"`
}

// DeliveryQuote is logistics-api's authoritative delivery price for a drop-off point.
// Ordering never recomputes it; see logistics-api docs/delivery-zones.md.
type DeliveryQuote struct {
	Serviceable   bool            `json:"serviceable"`
	Reason        string          `json:"reason,omitempty"`
	Method        string          `json:"method,omitempty"` // zone | per_km
	Fee           float64         `json:"fee"`
	Free          bool            `json:"free"`
	Currency      string          `json:"currency"`
	Zone          *ZoneRef        `json:"zone,omitempty"`
	NearestArea   *ZoneRef        `json:"nearest_area,omitempty"`
	NearestAreaKm float64         `json:"nearest_area_km,omitempty"`
	DistanceKm    float64         `json:"distance_km"`
	DistanceType  string          `json:"distance_type,omitempty"`
	EtaMinutes    int             `json:"eta_minutes,omitempty"`
	MinOrder      float64         `json:"min_order"`
	BelowMinOrder bool            `json:"below_min_order,omitempty"`
	Breakdown     *QuoteBreakdown `json:"breakdown,omitempty"`
	PolicyVersion string          `json:"policy_version"`
	CacheSeconds  int             `json:"cache_seconds"`
}

// QuoteRequest is one pricing request.
type QuoteRequest struct {
	Lat        float64    `json:"lat"`
	Lng        float64    `json:"lng"`
	OutletID   *uuid.UUID `json:"outlet_id,omitempty"`
	OrderTotal float64    `json:"order_total,omitempty"`
}

// SetCache enables short-lived Redis caching of quotes (shared across pods).
func (c *Client) SetCache(a *sharedcache.Aside) { c.cache = a }

// Quote prices a delivery through logistics-api's S2S quote route. Results are cached
// per tenant, outlet and pin (about 11 m) for the TTL logistics returns, so a customer
// moving through checkout does not re-price on every request. The order total is not
// part of the key: below_min_order is recomputed locally from min_order.
func (c *Client) Quote(ctx context.Context, tenantID uuid.UUID, req QuoteRequest) (*DeliveryQuote, error) {
	outlet := "nearest"
	if req.OutletID != nil && *req.OutletID != uuid.Nil {
		outlet = req.OutletID.String()
	}
	key := sharedcache.Key("ord", "dq", tenantID.String(), outlet, fmt.Sprintf("%.4f,%.4f", req.Lat, req.Lng))
	fetch := func(ctx context.Context) (*DeliveryQuote, error) {
		body := req
		body.OrderTotal = 0
		resp, err := c.serviceClient.Post(ctx, fmt.Sprintf("/api/v1/s2s/zones/%s/quote", tenantID.String()), body, c.headers(""))
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrQuoteUnavailable, err)
		}
		if !resp.IsSuccess() {
			return nil, fmt.Errorf("%w: %v", ErrQuoteUnavailable, c.parseError(resp))
		}
		var q DeliveryQuote
		if err := resp.DecodeJSON(&q); err != nil {
			return nil, fmt.Errorf("%w: decode: %v", ErrQuoteUnavailable, err)
		}
		return &q, nil
	}
	// Kept for as long as logistics says the quote stays valid (its quote_cache_seconds).
	q, err := sharedcache.GetOrSetTTL(ctx, c.cache, key, func(ctx context.Context) (*DeliveryQuote, time.Duration, error) {
		q, err := fetch(ctx)
		if err != nil {
			return nil, 0, err
		}
		return q, quoteTTL(q.CacheSeconds), nil
	})
	if err != nil {
		return nil, err
	}
	out := *q
	out.BelowMinOrder = req.OrderTotal > 0 && out.MinOrder > 0 && req.OrderTotal < out.MinOrder
	return &out, nil
}

// quoteTTL turns the quote's cache_seconds into a Redis TTL, bounded to 10 seconds to
// 30 minutes; 0 means do not cache.
func quoteTTL(seconds int) time.Duration {
	switch {
	case seconds <= 0:
		return 0
	case seconds < 10:
		return 10 * time.Second
	case seconds > 1800:
		return 30 * time.Minute
	}
	return time.Duration(seconds) * time.Second
}

// Coverage is the public summary of where a tenant delivers.
type Coverage struct {
	Zones    []CoverageZone `json:"zones"`
	MinFee   float64        `json:"min_fee"`
	HasFree  bool           `json:"has_free_zone"`
	Currency string         `json:"currency"`
}

// CoverageZone is one delivery area in a coverage summary.
type CoverageZone struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	ZoneType string  `json:"zone_type"`
	Fee      float64 `json:"fee"`
	Free     bool    `json:"free"`
}

// Coverage returns the tenant's delivery coverage summary (cached 5 minutes).
func (c *Client) Coverage(ctx context.Context, tenantID uuid.UUID) (*Coverage, error) {
	fetch := func(ctx context.Context) (*Coverage, error) {
		resp, err := c.serviceClient.Get(ctx, fmt.Sprintf("/api/v1/s2s/zones/%s/coverage", tenantID.String()), c.headers(""))
		if err != nil {
			return nil, err
		}
		if !resp.IsSuccess() {
			return nil, c.parseError(resp)
		}
		var cov Coverage
		if err := resp.DecodeJSON(&cov); err != nil {
			return nil, err
		}
		return &cov, nil
	}
	if c.cache == nil {
		return fetch(ctx)
	}
	return sharedcache.GetOrSet(ctx, c.cache, sharedcache.Key("ord", "dcov", tenantID.String()), sharedcache.TTLReference, fetch)
}
