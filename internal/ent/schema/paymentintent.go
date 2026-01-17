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

// PaymentIntent holds the schema definition for the PaymentIntent entity.
type PaymentIntent struct {
	ent.Schema
}

// Fields of the PaymentIntent.
func (PaymentIntent) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("tenant_id", uuid.UUID{}).
			Comment("Reference to tenant"),
		field.UUID("order_id", uuid.UUID{}).
			Comment("Reference to order"),
		field.UUID("payment_method_id", uuid.UUID{}).
			Optional().
			Nillable().
			Comment("Reference to payment method if saved"),
		field.Enum("provider").
			Values("mpesa", "stripe", "paystack", "flutterwave", "manual").
			Comment("Payment provider"),
		field.String("provider_intent_id").
			MaxLen(255).
			Optional().
			Comment("Provider's payment intent ID"),
		field.String("client_secret").
			MaxLen(500).
			Optional().
			Sensitive().
			Comment("Client secret for frontend confirmation"),
		field.Enum("status").
			Values("pending", "requires_action", "processing", "succeeded", "failed", "cancelled", "expired").
			Default("pending"),
		field.Float("amount").
			Comment("Payment amount"),
		field.String("currency").
			Default("KES").
			MaxLen(3),
		field.String("description").
			MaxLen(500).
			Optional().
			Comment("Payment description"),
		field.String("idempotency_key").
			MaxLen(255).
			Optional().
			Comment("Idempotency key for duplicate prevention"),
		field.String("mpesa_checkout_request_id").
			MaxLen(255).
			Optional().
			Comment("M-Pesa STK Push checkout request ID"),
		field.String("mpesa_phone_number").
			MaxLen(20).
			Optional().
			Comment("M-Pesa phone number for STK push"),
		field.Int("retry_count").
			Default(0).
			Comment("Number of retry attempts"),
		field.Time("last_retry_at").
			Optional().
			Nillable().
			Comment("Last retry timestamp"),
		field.Text("error_message").
			Optional().
			Comment("Last error message if failed"),
		field.String("error_code").
			MaxLen(100).
			Optional().
			Comment("Provider error code"),
		field.JSON("metadata", map[string]any{}).
			Optional().
			Comment("Additional metadata"),
		field.Time("expires_at").
			Optional().
			Nillable().
			Comment("When the intent expires"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the PaymentIntent.
func (PaymentIntent) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("order", Order.Type).
			Ref("payment_intents").
			Field("order_id").
			Unique().
			Required(),
		edge.From("payment_method", PaymentMethod.Type).
			Ref("payment_intents").
			Field("payment_method_id").
			Unique(),
		edge.To("payments", Payment.Type),
	}
}

// Annotations of the PaymentIntent.
func (PaymentIntent) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table: "payment_intents",
		},
	}
}

// Indexes of the PaymentIntent.
func (PaymentIntent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "order_id"),
		index.Fields("provider", "provider_intent_id"),
		index.Fields("status"),
		index.Fields("mpesa_checkout_request_id"),
		// Partial unique index: unique idempotency key when set
		index.Fields("idempotency_key").
			Annotations(entsql.IndexAnnotation{
				Where: "idempotency_key IS NOT NULL",
			}).
			Unique(),
		index.Fields("created_at"),
	}
}
