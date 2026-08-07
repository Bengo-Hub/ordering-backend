package catalog

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/platform/inventory"
)

// PublicEventTier is a sellable ticket tier with live remaining availability.
type PublicEventTier struct {
	TierID    string  `json:"tier_id"`
	Name      string  `json:"name"`
	Price     float64 `json:"price"`
	Capacity  int     `json:"capacity"`
	Remaining int     `json:"remaining"`
	SoldOut   bool    `json:"sold_out"`
}

// PublicEvent is the tenant-scoped public view of an event for the storefront page.
type PublicEvent struct {
	ID            uuid.UUID         `json:"id"`
	SKU           string            `json:"sku"`
	Name          string            `json:"name"`
	Description   string            `json:"description,omitempty"`
	ImageURL      string            `json:"image_url,omitempty"`
	EventStartAt  *time.Time        `json:"event_start_at,omitempty"`
	EventEndAt    *time.Time        `json:"event_end_at,omitempty"`
	Venue         string            `json:"venue,omitempty"`
	TotalCapacity int               `json:"total_capacity"`
	Remaining     int               `json:"remaining"`
	SoldOut       bool              `json:"sold_out"`
	Tiers         []PublicEventTier `json:"tiers"`
	ShareURL      string            `json:"share_url,omitempty"`
}

// ListEvents returns the tenant's SERVICE-type events for public browsing (capacity summary only).
func (s *ProxyService) ListEvents(ctx context.Context, tenantSlug string, limit, page int) ([]PublicEvent, int, error) {
	evs, total, err := s.inventoryClient.ListEvents(ctx, tenantSlug, limit, page)
	if err != nil {
		return nil, 0, fmt.Errorf("catalog: list events: %w", err)
	}
	out := make([]PublicEvent, 0, len(evs))
	for i := range evs {
		ev := &evs[i]
		total, remaining := capacity(ev.TotalCapacity, ev.BookedCapacity)
		venue := ""
		if ev.EventVenue != nil {
			venue = *ev.EventVenue
		}
		out = append(out, PublicEvent{
			ID:            ev.ID,
			SKU:           ev.SKU,
			Name:          ev.Name,
			Description:   ev.Description,
			ImageURL:      ev.ImageURL,
			EventStartAt:  ev.EventStartAt,
			EventEndAt:    ev.EventEndAt,
			Venue:         venue,
			TotalCapacity: total,
			Remaining:     remaining,
			SoldOut:       total > 0 && remaining <= 0,
		})
	}
	return out, total, nil
}

// GetEventDetail returns a single public event with live per-tier availability.
func (s *ProxyService) GetEventDetail(ctx context.Context, tenantSlug, eventID string) (*PublicEvent, error) {
	id, err := uuid.Parse(eventID)
	if err != nil {
		return nil, fmt.Errorf("catalog: invalid event id")
	}
	// inventory-api clamps any requested limit to its shared pagination cap (100), so a single
	// large-limit request silently truncates the list past the first ~100 events — page through
	// all of it, or an event beyond page 1 would incorrectly 404 here.
	evs, err := s.fetchAllEvents(ctx, tenantSlug)
	if err != nil {
		return nil, fmt.Errorf("catalog: load events: %w", err)
	}
	var found *PublicEvent
	for i := range evs {
		if evs[i].ID == id {
			ev := &evs[i]
			total, remaining := capacity(ev.TotalCapacity, ev.BookedCapacity)
			venue := ""
			if ev.EventVenue != nil {
				venue = *ev.EventVenue
			}
			found = &PublicEvent{
				ID:            ev.ID,
				SKU:           ev.SKU,
				Name:          ev.Name,
				Description:   ev.Description,
				ImageURL:      ev.ImageURL,
				EventStartAt:  ev.EventStartAt,
				EventEndAt:    ev.EventEndAt,
				Venue:         venue,
				TotalCapacity: total,
				Remaining:     remaining,
				SoldOut:       total > 0 && remaining <= 0,
				Tiers:         []PublicEventTier{},
			}
			break
		}
	}
	if found == nil {
		return nil, fmt.Errorf("catalog: event not found")
	}

	// Live per-tier availability (best-effort; event still renders if this fails).
	if av, aerr := s.inventoryClient.GetEventAvailability(ctx, tenantSlug, eventID); aerr == nil && av != nil {
		found.TotalCapacity = av.TotalCapacity
		found.Remaining = av.Remaining
		found.SoldOut = av.TotalCapacity > 0 && av.Remaining <= 0
		for _, t := range av.Tiers {
			found.Tiers = append(found.Tiers, PublicEventTier{
				TierID:    t.TierID,
				Name:      t.Name,
				Price:     t.Price,
				Capacity:  t.Capacity,
				Remaining: t.Remaining,
				SoldOut:   t.Capacity > 0 && t.Remaining <= 0,
			})
		}
	} else if aerr != nil {
		s.logger.Warn("catalog: event availability fetch failed", zap.String("event_id", eventID), zap.Error(aerr))
	}
	return found, nil
}

// fetchAllEvents pages through inventory-api's events listing until every row is retrieved.
// See fetchAllInventoryItems (proxy_service.go) for why a single large-limit request is unsafe.
func (s *ProxyService) fetchAllEvents(ctx context.Context, tenantSlug string) ([]inventory.EventResponse, error) {
	events := make([]inventory.EventResponse, 0, 2*inventoryPageSize)
	for page := 1; page <= 50; page++ { // runaway guard: 50 pages = 5k events
		pageEvents, total, err := s.inventoryClient.ListEvents(ctx, tenantSlug, inventoryPageSize, page)
		if err != nil {
			return nil, err
		}
		events = append(events, pageEvents...)
		if len(pageEvents) < inventoryPageSize || len(events) >= total {
			break
		}
	}
	return events, nil
}

func capacity(total, booked *int) (int, int) {
	t := 0
	if total != nil {
		t = *total
	}
	b := 0
	if booked != nil {
		b = *booked
	}
	rem := t - b
	if rem < 0 {
		rem = 0
	}
	return t, rem
}
