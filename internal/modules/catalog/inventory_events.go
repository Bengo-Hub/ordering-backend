package catalog

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
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

// InventoryEventHandler handles inventory events for catalog projection sync.
type InventoryEventHandler struct {
	service *Service
	logger  *zap.Logger
}

// NewInventoryEventHandler creates a new inventory event handler.
func NewInventoryEventHandler(service *Service, logger *zap.Logger) *InventoryEventHandler {
	return &InventoryEventHandler{
		service: service,
		logger:  logger.Named("catalog.inventory_events"),
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

	h.logger.Info("inventory event subscriptions active",
		zap.String("subjects", "inventory.item.created, inventory.item.updated"))
	return nil
}

// handleItemCreated upserts a CatalogItem from an inventory item created event.
func (h *InventoryEventHandler) handleItemCreated(ctx context.Context, evt *InventoryItemEvent) error {
	tenantID, err := uuid.Parse(evt.TenantID)
	if err != nil {
		return fmt.Errorf("invalid tenant_id: %w", err)
	}

	data := evt.Data
	sku, _ := data["sku"].(string)
	name, _ := data["name"].(string)
	description, _ := data["description"].(string)
	imageURL, _ := data["image_url"].(string)
	isActive, _ := data["is_active"].(bool)

	// Parse inventory item ID
	var inventoryItemID *uuid.UUID
	if idStr, ok := data["id"].(string); ok {
		if id, err := uuid.Parse(idStr); err == nil {
			inventoryItemID = &id
		}
	}

	// Check if item already exists by SKU
	existing, _ := h.service.repo.GetCatalogItemBySKU(ctx, tenantID, sku)
	if existing != nil {
		// Update existing item
		req := UpdateCatalogItemRequest{
			TenantID:    tenantID,
			ItemID:      existing.ID,
			Name:        &name,
			Description: &description,
			ImageURL:    &imageURL,
			IsAvailable: &isActive,
			SKU:         &sku,
		}
		if _, err := h.service.UpdateCatalogItem(ctx, req); err != nil {
			return fmt.Errorf("update catalog item from event: %w", err)
		}
		h.logger.Info("catalog item updated from inventory event",
			zap.String("sku", sku), zap.String("event", "inventory.item.created"))
		return nil
	}

	// Resolve default outlet for the tenant
	outletID, outletErr := h.service.resolveDefaultOutlet(ctx, tenantID)
	if outletErr != nil {
		h.logger.Warn("could not resolve default outlet, skipping catalog sync",
			zap.String("tenant_id", tenantID.String()), zap.Error(outletErr))
		return nil
	}

	// Resolve or create category
	categoryID, categoryErr := h.resolveCategory(ctx, tenantID, data)
	if categoryErr != nil {
		h.logger.Warn("could not resolve category, using nil", zap.Error(categoryErr))
	}

	req := CreateCatalogItemRequest{
		TenantID:        tenantID,
		OutletID:        outletID,
		CategoryID:      categoryID,
		InventoryItemID: inventoryItemID,
		Name:            name,
		Description:     description,
		ImageURL:        imageURL,
		IsAvailable:     isActive,
		SKU:             sku,
	}

	if _, err := h.service.CreateCatalogItem(ctx, req); err != nil {
		return fmt.Errorf("create catalog item from event: %w", err)
	}

	h.logger.Info("catalog item created from inventory event",
		zap.String("sku", sku), zap.String("name", name))
	return nil
}

// handleItemUpdated updates a CatalogItem from an inventory item updated event.
func (h *InventoryEventHandler) handleItemUpdated(ctx context.Context, evt *InventoryItemEvent) error {
	tenantID, err := uuid.Parse(evt.TenantID)
	if err != nil {
		return fmt.Errorf("invalid tenant_id: %w", err)
	}

	data := evt.Data
	sku, _ := data["sku"].(string)
	name, _ := data["name"].(string)
	description, _ := data["description"].(string)
	imageURL, _ := data["image_url"].(string)
	isActive, _ := data["is_active"].(bool)

	existing, _ := h.service.repo.GetCatalogItemBySKU(ctx, tenantID, sku)
	if existing == nil {
		// Item doesn't exist yet, create it
		return h.handleItemCreated(ctx, evt)
	}

	req := UpdateCatalogItemRequest{
		TenantID:    tenantID,
		ItemID:      existing.ID,
		Name:        &name,
		Description: &description,
		ImageURL:    &imageURL,
		IsAvailable: &isActive,
		SKU:         &sku,
	}

	if _, err := h.service.UpdateCatalogItem(ctx, req); err != nil {
		return fmt.Errorf("update catalog item from event: %w", err)
	}

	h.logger.Info("catalog item updated from inventory event",
		zap.String("sku", sku), zap.String("name", name))
	return nil
}

// resolveCategory attempts to find or create a category from event data.
func (h *InventoryEventHandler) resolveCategory(ctx context.Context, tenantID uuid.UUID, data map[string]interface{}) (uuid.UUID, error) {
	categoryName, _ := data["category_name"].(string)
	if categoryName == "" {
		return uuid.Nil, fmt.Errorf("no category_name in event data")
	}

	// Try to find existing category by name
	categories, _, err := h.service.repo.ListCategories(ctx, CategoryFilter{
		TenantID: tenantID,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("list categories: %w", err)
	}

	for _, cat := range categories {
		if cat.Name == categoryName {
			return cat.ID, nil
		}
	}

	// Create new category
	cat, err := h.service.CreateCategory(ctx, CreateCategoryRequest{
		TenantID: &tenantID,
		Name:     categoryName,
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("create category: %w", err)
	}

	return cat.ID, nil
}
