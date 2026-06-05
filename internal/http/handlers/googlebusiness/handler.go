// Package googlebusiness exposes the admin + public HTTP endpoints for the Google
// Business Profile integration. Every endpoint is safe when OAuth is unconfigured:
// it returns HTTP 503 {"error":"...","message":"Google integration not configured"}
// instead of panicking.
package googlebusiness

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Bengo-Hub/httpware"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	handlers "github.com/bengobox/ordering-backend/internal/http/handlers"
	identityhandler "github.com/bengobox/ordering-backend/internal/http/handlers/identity"
	"github.com/bengobox/ordering-backend/internal/modules/googlebusiness"
	"github.com/bengobox/ordering-backend/internal/modules/identity"
)

// Handler wires the GBP service to chi routes.
type Handler struct {
	logger *zap.Logger
	svc    *googlebusiness.Service
}

// NewHandler creates a new Google Business Profile handler.
func NewHandler(logger *zap.Logger, svc *googlebusiness.Service) *Handler {
	return &Handler{
		logger: logger.Named("googlebusiness.handler"),
		svc:    svc,
	}
}

// Register mounts the admin (auth-protected) and public callback routes.
// The public callback + review-url paths are exempted from auth in router.go.
func (h *Handler) Register(r chi.Router, auth *identityhandler.Authenticator) {
	r.Route("/{tenant}/admin/integrations/google", func(g chi.Router) {
		g.Use(auth.RequirePermissions(identity.PermissionAdminManage)) // ordering.config.manage
		g.Get("/status", h.Status)
		g.Get("/connect", h.Connect)
		g.Delete("/connect", h.Disconnect)
		g.Get("/locations", h.ListLocations)
		g.Put("/location", h.SelectLocation)
		g.Get("/reviews", h.ListReviews)
		g.Post("/reviews/{reviewId}/reply", h.Reply)
	})

	// PUBLIC: Google redirects the operator's browser here after consent.
	r.Get("/{tenant}/integrations/google/callback", h.Callback)
}

// notConfigured returns true (and writes a 503) when OAuth env is unset.
func (h *Handler) notConfigured(w http.ResponseWriter) bool {
	if h.svc == nil || !h.svc.IsConfigured() {
		handlers.RespondError(w, http.StatusServiceUnavailable, "Google integration not configured")
		return true
	}
	return false
}

// tenantUUID resolves the tenant UUID from request context (TenantV2 / JIT sync).
func tenantUUID(r *http.Request) (uuid.UUID, error) {
	idStr := httpware.GetTenantID(r.Context())
	if idStr == "" {
		return uuid.Nil, errors.New("tenant not resolved")
	}
	return uuid.Parse(idStr)
}

// outletPtr returns the optional outlet UUID from context (X-Outlet-ID), or nil.
func outletPtr(r *http.Request) *uuid.UUID {
	idStr := httpware.GetOutletID(r.Context())
	if idStr == "" {
		return nil
	}
	if id, err := uuid.Parse(idStr); err == nil {
		return &id
	}
	return nil
}

// Status reports configured/connected status.
// @Summary Google Business Profile status
// @Tags Integrations
// @Produce json
// @Param tenant path string true "Tenant slug or ID"
// @Success 200 {object} googlebusiness.StatusResult
// @Router /api/v1/{tenant}/admin/integrations/google/status [get]
func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	// Status is always answerable: when unconfigured it reports configured:false (200).
	tid, err := tenantUUID(r)
	if err != nil {
		// Still return a useful, non-crashing payload.
		handlers.RespondJSON(w, http.StatusOK, googlebusiness.StatusResult{
			Configured: h.svc != nil && h.svc.IsConfigured(),
			Status:     "disconnected",
		})
		return
	}
	res, err := h.svc.GetStatus(r.Context(), tid, outletPtr(r))
	if err != nil {
		h.logger.Error("google status failed", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "failed to read status")
		return
	}
	handlers.RespondJSON(w, http.StatusOK, res)
}

// Connect returns the Google consent-screen URL.
// @Summary Begin Google Business Profile connect
// @Tags Integrations
// @Produce json
// @Param tenant path string true "Tenant slug or ID"
// @Success 200 {object} map[string]string
// @Failure 503 {object} handlers.ErrorResponse
// @Router /api/v1/{tenant}/admin/integrations/google/connect [get]
func (h *Handler) Connect(w http.ResponseWriter, r *http.Request) {
	if h.notConfigured(w) {
		return
	}
	tid, err := tenantUUID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}
	slug := httpware.GetTenantSlug(r.Context())
	authURL, err := h.svc.Connect(tid, slug, outletPtr(r))
	if err != nil {
		if errors.Is(err, googlebusiness.ErrNotConfigured) {
			handlers.RespondError(w, http.StatusServiceUnavailable, "Google integration not configured")
			return
		}
		h.logger.Error("google connect failed", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "failed to build auth url")
		return
	}
	handlers.RespondJSON(w, http.StatusOK, map[string]string{"auth_url": authURL})
}

// Callback handles Google's OAuth redirect (PUBLIC).
// @Summary Google Business Profile OAuth callback
// @Tags Integrations
// @Param tenant path string true "Tenant slug or ID"
// @Param code query string false "Authorization code"
// @Param state query string false "Signed state"
// @Success 302
// @Router /api/v1/{tenant}/integrations/google/callback [get]
func (h *Handler) Callback(w http.ResponseWriter, r *http.Request) {
	slug := httpware.GetTenantSlug(r.Context())

	if h.svc == nil || !h.svc.IsConfigured() {
		http.Redirect(w, r, h.redirectFor(slug, "not_configured"), http.StatusFound)
		return
	}

	if errParam := r.URL.Query().Get("error"); errParam != "" {
		h.logger.Warn("google callback returned error", zap.String("error", errParam))
		http.Redirect(w, r, h.redirectFor(slug, "denied"), http.StatusFound)
		return
	}

	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if code == "" || state == "" {
		http.Redirect(w, r, h.redirectFor(slug, "invalid_request"), http.StatusFound)
		return
	}

	if _, err := h.svc.HandleCallback(r.Context(), state, code); err != nil {
		h.logger.Error("google callback failed", zap.Error(err))
		http.Redirect(w, r, h.redirectFor(slug, "error"), http.StatusFound)
		return
	}

	http.Redirect(w, r, h.redirectFor(slug, "connected"), http.StatusFound)
}

func (h *Handler) redirectFor(slug, status string) string {
	return h.svc.FrontendRedirect(slug, status)
}

// Disconnect clears stored tokens.
// @Summary Disconnect Google Business Profile
// @Tags Integrations
// @Produce json
// @Param tenant path string true "Tenant slug or ID"
// @Success 200 {object} map[string]string
// @Router /api/v1/{tenant}/admin/integrations/google/connect [delete]
func (h *Handler) Disconnect(w http.ResponseWriter, r *http.Request) {
	tid, err := tenantUUID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}
	if err := h.svc.Disconnect(r.Context(), tid, outletPtr(r)); err != nil {
		h.logger.Error("google disconnect failed", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "failed to disconnect")
		return
	}
	handlers.RespondJSON(w, http.StatusOK, map[string]string{"status": "disconnected"})
}

// ListLocations lists available locations.
// @Summary List Google Business locations
// @Tags Integrations
// @Produce json
// @Param tenant path string true "Tenant slug or ID"
// @Success 200 {object} map[string]interface{}
// @Failure 503 {object} handlers.ErrorResponse
// @Router /api/v1/{tenant}/admin/integrations/google/locations [get]
func (h *Handler) ListLocations(w http.ResponseWriter, r *http.Request) {
	if h.notConfigured(w) {
		return
	}
	tid, err := tenantUUID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}
	locs, err := h.svc.ListLocations(r.Context(), tid, outletPtr(r))
	if err != nil {
		h.respondServiceErr(w, err, "failed to list locations")
		return
	}
	handlers.RespondJSON(w, http.StatusOK, map[string]any{"data": locs, "total": len(locs)})
}

// selectLocationRequest is the body for selecting a location.
type selectLocationRequest struct {
	LocationName string `json:"location_name"`
	PlaceID      string `json:"place_id"`
	DisplayName  string `json:"display_name"`
}

// SelectLocation persists the chosen location + place_id and writes the review URL.
// @Summary Select Google Business location
// @Tags Integrations
// @Accept json
// @Produce json
// @Param tenant path string true "Tenant slug or ID"
// @Param request body selectLocationRequest true "Location selection"
// @Success 200 {object} map[string]string
// @Failure 503 {object} handlers.ErrorResponse
// @Router /api/v1/{tenant}/admin/integrations/google/location [put]
func (h *Handler) SelectLocation(w http.ResponseWriter, r *http.Request) {
	if h.notConfigured(w) {
		return
	}
	tid, err := tenantUUID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}
	var req selectLocationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.LocationName == "" {
		handlers.RespondError(w, http.StatusBadRequest, "location_name is required")
		return
	}
	if err := h.svc.SelectLocation(r.Context(), tid, outletPtr(r), req.LocationName, req.PlaceID, req.DisplayName); err != nil {
		h.respondServiceErr(w, err, "failed to select location")
		return
	}
	handlers.RespondJSON(w, http.StatusOK, map[string]string{"status": "connected"})
}

// ListReviews returns the location's reviews.
// @Summary List Google reviews
// @Tags Integrations
// @Produce json
// @Param tenant path string true "Tenant slug or ID"
// @Success 200 {object} map[string]interface{}
// @Failure 503 {object} handlers.ErrorResponse
// @Router /api/v1/{tenant}/admin/integrations/google/reviews [get]
func (h *Handler) ListReviews(w http.ResponseWriter, r *http.Request) {
	if h.notConfigured(w) {
		return
	}
	tid, err := tenantUUID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}
	reviews, err := h.svc.GetReviews(r.Context(), tid, outletPtr(r))
	if err != nil {
		h.respondServiceErr(w, err, "failed to list reviews")
		return
	}
	handlers.RespondJSON(w, http.StatusOK, map[string]any{"data": reviews, "total": len(reviews)})
}

// replyRequest is the body for replying to a review.
type replyRequest struct {
	Comment string `json:"comment"`
}

// Reply posts an owner reply to a review.
// @Summary Reply to a Google review
// @Tags Integrations
// @Accept json
// @Produce json
// @Param tenant path string true "Tenant slug or ID"
// @Param reviewId path string true "Review ID"
// @Param request body replyRequest true "Reply comment"
// @Success 200 {object} map[string]string
// @Failure 503 {object} handlers.ErrorResponse
// @Router /api/v1/{tenant}/admin/integrations/google/reviews/{reviewId}/reply [post]
func (h *Handler) Reply(w http.ResponseWriter, r *http.Request) {
	if h.notConfigured(w) {
		return
	}
	tid, err := tenantUUID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}
	reviewID := chi.URLParam(r, "reviewId")
	if reviewID == "" {
		handlers.RespondError(w, http.StatusBadRequest, "reviewId is required")
		return
	}
	var req replyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Comment == "" {
		handlers.RespondError(w, http.StatusBadRequest, "comment is required")
		return
	}
	if err := h.svc.Reply(r.Context(), tid, outletPtr(r), reviewID, req.Comment); err != nil {
		h.respondServiceErr(w, err, "failed to reply to review")
		return
	}
	handlers.RespondJSON(w, http.StatusOK, map[string]string{"status": "replied"})
}

// respondServiceErr maps service errors to HTTP responses (503 when unconfigured).
func (h *Handler) respondServiceErr(w http.ResponseWriter, err error, fallback string) {
	if errors.Is(err, googlebusiness.ErrNotConfigured) {
		handlers.RespondError(w, http.StatusServiceUnavailable, "Google integration not configured")
		return
	}
	h.logger.Error(fallback, zap.Error(err))
	handlers.RespondError(w, http.StatusBadGateway, fallback)
}
