package catalog

import (
	"context"
	"time"

	sharedcache "github.com/Bengo-Hub/cache"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Opening hours are owned by auth-api (tenant metadata opening_hours, with the tenant's timezone)
// and read through the shared tenant cache. An outlet may override them with its own
// outlets.opening_hours in the same shape. The clock is always read in the tenant's zone: pods run
// in UTC, so the server clock would open a Busia cafe three hours late.

// Schedule is the opening hours that apply to one outlet and the zone they are read in.
type Schedule struct {
	Hours sharedcache.OpeningHours
	Loc   *time.Location
}

// IsOpenAt reports whether the schedule is open at t. No hours set means always open.
func (sc Schedule) IsOpenAt(t time.Time) bool {
	return sc.Hours.IsOpenAt(t, sc.Loc)
}

// SetAuthURL wires the auth-api base URL the shared tenant cache fetches tenant details from
// (in-cluster Service URL).
func (s *ProxyService) SetAuthURL(u string) { s.authURL = u }

// tenantDetails returns the tenant's cached auth-api record, or false when it cannot be read (the
// caller then falls back to the outlet's own hours and the default zone).
func (s *ProxyService) tenantDetails(ctx context.Context, tenantID uuid.UUID) (sharedcache.TenantDetails, bool) {
	if s.cache == nil || s.authURL == "" || s.db == nil {
		return sharedcache.TenantDetails{}, false
	}
	t, err := s.db.Tenant.Get(ctx, tenantID)
	if err != nil || t.Slug == "" {
		return sharedcache.TenantDetails{}, false
	}
	d, err := sharedcache.GetTenantDetails(ctx, s.cache, s.authURL, t.Slug, sharedcache.DefaultTenantTTL)
	if err != nil {
		s.logger.Debug("tenant details unavailable for opening hours", zap.String("tenant", t.Slug), zap.Error(err))
		return sharedcache.TenantDetails{}, false
	}
	return d, true
}

// tenantSchedule resolves the tenant-wide hours and zone once, for applying to many outlets.
func (s *ProxyService) tenantSchedule(ctx context.Context, tenantID uuid.UUID) Schedule {
	d, ok := s.tenantDetails(ctx, tenantID)
	if !ok {
		return Schedule{Loc: sharedcache.TimezoneLocation("")}
	}
	return Schedule{Hours: d.TenantOpeningHours(), Loc: d.TenantLocation()}
}

// forOutlet applies an outlet's own hours over the tenant's: the outlet override wins when it has
// any day set, otherwise the tenant's central hours apply. The zone is always the tenant's.
func (sc Schedule) forOutlet(outletHours map[string]any) Schedule {
	if own := sharedcache.ParseOpeningHours(outletHours); own != nil {
		return Schedule{Hours: own, Loc: sc.Loc}
	}
	return sc
}

// OutletSchedule is the schedule for one outlet: its own hours, else the tenant's.
func (s *ProxyService) OutletSchedule(ctx context.Context, tenantID uuid.UUID, outletHours map[string]any) Schedule {
	return s.tenantSchedule(ctx, tenantID).forOutlet(outletHours)
}

// withOpenState sets IsOpen on outlet summaries at read time, so a cached listing never carries a
// stale open flag.
func (s *ProxyService) withOpenState(ctx context.Context, tenantID uuid.UUID, outlets []OutletSummary) []OutletSummary {
	if len(outlets) == 0 {
		return outlets
	}
	base := s.tenantSchedule(ctx, tenantID)
	now := time.Now()
	out := make([]OutletSummary, len(outlets))
	for i, o := range outlets {
		o.IsOpen = base.forOutlet(o.OpeningHours).IsOpenAt(now)
		out[i] = o
	}
	return out
}
