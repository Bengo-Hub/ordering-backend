package sla

import (
	"context"
	entsql "entgo.io/ent/dialect/sql"
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

// GetMetricStats summarises SLA metrics in one grouped SQL aggregate (served by the
// (tenant_id, measured_at) index), so the cost stays flat as metrics accumulate.
func (r *EntRepository) GetMetricStats(ctx context.Context, tenantID uuid.UUID, from, to time.Time) (*SLASummary, error) {
	var rows []slaStatRow
	err := r.client.SLAMetric.Query().
		Where(
			slametric.TenantID(tenantID),
			slametric.MeasuredAtGTE(from),
			slametric.MeasuredAtLTE(to),
			slametric.StatusIn(slametric.StatusMet, slametric.StatusBreached),
		).
		GroupBy(slametric.FieldMetricType, slametric.FieldStatus).
		Aggregate(
			func(s *entsql.Selector) string { return "COUNT(*) AS cnt" },
			func(s *entsql.Selector) string {
				return "COALESCE(SUM(" + s.C(slametric.FieldActualSeconds) + "), 0) AS sum_secs"
			},
			func(s *entsql.Selector) string {
				return "COUNT(" + s.C(slametric.FieldActualSeconds) + ") AS with_secs"
			},
			func(s *entsql.Selector) string {
				return "COALESCE(SUM(" + s.C(slametric.FieldBreachPercentage) + "), 0) AS sum_breach"
			},
			func(s *entsql.Selector) string {
				return "COUNT(" + s.C(slametric.FieldBreachPercentage) + ") AS with_breach"
			},
		).
		Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	summary := summarizeSLA(tenantID, from, to, rows)

	// Percentiles per metric type, in the same index-served range.
	var pct []slaPercentileRow
	err = r.client.SLAMetric.Query().
		Where(
			slametric.TenantID(tenantID),
			slametric.MeasuredAtGTE(from),
			slametric.MeasuredAtLTE(to),
			slametric.StatusIn(slametric.StatusMet, slametric.StatusBreached),
			slametric.ActualSecondsNotNil(),
		).
		GroupBy(slametric.FieldMetricType).
		Aggregate(
			func(s *entsql.Selector) string {
				return "COALESCE(percentile_cont(0.5) WITHIN GROUP (ORDER BY " + s.C(slametric.FieldActualSeconds) + "), 0) AS p50"
			},
			func(s *entsql.Selector) string {
				return "COALESCE(percentile_cont(0.9) WITHIN GROUP (ORDER BY " + s.C(slametric.FieldActualSeconds) + "), 0) AS p90"
			},
		).
		Scan(ctx, &pct)
	if err != nil {
		return nil, err
	}
	applyPercentiles(summary, pct)
	return summary, nil
}

// slaPercentileRow is the median and 90th percentile time of one metric type.
type slaPercentileRow struct {
	MetricType string  `json:"metric_type"`
	P50        float64 `json:"p50"`
	P90        float64 `json:"p90"`
}

func applyPercentiles(s *SLASummary, rows []slaPercentileRow) {
	for _, row := range rows {
		mt := MetricType(row.MetricType)
		ts, ok := s.ByType[mt]
		if !ok {
			continue
		}
		ts.P50Seconds, ts.P90Seconds = int(row.P50), int(row.P90)
		s.ByType[mt] = ts
	}
}

// slaStatRow is one (metric type, status) group of the SLA aggregate.
type slaStatRow struct {
	MetricType string  `json:"metric_type"`
	Status     string  `json:"status"`
	Count      int     `json:"cnt"`
	SumSecs    float64 `json:"sum_secs"`
	WithSecs   int     `json:"with_secs"`
	SumBreach  float64 `json:"sum_breach"`
	WithBreach int     `json:"with_breach"`
}

// summarizeSLA folds grouped rows into the summary. Averages divide by the rows that
// actually carry a value.
func summarizeSLA(tenantID uuid.UUID, from, to time.Time, rows []slaStatRow) *SLASummary {
	summary := &SLASummary{
		TenantID: tenantID,
		Period:   from.Format("2006-01-02") + " to " + to.Format("2006-01-02"),
		ByType:   make(map[MetricType]TypeSummary),
	}
	type acc struct {
		TypeSummary
		sumSecs  float64
		withSecs int
	}
	byType := map[MetricType]*acc{}
	var sumBreach float64
	var withBreach int
	for _, row := range rows {
		mt := MetricType(row.MetricType)
		a, ok := byType[mt]
		if !ok {
			a = &acc{}
			byType[mt] = a
		}
		a.Total += row.Count
		a.sumSecs += row.SumSecs
		a.withSecs += row.WithSecs
		summary.TotalMetrics += row.Count
		switch row.Status {
		case string(slametric.StatusMet):
			a.Met += row.Count
			summary.MetMetrics += row.Count
		case string(slametric.StatusBreached):
			a.Breached += row.Count
			summary.BreachedMetrics += row.Count
			sumBreach += row.SumBreach
			withBreach += row.WithBreach
		}
	}
	if summary.TotalMetrics > 0 {
		summary.ComplianceRate = float64(summary.MetMetrics) / float64(summary.TotalMetrics) * 100
	}
	if withBreach > 0 {
		summary.AverageBreachPct = sumBreach / float64(withBreach)
	}
	for mt, a := range byType {
		ts := a.TypeSummary
		if ts.Total > 0 {
			ts.ComplianceRate = float64(ts.Met) / float64(ts.Total) * 100
		}
		if a.withSecs > 0 {
			ts.AverageSeconds = int(a.sumSecs / float64(a.withSecs))
		}
		summary.ByType[mt] = ts
	}
	return summary
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
