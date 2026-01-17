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

// LogisticsEvent holds the schema definition for the LogisticsEvent entity.
// This stores events received from the logistics service for idempotent
// processing and audit trail (similar to TreasuryEvent for payments).
type LogisticsEvent struct {
	ent.Schema
}

// Fields of the LogisticsEvent.
func (LogisticsEvent) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("tenant_id", uuid.UUID{}).
			Optional().
			Nillable().
			Comment("Reference to tenant (if applicable)"),
		field.String("external_id").
			MaxLen(255).
			Unique().
			Comment("External event ID from logistics service for idempotency"),
		field.Enum("event_type").
			Values(
				// Task lifecycle events
				"task_created",
				"task_assigned",
				"task_accepted",
				"task_rejected",
				"task_en_route_pickup",
				"task_arrived_pickup",
				"task_picked_up",
				"task_en_route_dropoff",
				"task_arrived_dropoff",
				"task_completed",
				"task_cancelled",
				"task_failed",
				// Route/ETA events
				"route_updated",
				"eta_updated",
				"location_updated",
				// PoD events
				"pod_submitted",
				"pod_verified",
				// Other events
				"rider_unavailable",
				"reassignment_needed",
			).
			Comment("Type of logistics event"),
		field.UUID("order_id", uuid.UUID{}).
			Optional().
			Nillable().
			Comment("Reference to order if applicable"),
		field.UUID("assignment_id", uuid.UUID{}).
			Optional().
			Nillable().
			Comment("Reference to order assignment if applicable"),
		field.String("logistics_task_id").
			MaxLen(255).
			Optional().
			Comment("Logistics task ID"),
		field.String("rider_id").
			MaxLen(255).
			Optional().
			Comment("Rider ID from logistics service"),
		field.JSON("payload", map[string]any{}).
			Comment("Raw event payload"),
		field.JSON("headers", map[string]string{}).
			Optional().
			Comment("HTTP headers from webhook request"),
		field.String("signature").
			MaxLen(500).
			Optional().
			Comment("Webhook signature for verification"),
		field.Bool("signature_valid").
			Optional().
			Nillable().
			Comment("Whether signature was verified"),
		field.Enum("status").
			Values("pending", "processing", "processed", "failed", "skipped").
			Default("pending").
			Comment("Processing status"),
		field.Int("retry_count").
			Default(0).
			Comment("Number of processing attempts"),
		field.Time("last_retry_at").
			Optional().
			Nillable().
			Comment("Last retry timestamp"),
		field.Text("error_message").
			Optional().
			Comment("Error message if processing failed"),
		field.String("error_code").
			MaxLen(100).
			Optional().
			Comment("Error code if processing failed"),
		field.String("ip_address").
			MaxLen(45).
			Optional().
			Comment("IP address of webhook sender"),
		field.Time("received_at").
			Default(time.Now).
			Comment("When event was received"),
		field.Time("processed_at").
			Optional().
			Nillable().
			Comment("When event was processed"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the LogisticsEvent.
func (LogisticsEvent) Edges() []ent.Edge {
	return []ent.Edge{}
}

// Annotations of the LogisticsEvent.
func (LogisticsEvent) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table: "logistics_events",
		},
	}
}

// Indexes of the LogisticsEvent.
func (LogisticsEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "event_type"),
		index.Fields("order_id"),
		index.Fields("assignment_id"),
		index.Fields("logistics_task_id"),
		index.Fields("status"),
		index.Fields("received_at"),
		index.Fields("created_at"),
	}
}
