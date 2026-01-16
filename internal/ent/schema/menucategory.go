package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// MenuCategory holds the schema definition for the MenuCategory entity.
type MenuCategory struct {
	ent.Schema
}

// Fields of the MenuCategory.
func (MenuCategory) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("tenant_id", uuid.UUID{}).
			Comment("Reference to tenant"),
		field.UUID("cafe_id", uuid.UUID{}).
			Comment("Reference to cafe/outlet"),
		field.UUID("parent_id", uuid.UUID{}).
			Optional().
			Nillable().
			Comment("Parent category for hierarchy"),
		field.String("name").
			NotEmpty().
			MaxLen(255),
		field.Text("description").
			Optional(),
		field.Int("display_order").
			Default(0).
			Comment("Sort order for display"),
		field.Bool("is_active").
			Default(true),
		field.String("image_url").
			Optional().
			MaxLen(500),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the MenuCategory.
func (MenuCategory) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("items", MenuItem.Type),
		edge.To("children", MenuCategory.Type).
			From("parent").
			Field("parent_id").
			Unique(),
	}
}

// Indexes of the MenuCategory.
func (MenuCategory) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "cafe_id"),
		index.Fields("tenant_id", "is_active"),
		index.Fields("display_order"),
	}
}
