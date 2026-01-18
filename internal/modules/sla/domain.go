package sla

import (
	"time"

	"github.com/google/uuid"
)

// MetricType represents the type of SLA metric being tracked.
type MetricType string

const (
	MetricOrderToReady            MetricType = "order_to_ready"
	MetricOrderToPickup           MetricType = "order_to_pickup"
	MetricOrderToDelivery         MetricType = "order_to_delivery"
	MetricReadyToPickup           MetricType = "ready_to_pickup"
	MetricPickupToDelivery        MetricType = "pickup_to_delivery"
	MetricFirstResponseTime       MetricType = "first_response_time"
	MetricTicketResolutionTime    MetricType = "ticket_resolution_time"
)

// MetricStatus represents the status of an SLA metric.
type MetricStatus string

const (
	StatusTracking  MetricStatus = "tracking"
	StatusMet       MetricStatus = "met"
	StatusBreached  MetricStatus = "breached"
	StatusCancelled MetricStatus = "cancelled"
)

// SLAMetric represents an SLA metric for an order or ticket.
type SLAMetric struct {
	ID                uuid.UUID              `json:"id"`
	TenantID          uuid.UUID              `json:"tenant_id"`
	OrderID           uuid.UUID              `json:"order_id"`
	MetricType        MetricType             `json:"metric_type"`
	TargetSeconds     int                    `json:"target_seconds"`
	ActualSeconds     *int                   `json:"actual_seconds,omitempty"`
	Status            MetricStatus           `json:"status"`
	BreachPercentage  *float64               `json:"breach_percentage,omitempty"`
	StartedAt         time.Time              `json:"started_at"`
	EndedAt           *time.Time             `json:"ended_at,omitempty"`
	MeasuredAt        time.Time              `json:"measured_at"`
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt         time.Time              `json:"created_at"`
	UpdatedAt         time.Time              `json:"updated_at"`
}

// SLAConfig represents SLA configuration for a tenant.
type SLAConfig struct {
	TenantID                   uuid.UUID `json:"tenant_id"`
	OrderToReadySeconds        int       `json:"order_to_ready_seconds"`        // Default: 15 minutes (900)
	OrderToPickupSeconds       int       `json:"order_to_pickup_seconds"`       // Default: 30 minutes (1800)
	OrderToDeliverySeconds     int       `json:"order_to_delivery_seconds"`     // Default: 60 minutes (3600)
	ReadyToPickupSeconds       int       `json:"ready_to_pickup_seconds"`       // Default: 15 minutes (900)
	PickupToDeliverySeconds    int       `json:"pickup_to_delivery_seconds"`    // Default: 30 minutes (1800)
	FirstResponseSeconds       int       `json:"first_response_seconds"`        // Default: 4 hours (14400)
	TicketResolutionSeconds    int       `json:"ticket_resolution_seconds"`     // Default: 24 hours (86400)
}

// DefaultSLAConfig returns default SLA configuration.
func DefaultSLAConfig(tenantID uuid.UUID) *SLAConfig {
	return &SLAConfig{
		TenantID:                   tenantID,
		OrderToReadySeconds:        900,   // 15 minutes
		OrderToPickupSeconds:       1800,  // 30 minutes
		OrderToDeliverySeconds:     3600,  // 60 minutes
		ReadyToPickupSeconds:       900,   // 15 minutes
		PickupToDeliverySeconds:    1800,  // 30 minutes
		FirstResponseSeconds:       14400, // 4 hours
		TicketResolutionSeconds:    86400, // 24 hours
	}
}

// CreateMetricRequest represents a request to create an SLA metric.
type CreateMetricRequest struct {
	TenantID      uuid.UUID  `json:"tenant_id"`
	OrderID       uuid.UUID  `json:"order_id"`
	MetricType    MetricType `json:"metric_type"`
	TargetSeconds int        `json:"target_seconds"`
	StartedAt     time.Time  `json:"started_at"`
}

// CompleteMetricRequest represents a request to complete an SLA metric.
type CompleteMetricRequest struct {
	EndedAt       time.Time `json:"ended_at"`
	ActualSeconds int       `json:"actual_seconds"`
}

// MetricFilter defines filters for listing SLA metrics.
type MetricFilter struct {
	TenantID   uuid.UUID
	OrderID    *uuid.UUID
	MetricType *MetricType
	Status     *MetricStatus
	DateFrom   *time.Time
	DateTo     *time.Time
	Limit      int
	Offset     int
}

// SLASummary represents a summary of SLA performance.
type SLASummary struct {
	TenantID         uuid.UUID              `json:"tenant_id"`
	Period           string                 `json:"period"`
	TotalMetrics     int                    `json:"total_metrics"`
	MetMetrics       int                    `json:"met_metrics"`
	BreachedMetrics  int                    `json:"breached_metrics"`
	ComplianceRate   float64                `json:"compliance_rate"`
	AverageBreachPct float64                `json:"average_breach_percentage"`
	ByType           map[MetricType]TypeSummary `json:"by_type"`
}

// TypeSummary represents SLA summary for a specific metric type.
type TypeSummary struct {
	Total          int     `json:"total"`
	Met            int     `json:"met"`
	Breached       int     `json:"breached"`
	ComplianceRate float64 `json:"compliance_rate"`
	AverageSeconds int     `json:"average_seconds"`
}

// SLAViolation represents an SLA violation alert.
type SLAViolation struct {
	ID             uuid.UUID    `json:"id"`
	TenantID       uuid.UUID    `json:"tenant_id"`
	MetricID       uuid.UUID    `json:"metric_id"`
	OrderID        uuid.UUID    `json:"order_id"`
	MetricType     MetricType   `json:"metric_type"`
	TargetSeconds  int          `json:"target_seconds"`
	ActualSeconds  int          `json:"actual_seconds"`
	BreachPct      float64      `json:"breach_percentage"`
	Severity       string       `json:"severity"` // warning, critical
	OccurredAt     time.Time    `json:"occurred_at"`
}
