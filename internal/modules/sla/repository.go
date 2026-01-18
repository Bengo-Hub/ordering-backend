package sla

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Repository defines the interface for SLA metric data access.
type Repository interface {
	// Metric operations
	CreateMetric(ctx context.Context, tenantID uuid.UUID, metric *SLAMetric) error
	GetMetric(ctx context.Context, tenantID uuid.UUID, metricID uuid.UUID) (*SLAMetric, error)
	GetMetricByOrderAndType(ctx context.Context, tenantID uuid.UUID, orderID uuid.UUID, metricType MetricType) (*SLAMetric, error)
	ListMetrics(ctx context.Context, tenantID uuid.UUID, filter MetricFilter) ([]*SLAMetric, int, error)
	UpdateMetric(ctx context.Context, tenantID uuid.UUID, metricID uuid.UUID, updates map[string]interface{}) error
	CompleteMetric(ctx context.Context, tenantID uuid.UUID, metricID uuid.UUID, endedAt time.Time, actualSeconds int, status MetricStatus, breachPct *float64) error

	// Batch operations
	GetActiveMetricsByOrder(ctx context.Context, tenantID uuid.UUID, orderID uuid.UUID) ([]*SLAMetric, error)
	CancelMetricsByOrder(ctx context.Context, tenantID uuid.UUID, orderID uuid.UUID) error

	// Statistics
	GetMetricStats(ctx context.Context, tenantID uuid.UUID, from, to time.Time) (*SLASummary, error)
	GetBreachedMetrics(ctx context.Context, tenantID uuid.UUID, limit int) ([]*SLAMetric, error)
}
