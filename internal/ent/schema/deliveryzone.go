package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// DeliveryZone holds the schema definition for geographic delivery service areas.
type DeliveryZone struct {
	ent.Schema
}

// Fields of the DeliveryZone.
func (DeliveryZone) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("tenant_id", uuid.UUID{}).
			Comment("Reference to tenant"),
		field.UUID("outlet_id", uuid.UUID{}).
			Optional().
			Nillable().
			Comment("Reference to outlet; null = applies to all outlets"),
		field.String("name").
			NotEmpty().
			Comment("Zone display name (e.g., Downtown, Suburbs, Extended)"),
		field.String("slug").
			Optional().
			Comment("URL-safe identifier"),
		field.JSON("zone_polygon", map[string]any{}).
			Optional().
			Comment("GeoJSON polygon defining the delivery area boundary"),
		field.Float("delivery_fee").
			Default(0).
			Comment("Delivery fee for this zone in tenant currency"),
		field.Float("minimum_order").
			Default(0).
			Comment("Minimum order amount for delivery to this zone"),
		field.Int("estimated_time_minutes").
			Default(30).
			Comment("Estimated delivery time in minutes"),
		field.Bool("is_active").
			Default(true),
		field.Int("sort_order").
			Default(0),
		field.JSON("metadata", map[string]any{}).
			Default(map[string]any{}),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Indexes of the DeliveryZone.
func (DeliveryZone) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id"),
		index.Fields("tenant_id", "outlet_id"),
		index.Fields("tenant_id", "is_active"),
		index.Fields("tenant_id", "slug").Unique(),
	}
}
