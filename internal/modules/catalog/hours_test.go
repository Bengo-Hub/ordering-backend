package catalog

import (
	"testing"
	"time"

	sharedcache "github.com/Bengo-Hub/cache"
)

func TestScheduleOutletOverridesTenant(t *testing.T) {
	nbo := sharedcache.TimezoneLocation("Africa/Nairobi")
	tenant := Schedule{
		Hours: sharedcache.ParseOpeningHours(map[string]any{"monday": map[string]any{"open": "07:30", "close": "22:30"}}),
		Loc:   nbo,
	}
	// 22:00 Monday in Busia is 19:00 UTC: open on the tenant's hours.
	at := time.Date(2026, 11, 23, 19, 0, 0, 0, time.UTC)
	if !tenant.forOutlet(nil).IsOpenAt(at) {
		t.Fatal("tenant hours should be open at 22:00 EAT")
	}
	own := tenant.forOutlet(map[string]any{"monday": map[string]any{"open": "09:00", "close": "17:00"}})
	if own.IsOpenAt(at) {
		t.Fatal("the outlet's own hours should win over the tenant's")
	}
	if own.Loc != nbo {
		t.Fatal("an outlet override keeps the tenant's zone")
	}
}

func TestGenerateTimeSlotsInTenantZone(t *testing.T) {
	nbo := sharedcache.TimezoneLocation("Africa/Nairobi")
	sc := Schedule{
		Hours: sharedcache.ParseOpeningHours(map[string]any{
			"monday": map[string]any{"open": "07:30", "close": "22:30"},
			"friday": map[string]any{"open": "18:00", "close": "02:00"},
		}),
		Loc: nbo,
	}
	monday := time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC)
	// Now: Monday 21:00 EAT (18:00 UTC), 45 minutes lead, 30-minute slots: 22:00 is the first free one.
	now := time.Date(2026, 11, 23, 18, 0, 0, 0, time.UTC)
	slots := generateTimeSlotsAt(sc, monday, 30, 45, now)
	if len(slots) == 0 || slots[0].Start != "07:30" || slots[len(slots)-1].End != "22:30" {
		t.Fatalf("Monday slots = %v", slots)
	}
	for _, s := range slots {
		if want := s.Start >= "22:00"; s.Available != want {
			t.Errorf("slot %s available=%v, want %v", s.Start, s.Available, want)
		}
	}
	if got := generateTimeSlotsAt(sc, monday.AddDate(0, 0, 1), 30, 45, now); got != nil {
		t.Errorf("Tuesday is closed, got %v", got)
	}
	friday := generateTimeSlotsAt(sc, time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC), 60, 45, now)
	if len(friday) != 6 || friday[len(friday)-1].End != "24:00" {
		t.Errorf("Friday overnight slots = %v", friday)
	}
	if got := generateTimeSlotsAt(Schedule{Loc: nbo}, monday, 60, 45, now); len(got) != 14 {
		t.Errorf("no hours set: %d slots, want 14 (08:00 to 22:00)", len(got))
	}
}
