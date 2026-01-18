package sla

import (
	"context"
	"time"

	"github.com/bengobox/ordering-backend/internal/ent"
	"github.com/bengobox/ordering-backend/internal/ent/slametric"
	"github.com/google/uuid"
)

// EntRepository implements Repository using Ent ORM.
type EntRepository struct {
	client *ent.Client
}

// NewEntRepository creates a new EntRepository.
func NewEntRepository(client *ent.Client) *EntRepository {
	return &EntRepository{client: client}
}

func (r *EntRepository) CreateMetric(ctx context.Context, tenantID uuid.UUID, m *SLAMetric) error {
	create := r.client.SLAMetric.Create().
		SetID(m.ID).
		SetTenantID(tenantID).
		SetOrderID(m.OrderID).
		SetMetricType(slametric.MetricType(m.MetricType)).
		SetTargetSeconds(m.TargetSeconds).
		SetStatus(slametric.StatusTracking).
		SetStartedAt(m.StartedAt).
		SetMeasuredAt(time.Now())

	if m.Metadata != nil {
		create = create.SetMetadata(m.Metadata)
	}

	created, err := create.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return ErrMetricAlreadyExists
		}
		return err
	}
	m.ID = created.ID
	m.Status = StatusTracking
	m.CreatedAt = created.CreatedAt
	m.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *EntRepository) GetMetric(ctx context.Context, tenantID uuid.UUID, metricID uuid.UUID) (*SLAMetric, error) {
	m, err := r.client.SLAMetric.Query().
		Where(
			slametric.ID(metricID),
			slametric.TenantID(tenantID),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrMetricNotFound
		}
		return nil, err
	}
	return entMetricToModel(m), nil
}

func (r *EntRepository) GetMetricByOrderAndType(ctx context.Context, tenantID uuid.UUID, orderID uuid.UUID, metricType MetricType) (*SLAMetric, error) {
	m, err := r.client.SLAMetric.Query().
		Where(
			slametric.TenantID(tenantID),
			slametric.OrderID(orderID),
			slametric.MetricTypeEQ(slametric.MetricType(metricType)),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrMetricNotFound
		}
		return nil, err
	}
	return entMetricToModel(m), nil
}

func (r *EntRepository) ListMetrics(ctx context.Context, tenantID uuid.UUID, filter MetricFilter) ([]*SLAMetric, int, error) {
	query := r.client.SLAMetric.Query().
		Where(slametric.TenantID(tenantID))

	if filter.OrderID != nil {
		query = query.Where(slametric.OrderID(*filter.OrderID))
	}
	if filter.MetricType != nil {
		query = query.Where(slametric.MetricTypeEQ(slametric.MetricType(*filter.MetricType)))
	}
	if filter.Status != nil {
		query = query.Where(slametric.StatusEQ(slametric.Status(*filter.Status)))
	}
	if filter.DateFrom != nil {
		query = query.Where(slametric.MeasuredAtGTE(*filter.DateFrom))
	}
	if filter.DateTo != nil {
		query = query.Where(slametric.MeasuredAtLTE(*filter.DateTo))
	}

	total, err := query.Clone().Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}

	metrics, err := query.Order(ent.Desc(slametric.FieldCreatedAt)).All(ctx)
	if err != nil {
		return nil, 0, err
	}

	result := make([]*SLAMetric, len(metrics))
	for i, m := range metrics {
		result[i] = entMetricToModel(m)
	}
	return result, total, nil
}

func (r *EntRepository) UpdateMetric(ctx context.Context, tenantID uuid.UUID, metricID uuid.UUID, updates map[string]interface{}) error {
	update := r.client.SLAMetric.Update().
		Where(
			slametric.ID(metricID),
			slametric.TenantID(tenantID),
		)

	if v, ok := updates["metadata"].(map[string]interface{}); ok {
		update = update.SetMetadata(v)
	}

	n, err := update.Save(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrMetricNotFound
	}
	return nil
}

func (r *EntRepository) CompleteMetric(ctx context.Context, tenantID uuid.UUID, metricID uuid.UUID, endedAt time.Time, actualSeconds int, status MetricStatus, breachPct *float64) error {
	update := r.client.SLAMetric.Update().
		Where(
			slametric.ID(metricID),
			slametric.TenantID(tenantID),
			slametric.StatusEQ(slametric.StatusTracking),
		).
		SetEndedAt(endedAt).
		SetActualSeconds(actualSeconds).
		SetStatus(slametric.Status(status)).
		SetMeasuredAt(time.Now())

	if breachPct != nil {
		update = update.SetBreachPercentage(*breachPct)
	}

	n, err := update.Save(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		// Either not found or already completed
		m, err := r.GetMetric(ctx, tenantID, metricID)
		if err != nil {
			return err
		}
		if m.Status != StatusTracking {
			return ErrMetricAlreadyCompleted
		}
		return ErrMetricNotFound
	}
	return nil
}

func (r *EntRepository) GetActiveMetricsByOrder(ctx context.Context, tenantID uuid.UUID, orderID uuid.UUID) ([]*SLAMetric, error) {
	metrics, err := r.client.SLAMetric.Query().
		Where(
			slametric.TenantID(tenantID),
			slametric.OrderID(orderID),
			slametric.StatusEQ(slametric.StatusTracking),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]*SLAMetric, len(metrics))
	for i, m := range metrics {
		result[i] = entMetricToModel(m)
	}
	return result, nil
}

func (r *EntRepository) CancelMetricsByOrder(ctx context.Context, tenantID uuid.UUID, orderID uuid.UUID) error {
	_, err := r.client.SLAMetric.Update().
		Where(
			slametric.TenantID(tenantID),
			slametric.OrderID(orderID),
			slametric.StatusEQ(slametric.StatusTracking),
		).
		SetStatus(slametric.StatusCancelled).
		SetMeasuredAt(time.Now()).
		Save(ctx)
	return err
}

func (r *EntRepository) GetMetricStats(ctx context.Context, tenantID uuid.UUID, from, to time.Time) (*SLASummary, error) {
	metrics, err := r.client.SLAMetric.Query().
		Where(
			slametric.TenantID(tenantID),
			slametric.MeasuredAtGTE(from),
			slametric.MeasuredAtLTE(to),
			slametric.StatusIn(slametric.StatusMet, slametric.StatusBreached),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}

	summary := &SLASummary{
		TenantID:    tenantID,
		Period:      from.Format("2006-01-02") + " to " + to.Format("2006-01-02"),
		ByType:      make(map[MetricType]TypeSummary),
	}

	byType := make(map[MetricType]*TypeSummary)
	var totalBreachPct float64
	var breachCount int

	for _, m := range metrics {
		summary.TotalMetrics++
		mt := MetricType(m.MetricType)

		ts, ok := byType[mt]
		if !ok {
			ts = &TypeSummary{}
			byType[mt] = ts
		}
		ts.Total++

		if m.Status == slametric.StatusMet {
			summary.MetMetrics++
			ts.Met++
		} else if m.Status == slametric.StatusBreached {
			summary.BreachedMetrics++
			ts.Breached++
			if m.BreachPercentage != nil {
				totalBreachPct += *m.BreachPercentage
				breachCount++
			}
		}

		if m.ActualSeconds != nil {
			ts.AverageSeconds += *m.ActualSeconds
		}
	}

	// Calculate compliance rates
	if summary.TotalMetrics > 0 {
		summary.ComplianceRate = float64(summary.MetMetrics) / float64(summary.TotalMetrics) * 100
	}
	if breachCount > 0 {
		summary.AverageBreachPct = totalBreachPct / float64(breachCount)
	}

	for mt, ts := range byType {
		if ts.Total > 0 {
			ts.ComplianceRate = float64(ts.Met) / float64(ts.Total) * 100
			ts.AverageSeconds = ts.AverageSeconds / ts.Total
		}
		summary.ByType[mt] = *ts
	}

	return summary, nil
}

func (r *EntRepository) GetBreachedMetrics(ctx context.Context, tenantID uuid.UUID, limit int) ([]*SLAMetric, error) {
	metrics, err := r.client.SLAMetric.Query().
		Where(
			slametric.TenantID(tenantID),
			slametric.StatusEQ(slametric.StatusBreached),
		).
		Order(ent.Desc(slametric.FieldMeasuredAt)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]*SLAMetric, len(metrics))
	for i, m := range metrics {
		result[i] = entMetricToModel(m)
	}
	return result, nil
}

// Converter

func entMetricToModel(m *ent.SLAMetric) *SLAMetric {
	return &SLAMetric{
		ID:               m.ID,
		TenantID:         m.TenantID,
		OrderID:          m.OrderID,
		MetricType:       MetricType(m.MetricType),
		TargetSeconds:    m.TargetSeconds,
		ActualSeconds:    m.ActualSeconds,
		Status:           MetricStatus(m.Status),
		BreachPercentage: m.BreachPercentage,
		StartedAt:        m.StartedAt,
		EndedAt:          m.EndedAt,
		MeasuredAt:       m.MeasuredAt,
		Metadata:         m.Metadata,
		CreatedAt:        m.CreatedAt,
		UpdatedAt:        m.UpdatedAt,
	}
}
