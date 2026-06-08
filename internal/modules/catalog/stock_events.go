package catalog

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	sharedevents "github.com/Bengo-Hub/shared-events"
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
		{"inventory.item.updated", "ord-inventory-item-updated", h.handleItemUpdated},
	}

	for _, s := range subs {
		s := s
		sharedevents.SubscribeWithRebind(h.logger, js, s.subject, func(msg *nats.Msg) {
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
			"inventory.item.updated",
		}))
	return nil
}

// setSkuAvailability toggles catalog availability for a SKU. When the event
// carries an outlet_id it UPSERTS the (tenant, outlet, sku) override row, so a
// default-available item that has no override row yet is still toggled — the old
// UPDATE-only path silently affected 0 rows for such items, leaving sold-out
// recipes orderable. Only is_available is written; base_price stays 0, which the
// catalog merge treats as "no price override" (same as handleItemCreated). When
// no outlet_id is present (shared/HQ warehouse or legacy events) it falls back to
// updating every existing override for the SKU.
func (h *StockEventHandler) setSkuAvailability(ctx context.Context, tenantID uuid.UUID, outletRaw, sku string, available bool) (int, error) {
	if outletID, err := uuid.Parse(outletRaw); outletRaw != "" && err == nil {
		if uerr := h.db.CatalogOverride.Create().
			SetTenantID(tenantID).
			SetOutletID(outletID).
			SetInventorySku(sku).
			SetIsAvailable(available).
			OnConflictColumns(
				catalogoverride.FieldTenantID,
				catalogoverride.FieldOutletID,
				catalogoverride.FieldInventorySku,
			).
			Update(func(u *ent.CatalogOverrideUpsert) {
				u.SetIsAvailable(available)
				u.SetUpdatedAt(time.Now())
			}).
			Exec(ctx); uerr != nil {
			return 0, uerr
		}
		return 1, nil
	}
	return h.db.CatalogOverride.Update().
		Where(
			catalogoverride.TenantID(tenantID),
			catalogoverride.InventorySku(sku),
		).
		SetIsAvailable(available).
		Save(ctx)
}

// handleStockOut marks catalog items unavailable when stock runs out.
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
	count, err := h.setSkuAvailability(ctx, tenantID, outletRaw, sku, false)
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

	// Re-enable the (outlet, sku) override the stock-out cascade disabled. Manual
	// disabling is handled separately via item.updated events.
	outletRaw, _ := evt.Payload["outlet_id"].(string)
	count, err := h.setSkuAvailability(ctx, tenantID, outletRaw, sku, true)
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
		count, err := h.setSkuAvailability(ctx, tenantID, outletRaw, sku, false)
		if err != nil {
			return fmt.Errorf("mark item unavailable on deactivation: %w", err)
		}
		h.logger.Info("item marked unavailable due to inventory deactivation",
			zap.String("sku", sku),
			zap.Int("overrides_updated", count))
	}

	return nil
}
