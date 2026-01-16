package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// MenuItemTranslation holds the schema definition for the MenuItemTranslation entity.
// Stores localized content for menu items (e.g., English, Swahili).
type MenuItemTranslation struct {
	ent.Schema
}

// Fields of the MenuItemTranslation.
func (MenuItemTranslation) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("menu_item_id", uuid.UUID{}).
			Comment("Reference to parent menu item"),
		field.String("locale").
			NotEmpty().
			MaxLen(5).
			Comment("Locale code, e.g., 'en', 'sw'"),
		field.String("name").
			NotEmpty().
			MaxLen(255).
			Comment("Translated name"),
		field.Text("description").
			Optional().
			Comment("Translated description"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the MenuItemTranslation.
func (MenuItemTranslation) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("menu_item", MenuItem.Type).
			Ref("translations").
			Field("menu_item_id").
			Unique().
			Required(),
	}
}

// Indexes of the MenuItemTranslation.
func (MenuItemTranslation) Indexes() []ent.Index {
	return []ent.Index{
		// Composite unique index on (menu_item_id, locale)
		index.Fields("menu_item_id", "locale").Unique(),
	}
}
