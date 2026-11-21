package catalog

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	sharedcache "github.com/Bengo-Hub/cache"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/ent"
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
	Category        string   // filter by use_case
	Offers          bool     // only outlets with active promo codes
	MinRating       *float64 // minimum average rating
	MaxDeliveryFee  *float64 // maximum delivery fee
	MaxDeliveryTime *int     // maximum estimated delivery time in minutes
	Pickup          bool     // only outlets that support pickup
	Query           string   // text search on outlet name
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
	IsOpen        bool      `json:"isOpen"`
	AverageRating float64   `json:"average_rating"`
	TotalRatings  int       `json:"total_ratings"`
	DeliveryFee   float64   `json:"delivery_fee"`
	EstimatedTime int       `json:"estimated_time_minutes"`
	Distance      float64   `json:"distance_km,omitempty"`
	HasOffers     bool      `json:"has_offers"`
	UseCase       string    `json:"use_case,omitempty"`
	PromoBadge    string    `json:"promo_badge,omitempty"`
	PromoText     string    `json:"promo_text,omitempty"`
	// Deliverable is set when the customer's pin is known: false means this outlet does
	// not deliver there.
	Deliverable *bool `json:"deliverable,omitempty"`
	// AtOutlet: the customer's pin is at this outlet (pickup at the counter, not a refusal).
	AtOutlet bool `json:"at_outlet,omitempty"`
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

	// 3. Delivery fee, ETA and distance come from logistics: a quote per outlet for the
	// customer's pin, or the tenant's cheapest area fee when no pin is known yet.
	deliveryByOutlet := s.deliveryForOutlets(ctx, tenantID, outlets, params.Lat, params.Lng)

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
	// Also track best promo badge per outlet for storefront display
	hasGlobalOffer := false
	outletOfferSet := make(map[uuid.UUID]bool)
	type promoBadgeInfo struct {
		Badge string // e.g. "-15%"
		Text  string // e.g. "Free delivery"
	}
	var globalPromoBadge *promoBadgeInfo
	outletPromoBadgeMap := make(map[uuid.UUID]*promoBadgeInfo)
	for _, p := range promos {
		badge := &promoBadgeInfo{}
		switch p.DiscountType {
		case promocode.DiscountTypePercentage:
			badge.Badge = fmt.Sprintf("-%.0f%%", p.DiscountValue)
		case promocode.DiscountTypeFreeDelivery:
			badge.Text = "Free delivery"
		case promocode.DiscountTypeFixedAmount:
			badge.Badge = fmt.Sprintf("-%.0f", p.DiscountValue)
		}

		if p.OutletID == nil {
			hasGlobalOffer = true
			if globalPromoBadge == nil {
				globalPromoBadge = badge
			}
		} else {
			outletOfferSet[*p.OutletID] = true
			if existing, ok := outletPromoBadgeMap[*p.OutletID]; !ok || existing.Badge == "" {
				outletPromoBadgeMap[*p.OutletID] = badge
			}
		}
	}

	// 5. Enrich each outlet. Opening hours: the outlet's own, else the tenant's central hours,
	// read in the tenant's timezone (resolved once for the whole listing).
	schedule := s.tenantSchedule(ctx, tenantID)
	var results []OutletWithMeta
	for _, o := range outlets {
		meta := OutletWithMeta{
			ID:       o.ID,
			Name:     o.Name,
			Slug:     o.Slug,
			ImageURL: s.media.URL(o.ImageURL),
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
		meta.IsOpen = schedule.forOutlet(o.OpeningHours).IsOpenAt(now)

		// Rating
		if r, ok := ratingMap[o.ID]; ok {
			meta.AverageRating = r.AverageRating
			meta.TotalRatings = r.TotalRatings
		}

		// Delivery fee, time and distance (from logistics)
		if d, ok := deliveryByOutlet[o.ID]; ok {
			meta.DeliveryFee = d.Fee
			meta.EstimatedTime = d.EtaMinutes
			meta.Distance = d.DistanceKm
			meta.Deliverable = d.Deliverable
			meta.AtOutlet = d.AtOutlet
		}

		// Offers
		meta.HasOffers = hasGlobalOffer || outletOfferSet[o.ID]

		// Promo badges
		if badge, ok := outletPromoBadgeMap[o.ID]; ok {
			meta.PromoBadge = badge.Badge
			meta.PromoText = badge.Text
		} else if globalPromoBadge != nil {
			meta.PromoBadge = globalPromoBadge.Badge
			meta.PromoText = globalPromoBadge.Text
		}

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

// TimeSlot represents a deliverable time window.
type TimeSlot struct {
	Start     string `json:"start"` // HH:MM
	End       string `json:"end"`   // HH:MM
	Available bool   `json:"available"`
}

// noHoursSlotWindow is the slot window offered when neither the outlet nor the tenant has set
// opening hours (the outlet counts as always open, but slots through the night make no sense).
var noHoursSlotWindow = sharedcache.OpeningPeriod{Open: 8 * 60, Close: 22 * 60}

// GenerateTimeSlots generates delivery time slots for an outlet on a calendar date, inside the
// outlet's schedule (its own hours, else the tenant's) and read in the tenant's timezone.
// slotMinutes controls the slot duration (default 30). leadMinutes is the minimum prep time from
// now (default 45). A period that runs past midnight offers slots until midnight on the date.
func GenerateTimeSlots(sc Schedule, date time.Time, slotMinutes, leadMinutes int) []TimeSlot {
	return generateTimeSlotsAt(sc, date, slotMinutes, leadMinutes, time.Now())
}

func generateTimeSlotsAt(sc Schedule, date time.Time, slotMinutes, leadMinutes int, now time.Time) []TimeSlot {
	if slotMinutes <= 0 {
		slotMinutes = 30
	}
	if leadMinutes <= 0 {
		leadMinutes = 45
	}
	loc := sc.Loc
	if loc == nil {
		loc = sharedcache.TimezoneLocation("")
	}

	// The calendar date as the customer means it, in the tenant's zone.
	date = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, loc)
	now = now.In(loc)
	isToday := date.Year() == now.Year() && date.YearDay() == now.YearDay()

	// Earliest available minute for today (now + lead time), rounded up to a slot boundary.
	earliestMinute := 0
	if isToday {
		earliestMinute = now.Hour()*60 + now.Minute() + leadMinutes
		earliestMinute = ((earliestMinute + slotMinutes - 1) / slotMinutes) * slotMinutes
	}

	periods := []sharedcache.OpeningPeriod{noHoursSlotWindow}
	if sc.Hours != nil {
		periods = sc.Hours.PeriodsOn(date)
		if len(periods) == 0 {
			return nil // closed on this day
		}
	}

	var slots []TimeSlot
	for _, p := range periods {
		closeAt := p.Close
		if closeAt <= p.Open {
			closeAt = 24 * 60 // runs past midnight: slots until the end of this date
		}
		for start := p.Open; start+slotMinutes <= closeAt; start += slotMinutes {
			end := start + slotMinutes
			available := start >= earliestMinute
			slots = append(slots, TimeSlot{
				Start:     minutesToTime(start),
				End:       minutesToTime(end),
				Available: available,
			})
		}
	}
	return slots
}

func minutesToTime(m int) string {
	h := m / 60
	min := m % 60
	return fmt.Sprintf("%02d:%02d", h, min)
}
