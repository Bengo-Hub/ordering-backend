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

// InventoryItemEvent represents an inventory item event from inventory-service.
type InventoryItemEvent struct {
	ID            string                 `json:"id"`
	TenantID      string                 `json:"tenantId"`
	AggregateType string                 `json:"aggregateType"`
	AggregateID   string                 `json:"aggregateId"`
	EventType     string                 `json:"type"`
	Data          map[string]interface{} `json:"data"`
	Timestamp     string                 `json:"timestamp"`
}

// InventoryEventHandler handles inventory events by auto-creating CatalogOverride entries.
type InventoryEventHandler struct {
	db     *ent.Client
	logger *zap.Logger
}

// NewInventoryEventHandler creates a new inventory event handler.
func NewInventoryEventHandler(db *ent.Client, logger *zap.Logger) *InventoryEventHandler {
	return &InventoryEventHandler{
		db:     db,
		logger: logger.Named("catalog.inventory_events"),
	}
}

// SubscribeToInventoryEvents subscribes to inventory-service events via NATS.
func (h *InventoryEventHandler) SubscribeToInventoryEvents(nc *nats.Conn) error {
	// Subscribe to inventory.item.created
	_, err := nc.Subscribe("inventory.item.created", func(msg *nats.Msg) {
		var evt InventoryItemEvent
		if err := json.Unmarshal(msg.Data, &evt); err != nil {
			h.logger.Error("failed to unmarshal inventory.item.created event", zap.Error(err))
			return
		}

		ctx := context.Background()
		if err := h.handleItemCreated(ctx, &evt); err != nil {
			h.logger.Error("failed to handle inventory.item.created event", zap.Error(err))
			return
		}
		_ = msg.Ack()
	})
	if err != nil {
		return fmt.Errorf("catalog: subscribe to inventory.item.created: %w", err)
	}

	h.logger.Info("inventory event subscriptions active",
		zap.String("subjects", "inventory.item.created"))
	return nil
}

// handleItemCreated auto-creates a CatalogOverride entry when an inventory item is created.
func (h *InventoryEventHandler) handleItemCreated(ctx context.Context, evt *InventoryItemEvent) error {
	tenantID, err := uuid.Parse(evt.TenantID)
	if err != nil {
		return fmt.Errorf("invalid tenant_id: %w", err)
	}

	data := evt.Data
	sku, _ := data["sku"].(string)
	if sku == "" {
		return fmt.Errorf("no sku in event data")
	}

	isActive, _ := data["is_active"].(bool)

	// Check if override already exists
	exists, _ := h.db.CatalogOverride.Query().
		Where(
			catalogoverride.TenantID(tenantID),
			catalogoverride.InventorySku(sku),
		).Exist(ctx)

	if exists {
		h.logger.Info("catalog override already exists, skipping",
			zap.String("sku", sku))
		return nil
	}

	// Resolve default outlet for the tenant
	outlets, err := h.db.Outlet.Query().All(ctx)
	if err != nil || len(outlets) == 0 {
		h.logger.Warn("could not resolve default outlet, skipping catalog override creation",
			zap.String("tenant_id", tenantID.String()))
		return nil
	}

	outletID := outlets[0].ID

	// Create override with default values
	_, err = h.db.CatalogOverride.Create().
		SetTenantID(tenantID).
		SetOutletID(outletID).
		SetInventorySku(sku).
		SetIsAvailable(isActive).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("create catalog override from event: %w", err)
	}

	h.logger.Info("catalog override created from inventory event",
		zap.String("sku", sku))
	return nil
}
