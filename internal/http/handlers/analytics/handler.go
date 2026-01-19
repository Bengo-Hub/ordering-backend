package analytics

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	authclient "github.com/Bengo-Hub/shared-auth-client"
	handlers "github.com/bengobox/ordering-backend/internal/http/handlers"
	identityhandler "github.com/bengobox/ordering-backend/internal/http/handlers/identity"
	"github.com/bengobox/ordering-backend/internal/modules/analytics"
	"github.com/bengobox/ordering-backend/internal/modules/identity"
)

// Handler handles analytics HTTP requests.
type Handler struct {
	logger      *zap.Logger
	analyticsSvc *analytics.Service
}

// NewHandler creates a new analytics handler.
func NewHandler(logger *zap.Logger, analyticsSvc *analytics.Service) *Handler {
	return &Handler{
		logger:      logger.Named("analytics.handler"),
		analyticsSvc: analyticsSvc,
	}
}

// Register registers the analytics routes.
func (h *Handler) Register(r chi.Router, auth *identityhandler.Authenticator) {
	r.Route("/{tenant}/analytics", func(ar chi.Router) {
		ar.Use(auth.RequireAuth)
		// Require PROFESSIONAL plan or higher for analytics access
		// This ensures only tenants with paid subscriptions can access advanced analytics
		ar.Use(authclient.RequirePlan("PROFESSIONAL"))

		// Dashboard endpoints
		ar.Get("/dashboards", h.ListDashboards)
		ar.Get("/dashboards/{module}", h.GetDashboardInfo)
		ar.Get("/dashboards/{module}/embed", h.GetDashboardEmbed)

		// Health/status
		ar.Get("/status", h.GetStatus)
	})
}

// ListDashboards lists available dashboards.
// @Summary List available dashboards
// @Tags Analytics
// @Produce json
// @Param tenant path string true "Tenant slug"
// @Success 200 {array} analytics.DashboardInfo
// @Router /api/v1/{tenant}/analytics/dashboards [get]
func (h *Handler) ListDashboards(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, tenantID, err := getUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	dashboards, err := h.analyticsSvc.ListAvailableDashboards(ctx, tenantID)
	if err != nil {
		h.logger.Error("Failed to list dashboards", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "failed to list dashboards")
		return
	}

	handlers.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"data": dashboards,
	})
}

// GetDashboardInfo returns information about a specific dashboard.
// @Summary Get dashboard info
// @Tags Analytics
// @Produce json
// @Param tenant path string true "Tenant slug"
// @Param module path string true "Dashboard module (orders, revenue, customers, operations, subscription)"
// @Success 200 {object} analytics.DashboardInfo
// @Router /api/v1/{tenant}/analytics/dashboards/{module} [get]
func (h *Handler) GetDashboardInfo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	moduleStr := chi.URLParam(r, "module")
	module, valid := analytics.ValidateModule(moduleStr)
	if !valid {
		handlers.RespondError(w, http.StatusBadRequest, "invalid dashboard module")
		return
	}

	info, err := h.analyticsSvc.GetDashboardInfo(ctx, module)
	if err != nil {
		if errors.Is(err, analytics.ErrDashboardNotConfigured) {
			handlers.RespondError(w, http.StatusNotFound, "dashboard not configured")
			return
		}
		h.logger.Error("Failed to get dashboard info", zap.Error(err), zap.String("module", moduleStr))
		handlers.RespondError(w, http.StatusInternalServerError, "failed to get dashboard info")
		return
	}

	handlers.RespondJSON(w, http.StatusOK, info)
}

// GetDashboardEmbed returns an embed URL for the specified dashboard.
// @Summary Get dashboard embed URL
// @Tags Analytics
// @Produce json
// @Param tenant path string true "Tenant slug"
// @Param module path string true "Dashboard module (orders, revenue, customers, operations, subscription)"
// @Success 200 {object} analytics.DashboardEmbed
// @Router /api/v1/{tenant}/analytics/dashboards/{module}/embed [get]
func (h *Handler) GetDashboardEmbed(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, tenantID, err := getUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	moduleStr := chi.URLParam(r, "module")
	module, valid := analytics.ValidateModule(moduleStr)
	if !valid {
		handlers.RespondError(w, http.StatusBadRequest, "invalid dashboard module")
		return
	}

	embed, err := h.analyticsSvc.GetDashboardEmbed(ctx, module, tenantID, user.ID)
	if err != nil {
		if errors.Is(err, analytics.ErrDashboardNotConfigured) {
			handlers.RespondError(w, http.StatusNotFound, "dashboard not configured")
			return
		}
		if errors.Is(err, analytics.ErrDashboardNotFound) {
			handlers.RespondError(w, http.StatusNotFound, "dashboard not found in superset")
			return
		}
		if errors.Is(err, analytics.ErrSupersetUnavailable) {
			handlers.RespondError(w, http.StatusServiceUnavailable, "analytics service unavailable")
			return
		}

		h.logger.Error("Failed to get dashboard embed",
			zap.Error(err),
			zap.String("module", moduleStr),
			zap.String("user_id", user.ID.String()))
		handlers.RespondError(w, http.StatusInternalServerError, "failed to get dashboard embed")
		return
	}

	handlers.RespondJSON(w, http.StatusOK, embed)
}

// GetStatus returns the status of the analytics service.
// @Summary Get analytics service status
// @Tags Analytics
// @Produce json
// @Param tenant path string true "Tenant slug"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/{tenant}/analytics/status [get]
func (h *Handler) GetStatus(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"enabled":      h.analyticsSvc.IsEnabled(),
		"superset_url": h.analyticsSvc.GetSupersetBaseURL(),
	}

	handlers.RespondJSON(w, http.StatusOK, status)
}

// Helper function to get user from context
func getUserFromContext(r *http.Request) (*identity.User, uuid.UUID, error) {
	user, ok := identity.UserFromContext(r.Context())
	if !ok {
		return nil, uuid.Nil, errors.New("user not found in context")
	}
	tenantID, err := uuid.Parse(user.TenantID)
	if err != nil {
		return nil, uuid.Nil, errors.New("invalid tenant ID")
	}
	return user, tenantID, nil
}
