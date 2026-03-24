package catalog

import (
	"context"
	"encoding/json"
	"fmt"

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

// SubscribeToStockEvents subscribes to inventory stock-out and item-updated events via NATS.
func (h *StockEventHandler) SubscribeToStockEvents(nc *nats.Conn) error {
	if nc == nil {
		h.logger.Warn("NATS connection not available, skipping stock event subscriptions")
		return nil
	}

	// Subscribe to inventory.stock.out
	_, err := nc.Subscribe("inventory.stock.out", func(msg *nats.Msg) {
		var evt InventoryItemEvent
		if err := json.Unmarshal(msg.Data, &evt); err != nil {
			h.logger.Error("failed to unmarshal inventory.stock.out event", zap.Error(err))
			return
		}
		ctx := context.Background()
		if err := h.handleStockOut(ctx, &evt); err != nil {
			h.logger.Error("failed to handle inventory.stock.out event", zap.Error(err))
			return
		}
		_ = msg.Ack()
	})
	if err != nil {
		return fmt.Errorf("catalog: subscribe to inventory.stock.out: %w", err)
	}

	// Subscribe to inventory.item.updated
	_, err = nc.Subscribe("inventory.item.updated", func(msg *nats.Msg) {
		var evt InventoryItemEvent
		if err := json.Unmarshal(msg.Data, &evt); err != nil {
			h.logger.Error("failed to unmarshal inventory.item.updated event", zap.Error(err))
			return
		}
		ctx := context.Background()
		if err := h.handleItemUpdated(ctx, &evt); err != nil {
			h.logger.Error("failed to handle inventory.item.updated event", zap.Error(err))
			return
		}
		_ = msg.Ack()
	})
	if err != nil {
		return fmt.Errorf("catalog: subscribe to inventory.item.updated: %w", err)
	}

	h.logger.Info("stock event subscriptions active",
		zap.Strings("subjects", []string{"inventory.stock.out", "inventory.item.updated"}))
	return nil
}

// handleStockOut marks catalog items unavailable when stock runs out.
func (h *StockEventHandler) handleStockOut(ctx context.Context, evt *InventoryItemEvent) error {
	tenantID, err := uuid.Parse(evt.TenantID)
	if err != nil {
		return fmt.Errorf("invalid tenant_id: %w", err)
	}

	sku, _ := evt.Data["sku"].(string)
	if sku == "" {
		return fmt.Errorf("no sku in stock-out event data")
	}

	// Find and update all CatalogOverride entries for this SKU
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

// handleItemUpdated syncs catalog override metadata when an inventory item is updated.
func (h *StockEventHandler) handleItemUpdated(ctx context.Context, evt *InventoryItemEvent) error {
	tenantID, err := uuid.Parse(evt.TenantID)
	if err != nil {
		return fmt.Errorf("invalid tenant_id: %w", err)
	}

	sku, _ := evt.Data["sku"].(string)
	if sku == "" {
		return fmt.Errorf("no sku in item-updated event data")
	}

	isActive, hasActive := evt.Data["is_active"].(bool)
	if !hasActive {
		// No is_active field in update; nothing to do
		h.logger.Debug("inventory.item.updated without is_active, skipping",
			zap.String("sku", sku))
		return nil
	}

	if !isActive {
		// Item deactivated in inventory: mark catalog overrides unavailable
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
