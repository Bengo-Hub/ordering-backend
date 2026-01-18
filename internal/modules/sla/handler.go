package sla

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	authclient "github.com/Bengo-Hub/shared-auth-client"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Handler handles HTTP requests for SLA operations.
type Handler struct {
	service *Service
	logger  *zap.Logger
}

// NewHandler creates a new SLA handler.
func NewHandler(service *Service, logger *zap.Logger) *Handler {
	return &Handler{
		service: service,
		logger:  logger.Named("sla-handler"),
	}
}

// RegisterRoutes registers the SLA routes.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Route("/sla", func(sla chi.Router) {
		sla.Get("/metrics", h.ListMetrics)
		sla.Get("/metrics/{metricId}", h.GetMetric)
		sla.Get("/stats", h.GetStats)
		sla.Get("/breached", h.GetBreachedMetrics)

		// Order-specific routes
		sla.Get("/orders/{orderId}/metrics", h.GetOrderMetrics)
		sla.Post("/orders/{orderId}/start", h.StartOrderTracking)
		sla.Post("/orders/{orderId}/complete/{metricType}", h.CompleteMetric)
		sla.Post("/orders/{orderId}/cancel", h.CancelOrderMetrics)
	})
}

// ListMetrics lists SLA metrics with filtering.
func (h *Handler) ListMetrics(w http.ResponseWriter, r *http.Request) {
	claims, ok := authclient.ClaimsFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	tenantID, err := claims.TenantUUID()
	if err != nil || tenantID == nil {
		respondError(w, http.StatusBadRequest, "tenant ID required")
		return
	}

	filter := MetricFilter{
		TenantID: *tenantID,
		Limit:    50,
	}

	if v := r.URL.Query().Get("orderId"); v != "" {
		if id, err := uuid.Parse(v); err == nil {
			filter.OrderID = &id
		}
	}
	if v := r.URL.Query().Get("metricType"); v != "" {
		mt := MetricType(v)
		filter.MetricType = &mt
	}
	if v := r.URL.Query().Get("status"); v != "" {
		status := MetricStatus(v)
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

	metrics, total, err := h.service.ListMetrics(r.Context(), *tenantID, filter)
	if err != nil {
		h.handleError(w, err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"metrics": metrics,
		"total":   total,
	})
}

// GetMetric retrieves an SLA metric by ID.
func (h *Handler) GetMetric(w http.ResponseWriter, r *http.Request) {
	claims, ok := authclient.ClaimsFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	tenantID, err := claims.TenantUUID()
	if err != nil || tenantID == nil {
		respondError(w, http.StatusBadRequest, "tenant ID required")
		return
	}

	metricID, err := uuid.Parse(chi.URLParam(r, "metricId"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid metric ID")
		return
	}

	metric, err := h.service.GetMetric(r.Context(), *tenantID, metricID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	respondJSON(w, http.StatusOK, metric)
}

// GetStats returns SLA statistics.
func (h *Handler) GetStats(w http.ResponseWriter, r *http.Request) {
	claims, ok := authclient.ClaimsFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	tenantID, err := claims.TenantUUID()
	if err != nil || tenantID == nil {
		respondError(w, http.StatusBadRequest, "tenant ID required")
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

	stats, err := h.service.GetStats(r.Context(), *tenantID, from, to)
	if err != nil {
		h.handleError(w, err)
		return
	}

	respondJSON(w, http.StatusOK, stats)
}

// GetBreachedMetrics returns recent breached metrics.
func (h *Handler) GetBreachedMetrics(w http.ResponseWriter, r *http.Request) {
	claims, ok := authclient.ClaimsFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	tenantID, err := claims.TenantUUID()
	if err != nil || tenantID == nil {
		respondError(w, http.StatusBadRequest, "tenant ID required")
		return
	}

	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if l, err := strconv.Atoi(v); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	metrics, err := h.service.GetBreachedMetrics(r.Context(), *tenantID, limit)
	if err != nil {
		h.handleError(w, err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"metrics": metrics,
		"total":   len(metrics),
	})
}

// GetOrderMetrics returns all SLA metrics for an order.
func (h *Handler) GetOrderMetrics(w http.ResponseWriter, r *http.Request) {
	claims, ok := authclient.ClaimsFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	tenantID, err := claims.TenantUUID()
	if err != nil || tenantID == nil {
		respondError(w, http.StatusBadRequest, "tenant ID required")
		return
	}

	orderID, err := uuid.Parse(chi.URLParam(r, "orderId"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid order ID")
		return
	}

	filter := MetricFilter{
		TenantID: *tenantID,
		OrderID:  &orderID,
		Limit:    100,
	}

	metrics, total, err := h.service.ListMetrics(r.Context(), *tenantID, filter)
	if err != nil {
		h.handleError(w, err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"metrics": metrics,
		"total":   total,
	})
}

// StartOrderTracking starts SLA tracking for an order.
func (h *Handler) StartOrderTracking(w http.ResponseWriter, r *http.Request) {
	claims, ok := authclient.ClaimsFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	tenantID, err := claims.TenantUUID()
	if err != nil || tenantID == nil {
		respondError(w, http.StatusBadRequest, "tenant ID required")
		return
	}

	orderID, err := uuid.Parse(chi.URLParam(r, "orderId"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid order ID")
		return
	}

	// Optional: custom config from request body
	var config *SLAConfig
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
			respondError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}

	if err := h.service.StartOrderTracking(r.Context(), *tenantID, orderID, config); err != nil {
		h.handleError(w, err)
		return
	}

	respondJSON(w, http.StatusCreated, map[string]string{"status": "tracking_started"})
}

// CompleteMetric completes an SLA metric for an order.
func (h *Handler) CompleteMetric(w http.ResponseWriter, r *http.Request) {
	claims, ok := authclient.ClaimsFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	tenantID, err := claims.TenantUUID()
	if err != nil || tenantID == nil {
		respondError(w, http.StatusBadRequest, "tenant ID required")
		return
	}

	orderID, err := uuid.Parse(chi.URLParam(r, "orderId"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid order ID")
		return
	}

	metricType := MetricType(chi.URLParam(r, "metricType"))
	if metricType == "" {
		respondError(w, http.StatusBadRequest, "metric type required")
		return
	}

	metric, err := h.service.CompleteTracking(r.Context(), *tenantID, orderID, metricType)
	if err != nil {
		h.handleError(w, err)
		return
	}

	respondJSON(w, http.StatusOK, metric)
}

// CancelOrderMetrics cancels all SLA metrics for an order.
func (h *Handler) CancelOrderMetrics(w http.ResponseWriter, r *http.Request) {
	claims, ok := authclient.ClaimsFromContext(r.Context())
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	tenantID, err := claims.TenantUUID()
	if err != nil || tenantID == nil {
		respondError(w, http.StatusBadRequest, "tenant ID required")
		return
	}

	orderID, err := uuid.Parse(chi.URLParam(r, "orderId"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid order ID")
		return
	}

	if err := h.service.CancelMetricsByOrder(r.Context(), *tenantID, orderID); err != nil {
		h.handleError(w, err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

// Helper functions

func (h *Handler) handleError(w http.ResponseWriter, err error) {
	switch err {
	case ErrMetricNotFound:
		respondError(w, http.StatusNotFound, err.Error())
	case ErrMetricAlreadyExists:
		respondError(w, http.StatusConflict, err.Error())
	case ErrMetricAlreadyCompleted:
		respondError(w, http.StatusConflict, err.Error())
	case ErrInvalidMetricType, ErrInvalidStatus:
		respondError(w, http.StatusBadRequest, err.Error())
	default:
		h.logger.Error("internal error", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "internal error")
	}
}

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}
