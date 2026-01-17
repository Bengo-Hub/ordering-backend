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

// TreasuryEvent holds the schema definition for the TreasuryEvent entity.
// This stores webhook events received from the treasury service for
// idempotent processing and audit trail.
type TreasuryEvent struct {
	ent.Schema
}

// Fields of the TreasuryEvent.
func (TreasuryEvent) Fields() []ent.Field {
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
			Comment("External event ID from treasury service for idempotency"),
		field.Enum("event_type").
			Values(
				"payment_initiated",
				"payment_processing",
				"payment_succeeded",
				"payment_failed",
				"payment_cancelled",
				"payment_expired",
				"refund_initiated",
				"refund_processing",
				"refund_succeeded",
				"refund_failed",
				"mpesa_stk_push_initiated",
				"mpesa_stk_push_success",
				"mpesa_stk_push_failed",
				"mpesa_stk_push_timeout",
				"mpesa_c2b_received",
				"settlement_initiated",
				"settlement_completed",
				"payout_initiated",
				"payout_completed",
				"payout_failed",
			).
			Comment("Type of treasury event"),
		field.Enum("provider").
			Values("mpesa", "stripe", "paystack", "flutterwave", "manual", "treasury").
			Optional().
			Comment("Payment provider that generated event"),
		field.UUID("order_id", uuid.UUID{}).
			Optional().
			Nillable().
			Comment("Reference to order if applicable"),
		field.UUID("payment_id", uuid.UUID{}).
			Optional().
			Nillable().
			Comment("Reference to payment if applicable"),
		field.UUID("payment_intent_id", uuid.UUID{}).
			Optional().
			Nillable().
			Comment("Reference to payment intent if applicable"),
		field.UUID("refund_id", uuid.UUID{}).
			Optional().
			Nillable().
			Comment("Reference to refund if applicable"),
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

// Edges of the TreasuryEvent.
func (TreasuryEvent) Edges() []ent.Edge {
	return []ent.Edge{}
}

// Annotations of the TreasuryEvent.
func (TreasuryEvent) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table: "treasury_events",
		},
	}
}

// Indexes of the TreasuryEvent.
func (TreasuryEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "event_type"),
		index.Fields("order_id"),
		index.Fields("payment_id"),
		index.Fields("payment_intent_id"),
		index.Fields("status"),
		index.Fields("received_at"),
		index.Fields("created_at"),
	}
}
