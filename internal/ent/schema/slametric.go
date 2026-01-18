package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// SLAMetric holds the schema definition for the SLAMetric entity.
// This stores SLA metrics for orders to track compliance.
type SLAMetric struct {
	ent.Schema
}

// Fields of the SLAMetric.
func (SLAMetric) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("tenant_id", uuid.UUID{}).
			Comment("Reference to tenant"),
		field.UUID("order_id", uuid.UUID{}).
			Comment("Reference to order"),
		field.Enum("metric_type").
			Values(
				"order_to_ready",         // Time from order placed to ready for pickup
				"order_to_pickup",        // Time from order placed to rider pickup
				"order_to_delivery",      // Time from order placed to delivered
				"ready_to_pickup",        // Time from ready to rider pickup
				"pickup_to_delivery",     // Time from pickup to delivered
				"first_response_time",    // Support ticket first response
				"ticket_resolution_time", // Support ticket resolution
			).
			Comment("Type of SLA metric being tracked"),
		field.Int("target_seconds").
			Comment("Target time in seconds for this SLA"),
		field.Int("actual_seconds").
			Optional().
			Nillable().
			Comment("Actual time taken in seconds"),
		field.Enum("status").
			Values("tracking", "met", "breached", "cancelled").
			Default("tracking").
			Comment("SLA status"),
		field.Float("breach_percentage").
			Optional().
			Nillable().
			Comment("How much over SLA (e.g., 1.5 = 50% over)"),
		field.Time("started_at").
			Comment("When SLA tracking started"),
		field.Time("ended_at").
			Optional().
			Nillable().
			Comment("When SLA tracking ended"),
		field.Time("measured_at").
			Default(time.Now).
			Comment("When this metric was recorded"),
		field.JSON("metadata", map[string]any{}).
			Optional().
			Comment("Additional metric metadata"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the SLAMetric.
func (SLAMetric) Edges() []ent.Edge {
	return []ent.Edge{}
}

// Annotations of the SLAMetric.
func (SLAMetric) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table: "sla_metrics",
		},
	}
}

// Indexes of the SLAMetric.
func (SLAMetric) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "metric_type"),
		index.Fields("tenant_id", "status"),
		index.Fields("tenant_id", "measured_at"),
		index.Fields("order_id", "metric_type").Unique(),
		index.Fields("status"),
		index.Fields("measured_at"),
	}
}
