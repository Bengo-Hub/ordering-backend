package ordering

import "strings"

// outletLocationPayload builds the outlet_location block of ordering.order.ready, the pickup point
// logistics sends the rider to. The name, address and phone always go out, because a rider can
// find and call an outlet that has never been pinned on a map. Coordinates are added only when
// both are set. It returns nil when the outlet has nothing at all to describe it.
func outletLocationPayload(loc OutletLocation) map[string]interface{} {
	out := map[string]interface{}{}
	if v := strings.TrimSpace(loc.Name); v != "" {
		out["name"] = v
	}
	if v := strings.TrimSpace(loc.Address); v != "" {
		out["address"] = v
	}
	if v := strings.TrimSpace(loc.Phone); v != "" {
		out["phone"] = v
	}
	if loc.Latitude != nil && loc.Longitude != nil {
		out["latitude"] = *loc.Latitude
		out["longitude"] = *loc.Longitude
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
