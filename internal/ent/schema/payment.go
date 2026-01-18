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

// Payment holds the schema definition for the Payment entity.
type Payment struct {
	ent.Schema
}

// Fields of the Payment.
func (Payment) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("tenant_id", uuid.UUID{}).
			Comment("Reference to tenant"),
		field.UUID("payment_intent_id", uuid.UUID{}).
			Comment("Reference to payment intent"),
		field.UUID("order_id", uuid.UUID{}).
			Comment("Reference to order"),
		field.Float("amount").
			Comment("Payment amount"),
		field.String("currency").
			Default("KES").
			MaxLen(3),
		field.Enum("status").
			Values("pending", "processing", "succeeded", "failed", "refunded", "partially_refunded").
			Default("pending"),
		field.Enum("provider").
			Values("mpesa", "stripe", "paystack", "flutterwave", "manual").
			Comment("Payment provider"),
		field.String("provider_reference").
			MaxLen(255).
			Optional().
			Comment("Provider's transaction reference"),
		field.String("provider_receipt").
			MaxLen(255).
			Optional().
			Comment("Provider's receipt number (e.g., M-Pesa receipt)"),
		field.String("mpesa_transaction_id").
			MaxLen(50).
			Optional().
			Comment("M-Pesa transaction ID"),
		field.String("mpesa_phone_number").
			MaxLen(20).
			Optional().
			Comment("M-Pesa phone number used"),
		field.Float("refunded_amount").
			Default(0).
			Comment("Total amount refunded"),
		field.JSON("provider_response", map[string]any{}).
			Optional().
			Comment("Raw provider response"),
		field.JSON("metadata", map[string]any{}).
			Optional().
			Comment("Additional metadata"),
		field.Time("processed_at").
			Optional().
			Nillable().
			Comment("When payment was processed"),
		field.Time("captured_at").
			Optional().
			Nillable().
			Comment("When payment was captured"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the Payment.
func (Payment) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("payment_intent", PaymentIntent.Type).
			Ref("payments").
			Field("payment_intent_id").
			Unique().
			Required(),
		edge.From("order", Order.Type).
			Ref("payments").
			Field("order_id").
			Unique().
			Required(),
		edge.To("refunds", Refund.Type),
	}
}

// Annotations of the Payment.
func (Payment) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table: "payments",
		},
	}
}

// Indexes of the Payment.
func (Payment) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "order_id"),
		index.Fields("tenant_id", "status"),
		index.Fields("tenant_id", "provider"),
		// Composite indexes for common filtering patterns
		index.Fields("tenant_id", "status", "created_at"),
		index.Fields("provider", "provider_reference"),
		index.Fields("mpesa_transaction_id"),
		index.Fields("mpesa_phone_number"),
		index.Fields("status"),
		index.Fields("payment_intent_id"),
		// Time-based indexes for reporting
		index.Fields("created_at"),
		index.Fields("processed_at"),
		index.Fields("captured_at"),
	}
}
