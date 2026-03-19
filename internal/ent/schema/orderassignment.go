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

// OrderAssignment holds the schema definition for the OrderAssignment entity.
// This tracks the assignment of orders to riders via the logistics service.
type OrderAssignment struct {
	ent.Schema
}

// Fields of the OrderAssignment.
func (OrderAssignment) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("tenant_id", uuid.UUID{}).
			Comment("Reference to tenant"),
		field.UUID("order_id", uuid.UUID{}).
			Comment("Reference to order"),
		field.String("logistics_task_id").
			MaxLen(255).
			Comment("Task ID from logistics service"),
		field.String("rider_id").
			MaxLen(255).
			Optional().
			Comment("Rider ID from logistics service (NOT stored locally, just reference)"),
		field.Enum("status").
			Values(
				"pending",       // Task created, awaiting assignment
				"assigned",      // Rider assigned
				"accepted",      // Rider accepted
				"en_route_pickup", // Rider heading to pickup
				"arrived_pickup",  // Rider arrived at pickup
				"picked_up",     // Order picked up
				"en_route_dropoff", // Rider heading to delivery
				"arrived_dropoff",  // Rider arrived at delivery
				"completed",     // Delivery completed
				"cancelled",     // Task cancelled
				"failed",        // Delivery failed
			).
			Default("pending"),
		field.Enum("priority").
			Values("low", "normal", "high", "urgent").
			Default("normal").
			Comment("Delivery priority"),
		field.Text("special_instructions").
			Optional().
			Comment("Special delivery instructions"),
		field.String("rejection_reason").
			MaxLen(500).
			Optional().
			Comment("Reason if rider rejected"),
		field.String("cancellation_reason").
			MaxLen(500).
			Optional().
			Comment("Reason if task cancelled"),
		field.String("failure_reason").
			MaxLen(500).
			Optional().
			Comment("Reason if delivery failed"),
		field.Int("attempt_count").
			Default(1).
			Comment("Delivery attempt number"),
		field.JSON("metadata", map[string]any{}).
			Optional().
			Comment("Additional metadata from logistics"),
		field.Time("assigned_at").
			Optional().
			Nillable().
			Comment("When rider was assigned"),
		field.Time("accepted_at").
			Optional().
			Nillable().
			Comment("When rider accepted"),
		field.Time("picked_up_at").
			Optional().
			Nillable().
			Comment("When order was picked up"),
		field.Time("completed_at").
			Optional().
			Nillable().
			Comment("When delivery was completed"),
		field.Time("cancelled_at").
			Optional().
			Nillable().
			Comment("When task was cancelled"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the OrderAssignment.
func (OrderAssignment) Edges() []ent.Edge {
		return []ent.Edge{
		edge.From("order", Order.Type).
			Ref("assignments").
			Field("order_id").
			Unique().
			Required(),
		edge.To("delivery_windows", DeliveryWindow.Type),
	}
}

// Annotations of the OrderAssignment.
func (OrderAssignment) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table: "order_assignments",
		},
	}
}

// Indexes of the OrderAssignment.
func (OrderAssignment) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "order_id"),
		index.Fields("logistics_task_id").Unique(),
		index.Fields("rider_id"),
		index.Fields("status"),
		index.Fields("created_at"),
	}
}
