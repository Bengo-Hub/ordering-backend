package paymentshandler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/Bengo-Hub/httpware"
	"github.com/bengobox/ordering-backend/internal/http/handlers"
	identityhandler "github.com/bengobox/ordering-backend/internal/http/handlers/identity"
	"github.com/bengobox/ordering-backend/internal/modules/identity"
	"github.com/bengobox/ordering-backend/internal/platform/treasury"
)

// Permissions for tenant gateway management. These map to treasury-backed payment
// configuration: view to read available/selected gateways, manage to select/deactivate.
const (
	permGatewayView   = identity.Permission("ordering.config.view")
	permGatewayManage = identity.Permission("ordering.config.manage")
)

// AvailableGatewayResponse is one entry of the available-gateways proxy response. Mirrors
// treasury-api's S2S available-gateway shape.
type AvailableGatewayResponse struct {
	GatewayType        string `json:"gateway_type"`
	Name               string `json:"name"`
	TransactionFeeType string `json:"transaction_fee_type"`
	SupportsStkPush    bool   `json:"supports_stk_push"`
}

// AvailableGatewaysResponse is the proxy payload for GET /payments/gateways/available.
type AvailableGatewaysResponse struct {
	Gateways []AvailableGatewayResponse `json:"gateways"`
}

// SelectedGatewaysResponse is the proxy payload for GET /payments/gateways/selected. The
// selected entries are forwarded from treasury (including provider url fields).
type SelectedGatewaysResponse struct {
	Selected []treasury.SelectedGateway `json:"selected"`
}

// SelectGatewayRequest is the body for POST /payments/gateways/select/{gatewayType}.
type SelectGatewayRequest struct {
	IsPrimary bool `json:"is_primary"`
}

// gatewayActionResponse is the {message} payload returned by select/deactivate.
type gatewayActionResponse struct {
	Message string `json:"message"`
}

// RegisterGateways mounts the tenant gateway-management proxy routes under /payments/gateways.
// These proxy to treasury-api's S2S gateway routes (X-API-Key auth) using the treasury client.
// Routes are tenant-scoped (mounted under /api/v1/{tenant}) and guarded by config permissions.
func (h *PaymentMethodHandler) RegisterGateways(r chi.Router, auth *identityhandler.Authenticator) {
	r.Route("/payments/gateways", func(gw chi.Router) {
		gw.Use(auth.RequireAuth)

		gw.With(auth.RequirePermissions(permGatewayView)).Get("/available", h.ListAvailableGateways)
		gw.With(auth.RequirePermissions(permGatewayView)).Get("/selected", h.ListSelectedGateways)
		gw.With(auth.RequirePermissions(permGatewayManage)).Post("/select/{gatewayType}", h.SelectGateway)
		gw.With(auth.RequirePermissions(permGatewayManage)).Delete("/select/{gatewayType}", h.DeactivateGateway)
	})
}

// tenantUUIDFromContext resolves the tenant UUID from request context. The TenantV2 middleware
// (and JIT sync backfill) populate the tenant UUID for both authenticated and slug-only requests.
func tenantUUIDFromContext(r *http.Request) (uuid.UUID, bool) {
	idStr := httpware.GetTenantID(r.Context())
	if idStr == "" {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

// ListAvailableGateways proxies treasury's available-gateways list for the tenant.
// @Summary List available payment gateways
// @Description Gateway types the tenant could enable, proxied from treasury-api.
// @Tags Payment Gateways
// @Produce json
// @Param tenant path string true "Tenant slug or ID"
// @Success 200 {object} AvailableGatewaysResponse
// @Router /payments/gateways/available [get]
func (h *PaymentMethodHandler) ListAvailableGateways(w http.ResponseWriter, r *http.Request) {
	resp := AvailableGatewaysResponse{Gateways: []AvailableGatewayResponse{}}

	// Treasury not configured: degrade gracefully with an empty list (mirrors the aggregate's
	// best-effort handling) rather than panicking on a nil client.
	if h.treasury == nil {
		handlers.RespondJSON(w, http.StatusOK, resp)
		return
	}

	tenantID, ok := tenantUUIDFromContext(r)
	if !ok {
		handlers.RespondError(w, http.StatusBadRequest, "invalid or missing tenant")
		return
	}

	gateways, err := h.treasury.ListAvailableGateways(r.Context(), tenantID)
	if err != nil {
		h.log.Warn("failed to list available gateways", zap.Error(err), zap.String("tenant_id", tenantID.String()))
		handlers.RespondError(w, http.StatusServiceUnavailable, "payment gateways unavailable")
		return
	}

	out := make([]AvailableGatewayResponse, 0, len(gateways))
	for _, g := range gateways {
		out = append(out, AvailableGatewayResponse{
			GatewayType:        g.GatewayType,
			Name:               g.Name,
			TransactionFeeType: g.TransactionFeeType,
			SupportsStkPush:    g.SupportsStkPush,
		})
	}
	resp.Gateways = out
	handlers.RespondJSON(w, http.StatusOK, resp)
}

// ListSelectedGateways proxies treasury's selected-gateways list for the tenant.
// @Summary List selected payment gateways
// @Description Gateways the tenant has selected/configured, proxied from treasury-api.
// @Tags Payment Gateways
// @Produce json
// @Param tenant path string true "Tenant slug or ID"
// @Success 200 {object} SelectedGatewaysResponse
// @Router /payments/gateways/selected [get]
func (h *PaymentMethodHandler) ListSelectedGateways(w http.ResponseWriter, r *http.Request) {
	resp := SelectedGatewaysResponse{Selected: []treasury.SelectedGateway{}}

	if h.treasury == nil {
		handlers.RespondJSON(w, http.StatusOK, resp)
		return
	}

	tenantID, ok := tenantUUIDFromContext(r)
	if !ok {
		handlers.RespondError(w, http.StatusBadRequest, "invalid or missing tenant")
		return
	}

	selected, err := h.treasury.ListSelectedGateways(r.Context(), tenantID)
	if err != nil {
		h.log.Warn("failed to list selected gateways", zap.Error(err), zap.String("tenant_id", tenantID.String()))
		handlers.RespondError(w, http.StatusServiceUnavailable, "payment gateways unavailable")
		return
	}
	if selected != nil {
		resp.Selected = selected
	}
	handlers.RespondJSON(w, http.StatusOK, resp)
}

// SelectGateway proxies treasury's select-gateway action for the tenant.
// @Summary Select (enable) a payment gateway
// @Tags Payment Gateways
// @Accept json
// @Produce json
// @Param tenant path string true "Tenant slug or ID"
// @Param gatewayType path string true "Gateway type (e.g. paystack, mpesa_paybill, cod, wallet)"
// @Param request body SelectGatewayRequest true "Select gateway request"
// @Success 200 {object} gatewayActionResponse
// @Router /payments/gateways/select/{gatewayType} [post]
func (h *PaymentMethodHandler) SelectGateway(w http.ResponseWriter, r *http.Request) {
	if h.treasury == nil {
		handlers.RespondError(w, http.StatusServiceUnavailable, "payment gateways not configured")
		return
	}

	tenantID, ok := tenantUUIDFromContext(r)
	if !ok {
		handlers.RespondError(w, http.StatusBadRequest, "invalid or missing tenant")
		return
	}

	gatewayType := chi.URLParam(r, "gatewayType")
	if gatewayType == "" {
		handlers.RespondError(w, http.StatusBadRequest, "missing gatewayType")
		return
	}

	var req SelectGatewayRequest
	if err := handlers.DecodeJSON(r, &req); err != nil {
		// Body is optional (is_primary defaults to false); only reject malformed JSON.
		if r.ContentLength > 0 {
			handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}

	if err := h.treasury.SelectGateway(r.Context(), tenantID, gatewayType, req.IsPrimary); err != nil {
		h.log.Warn("failed to select gateway", zap.Error(err), zap.String("tenant_id", tenantID.String()), zap.String("gateway_type", gatewayType))
		handlers.RespondError(w, http.StatusBadGateway, "failed to select gateway")
		return
	}

	handlers.RespondJSON(w, http.StatusOK, gatewayActionResponse{Message: "gateway selected"})
}

// DeactivateGateway proxies treasury's deselect-gateway action for the tenant.
// @Summary Deactivate (disable) a payment gateway
// @Tags Payment Gateways
// @Produce json
// @Param tenant path string true "Tenant slug or ID"
// @Param gatewayType path string true "Gateway type (e.g. paystack, mpesa_paybill, cod, wallet)"
// @Success 200 {object} gatewayActionResponse
// @Router /payments/gateways/select/{gatewayType} [delete]
func (h *PaymentMethodHandler) DeactivateGateway(w http.ResponseWriter, r *http.Request) {
	if h.treasury == nil {
		handlers.RespondError(w, http.StatusServiceUnavailable, "payment gateways not configured")
		return
	}

	tenantID, ok := tenantUUIDFromContext(r)
	if !ok {
		handlers.RespondError(w, http.StatusBadRequest, "invalid or missing tenant")
		return
	}

	gatewayType := chi.URLParam(r, "gatewayType")
	if gatewayType == "" {
		handlers.RespondError(w, http.StatusBadRequest, "missing gatewayType")
		return
	}

	if err := h.treasury.DeactivateGateway(r.Context(), tenantID, gatewayType); err != nil {
		h.log.Warn("failed to deactivate gateway", zap.Error(err), zap.String("tenant_id", tenantID.String()), zap.String("gateway_type", gatewayType))
		handlers.RespondError(w, http.StatusBadGateway, "failed to deactivate gateway")
		return
	}

	handlers.RespondJSON(w, http.StatusOK, gatewayActionResponse{Message: "gateway deactivated"})
}
