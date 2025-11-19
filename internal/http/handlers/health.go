package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type dbPinger interface {
	Ping(context.Context) error
}

// HealthHandler exposes readiness and liveness endpoints.
type HealthHandler struct {
	log     *zap.Logger
	db      dbPinger
	cache   *redis.Client
	eventNC *nats.Conn
}

// NewHealthHandler constructs a HealthHandler.
func NewHealthHandler(log *zap.Logger, db dbPinger, cache *redis.Client, eventNC *nats.Conn) *HealthHandler {
	return &HealthHandler{
		log:     log,
		db:      db,
		cache:   cache,
		eventNC: eventNC,
	}
}

// livenessResponse models the JSON payload returned by the liveness probe.
type livenessResponse struct {
	Status  string `json:"status" example:"ok"`
	Service string `json:"service" example:"cafe-backend"`
}

// readinessResponse models the JSON payload returned by the readiness probe.
type readinessResponse struct {
	Status       string            `json:"status" example:"OK"`
	Dependencies map[string]string `json:"dependencies"`
}

// Liveness returns the service status without touching external deps.
// @Summary Service liveness probe
// @Description Returns OK when the API process is healthy.
// @Tags Health
// @Produce json
// @Success 200 {object} livenessResponse
// @Router /healthz [get]
func (h *HealthHandler) Liveness(w http.ResponseWriter, r *http.Request) {
	RespondJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"service": "cafe-backend",
	})
}

// Status checks critical dependencies to determine readiness.
// @Summary Readiness probe
// @Description Checks connectivity to Postgres, Redis, and NATS (if configured).
// @Tags Health
// @Produce json
// @Success 200 {object} readinessResponse
// @Failure 503 {object} readinessResponse
// @Router /status [get]
func (h *HealthHandler) Status(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	issues := make(map[string]string)

	if h.db != nil {
		if err := h.db.Ping(ctx); err != nil {
			issues["postgres"] = err.Error()
		}
	}

	if h.cache != nil {
		if err := h.cache.Ping(ctx).Err(); err != nil {
			issues["redis"] = err.Error()
		}
	}

	if h.eventNC != nil && !h.eventNC.IsConnected() {
		issues["nats"] = "not connected"
	}

	status := http.StatusOK
	if len(issues) > 0 {
		status = http.StatusServiceUnavailable
	}

	RespondJSON(w, status, map[string]any{
		"status":       http.StatusText(status),
		"dependencies": issues,
	})
}

// Metrics exposes Prometheus metrics.
// @Summary Prometheus metrics
// @Description Exposes Prometheus metrics in the text format.
// @Tags Health
// @Produce plain
// @Success 200 {string} string "Prometheus metrics payload"
// @Router /metrics [get]
func (h *HealthHandler) Metrics(w http.ResponseWriter, r *http.Request) {
	promhttp.Handler().ServeHTTP(w, r)
}
