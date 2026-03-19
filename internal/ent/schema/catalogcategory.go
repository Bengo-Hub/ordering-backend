package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// CatalogCategory holds the schema definition for the CatalogCategory entity.
type CatalogCategory struct {
	ent.Schema
}

// Fields of the CatalogCategory.
func (CatalogCategory) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("tenant_id", uuid.UUID{}).
			Optional().
			Nillable().
			Comment("Reference to tenant"),
		field.UUID("outlet_id", uuid.UUID{}).
			Optional().
			Nillable().
			Comment("Reference to outlet"),
		field.UUID("parent_id", uuid.UUID{}).
			Optional().
			Nillable().
			Comment("Parent category for hierarchy"),
		field.Int("display_order").
			Default(0).
			Comment("Sort order for display"),
		field.Bool("is_active").
			Default(true),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the CatalogCategory.
func (CatalogCategory) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("outlet", Outlet.Type).
			Ref("catalog_categories").
			Field("outlet_id").
			Unique(),
		edge.To("items", CatalogItem.Type),
		edge.To("children", CatalogCategory.Type).
			From("parent").
			Field("parent_id").
			Unique(),
	}
}

// Indexes of the CatalogCategory.
func (CatalogCategory) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "outlet_id"),
		index.Fields("tenant_id", "is_active"),
		index.Fields("display_order"),
	}
}
