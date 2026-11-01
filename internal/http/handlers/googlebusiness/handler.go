// Package googlebusiness exposes the admin + public HTTP endpoints for the Google
// Business Profile integration.
//
// Two ways to collect reviews live here. The review link (a pasted Google review link or
// Place ID) always works. The OAuth connection (locations, reviews, replies) needs the
// platform's Google OAuth client; until that is set those endpoints answer 503
// "Google integration not configured" instead of failing.
//
// The integration is tenant-wide. The staff dashboard sends X-Outlet-ID when an outlet
// is picked in the filter, but the review link is one per tenant, so the outlet header
// is ignored here and every call works on the tenant-wide connection.
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

// maxReplyLength is Google's limit for an owner reply.
const maxReplyLength = 4096

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

// Register mounts the admin (auth-protected) routes and the tenant callback. r is the
// tenant router (already under /api/v1/{tenant}), so paths here carry no tenant segment.
// The public callback path is exempted from auth in router.go. The tenant-less callback
// (RegisterPublicCallback) is the one to register with Google.
func (h *Handler) Register(r chi.Router, auth *identityhandler.Authenticator) {
	r.Route("/admin/integrations/google", func(g chi.Router) {
		g.Use(auth.RequirePermissions(identity.PermissionAdminManage)) // ordering.config.manage
		g.Get("/status", h.Status)
		g.Get("/connect", h.Connect)
		g.Delete("/connect", h.Disconnect)
		g.Get("/locations", h.ListLocations)
		g.Put("/location", h.SelectLocation)
		g.Get("/reviews", h.ListReviews)
		g.Post("/reviews/{reviewId}/reply", h.Reply)
		g.Put("/review-link", h.SetReviewLink)
		g.Delete("/review-link", h.ClearReviewLink)
	})

	// PUBLIC: older consents may still redirect here. The URL tenant must match the state.
	r.Get("/integrations/google/callback", h.Callback)
}

// RegisterPublicCallback mounts GET /integrations/google/callback outside the tenant
// router (on /api/v1). Google needs one fixed redirect URI for every tenant; the signed
// state says which tenant the consent belongs to.
func (h *Handler) RegisterPublicCallback(r chi.Router) {
	r.Get("/integrations/google/callback", h.Callback)
}

// notConfigured returns true (and writes a 503) when OAuth env is unset.
func (h *Handler) notConfigured(w http.ResponseWriter) bool {
	if h.svc == nil || !h.svc.IsConfigured() {
		handlers.RespondError(w, http.StatusServiceUnavailable, "Google integration not configured")
		return true
	}
	return false
}

// tenantUUID resolves the tenant UUID from request context (TenantV2 / JIT sync). On
// tenant routes that context is always the URL tenant: router.go refuses a token minted
// for another tenant, and only platform owners may act inside any tenant.
func tenantUUID(r *http.Request) (uuid.UUID, error) {
	idStr := httpware.GetTenantID(r.Context())
	if idStr == "" {
		return uuid.Nil, errors.New("tenant not resolved")
	}
	return uuid.Parse(idStr)
}

// Status reports configured/connected status and the customer review link.
// @Summary Google Business Profile status
// @Tags Integrations
// @Produce json
// @Param tenant path string true "Tenant slug or ID"
// @Success 200 {object} googlebusiness.StatusResult
// @Router /api/v1/{tenant}/admin/integrations/google/status [get]
func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	tid, err := tenantUUID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}
	res, err := h.svc.GetStatus(r.Context(), tid, nil)
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
	if slug == "" {
		slug = chi.URLParam(r, "tenant")
	}
	authURL, err := h.svc.Connect(tid, slug, nil)
	if err != nil {
		h.respondServiceErr(w, err, "failed to build auth url")
		return
	}
	handlers.RespondJSON(w, http.StatusOK, map[string]string{"auth_url": authURL})
}

// Callback handles Google's OAuth redirect (PUBLIC). It is served both at
// /api/v1/integrations/google/callback and, for older consents, under a tenant.
// @Summary Google Business Profile OAuth callback
// @Tags Integrations
// @Param code query string false "Authorization code"
// @Param state query string false "Signed state"
// @Success 302
// @Router /api/v1/integrations/google/callback [get]
func (h *Handler) Callback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	// Only used to pick the page to land on; the tenant itself comes from the verified state.
	slug := googlebusiness.StateSlug(state)

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
	if code == "" || state == "" {
		http.Redirect(w, r, h.redirectFor(slug, "invalid_request"), http.StatusFound)
		return
	}

	// On the tenant route the URL tenant must be the state's tenant.
	var expected *uuid.UUID
	if chi.URLParam(r, "tenant") != "" {
		if tid, err := tenantUUID(r); err == nil {
			expected = &tid
		}
	}

	if _, err := h.svc.HandleCallback(r.Context(), state, code, expected); err != nil {
		h.logger.Error("google callback failed", zap.Error(err))
		http.Redirect(w, r, h.redirectFor(slug, "error"), http.StatusFound)
		return
	}

	http.Redirect(w, r, h.redirectFor(slug, "connected"), http.StatusFound)
}

func (h *Handler) redirectFor(slug, status string) string {
	if h.svc == nil {
		return "/"
	}
	return h.svc.FrontendRedirect(slug, status)
}

// Disconnect clears stored tokens. The review link stays.
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
	if err := h.svc.Disconnect(r.Context(), tid, nil); err != nil {
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
	locs, err := h.svc.ListLocations(r.Context(), tid, nil)
	if err != nil {
		h.respondServiceErr(w, err, "failed to list locations")
		return
	}
	if locs == nil {
		locs = []googlebusiness.Location{}
	}
	handlers.RespondJSON(w, http.StatusOK, map[string]any{"data": locs, "total": len(locs)})
}

// selectLocationRequest is the body for selecting a location.
type selectLocationRequest struct {
	LocationName string `json:"location_name"`
	PlaceID      string `json:"place_id"`
	DisplayName  string `json:"display_name"`
	// NewReviewURI is Google's own review link for the location, used when it has no Place ID.
	NewReviewURI string `json:"new_review_uri"`
}

// SelectLocation persists the chosen location + place_id and writes the review link.
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
	if err := h.svc.SelectLocation(r.Context(), tid, nil, req.LocationName, req.PlaceID, req.DisplayName, req.NewReviewURI); err != nil {
		h.respondServiceErr(w, err, "failed to select location")
		return
	}
	handlers.RespondJSON(w, http.StatusOK, map[string]string{"status": "connected"})
}

// ListReviews returns one page of the location's reviews.
// @Summary List Google reviews
// @Tags Integrations
// @Produce json
// @Param tenant path string true "Tenant slug or ID"
// @Param page_token query string false "Next page token from the previous page"
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
	page, err := h.svc.GetReviews(r.Context(), tid, nil, r.URL.Query().Get("page_token"))
	if err != nil {
		h.respondServiceErr(w, err, "failed to list reviews")
		return
	}
	handlers.RespondJSON(w, http.StatusOK, map[string]any{
		"data":            page.Reviews,
		"total":           page.TotalReviewCount,
		"average_rating":  page.AverageRating,
		"next_page_token": page.NextPageToken,
	})
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
	if len(req.Comment) > maxReplyLength {
		handlers.RespondError(w, http.StatusBadRequest, "reply is too long (4096 characters at most)")
		return
	}
	if err := h.svc.Reply(r.Context(), tid, nil, reviewID, req.Comment); err != nil {
		h.respondServiceErr(w, err, "failed to reply to review")
		return
	}
	handlers.RespondJSON(w, http.StatusOK, map[string]string{"status": "replied"})
}

// reviewLinkRequest is the body for saving the review link.
type reviewLinkRequest struct {
	// Value is a Google review link or a Place ID.
	Value string `json:"value"`
}

// SetReviewLink saves a pasted Google review link or Place ID. Works without OAuth.
// @Summary Save the Google review link
// @Tags Integrations
// @Accept json
// @Produce json
// @Param tenant path string true "Tenant slug or ID"
// @Param request body reviewLinkRequest true "Review link or Place ID"
// @Success 200 {object} googlebusiness.ReviewLink
// @Failure 400 {object} handlers.ErrorResponse
// @Router /api/v1/{tenant}/admin/integrations/google/review-link [put]
func (h *Handler) SetReviewLink(w http.ResponseWriter, r *http.Request) {
	tid, err := tenantUUID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}
	var req reviewLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	link, err := h.svc.SetReviewLink(r.Context(), tid, req.Value)
	if err != nil {
		if errors.Is(err, googlebusiness.ErrInvalidReviewLink) {
			handlers.RespondError(w, http.StatusBadRequest, "Enter a Google review link or a Place ID")
			return
		}
		h.logger.Error("save google review link failed", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "failed to save the review link")
		return
	}
	handlers.RespondJSON(w, http.StatusOK, link)
}

// ClearReviewLink removes the review link, which hides the customer button.
// @Summary Remove the Google review link
// @Tags Integrations
// @Produce json
// @Param tenant path string true "Tenant slug or ID"
// @Success 200 {object} map[string]string
// @Router /api/v1/{tenant}/admin/integrations/google/review-link [delete]
func (h *Handler) ClearReviewLink(w http.ResponseWriter, r *http.Request) {
	tid, err := tenantUUID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}
	if err := h.svc.ClearReviewLink(r.Context(), tid); err != nil {
		h.logger.Error("clear google review link failed", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "failed to remove the review link")
		return
	}
	handlers.RespondJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// respondServiceErr maps service errors to HTTP responses.
func (h *Handler) respondServiceErr(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, googlebusiness.ErrNotConfigured):
		handlers.RespondError(w, http.StatusServiceUnavailable, "Google integration not configured")
	case errors.Is(err, googlebusiness.ErrNotConnected):
		handlers.RespondError(w, http.StatusConflict, "Connect your Google Business Profile first")
	case errors.Is(err, googlebusiness.ErrNoLocation):
		handlers.RespondError(w, http.StatusConflict, "Choose your business location first")
	case errors.Is(err, googlebusiness.ErrReconnect):
		handlers.RespondError(w, http.StatusConflict, "Google access has expired. Connect again")
	case errors.Is(err, googlebusiness.ErrForeignReview):
		handlers.RespondError(w, http.StatusBadRequest, "That review is not on your chosen location")
	default:
		h.logger.Error(fallback, zap.Error(err))
		handlers.RespondError(w, http.StatusBadGateway, fallback)
	}
}
