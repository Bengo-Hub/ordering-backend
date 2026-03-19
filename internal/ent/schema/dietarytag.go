package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

// DietaryTag holds the schema definition for the DietaryTag entity.
// Represents dietary classifications like vegetarian, vegan, gluten-free, etc.
type DietaryTag struct {
	ent.Schema
}

// Fields of the DietaryTag.
func (DietaryTag) Fields() []ent.Field {
	return []ent.Field{
		field.String("code").
			NotEmpty().
			MaxLen(50).
			Unique().
			Comment("Unique code, e.g., 'vegetarian', 'vegan', 'gluten-free'"),
		field.String("label").
			NotEmpty().
			MaxLen(100).
			Comment("Display label"),
		field.Text("description").
			Optional(),
		field.String("icon_url").
			Optional().
			MaxLen(500),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
	}
}

// Edges of the DietaryTag.
func (DietaryTag) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("catalog_items", CatalogItem.Type).
			Ref("dietary_tags"),
	}
}
