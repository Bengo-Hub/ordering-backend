package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// CartItem holds the schema definition for the CartItem entity.
type CartItem struct {
	ent.Schema
}

// Fields of the CartItem.
func (CartItem) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("cart_id", uuid.UUID{}).
			Comment("Reference to cart"),
		field.String("inventory_sku").
			NotEmpty().
			MaxLen(100).
			Comment("SKU from inventory-api master data"),
		field.UUID("variant_id", uuid.UUID{}).
			Optional().
			Nillable().
			Comment("Reference to catalog item variant"),
		field.String("name_snapshot").
			MaxLen(255).
			Comment("Catalog item name at time of adding to cart"),
		field.String("variant_name_snapshot").
			Optional().
			MaxLen(255).
			Comment("Variant name at time of adding to cart"),
		field.Int("quantity").
			Positive().
			Default(1),
		field.Float("unit_price").
			Positive().
			Comment("Price per unit at time of adding to cart"),
		field.Float("total_price").
			Comment("quantity * unit_price"),
		field.Text("notes").
			Optional().
			Comment("Special instructions for this item"),
		field.JSON("modifiers", []map[string]any{}).
			Optional().
			Comment("Selected modifiers/add-ons"),
		field.JSON("metadata", map[string]any{}).
			Optional().
			Comment("Additional metadata"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the CartItem.
func (CartItem) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("cart", Cart.Type).
			Ref("items").
			Field("cart_id").
			Unique().
			Required(),
	}
}

// Indexes of the CartItem.
func (CartItem) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("cart_id"),
		index.Fields("inventory_sku"),
	}
}
