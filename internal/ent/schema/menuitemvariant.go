package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// MenuItemVariant holds the schema definition for the MenuItemVariant entity.
// Represents size, flavor, or other variations of a menu item.
type MenuItemVariant struct {
	ent.Schema
}

// Fields of the MenuItemVariant.
func (MenuItemVariant) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("menu_item_id", uuid.UUID{}).
			Comment("Reference to parent menu item"),
		field.String("name").
			NotEmpty().
			MaxLen(255).
			Comment("Variant name, e.g., 'Large', 'Extra Cheese'"),
		field.Float("price_delta").
			Default(0).
			Comment("Price difference from base price (can be negative)"),
		field.Bool("is_available").
			Default(true),
		field.String("sku").
			Optional().
			MaxLen(100).
			Comment("SKU for inventory tracking"),
		field.Int("display_order").
			Default(0),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the MenuItemVariant.
func (MenuItemVariant) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("menu_item", MenuItem.Type).
			Ref("variants").
			Field("menu_item_id").
			Unique().
			Required(),
		edge.To("cart_items", CartItem.Type),
	}
}

// Indexes of the MenuItemVariant.
func (MenuItemVariant) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("menu_item_id"),
		index.Fields("sku"),
		index.Fields("is_available"),
	}
}
