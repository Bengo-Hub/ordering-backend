package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// MenuItem holds the schema definition for the MenuItem entity.
type MenuItem struct {
	ent.Schema
}

// Fields of the MenuItem.
func (MenuItem) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("tenant_id", uuid.UUID{}).
			Comment("Reference to tenant"),
		field.UUID("cafe_id", uuid.UUID{}).
			Comment("Reference to cafe/outlet"),
		field.UUID("category_id", uuid.UUID{}).
			Comment("Reference to menu category"),
		field.String("name").
			NotEmpty().
			MaxLen(255),
		field.Text("description").
			Optional(),
		field.Float("base_price").
			Positive().
			Comment("Base price in smallest currency unit"),
		field.String("currency").
			Default("KES").
			MaxLen(3),
		field.Bool("is_available").
			Default(true),
		field.Int("lead_time_minutes").
			Optional().
			Nillable().
			Comment("Preparation time in minutes"),
		field.String("image_url").
			Optional().
			MaxLen(500),
		field.JSON("nutrition_json", map[string]any{}).
			Optional().
			Comment("Nutritional information"),
		field.String("sku").
			Optional().
			MaxLen(100).
			Comment("Stock keeping unit for inventory integration"),
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

// Edges of the MenuItem.
func (MenuItem) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("category", MenuCategory.Type).
			Ref("items").
			Field("category_id").
			Unique().
			Required(),
		edge.To("variants", MenuItemVariant.Type),
		edge.To("translations", MenuItemTranslation.Type),
		edge.To("dietary_tags", DietaryTag.Type),
		edge.To("assets", MenuItemAsset.Type),
		edge.To("schedules", MenuItemSchedule.Type),
		edge.To("cart_items", CartItem.Type),
	}
}

// Indexes of the MenuItem.
func (MenuItem) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "cafe_id"),
		index.Fields("tenant_id", "category_id"),
		index.Fields("tenant_id", "is_available"),
		index.Fields("sku"),
		index.Fields("display_order"),
	}
}
