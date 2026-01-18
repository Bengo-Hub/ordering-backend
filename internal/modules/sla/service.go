package sla

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Service provides SLA business logic.
type Service struct {
	repo   Repository
	logger *zap.Logger
}

// NewService creates a new SLA service.
func NewService(repo Repository, logger *zap.Logger) *Service {
	return &Service{
		repo:   repo,
		logger: logger.Named("sla"),
	}
}

// StartTracking starts SLA tracking for an order metric.
func (s *Service) StartTracking(ctx context.Context, req *CreateMetricRequest) (*SLAMetric, error) {
	// Check if metric already exists
	existing, err := s.repo.GetMetricByOrderAndType(ctx, req.TenantID, req.OrderID, req.MetricType)
	if err == nil && existing != nil {
		// Already tracking this metric
		return existing, nil
	}
	if err != nil && err != ErrMetricNotFound {
		return nil, err
	}

	metric := &SLAMetric{
		ID:            uuid.New(),
		TenantID:      req.TenantID,
		OrderID:       req.OrderID,
		MetricType:    req.MetricType,
		TargetSeconds: req.TargetSeconds,
		Status:        StatusTracking,
		StartedAt:     req.StartedAt,
		MeasuredAt:    time.Now(),
	}

	if err := s.repo.CreateMetric(ctx, req.TenantID, metric); err != nil {
		s.logger.Error("failed to create SLA metric",
			zap.Error(err),
			zap.String("order_id", req.OrderID.String()),
			zap.String("metric_type", string(req.MetricType)),
		)
		return nil, err
	}

	s.logger.Info("SLA tracking started",
		zap.String("metric_id", metric.ID.String()),
		zap.String("order_id", req.OrderID.String()),
		zap.String("metric_type", string(req.MetricType)),
		zap.Int("target_seconds", req.TargetSeconds),
	)

	return metric, nil
}

// CompleteTracking completes SLA tracking for an order metric.
func (s *Service) CompleteTracking(ctx context.Context, tenantID uuid.UUID, orderID uuid.UUID, metricType MetricType) (*SLAMetric, error) {
	metric, err := s.repo.GetMetricByOrderAndType(ctx, tenantID, orderID, metricType)
	if err != nil {
		return nil, err
	}

	if metric.Status != StatusTracking {
		return nil, ErrMetricAlreadyCompleted
	}

	endedAt := time.Now()
	actualSeconds := int(endedAt.Sub(metric.StartedAt).Seconds())

	// Determine status and breach percentage
	var status MetricStatus
	var breachPct *float64

	if actualSeconds <= metric.TargetSeconds {
		status = StatusMet
	} else {
		status = StatusBreached
		pct := (float64(actualSeconds) / float64(metric.TargetSeconds)) - 1.0
		breachPct = &pct
	}

	if err := s.repo.CompleteMetric(ctx, tenantID, metric.ID, endedAt, actualSeconds, status, breachPct); err != nil {
		s.logger.Error("failed to complete SLA metric",
			zap.Error(err),
			zap.String("metric_id", metric.ID.String()),
		)
		return nil, err
	}

	// Update local model
	metric.EndedAt = &endedAt
	metric.ActualSeconds = &actualSeconds
	metric.Status = status
	metric.BreachPercentage = breachPct

	s.logger.Info("SLA tracking completed",
		zap.String("metric_id", metric.ID.String()),
		zap.String("order_id", orderID.String()),
		zap.String("metric_type", string(metricType)),
		zap.String("status", string(status)),
		zap.Int("actual_seconds", actualSeconds),
		zap.Int("target_seconds", metric.TargetSeconds),
	)

	return metric, nil
}

// GetMetric retrieves an SLA metric by ID.
func (s *Service) GetMetric(ctx context.Context, tenantID uuid.UUID, metricID uuid.UUID) (*SLAMetric, error) {
	return s.repo.GetMetric(ctx, tenantID, metricID)
}

// GetMetricByOrderAndType retrieves an SLA metric by order and type.
func (s *Service) GetMetricByOrderAndType(ctx context.Context, tenantID uuid.UUID, orderID uuid.UUID, metricType MetricType) (*SLAMetric, error) {
	return s.repo.GetMetricByOrderAndType(ctx, tenantID, orderID, metricType)
}

// ListMetrics lists SLA metrics with filtering.
func (s *Service) ListMetrics(ctx context.Context, tenantID uuid.UUID, filter MetricFilter) ([]*SLAMetric, int, error) {
	filter.TenantID = tenantID
	return s.repo.ListMetrics(ctx, tenantID, filter)
}

// GetActiveMetricsByOrder returns all active (tracking) metrics for an order.
func (s *Service) GetActiveMetricsByOrder(ctx context.Context, tenantID uuid.UUID, orderID uuid.UUID) ([]*SLAMetric, error) {
	return s.repo.GetActiveMetricsByOrder(ctx, tenantID, orderID)
}

// CancelMetricsByOrder cancels all active metrics for an order (e.g., when order is cancelled).
func (s *Service) CancelMetricsByOrder(ctx context.Context, tenantID uuid.UUID, orderID uuid.UUID) error {
	if err := s.repo.CancelMetricsByOrder(ctx, tenantID, orderID); err != nil {
		s.logger.Error("failed to cancel SLA metrics",
			zap.Error(err),
			zap.String("order_id", orderID.String()),
		)
		return err
	}

	s.logger.Info("SLA metrics cancelled for order",
		zap.String("order_id", orderID.String()),
	)

	return nil
}

// GetStats returns SLA statistics for a time period.
func (s *Service) GetStats(ctx context.Context, tenantID uuid.UUID, from, to time.Time) (*SLASummary, error) {
	return s.repo.GetMetricStats(ctx, tenantID, from, to)
}

// GetBreachedMetrics returns recent breached metrics.
func (s *Service) GetBreachedMetrics(ctx context.Context, tenantID uuid.UUID, limit int) ([]*SLAMetric, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.repo.GetBreachedMetrics(ctx, tenantID, limit)
}

// GetTargetForMetricType returns the target seconds for a metric type based on config.
func (s *Service) GetTargetForMetricType(config *SLAConfig, metricType MetricType) int {
	switch metricType {
	case MetricOrderToReady:
		return config.OrderToReadySeconds
	case MetricOrderToPickup:
		return config.OrderToPickupSeconds
	case MetricOrderToDelivery:
		return config.OrderToDeliverySeconds
	case MetricReadyToPickup:
		return config.ReadyToPickupSeconds
	case MetricPickupToDelivery:
		return config.PickupToDeliverySeconds
	case MetricFirstResponseTime:
		return config.FirstResponseSeconds
	case MetricTicketResolutionTime:
		return config.TicketResolutionSeconds
	default:
		return 3600 // Default 1 hour
	}
}

// StartOrderTracking starts all relevant SLA tracking for a new order.
func (s *Service) StartOrderTracking(ctx context.Context, tenantID uuid.UUID, orderID uuid.UUID, config *SLAConfig) error {
	if config == nil {
		config = DefaultSLAConfig(tenantID)
	}

	now := time.Now()

	// Start order-to-ready tracking
	_, err := s.StartTracking(ctx, &CreateMetricRequest{
		TenantID:      tenantID,
		OrderID:       orderID,
		MetricType:    MetricOrderToReady,
		TargetSeconds: config.OrderToReadySeconds,
		StartedAt:     now,
	})
	if err != nil {
		return err
	}

	// Start order-to-delivery tracking
	_, err = s.StartTracking(ctx, &CreateMetricRequest{
		TenantID:      tenantID,
		OrderID:       orderID,
		MetricType:    MetricOrderToDelivery,
		TargetSeconds: config.OrderToDeliverySeconds,
		StartedAt:     now,
	})
	if err != nil {
		return err
	}

	return nil
}
