package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// MenuItemAsset holds the schema definition for the MenuItemAsset entity.
// Stores images and other assets associated with menu items.
type MenuItemAsset struct {
	ent.Schema
}

// Fields of the MenuItemAsset.
func (MenuItemAsset) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("menu_item_id", uuid.UUID{}).
			Comment("Reference to parent menu item"),
		field.String("asset_type").
			NotEmpty().
			MaxLen(50).
			Comment("Type: 'image', 'video', 'thumbnail'"),
		field.String("url").
			NotEmpty().
			MaxLen(500).
			Comment("CDN URL of the asset"),
		field.JSON("metadata", map[string]any{}).
			Optional().
			Comment("Additional metadata like dimensions, size, etc."),
		field.Int("display_order").
			Default(0),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
	}
}

// Edges of the MenuItemAsset.
func (MenuItemAsset) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("menu_item", MenuItem.Type).
			Ref("assets").
			Field("menu_item_id").
			Unique().
			Required(),
	}
}

// Indexes of the MenuItemAsset.
func (MenuItemAsset) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("menu_item_id"),
		index.Fields("asset_type"),
	}
}
