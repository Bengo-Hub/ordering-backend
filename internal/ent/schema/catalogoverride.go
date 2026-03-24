package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// CatalogOverride stores ordering-specific overrides for inventory-api items.
// Master data (name, description, image, category, type) comes from inventory-api.
// This table stores ONLY ordering-specific fields: pricing, availability, featured status, etc.
type CatalogOverride struct {
	ent.Schema
}

// Fields of the CatalogOverride.
func (CatalogOverride) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("tenant_id", uuid.UUID{}).
			Comment("Reference to tenant"),
		field.UUID("outlet_id", uuid.UUID{}).
			Comment("Reference to outlet — allows per-outlet pricing"),
		field.String("inventory_sku").
			NotEmpty().
			MaxLen(100).
			Comment("SKU from inventory-api master data"),
		field.Float("base_price").
			Default(0).
			Comment("Ordering price (may differ from cost price in inventory)"),
		field.String("currency").
			Default("KES").
			MaxLen(3),
		field.Bool("is_available").
			Default(true).
			Comment("Available for online ordering"),
		field.Bool("is_featured").
			Default(false).
			Comment("Highlighted on the online storefront"),
		field.Int("lead_time_minutes").
			Optional().
			Nillable().
			Comment("Preparation time in minutes"),
		field.Int("display_order").
			Default(0),
		field.String("display_section").
			Default("default").
			MaxLen(50).
			Comment("Display section: featured, new, top_rated, stores, default"),
		field.Float("packaging_fee").
			Default(0).
			Comment("Per-item packaging fee"),
		field.Float("service_fee_percent").
			Default(0).
			Comment("Service fee percentage for this item"),
		field.Bool("requires_age_verification").
			Default(false).
			Comment("Synced from inventory — liquor, tobacco, 18+"),
		field.String("item_type").
			Optional().
			Comment("Synced from inventory — GOODS, SERVICE, RECIPE, etc."),
		field.JSON("variant_options", map[string]string{}).
			Optional().
			Comment("Available variant attributes for customer selection"),
		field.String("image_url_override").
			Optional().
			MaxLen(500).
			Comment("Override inventory image if needed"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Indexes of the CatalogOverride.
func (CatalogOverride) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "outlet_id", "inventory_sku").Unique(),
		index.Fields("tenant_id", "outlet_id"),
		index.Fields("tenant_id", "is_available"),
		index.Fields("tenant_id", "display_section"),
		index.Fields("inventory_sku"),
	}
}
