package slahandler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/Bengo-Hub/httpware"
	"github.com/bengobox/ordering-backend/internal/http/handlers"
	identityhandler "github.com/bengobox/ordering-backend/internal/http/handlers/identity"
	"github.com/bengobox/ordering-backend/internal/modules/identity"
	"github.com/bengobox/ordering-backend/internal/modules/sla"
)

// Handler handles SLA HTTP requests.
type Handler struct {
	logger  *zap.Logger
	service *sla.Service
}

// NewHandler creates a new SLA handler.
func NewHandler(logger *zap.Logger, service *sla.Service) *Handler {
	return &Handler{
		logger:  logger.Named("sla.handler"),
		service: service,
	}
}

// Register registers the SLA routes.
func (h *Handler) Register(r chi.Router, auth *identityhandler.Authenticator) {
	r.Route("/sla", func(slaRouter chi.Router) {
		slaRouter.Use(auth.RequireAuth)

		// Metrics - analytics permissions for viewing SLA data
		slaRouter.With(auth.RequirePermissions(identity.PermissionAnalyticsView)).
			Get("/metrics", h.ListMetrics)
		slaRouter.With(auth.RequirePermissions(identity.PermissionAnalyticsView)).
			Get("/metrics/{metricId}", h.GetMetric)
		slaRouter.With(auth.RequirePermissions(identity.PermissionAnalyticsView)).
			Get("/stats", h.GetStats)
		slaRouter.With(auth.RequirePermissions(identity.PermissionAnalyticsView)).
			Get("/breached", h.GetBreachedMetrics)

		// Order-specific SLA routes
		slaRouter.With(auth.RequirePermissions(identity.PermissionOrdersView)).
			Get("/orders/{orderId}/metrics", h.GetOrderMetrics)
		slaRouter.With(auth.RequirePermissions(identity.PermissionOrdersManage)).
			Post("/orders/{orderId}/start", h.StartOrderTracking)
		slaRouter.With(auth.RequirePermissions(identity.PermissionOrdersManage)).
			Post("/orders/{orderId}/complete/{metricType}", h.CompleteMetric)
		slaRouter.With(auth.RequirePermissions(identity.PermissionOrdersManage)).
			Post("/orders/{orderId}/cancel", h.CancelOrderMetrics)
	})
}

// ListMetrics lists SLA metrics with filtering.
func (h *Handler) ListMetrics(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	filter := sla.MetricFilter{
		TenantID: tenantID,
		Limit:    50,
	}

	if v := r.URL.Query().Get("orderId"); v != "" {
		if id, err := uuid.Parse(v); err == nil {
			filter.OrderID = &id
		}
	}
	if v := r.URL.Query().Get("metricType"); v != "" {
		mt := sla.MetricType(v)
		filter.MetricType = &mt
	}
	if v := r.URL.Query().Get("status"); v != "" {
		status := sla.MetricStatus(v)
		filter.Status = &status
	}
	if v := r.URL.Query().Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.DateFrom = &t
		}
	}
	if v := r.URL.Query().Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.DateTo = &t
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if offset, err := strconv.Atoi(v); err == nil {
			filter.Offset = offset
		}
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if limit, err := strconv.Atoi(v); err == nil && limit > 0 && limit <= 100 {
			filter.Limit = limit
		}
	}

	metrics, total, err := h.service.ListMetrics(r.Context(), tenantID, filter)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"metrics": metrics,
		"total":   total,
	})
}

// GetMetric retrieves an SLA metric by ID.
func (h *Handler) GetMetric(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	metricID, err := uuid.Parse(chi.URLParam(r, "metricId"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid metric ID")
		return
	}

	metric, err := h.service.GetMetric(r.Context(), tenantID, metricID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, metric)
}

// GetStats returns SLA statistics.
func (h *Handler) GetStats(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Default to last 7 days
	to := time.Now()
	from := to.AddDate(0, 0, -7)

	if v := r.URL.Query().Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			from = t
		}
	}
	if v := r.URL.Query().Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			to = t
		}
	}

	stats, err := h.service.GetStats(r.Context(), tenantID, from, to)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, stats)
}

// GetBreachedMetrics returns recent breached metrics.
func (h *Handler) GetBreachedMetrics(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if l, err := strconv.Atoi(v); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	metrics, err := h.service.GetBreachedMetrics(r.Context(), tenantID, limit)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"metrics": metrics,
		"total":   len(metrics),
	})
}

// GetOrderMetrics returns all SLA metrics for an order.
func (h *Handler) GetOrderMetrics(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	orderID, err := uuid.Parse(chi.URLParam(r, "orderId"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid order ID")
		return
	}

	filter := sla.MetricFilter{
		TenantID: tenantID,
		OrderID:  &orderID,
		Limit:    100,
	}

	metrics, total, err := h.service.ListMetrics(r.Context(), tenantID, filter)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"metrics": metrics,
		"total":   total,
	})
}

// StartOrderTracking starts SLA tracking for an order.
func (h *Handler) StartOrderTracking(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	orderID, err := uuid.Parse(chi.URLParam(r, "orderId"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid order ID")
		return
	}

	// Optional: custom config from request body
	var config *sla.SLAConfig
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}

	if err := h.service.StartOrderTracking(r.Context(), tenantID, orderID, config); err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusCreated, map[string]string{"status": "tracking_started"})
}

// CompleteMetric completes an SLA metric for an order.
func (h *Handler) CompleteMetric(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	orderID, err := uuid.Parse(chi.URLParam(r, "orderId"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid order ID")
		return
	}

	metricType := sla.MetricType(chi.URLParam(r, "metricType"))
	if metricType == "" {
		handlers.RespondError(w, http.StatusBadRequest, "metric type required")
		return
	}

	metric, err := h.service.CompleteTracking(r.Context(), tenantID, orderID, metricType)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, metric)
}

// CancelOrderMetrics cancels all SLA metrics for an order.
func (h *Handler) CancelOrderMetrics(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	orderID, err := uuid.Parse(chi.URLParam(r, "orderId"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid order ID")
		return
	}

	if err := h.service.CancelMetricsByOrder(r.Context(), tenantID, orderID); err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

// --- Helper Functions ---

func (h *Handler) handleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sla.ErrMetricNotFound):
		handlers.RespondError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, sla.ErrMetricAlreadyExists):
		handlers.RespondError(w, http.StatusConflict, err.Error())
	case errors.Is(err, sla.ErrMetricAlreadyCompleted):
		handlers.RespondError(w, http.StatusConflict, err.Error())
	case errors.Is(err, sla.ErrInvalidMetricType), errors.Is(err, sla.ErrInvalidStatus):
		handlers.RespondError(w, http.StatusBadRequest, err.Error())
	default:
		h.logger.Error("internal error", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "internal server error")
	}
}

func getTenantID(r *http.Request) (uuid.UUID, error) {
	ctx := r.Context()
	// Platform owner query-param override
	if httpware.IsPlatformOwner(ctx) {
		if q := r.URL.Query().Get("tenantId"); q != "" {
			return uuid.Parse(q)
		}
	}
	user, ok := identity.UserFromContext(ctx)
	if !ok {
		return uuid.Nil, errors.New("user not found in context")
	}
	return uuid.Parse(user.TenantID)
}
