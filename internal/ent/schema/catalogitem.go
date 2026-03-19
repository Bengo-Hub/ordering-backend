package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// CatalogItem holds the schema definition for the CatalogItem entity.
type CatalogItem struct {
	ent.Schema
}

// Fields of the CatalogItem.
func (CatalogItem) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("tenant_id", uuid.UUID{}).
			Comment("Reference to tenant"),
		field.UUID("outlet_id", uuid.UUID{}).
			Comment("Reference to outlet"),
		field.UUID("inventory_item_id", uuid.UUID{}).
			Optional().
			Nillable().
			Comment("Reference to product master in inventory-api"),
		field.UUID("category_id", uuid.UUID{}).
			Comment("Reference to catalog category"),
		field.String("name").
			NotEmpty().
			Comment("Projected from inventory-api"),
		field.String("description").
			Optional().
			Comment("Projected from inventory-api"),
		field.Float("base_price").
			Default(0).
			Comment("Base price excluding variants"),
		field.String("currency").
			Default("KES").
			MaxLen(3),
		field.Bool("is_available").
			Default(true),
		field.Bool("is_featured").
			Default(false).
			Comment("Highlighted on the online storefront"),
		field.Int("lead_time_minutes").
			Optional().
			Nillable().
			Comment("Preparation time in minutes"),
		field.String("image_url").
			Optional().
			MaxLen(500),
		field.String("sku").
			Optional().
			MaxLen(100).
			Comment("Stock keeping unit for inventory integration"),
		field.UUID("recipe_id", uuid.UUID{}).
			Optional().
			Nillable().
			Comment("Reference to inventory-api recipe; get details via inventory client."),
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

// Edges of the CatalogItem.
func (CatalogItem) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("outlet", Outlet.Type).
			Ref("catalog_items").
			Field("outlet_id").
			Unique().
			Required(),
		edge.From("category", CatalogCategory.Type).
			Ref("items").
			Field("category_id").
			Unique().
			Required(),
		edge.To("dietary_tags", DietaryTag.Type),
		edge.To("assets", CatalogItemAsset.Type),
		edge.To("schedules", CatalogItemSchedule.Type),
		edge.To("cart_items", CartItem.Type),
		edge.From("favorited_by", User.Type).
			Ref("favorite_items"),
	}
}

// Indexes of the CatalogItem.
func (CatalogItem) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "outlet_id"),
		index.Fields("tenant_id", "category_id"),
		index.Fields("tenant_id", "is_available"),
		index.Fields("sku"),
		index.Fields("display_order"),
	}
}
