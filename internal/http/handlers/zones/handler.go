package zoneshandler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/Bengo-Hub/httpware"
	"github.com/bengobox/ordering-backend/internal/ent"
	"github.com/bengobox/ordering-backend/internal/ent/deliveryzone"
	"github.com/bengobox/ordering-backend/internal/http/handlers"
	identityhandler "github.com/bengobox/ordering-backend/internal/http/handlers/identity"
	"github.com/bengobox/ordering-backend/internal/modules/identity"
)

// Handler manages delivery zone endpoints.
type Handler struct {
	log    *zap.Logger
	client *ent.Client
}

// New creates a delivery zone handler.
func New(log *zap.Logger, client *ent.Client) *Handler {
	return &Handler{log: log.Named("zones.Handler"), client: client}
}

// Register mounts delivery zone routes on the supplied router.
func (h *Handler) Register(r chi.Router, auth *identityhandler.Authenticator) {
	r.Route("/zones", func(zr chi.Router) {
		// Public endpoints (no auth — used by guest checkout flow)
		zr.Get("/", h.ListZones)
		zr.Get("/check", h.CheckAvailabilityPublic) // must be before /{id}
		zr.Get("/{id}", h.GetZone)

		// Admin: manage zones (requires auth + permissions)
		zr.Group(func(admin chi.Router) {
			admin.Use(auth.RequireAuth)
			admin.With(auth.RequirePermissions(identity.PermissionZonesManage)).Post("/", h.CreateZone)
			admin.With(auth.RequirePermissions(identity.PermissionZonesManage)).Put("/{id}", h.UpdateZone)
			admin.With(auth.RequirePermissions(identity.PermissionZonesManage)).Delete("/{id}", h.DeleteZone)
			admin.With(auth.RequirePermissions(identity.PermissionZonesView)).Post("/check", h.CheckAvailability)
		})
	})
}

type createZoneRequest struct {
	OutletID             *uuid.UUID     `json:"outletId,omitempty"`
	Name                 string         `json:"name"`
	Slug                 string         `json:"slug,omitempty"`
	ZonePolygon          map[string]any `json:"zonePolygon,omitempty"`
	DeliveryFee          float64        `json:"deliveryFee"`
	MinimumOrder         float64        `json:"minimumOrder"`
	EstimatedTimeMinutes int            `json:"estimatedTimeMinutes"`
	IsActive             bool           `json:"isActive"`
	SortOrder            int            `json:"sortOrder"`
}

// ListZones returns active delivery zones for the tenant.
func (h *Handler) ListZones(w http.ResponseWriter, r *http.Request) {
	tenantID, err := h.getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}

	query := h.client.DeliveryZone.Query().
		Where(deliveryzone.TenantID(tenantID), deliveryzone.IsActive(true)).
		Order(ent.Asc(deliveryzone.FieldSortOrder))

	if outletIDStr := r.URL.Query().Get("outlet_id"); outletIDStr != "" {
		oid, err := uuid.Parse(outletIDStr)
		if err == nil {
			query = query.Where(deliveryzone.OutletID(oid))
		}
	}

	zones, err := query.All(r.Context())
	if err != nil {
		h.log.Error("list zones failed", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "internal error")
		return
	}

	handlers.RespondJSON(w, http.StatusOK, map[string]any{
		"data":  zones,
		"total": len(zones),
	})
}

// GetZone returns a single zone.
func (h *Handler) GetZone(w http.ResponseWriter, r *http.Request) {
	tenantID, err := h.getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}

	zoneID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid zone id")
		return
	}

	zone, err := h.client.DeliveryZone.Query().
		Where(deliveryzone.ID(zoneID), deliveryzone.TenantID(tenantID)).
		Only(r.Context())
	if err != nil {
		if ent.IsNotFound(err) {
			handlers.RespondError(w, http.StatusNotFound, "zone not found")
			return
		}
		handlers.RespondError(w, http.StatusInternalServerError, "internal error")
		return
	}

	handlers.RespondJSON(w, http.StatusOK, zone)
}

// CreateZone creates a new delivery zone.
func (h *Handler) CreateZone(w http.ResponseWriter, r *http.Request) {
	tenantID, err := h.getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}

	var req createZoneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		handlers.RespondError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.EstimatedTimeMinutes <= 0 {
		req.EstimatedTimeMinutes = 30
	}

	builder := h.client.DeliveryZone.Create().
		SetTenantID(tenantID).
		SetName(req.Name).
		SetSlug(req.Slug).
		SetDeliveryFee(req.DeliveryFee).
		SetMinimumOrder(req.MinimumOrder).
		SetEstimatedTimeMinutes(req.EstimatedTimeMinutes).
		SetIsActive(req.IsActive).
		SetSortOrder(req.SortOrder)

	if req.OutletID != nil {
		builder.SetOutletID(*req.OutletID)
	}
	if req.ZonePolygon != nil {
		builder.SetZonePolygon(req.ZonePolygon)
	}

	zone, err := builder.Save(r.Context())
	if err != nil {
		h.log.Error("create zone failed", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "failed to create zone")
		return
	}

	handlers.RespondJSON(w, http.StatusCreated, zone)
}

// UpdateZone updates an existing delivery zone.
func (h *Handler) UpdateZone(w http.ResponseWriter, r *http.Request) {
	tenantID, err := h.getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}

	zoneID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid zone id")
		return
	}

	zone, err := h.client.DeliveryZone.Query().
		Where(deliveryzone.ID(zoneID), deliveryzone.TenantID(tenantID)).
		Only(r.Context())
	if err != nil {
		handlers.RespondError(w, http.StatusNotFound, "zone not found")
		return
	}

	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	updater := zone.Update()
	if v, ok := req["name"].(string); ok {
		updater.SetName(v)
	}
	if v, ok := req["slug"].(string); ok {
		updater.SetSlug(v)
	}
	if v, ok := req["deliveryFee"].(float64); ok {
		updater.SetDeliveryFee(v)
	}
	if v, ok := req["minimumOrder"].(float64); ok {
		updater.SetMinimumOrder(v)
	}
	if v, ok := req["estimatedTimeMinutes"].(float64); ok {
		updater.SetEstimatedTimeMinutes(int(v))
	}
	if v, ok := req["isActive"].(bool); ok {
		updater.SetIsActive(v)
	}
	if v, ok := req["zonePolygon"].(map[string]any); ok {
		updater.SetZonePolygon(v)
	}

	updated, err := updater.Save(r.Context())
	if err != nil {
		handlers.RespondError(w, http.StatusInternalServerError, "update failed")
		return
	}

	handlers.RespondJSON(w, http.StatusOK, updated)
}

// DeleteZone soft-deletes a zone.
func (h *Handler) DeleteZone(w http.ResponseWriter, r *http.Request) {
	tenantID, err := h.getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}

	zoneID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid zone id")
		return
	}

	_, err = h.client.DeliveryZone.Update().
		Where(deliveryzone.ID(zoneID), deliveryzone.TenantID(tenantID)).
		SetIsActive(false).
		Save(r.Context())
	if err != nil {
		handlers.RespondError(w, http.StatusInternalServerError, "delete failed")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// CheckAvailability checks if a given lat/lng is within an active delivery zone.
func (h *Handler) CheckAvailability(w http.ResponseWriter, r *http.Request) {
	tenantID, err := h.getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}

	var req struct {
		Latitude  float64    `json:"latitude"`
		Longitude float64    `json:"longitude"`
		OutletID  *uuid.UUID `json:"outletId,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	query := h.client.DeliveryZone.Query().
		Where(deliveryzone.TenantID(tenantID), deliveryzone.IsActive(true))

	if req.OutletID != nil {
		query = query.Where(deliveryzone.OutletID(*req.OutletID))
	}

	zones, err := query.All(r.Context())
	if err != nil {
		handlers.RespondError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Point-in-polygon check: find the zone that contains the given coordinates.
	// Polygon matches take priority over fallback (no-polygon) zones.
	var bestZone *ent.DeliveryZone
	var fallbackZone *ent.DeliveryZone
	for _, zone := range zones {
		if zone.ZonePolygon == nil {
			// Zone without polygon: treat as universal (fallback zone)
			if fallbackZone == nil {
				fallbackZone = zone
			}
			continue
		}
		if pointInGeoJSONPolygon(req.Latitude, req.Longitude, zone.ZonePolygon) {
			// Prefer the zone with the lowest delivery fee if multiple match
			if bestZone == nil || zone.DeliveryFee < bestZone.DeliveryFee {
				bestZone = zone
			}
		}
	}
	if bestZone == nil {
		bestZone = fallbackZone
	}
	serviceable := bestZone != nil

	result := map[string]any{
		"serviceable": serviceable,
		"latitude":    req.Latitude,
		"longitude":   req.Longitude,
	}
	if bestZone != nil {
		result["zone"] = map[string]any{
			"id":                   bestZone.ID,
			"name":                 bestZone.Name,
			"deliveryFee":          bestZone.DeliveryFee,
			"minimumOrder":         bestZone.MinimumOrder,
			"estimatedTimeMinutes": bestZone.EstimatedTimeMinutes,
		}
	}

	handlers.RespondJSON(w, http.StatusOK, result)
}

// CheckAvailabilityPublic is a public GET endpoint for checking delivery zone availability.
// Used by the guest checkout flow — no auth required.
// Accepts query params: lat, lng, outlet_id (optional).
// Returns a flat response matching the frontend's ZoneCheckResult interface.
func (h *Handler) CheckAvailabilityPublic(w http.ResponseWriter, r *http.Request) {
	tenantID, err := h.getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}

	latStr := r.URL.Query().Get("lat")
	lngStr := r.URL.Query().Get("lng")
	if latStr == "" || lngStr == "" {
		handlers.RespondError(w, http.StatusBadRequest, "lat and lng query parameters are required")
		return
	}

	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid lat value")
		return
	}
	lng, err := strconv.ParseFloat(lngStr, 64)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid lng value")
		return
	}

	query := h.client.DeliveryZone.Query().
		Where(deliveryzone.TenantID(tenantID), deliveryzone.IsActive(true))

	if outletIDStr := r.URL.Query().Get("outlet_id"); outletIDStr != "" {
		if oid, err := uuid.Parse(outletIDStr); err == nil {
			query = query.Where(deliveryzone.OutletID(oid))
		}
	}

	zones, err := query.All(r.Context())
	if err != nil {
		handlers.RespondError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Point-in-polygon check with fallback to no-polygon zones
	var bestZone *ent.DeliveryZone
	var fallbackZone *ent.DeliveryZone
	for _, zone := range zones {
		if zone.ZonePolygon == nil {
			if fallbackZone == nil {
				fallbackZone = zone
			}
			continue
		}
		if pointInGeoJSONPolygon(lat, lng, zone.ZonePolygon) {
			if bestZone == nil || zone.DeliveryFee < bestZone.DeliveryFee {
				bestZone = zone
			}
		}
	}
	if bestZone == nil {
		bestZone = fallbackZone
	}

	if bestZone != nil {
		// Zone match found — return zone fee
		handlers.RespondJSON(w, http.StatusOK, map[string]any{
			"serviceable":    true,
			"zone_id":        bestZone.ID.String(),
			"delivery_fee":   bestZone.DeliveryFee,
			"min_order":      bestZone.MinimumOrder,
			"estimated_time": bestZone.EstimatedTimeMinutes,
		})
		return
	}

	// No zone match — return serviceable with auto-calculated fallback fee.
	// The actual per-km fee will be calculated at checkout using the outlet's
	// coordinates. Here we return the base fee as an estimate.
	handlers.RespondJSON(w, http.StatusOK, map[string]any{
		"serviceable":    true,
		"zone_id":        "",
		"delivery_fee":   100.0, // Base fee — actual fee calculated at checkout
		"min_order":      0,
		"estimated_time": 30,
		"auto_calculated": true,
	})
}

// pointInGeoJSONPolygon checks if a lat/lng point is inside a GeoJSON polygon.
// Uses the ray-casting algorithm for point-in-polygon containment.
// Supports GeoJSON Polygon type with coordinates as [[[lng, lat], ...]] format.
func pointInGeoJSONPolygon(lat, lng float64, polygon map[string]interface{}) bool {
	coords, ok := polygon["coordinates"]
	if !ok {
		return false
	}

	// GeoJSON Polygon coordinates: [[[lng, lat], [lng, lat], ...]]
	rings, ok := coords.([]interface{})
	if !ok || len(rings) == 0 {
		return false
	}

	// Use only the outer ring (first ring); holes not supported for simplicity
	outerRing, ok := rings[0].([]interface{})
	if !ok || len(outerRing) < 3 {
		return false
	}

	// Parse coordinate pairs
	points := make([][2]float64, 0, len(outerRing))
	for _, pt := range outerRing {
		pair, ok := pt.([]interface{})
		if !ok || len(pair) < 2 {
			continue
		}
		pLng, okLng := toFloat64(pair[0])
		pLat, okLat := toFloat64(pair[1])
		if !okLng || !okLat {
			continue
		}
		points = append(points, [2]float64{pLng, pLat})
	}

	if len(points) < 3 {
		return false
	}

	return raycast(lng, lat, points)
}

// raycast implements the ray-casting algorithm for point-in-polygon.
// testX, testY is the point; polygon is a slice of [x, y] coordinate pairs.
func raycast(testX, testY float64, polygon [][2]float64) bool {
	n := len(polygon)
	inside := false
	j := n - 1
	for i := 0; i < n; i++ {
		xi, yi := polygon[i][0], polygon[i][1]
		xj, yj := polygon[j][0], polygon[j][1]
		if ((yi > testY) != (yj > testY)) &&
			(testX < (xj-xi)*(testY-yi)/(yj-yi)+xi) {
			inside = !inside
		}
		j = i
	}
	return inside
}

// toFloat64 safely converts an interface{} to float64.
func toFloat64(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

func (h *Handler) getTenantID(r *http.Request) (uuid.UUID, error) {
	tenantIDStr := httpware.GetTenantID(r.Context())
	if tenantIDStr == "" {
		// Try URL param
		tenantIDStr = chi.URLParam(r, "tenant")
	}
	return uuid.Parse(tenantIDStr)
}
