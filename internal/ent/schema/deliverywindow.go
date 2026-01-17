package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// DeliveryWindow holds the schema definition for the DeliveryWindow entity.
// This tracks ETAs and actual delivery times.
type DeliveryWindow struct {
	ent.Schema
}

// Fields of the DeliveryWindow.
func (DeliveryWindow) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("tenant_id", uuid.UUID{}).
			Comment("Reference to tenant"),
		field.UUID("order_id", uuid.UUID{}).
			Comment("Reference to order"),
		field.UUID("assignment_id", uuid.UUID{}).
			Comment("Reference to order assignment"),
		field.Time("eta_start").
			Comment("Estimated arrival window start"),
		field.Time("eta_end").
			Comment("Estimated arrival window end"),
		field.Int("eta_minutes").
			Optional().
			Nillable().
			Comment("Estimated minutes until arrival"),
		field.Float("distance_km").
			Optional().
			Nillable().
			Comment("Distance in kilometers"),
		field.Time("actual_arrival").
			Optional().
			Nillable().
			Comment("Actual arrival at delivery address"),
		field.Time("actual_dropoff").
			Optional().
			Nillable().
			Comment("Actual dropoff time"),
		field.String("source").
			MaxLen(50).
			Default("logistics").
			Comment("Source of ETA (logistics, manual, system)"),
		field.Bool("is_current").
			Default(true).
			Comment("Whether this is the current active ETA"),
		field.JSON("route_info", map[string]any{}).
			Optional().
			Comment("Route information from logistics"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the DeliveryWindow.
func (DeliveryWindow) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("assignment", OrderAssignment.Type).
			Ref("delivery_windows").
			Field("assignment_id").
			Unique().
			Required(),
	}
}

// Annotations of the DeliveryWindow.
func (DeliveryWindow) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table: "delivery_windows",
		},
	}
}

// Indexes of the DeliveryWindow.
func (DeliveryWindow) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "order_id"),
		index.Fields("assignment_id"),
		index.Fields("is_current"),
		index.Fields("created_at"),
	}
}
