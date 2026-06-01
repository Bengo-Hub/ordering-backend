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
		_, err := js.Subscribe(s.subject, func(msg *nats.Msg) {
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
		if err != nil {
			return fmt.Errorf("catalog: subscribe to %s: %w", s.subject, err)
		}
	}

	h.logger.Info("stock event subscriptions active (JetStream)",
		zap.Strings("subjects", []string{
			"inventory.stock.out",
			"inventory.stock.in",
			"inventory.item.updated",
		}))
	return nil
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

	count, err := h.db.CatalogOverride.Update().
		Where(
			catalogoverride.TenantID(tenantID),
			catalogoverride.InventorySku(sku),
		).
		SetIsAvailable(false).
		Save(ctx)
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

	// Only re-enable overrides that were blocked by a stock-out cascade (not manually disabled).
	// We re-enable ALL overrides for this SKU — manual disabling is handled via item.updated events.
	count, err := h.db.CatalogOverride.Update().
		Where(
			catalogoverride.TenantID(tenantID),
			catalogoverride.InventorySku(sku),
		).
		SetIsAvailable(true).
		Save(ctx)
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
		count, err := h.db.CatalogOverride.Update().
			Where(
				catalogoverride.TenantID(tenantID),
				catalogoverride.InventorySku(sku),
			).
			SetIsAvailable(false).
			Save(ctx)
		if err != nil {
			return fmt.Errorf("mark item unavailable on deactivation: %w", err)
		}
		h.logger.Info("item marked unavailable due to inventory deactivation",
			zap.String("sku", sku),
			zap.Int("overrides_updated", count))
	}

	return nil
}
