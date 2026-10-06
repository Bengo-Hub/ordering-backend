package ordering

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Revenue counts only money the business keeps: the order was paid (online, or collected at the
// door or counter, which marks it paid) and was not cancelled, refunded or timed out. Order counts
// include every order placed in the range.
const revenueCondition = `status NOT IN ('cancelled','refunded','payment_timeout') AND payment_status IN ('paid','cod_collected')`

// topSellingLimit caps the best-seller list on the summary.
const topSellingLimit = 10

type statusCurrencyRow struct {
	Status   string
	Currency string
	Orders   int
	Revenue  float64
}

type dailyRow struct {
	Date    string
	Orders  int
	Revenue float64
}

// GetAnalyticsSummary aggregates the tenant's orders for the range in SQL (GROUP BY status and
// currency, by day, and by item) instead of loading every order and line into memory.
func (r *EntRepository) GetAnalyticsSummary(ctx context.Context, tenantID uuid.UUID, dateFrom, dateTo time.Time) (*AnalyticsSummary, error) {
	if r.db == nil {
		return nil, errors.New("analytics: database handle not configured")
	}

	statusRows, err := r.db.QueryContext(ctx, `
SELECT status, currency, count(*), coalesce(sum(grand_total) FILTER (WHERE `+revenueCondition+`), 0)
FROM orders
WHERE tenant_id = $1 AND created_at >= $2 AND created_at <= $3
GROUP BY status, currency`, tenantID, dateFrom, dateTo)
	if err != nil {
		return nil, err
	}
	var byStatus []statusCurrencyRow
	for statusRows.Next() {
		var row statusCurrencyRow
		if err := statusRows.Scan(&row.Status, &row.Currency, &row.Orders, &row.Revenue); err != nil {
			statusRows.Close()
			return nil, err
		}
		byStatus = append(byStatus, row)
	}
	statusRows.Close()
	if err := statusRows.Err(); err != nil {
		return nil, err
	}

	dayRows, err := r.db.QueryContext(ctx, `
SELECT to_char((created_at AT TIME ZONE 'UTC')::date, 'YYYY-MM-DD'), count(*),
       coalesce(sum(grand_total) FILTER (WHERE `+revenueCondition+`), 0)
FROM orders
WHERE tenant_id = $1 AND created_at >= $2 AND created_at <= $3
GROUP BY 1`, tenantID, dateFrom, dateTo)
	if err != nil {
		return nil, err
	}
	var daily []dailyRow
	for dayRows.Next() {
		var row dailyRow
		if err := dayRows.Scan(&row.Date, &row.Orders, &row.Revenue); err != nil {
			dayRows.Close()
			return nil, err
		}
		daily = append(daily, row)
	}
	dayRows.Close()
	if err := dayRows.Err(); err != nil {
		return nil, err
	}

	itemRows, err := r.db.QueryContext(ctx, `
SELECT oi.inventory_sku, max(oi.name_snapshot), sum(oi.quantity), sum(oi.total_price)
FROM order_items oi
JOIN orders o ON o.id = oi.order_id
WHERE o.tenant_id = $1 AND o.created_at >= $2 AND o.created_at <= $3
  AND o.status NOT IN ('cancelled','refunded','payment_timeout')
  AND o.payment_status IN ('paid','cod_collected')
GROUP BY oi.inventory_sku
ORDER BY 4 DESC
LIMIT $4`, tenantID, dateFrom, dateTo, topSellingLimit)
	if err != nil {
		return nil, err
	}
	top := make([]ItemSalesSummary, 0, topSellingLimit)
	for itemRows.Next() {
		var it ItemSalesSummary
		if err := itemRows.Scan(&it.InventorySKU, &it.NameSnapshot, &it.Quantity, &it.Revenue); err != nil {
			itemRows.Close()
			return nil, err
		}
		top = append(top, it)
	}
	itemRows.Close()
	if err := itemRows.Err(); err != nil {
		return nil, err
	}

	return buildAnalyticsSummary(dateFrom, dateTo, byStatus, daily, top), nil
}

// buildAnalyticsSummary assembles the grouped rows into the summary: totals, per status and per
// currency figures, and a trend with one entry for every UTC day in the range (days without
// orders show zero).
func buildAnalyticsSummary(dateFrom, dateTo time.Time, byStatus []statusCurrencyRow, daily []dailyRow, top []ItemSalesSummary) *AnalyticsSummary {
	summary := &AnalyticsSummary{
		OrdersByStatus:    map[string]int{},
		RevenueByCurrency: map[string]float64{},
		TopSellingItems:   top,
		Trend:             []DailyMetric{},
	}
	if summary.TopSellingItems == nil {
		summary.TopSellingItems = []ItemSalesSummary{}
	}
	for _, row := range byStatus {
		summary.TotalOrders += row.Orders
		summary.TotalRevenue += row.Revenue
		summary.OrdersByStatus[row.Status] += row.Orders
		if row.Revenue != 0 {
			summary.RevenueByCurrency[row.Currency] += row.Revenue
		}
		if row.Status == string(OrderStatusCancelled) {
			summary.CancelledOrders += row.Orders
		}
	}

	byDay := make(map[string]dailyRow, len(daily))
	for _, d := range daily {
		byDay[d.Date] = d
	}
	from := dateFrom.UTC().Truncate(24 * time.Hour)
	to := dateTo.UTC().Truncate(24 * time.Hour)
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		key := d.Format("2006-01-02")
		row := byDay[key]
		summary.Trend = append(summary.Trend, DailyMetric{Date: key, Orders: row.Orders, Revenue: row.Revenue})
	}
	return summary
}

// StatusCounts returns how many of the tenant's orders sit in each open status (pending through
// out_for_delivery), optionally for one outlet. Served by the (tenant_id, status) indexes.
func (r *EntRepository) StatusCounts(ctx context.Context, tenantID uuid.UUID, outletID *uuid.UUID) (map[string]int, error) {
	if r.db == nil {
		return nil, errors.New("status counts: database handle not configured")
	}
	query := `
SELECT status, count(*) FROM orders
WHERE tenant_id = $1 AND status IN ('pending','confirmed','preparing','ready','out_for_delivery')`
	args := []any{tenantID}
	if outletID != nil {
		query += ` AND outlet_id = $2`
		args = append(args, *outletID)
	}
	query += ` GROUP BY status`
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			return nil, err
		}
		counts[status] = n
	}
	return counts, rows.Err()
}
