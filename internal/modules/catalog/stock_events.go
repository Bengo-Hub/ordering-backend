package catalog

import (
	"context"
	"fmt"
	"time"

	sharedevents "github.com/Bengo-Hub/shared-events"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/ent"
	"github.com/bengobox/ordering-backend/internal/ent/catalogoverride"
)

// StockEventHandler handles inventory stock-out and item-updated events.
type StockEventHandler struct {
	db     *ent.Client
	logger *zap.Logger
}

// NewStockEventHandler creates a new stock event handler.
func NewStockEventHandler(db *ent.Client, logger *zap.Logger) *StockEventHandler {
	return &StockEventHandler{
		db:     db,
		logger: logger.Named("catalog.stock_events"),
	}
}

// SubscribeToStockEvents subscribes to inventory stock events via JetStream.
func (h *StockEventHandler) SubscribeToStockEvents(js nats.JetStreamContext) error {
	// ensureInventoryStream is defined in inventory_events.go (same package).
	if err := ensureInventoryStream(js); err != nil {
		return fmt.Errorf("catalog: ensure inventory stream for stock: %w", err)
	}

	type sub struct {
		subject string
		durable string
		handler func(context.Context, *sharedevents.Event) error
	}
	subs := []sub{
		{"inventory.stock.out", "ord-inventory-stock-out", h.handleStockOut},
		{"inventory.stock.in", "ord-inventory-stock-in", h.handleStockIn},
		{"inventory.stock.updated", "ord-inventory-stock-updated", h.handleStockUpdated},
		{"inventory.item.updated", "ord-inventory-item-updated", h.handleItemUpdated},
	}

	for _, s := range subs {
		s := s
		sharedevents.SubscribeQueueWithRebind(h.logger, js, inventoryStreamName, s.subject, s.durable, func(msg *nats.Msg) {
			evt, parseErr := sharedevents.FromJSON(msg.Data)
			if parseErr != nil {
				h.logger.Error("failed to parse stock event envelope",
					zap.String("subject", s.subject), zap.Error(parseErr))
				_ = msg.Ack()
				return
			}
			ctx := context.Background()
			if err := s.handler(ctx, evt); err != nil {
				h.logger.Error("failed to handle stock event",
					zap.String("subject", s.subject), zap.Error(err))
				_ = msg.Nak()
				return
			}
			_ = msg.Ack()
		},
			nats.Durable(s.durable),
			nats.DeliverAll(),
			nats.AckExplicit(),
			nats.AckWait(30*time.Second),
			nats.MaxDeliver(5),
			nats.BindStream(inventoryStreamName),
		)
	}

	h.logger.Info("stock event subscriptions active (JetStream)",
		zap.Strings("subjects", []string{
			"inventory.stock.out",
			"inventory.stock.in",
			"inventory.stock.updated",
			"inventory.item.updated",
		}))
	return nil
}

// handleStockUpdated keeps the quantity-aware projection (STK-5) fresh for direct items whose
// on-hand quantity changes WITHOUT crossing the sold-out / restock boolean threshold (e.g. an
// item drops from 50 → 12 but is still available). It updates available_quantity only; the
// boolean is_available remains owned by the stock.out/stock.in cascade and item.updated. Rows
// are only updated where they already exist — a stock.updated for an item with no catalog
// override yet is a no-op (creation happens via inventory.item.created).
func (h *StockEventHandler) handleStockUpdated(ctx context.Context, evt *sharedevents.Event) error {
	tenantID := evt.TenantID
	if tenantID == uuid.Nil {
		return fmt.Errorf("invalid or missing tenant_id in stock-updated event")
	}
	sku, _ := evt.Payload["sku"].(string)
	if sku == "" {
		return fmt.Errorf("no sku in stock-updated event payload")
	}
	qty := eventQuantity(evt.Payload)
	if qty == nil {
		return nil // nothing quantity-aware to record
	}

	q := h.db.CatalogOverride.Update().
		Where(catalogoverride.TenantID(tenantID), catalogoverride.InventorySku(sku))
	if outletID, err := uuid.Parse(getString(evt.Payload, "outlet_id")); err == nil {
		q = q.Where(catalogoverride.OutletID(outletID))
	}
	count, err := q.SetAvailableQuantity(*qty).Save(ctx)
	if err != nil {
		return fmt.Errorf("update available_quantity on stock.updated: %w", err)
	}
	h.logger.Debug("catalog available_quantity refreshed from stock.updated",
		zap.String("sku", sku), zap.Float64p("available_quantity", qty), zap.Int("rows", count))
	return nil
}

// getString safely reads a string payload field (empty when absent or wrong type).
func getString(payload map[string]interface{}, key string) string {
	s, _ := payload[key].(string)
	return s
}

// setSkuAvailability toggles catalog availability for a SKU. When the event
// carries an outlet_id it UPSERTS the (tenant, outlet, sku) override row, so a
// default-available item that has no override row yet is still toggled — the old
// UPDATE-only path silently affected 0 rows for such items, leaving sold-out
// recipes orderable. Only is_available is written; base_price stays 0, which the
// catalog merge treats as "no price override" (same as handleItemCreated). When
// no outlet_id is present (shared/HQ warehouse or legacy events) it falls back to
// updating every existing override for the SKU.
func (h *StockEventHandler) setSkuAvailability(ctx context.Context, tenantID uuid.UUID, outletRaw, sku string, available bool, qty *float64) (int, error) {
	if outletID, err := uuid.Parse(outletRaw); outletRaw != "" && err == nil {
		builder := h.db.CatalogOverride.Create().
			SetTenantID(tenantID).
			SetOutletID(outletID).
			SetInventorySku(sku).
			SetIsAvailable(available)
		if qty != nil {
			builder = builder.SetAvailableQuantity(*qty)
		}
		if uerr := builder.
			OnConflictColumns(
				catalogoverride.FieldTenantID,
				catalogoverride.FieldOutletID,
				catalogoverride.FieldInventorySku,
			).
			Update(func(u *ent.CatalogOverrideUpsert) {
				u.SetIsAvailable(available)
				u.SetUpdatedAt(time.Now())
				// Quantity-aware projection (STK-5): persist the on-hand/producible qty the
				// event carried so the catalog reflects real stock levels, not just a boolean.
				if qty != nil {
					u.SetAvailableQuantity(*qty)
				}
			}).
			Exec(ctx); uerr != nil {
			return 0, uerr
		}
		return 1, nil
	}
	upd := h.db.CatalogOverride.Update().
		Where(
			catalogoverride.TenantID(tenantID),
			catalogoverride.InventorySku(sku),
		).
		SetIsAvailable(available)
	if qty != nil {
		upd = upd.SetAvailableQuantity(*qty)
	}
	return upd.Save(ctx)
}

// eventQuantity extracts the on-hand/available quantity from a stock event payload, preferring
// "available", then "on_hand", then "quantity_after". Returns nil when none is present so the
// boolean-only path (and existing rows) stay untouched. JSON numbers decode as float64.
func eventQuantity(payload map[string]interface{}) *float64 {
	for _, key := range []string{"available", "on_hand", "quantity_after"} {
		if v, ok := payload[key]; ok {
			if f, ok := v.(float64); ok {
				return &f
			}
		}
	}
	return nil
}

// affectsAvailability reports whether a stock.out/stock.in event may change is_available.
// inventory-api sets it from the tenant's auto_hide_on_stock_out policy (off by default:
// availability is manual-only and a stock-out is just an alert). A missing field means an
// event published before the policy existed, which keeps its original meaning (true).
func affectsAvailability(payload map[string]interface{}) bool {
	v, ok := payload["affects_availability"].(bool)
	return !ok || v
}

// refreshQuantityOnly records the event's quantity on EXISTING override rows without touching
// is_available or creating rows. Used for alert-only stock events under manual availability.
func (h *StockEventHandler) refreshQuantityOnly(ctx context.Context, tenantID uuid.UUID, outletRaw, sku string, qty *float64) (int, error) {
	if qty == nil {
		return 0, nil
	}
	q := h.db.CatalogOverride.Update().
		Where(catalogoverride.TenantID(tenantID), catalogoverride.InventorySku(sku))
	if outletID, err := uuid.Parse(outletRaw); outletRaw != "" && err == nil {
		q = q.Where(catalogoverride.OutletID(outletID))
	}
	return q.SetAvailableQuantity(*qty).Save(ctx)
}

// handleStockOut marks catalog items unavailable when stock runs out, but only for tenants
// whose inventory policy auto-hides on stock-out; otherwise it is an alert and the item stays
// orderable (staff-only availability).
func (h *StockEventHandler) handleStockOut(ctx context.Context, evt *sharedevents.Event) error {
	tenantID := evt.TenantID
	if tenantID == uuid.Nil {
		return fmt.Errorf("invalid or missing tenant_id in stock-out event")
	}

	sku, _ := evt.Payload["sku"].(string)
	if sku == "" {
		return fmt.Errorf("no sku in stock-out event payload")
	}

	outletRaw, _ := evt.Payload["outlet_id"].(string)
	if !affectsAvailability(evt.Payload) {
		if _, err := h.refreshQuantityOnly(ctx, tenantID, outletRaw, sku, eventQuantity(evt.Payload)); err != nil {
			return fmt.Errorf("refresh quantity on alert-only stock-out: %w", err)
		}
		return nil
	}
	// stock.out carries available (0 for a depleted item / blocked recipe); persist it so the
	// projection records a real zero rather than leaving the last-seen quantity stale.
	count, err := h.setSkuAvailability(ctx, tenantID, outletRaw, sku, false, eventQuantity(evt.Payload))
	if err != nil {
		return fmt.Errorf("mark item unavailable: %w", err)
	}

	h.logger.Info("item marked unavailable due to stock-out",
		zap.String("sku", sku),
		zap.String("tenant_id", tenantID.String()),
		zap.Int("overrides_updated", count))
	return nil
}

// handleStockIn re-enables catalog overrides when a recipe's ingredients are restocked.
// Triggered by the inventory-service cascade when all ingredients become available again.
func (h *StockEventHandler) handleStockIn(ctx context.Context, evt *sharedevents.Event) error {
	tenantID := evt.TenantID
	if tenantID == uuid.Nil {
		return fmt.Errorf("invalid or missing tenant_id in stock-in event")
	}

	sku, _ := evt.Payload["sku"].(string)
	if sku == "" {
		return fmt.Errorf("no sku in stock-in event payload")
	}

	// Re-enable the (outlet, sku) override the stock-out cascade disabled. Only for tenants
	// that auto-hide; under manual availability a restock must never undo a staff toggle.
	outletRaw, _ := evt.Payload["outlet_id"].(string)
	if !affectsAvailability(evt.Payload) {
		if _, err := h.refreshQuantityOnly(ctx, tenantID, outletRaw, sku, eventQuantity(evt.Payload)); err != nil {
			return fmt.Errorf("refresh quantity on alert-only stock-in: %w", err)
		}
		return nil
	}
	// stock.in now carries the producible/on-hand quantity (STK-5); persist it alongside re-enabling.
	count, err := h.setSkuAvailability(ctx, tenantID, outletRaw, sku, true, eventQuantity(evt.Payload))
	if err != nil {
		return fmt.Errorf("re-enable item after restock: %w", err)
	}

	h.logger.Info("item re-enabled after ingredient restock",
		zap.String("sku", sku),
		zap.String("tenant_id", tenantID.String()),
		zap.Int("overrides_updated", count))
	return nil
}

// handleItemUpdated syncs catalog override metadata when an inventory item is updated.
func (h *StockEventHandler) handleItemUpdated(ctx context.Context, evt *sharedevents.Event) error {
	tenantID := evt.TenantID
	if tenantID == uuid.Nil {
		return fmt.Errorf("invalid or missing tenant_id in item-updated event")
	}

	sku, _ := evt.Payload["sku"].(string)
	if sku == "" {
		return fmt.Errorf("no sku in item-updated event payload")
	}

	isActive, hasActive := evt.Payload["is_active"].(bool)
	if !hasActive {
		h.logger.Debug("inventory.item.updated without is_active, skipping",
			zap.String("sku", sku))
		return nil
	}

	if !isActive {
		outletRaw, _ := evt.Payload["outlet_id"].(string)
		// Deactivation is a hard off-switch; force quantity to 0 so the projection isn't stale.
		zero := 0.0
		count, err := h.setSkuAvailability(ctx, tenantID, outletRaw, sku, false, &zero)
		if err != nil {
			return fmt.Errorf("mark item unavailable on deactivation: %w", err)
		}
		h.logger.Info("item marked unavailable due to inventory deactivation",
			zap.String("sku", sku),
			zap.Int("overrides_updated", count))
	}

	return nil
}
