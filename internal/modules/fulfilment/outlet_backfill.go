package fulfilment

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// TaskOutletSetter is the logistics call the backfill uses (logistics.Client.SetTaskOutlet).
type TaskOutletSetter interface {
	SetTaskOutlet(ctx context.Context, tenantID uuid.UUID, externalRef string, outletID uuid.UUID) (int, error)
}

// BackfillTaskOutlets gives logistics the outlet of every delivery task an order created before
// tasks carried one (owner, 18 November 2026: outlet-scoped dispatch boards showed nothing for
// them). The order owns its outlet, so ordering sends it; logistics changes only tasks with no
// outlet, so the run is idempotent and safe on every replica and every restart. Bounded: one
// small batch per order with assignments, a short timeout per call.
func BackfillTaskOutlets(ctx context.Context, db *sql.DB, setter TaskOutletSetter, log *zap.Logger) {
	if db == nil || setter == nil {
		return
	}
	rows, err := db.QueryContext(ctx, `
		SELECT DISTINCT o.id, o.tenant_id, o.outlet_id
		FROM orders o
		JOIN order_assignments a ON a.order_id = o.id
		WHERE o.outlet_id IS NOT NULL AND o.outlet_id <> '00000000-0000-0000-0000-000000000000'`)
	if err != nil {
		log.Warn("task outlet backfill: list orders", zap.Error(err))
		return
	}
	type row struct{ order, tenant, outlet uuid.UUID }
	var todo []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.order, &r.tenant, &r.outlet); err == nil {
			todo = append(todo, r)
		}
	}
	_ = rows.Close()
	updated, failed := 0, 0
	for _, r := range todo {
		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		n, err := setter.SetTaskOutlet(cctx, r.tenant, "order:"+r.order.String(), r.outlet)
		cancel()
		if err != nil {
			failed++
			continue
		}
		updated += n
	}
	log.Info("task outlet backfill done", zap.Int("orders", len(todo)), zap.Int("tasks_updated", updated), zap.Int("failed", failed))
}
