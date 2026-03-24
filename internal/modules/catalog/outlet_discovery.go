package catalog

import (
	"context"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/ent"
	"github.com/bengobox/ordering-backend/internal/ent/deliveryzone"
	"github.com/bengobox/ordering-backend/internal/ent/outlet"
	"github.com/bengobox/ordering-backend/internal/ent/outletrating"
	"github.com/bengobox/ordering-backend/internal/ent/promocode"
)

// OutletListParams defines the query parameters for listing outlets with
// sort, filter, and search capabilities.
type OutletListParams struct {
	// Sort mode: relevance (default), distance, rating, delivery_fee, fastest
	Sort string
	// Customer coordinates (required for distance/delivery_fee/fastest sorting)
	Lat *float64
	Lng *float64
	// Filters
	Category       string   // filter by use_case
	Offers         bool     // only outlets with active promo codes
	MinRating      *float64 // minimum average rating
	MaxDeliveryFee *float64 // maximum delivery fee
	MaxDeliveryTime *int    // maximum estimated delivery time in minutes
	Pickup         bool     // only outlets that support pickup
	Query          string   // text search on outlet name
	// Pagination
	Page  int
	Limit int
}

// OutletWithMeta is the enriched outlet response for discovery endpoints.
type OutletWithMeta struct {
	ID            uuid.UUID `json:"id"`
	Name          string    `json:"name"`
	Slug          string    `json:"slug"`
	ImageURL      string    `json:"image_url,omitempty"`
	Address       string    `json:"address,omitempty"`
	Latitude      float64   `json:"latitude,omitempty"`
	Longitude     float64   `json:"longitude,omitempty"`
	IsOpen        bool      `json:"is_open"`
	AverageRating float64   `json:"average_rating"`
	TotalRatings  int       `json:"total_ratings"`
	DeliveryFee   float64   `json:"delivery_fee"`
	EstimatedTime int       `json:"estimated_time_minutes"`
	Distance      float64   `json:"distance_km,omitempty"`
	HasOffers     bool      `json:"has_offers"`
	UseCase       string    `json:"use_case,omitempty"`
}

// ListOutletsDiscovery returns outlets enriched with rating, delivery, distance,
// and open/closed metadata for the storefront discovery view.
func (s *ProxyService) ListOutletsDiscovery(ctx context.Context, tenantID uuid.UUID, params OutletListParams) ([]OutletWithMeta, int, error) {
	// 1. Query active outlets
	q := s.db.Outlet.Query().Where(outlet.TenantID(tenantID), outlet.StatusEQ("active"))
	if params.Category != "" {
		q = q.Where(outlet.UseCaseEQ(params.Category))
	}
	if params.Query != "" {
		q = q.Where(outlet.NameContainsFold(params.Query))
	}
	if params.Pickup {
		q = q.Where(outlet.SupportsPickupEQ(true))
	}

	outlets, err := q.All(ctx)
	if err != nil {
		return nil, 0, err
	}

	if len(outlets) == 0 {
		return []OutletWithMeta{}, 0, nil
	}

	// 2. Batch-load ratings
	outletIDs := make([]uuid.UUID, len(outlets))
	for i, o := range outlets {
		outletIDs[i] = o.ID
	}
	ratings, err := s.db.OutletRating.Query().
		Where(outletrating.TenantID(tenantID), outletrating.OutletIDIn(outletIDs...)).
		All(ctx)
	if err != nil {
		s.logger.Warn("failed to load outlet ratings", zap.Error(err))
	}
	ratingMap := make(map[uuid.UUID]*ent.OutletRating, len(ratings))
	for _, r := range ratings {
		ratingMap[r.OutletID] = r
	}

	// 3. Batch-load delivery zones for fee/time (pick cheapest zone per outlet)
	zones, err := s.db.DeliveryZone.Query().
		Where(deliveryzone.TenantID(tenantID), deliveryzone.IsActive(true)).
		All(ctx)
	if err != nil {
		s.logger.Warn("failed to load delivery zones", zap.Error(err))
	}
	// Map outlet_id -> best zone (lowest fee); nil outlet_id = applies to all
	type zoneInfo struct {
		Fee  float64
		Time int
	}
	zoneMap := make(map[uuid.UUID]zoneInfo)
	var globalZone *zoneInfo
	for _, z := range zones {
		zi := zoneInfo{Fee: z.DeliveryFee, Time: z.EstimatedTimeMinutes}
		if z.OutletID == nil {
			if globalZone == nil || zi.Fee < globalZone.Fee {
				globalZone = &zi
			}
		} else {
			if existing, ok := zoneMap[*z.OutletID]; !ok || zi.Fee < existing.Fee {
				zoneMap[*z.OutletID] = zi
			}
		}
	}

	// 4. Batch-load active promo codes to determine has_offers
	now := time.Now()
	promos, err := s.db.PromoCode.Query().
		Where(
			promocode.TenantID(tenantID),
			promocode.IsActive(true),
			promocode.Or(
				promocode.StartsAtIsNil(),
				promocode.StartsAtLTE(now),
			),
			promocode.Or(
				promocode.EndsAtIsNil(),
				promocode.EndsAtGTE(now),
			),
		).All(ctx)
	if err != nil {
		s.logger.Warn("failed to load promo codes", zap.Error(err))
	}
	// Build set of outlet IDs that have offers (nil outlet_id = all outlets)
	hasGlobalOffer := false
	outletOfferSet := make(map[uuid.UUID]bool)
	for _, p := range promos {
		if p.OutletID == nil {
			hasGlobalOffer = true
		} else {
			outletOfferSet[*p.OutletID] = true
		}
	}

	// 5. Enrich each outlet
	var results []OutletWithMeta
	for _, o := range outlets {
		meta := OutletWithMeta{
			ID:       o.ID,
			Name:     o.Name,
			Slug:     o.Slug,
			ImageURL: o.ImageURL,
			Address:  o.Address,
			UseCase:  o.UseCase,
		}

		if o.Latitude != nil {
			meta.Latitude = *o.Latitude
		}
		if o.Longitude != nil {
			meta.Longitude = *o.Longitude
		}

		// Open/closed status
		meta.IsOpen = IsOutletOpen(o.OpeningHours)

		// Rating
		if r, ok := ratingMap[o.ID]; ok {
			meta.AverageRating = r.AverageRating
			meta.TotalRatings = r.TotalRatings
		}

		// Delivery fee & time
		if zi, ok := zoneMap[o.ID]; ok {
			meta.DeliveryFee = zi.Fee
			meta.EstimatedTime = zi.Time
		} else if globalZone != nil {
			meta.DeliveryFee = globalZone.Fee
			meta.EstimatedTime = globalZone.Time
		}

		// Distance
		if params.Lat != nil && params.Lng != nil && o.Latitude != nil && o.Longitude != nil {
			meta.Distance = haversineKm(*params.Lat, *params.Lng, *o.Latitude, *o.Longitude)
		}

		// Offers
		meta.HasOffers = hasGlobalOffer || outletOfferSet[o.ID]

		results = append(results, meta)
	}

	// 6. Apply post-query filters
	filtered := results[:0]
	for _, m := range results {
		if params.MinRating != nil && m.AverageRating < *params.MinRating {
			continue
		}
		if params.MaxDeliveryFee != nil && m.DeliveryFee > *params.MaxDeliveryFee {
			continue
		}
		if params.MaxDeliveryTime != nil && m.EstimatedTime > *params.MaxDeliveryTime {
			continue
		}
		if params.Offers && !m.HasOffers {
			continue
		}
		filtered = append(filtered, m)
	}
	results = filtered

	// 7. Sort
	sortMode := strings.ToLower(params.Sort)
	if sortMode == "" {
		sortMode = "relevance"
	}
	switch sortMode {
	case "distance":
		sort.Slice(results, func(i, j int) bool { return results[i].Distance < results[j].Distance })
	case "rating":
		sort.Slice(results, func(i, j int) bool { return results[i].AverageRating > results[j].AverageRating })
	case "delivery_fee":
		sort.Slice(results, func(i, j int) bool { return results[i].DeliveryFee < results[j].DeliveryFee })
	case "fastest":
		sort.Slice(results, func(i, j int) bool { return results[i].EstimatedTime < results[j].EstimatedTime })
	default: // relevance: open first, then by rating
		sort.Slice(results, func(i, j int) bool {
			if results[i].IsOpen != results[j].IsOpen {
				return results[i].IsOpen
			}
			return results[i].AverageRating > results[j].AverageRating
		})
	}

	// 8. Paginate
	total := len(results)
	page := params.Page
	if page < 1 {
		page = 1
	}
	limit := params.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	start := (page - 1) * limit
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}

	return results[start:end], total, nil
}

// haversineKm calculates the distance between two geographic coordinates in km.
func haversineKm(lat1, lng1, lat2, lng2 float64) float64 {
	const R = 6371.0 // Earth radius in km
	dLat := (lat2 - lat1) * math.Pi / 180
	dLng := (lng2 - lng1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dLng/2)*math.Sin(dLng/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return R * c
}

// IsOutletOpen checks whether an outlet is currently open based on its opening_hours JSON.
// Expected formats:
//
//	{ "monday": { "open": "08:00", "close": "22:00" }, ... }
//	{ "monday": [{ "open": "08:00", "close": "14:00" }, { "open": "17:00", "close": "22:00" }], ... }
//
// Returns true if opening_hours is nil/empty (assume always open).
func IsOutletOpen(openingHours map[string]any) bool {
	if len(openingHours) == 0 {
		return true // no schedule = always open
	}

	now := time.Now()
	dayName := strings.ToLower(now.Weekday().String())
	currentMinutes := now.Hour()*60 + now.Minute()

	dayData, ok := openingHours[dayName]
	if !ok {
		return false // day not listed = closed
	}

	// Single period: { "open": "08:00", "close": "22:00" }
	if m, ok := dayData.(map[string]any); ok {
		return isWithinPeriod(m, currentMinutes)
	}

	// Multiple periods: [{ "open": "08:00", "close": "14:00" }, ...]
	if arr, ok := dayData.([]any); ok {
		for _, item := range arr {
			if m, ok := item.(map[string]any); ok {
				if isWithinPeriod(m, currentMinutes) {
					return true
				}
			}
		}
	}

	return false
}

func isWithinPeriod(period map[string]any, currentMinutes int) bool {
	openStr, _ := period["open"].(string)
	closeStr, _ := period["close"].(string)
	if openStr == "" || closeStr == "" {
		return false
	}
	openMin := parseTimeToMinutes(openStr)
	closeMin := parseTimeToMinutes(closeStr)
	if openMin < 0 || closeMin < 0 {
		return false
	}
	// Handle overnight spans (e.g. open 22:00, close 02:00)
	if closeMin < openMin {
		return currentMinutes >= openMin || currentMinutes < closeMin
	}
	return currentMinutes >= openMin && currentMinutes < closeMin
}

func parseTimeToMinutes(t string) int {
	parts := strings.Split(t, ":")
	if len(parts) != 2 {
		return -1
	}
	h, m := 0, 0
	for _, c := range parts[0] {
		h = h*10 + int(c-'0')
	}
	for _, c := range parts[1] {
		m = m*10 + int(c-'0')
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return -1
	}
	return h*60 + m
}
