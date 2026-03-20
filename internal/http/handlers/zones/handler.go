package zoneshandler

import (
	"encoding/json"
	"net/http"

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
		// Public: list active zones for an outlet
		zr.Get("/", h.ListZones)
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

	// For MVP: return all active zones for the tenant/outlet.
	// Full geo-fencing (point-in-polygon check) is a post-MVP enhancement.
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

	serviceable := len(zones) > 0
	var bestZone *ent.DeliveryZone
	if serviceable {
		bestZone = zones[0]
	}

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

func (h *Handler) getTenantID(r *http.Request) (uuid.UUID, error) {
	tenantIDStr := httpware.GetTenantID(r.Context())
	if tenantIDStr == "" {
		// Try URL param
		tenantIDStr = chi.URLParam(r, "tenant")
	}
	return uuid.Parse(tenantIDStr)
}
