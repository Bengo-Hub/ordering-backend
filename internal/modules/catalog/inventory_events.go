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

const (
	inventoryStreamName     = "inventory"
	inventoryStreamSubjects = "inventory.>"
	inventoryStreamMaxAge   = 72 * time.Hour
)

// ensureInventoryStream ensures the "inventory" JetStream stream exists.
func ensureInventoryStream(js nats.JetStreamContext) error {
	_, err := js.StreamInfo(inventoryStreamName)
	if err == nil {
		return nil
	}
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     inventoryStreamName,
		Subjects: []string{inventoryStreamSubjects},
		MaxAge:   inventoryStreamMaxAge,
		Storage:  nats.FileStorage,
		Replicas: 1,
	})
	if err != nil {
		return fmt.Errorf("create inventory stream: %w", err)
	}
	return nil
}

// InventoryEventHandler handles inventory events by auto-creating CatalogOverride entries.
type InventoryEventHandler struct {
	db     *ent.Client
	logger *zap.Logger
	// hasFeature gates inventory→catalog sync by subscription entitlement. When set, catalog
	// overrides are only created for tenants entitled to basic_inventory_access. Nil → fail open.
	hasFeature func(ctx context.Context, tenantID, feature string) bool
}

// NewInventoryEventHandler creates a new inventory event handler.
func NewInventoryEventHandler(db *ent.Client, logger *zap.Logger) *InventoryEventHandler {
	return &InventoryEventHandler{
		db:     db,
		logger: logger.Named("catalog.inventory_events"),
	}
}

// SetFeatureGate wires the subscription entitlement check used to gate catalog sync.
func (h *InventoryEventHandler) SetFeatureGate(fn func(ctx context.Context, tenantID, feature string) bool) {
	h.hasFeature = fn
}

// SubscribeToInventoryEvents subscribes to inventory-service events via JetStream durable consumers.
func (h *InventoryEventHandler) SubscribeToInventoryEvents(js nats.JetStreamContext) error {
	if err := ensureInventoryStream(js); err != nil {
		return fmt.Errorf("catalog: ensure inventory stream: %w", err)
	}

	sharedevents.SubscribeQueueWithRebind(h.logger, js, inventoryStreamName, "inventory.item.created", "ord-inventory-item-created", func(msg *nats.Msg) {
		evt, parseErr := sharedevents.FromJSON(msg.Data)
		if parseErr != nil {
			h.logger.Error("failed to parse inventory.item.created envelope", zap.Error(parseErr))
			_ = msg.Ack()
			return
		}

		ctx := context.Background()
		if err := h.handleItemCreated(ctx, evt); err != nil {
			h.logger.Error("failed to handle inventory.item.created event", zap.Error(err))
			_ = msg.Nak()
			return
		}
		_ = msg.Ack()
	},
		nats.Durable("ord-inventory-item-created"),
		nats.DeliverAll(),
		nats.AckExplicit(),
		nats.AckWait(30*time.Second),
		nats.MaxDeliver(5),
		nats.BindStream(inventoryStreamName),
	)

	h.logger.Info("inventory event subscriptions active (JetStream)",
		zap.String("subject", "inventory.item.created"),
		zap.String("durable", "ord-inventory-item-created"))
	return nil
}

// handleItemCreated auto-creates a CatalogOverride entry when an inventory item is created.
func (h *InventoryEventHandler) handleItemCreated(ctx context.Context, evt *sharedevents.Event) error {
	tenantID := evt.TenantID
	if tenantID == uuid.Nil {
		return fmt.Errorf("invalid or missing tenant_id in event")
	}

	sku, _ := evt.Payload["sku"].(string)
	if sku == "" {
		return fmt.Errorf("no sku in event payload")
	}

	// Gate inventory→catalog sync: only tenants entitled to basic_inventory_access get
	// inventory items projected into their ordering catalog. Fails open if no gate wired.
	if h.hasFeature != nil && !h.hasFeature(ctx, tenantID.String(), "basic_inventory_access") {
		h.logger.Debug("inventory.item.created: tenant lacks basic_inventory_access — skipping catalog sync",
			zap.String("tenant_id", tenantID.String()))
		return nil
	}

	isActive, _ := evt.Payload["is_active"].(bool)

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
