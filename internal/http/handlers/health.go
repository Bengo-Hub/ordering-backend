package handlers

import (
	"context"
	"encoding/json"
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

// Liveness returns the service status without touching external deps.
func (h *HealthHandler) Liveness(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
		"service": "food-delivery-backend",
	})
}

// Status checks critical dependencies to determine readiness.
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

	respondJSON(w, status, map[string]any{
		"status":       http.StatusText(status),
		"dependencies": issues,
	})
}

// Metrics exposes Prometheus metrics.
func (h *HealthHandler) Metrics(w http.ResponseWriter, r *http.Request) {
	promhttp.Handler().ServeHTTP(w, r)
}

func respondJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		// fall back to plain text if JSON encoding fails
		_, _ = w.Write([]byte(`{"status":"serialization_error"}`))
	}
}
