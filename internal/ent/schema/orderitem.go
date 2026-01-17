package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// OrderItem holds the schema definition for the OrderItem entity.
type OrderItem struct {
	ent.Schema
}

// Fields of the OrderItem.
func (OrderItem) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("order_id", uuid.UUID{}).
			Comment("Reference to order"),
		field.UUID("menu_item_id", uuid.UUID{}).
			Comment("Reference to menu item"),
		field.UUID("variant_id", uuid.UUID{}).
			Optional().
			Nillable().
			Comment("Reference to menu item variant"),
		field.String("name_snapshot").
			MaxLen(255).
			Comment("Menu item name at time of order"),
		field.String("variant_name_snapshot").
			Optional().
			MaxLen(255).
			Comment("Variant name at time of order"),
		field.Int("quantity").
			Positive(),
		field.Float("unit_price").
			Positive().
			Comment("Price per unit at time of order"),
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
	}
}

// Edges of the OrderItem.
func (OrderItem) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("order", Order.Type).
			Ref("items").
			Field("order_id").
			Unique().
			Required(),
	}
}

// Indexes of the OrderItem.
func (OrderItem) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("order_id"),
		index.Fields("menu_item_id"),
	}
}
