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

// Refund holds the schema definition for the Refund entity.
type Refund struct {
	ent.Schema
}

// Fields of the Refund.
func (Refund) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("tenant_id", uuid.UUID{}).
			Comment("Reference to tenant"),
		field.UUID("payment_id", uuid.UUID{}).
			Comment("Reference to payment"),
		field.UUID("order_id", uuid.UUID{}).
			Comment("Reference to order"),
		field.Float("amount").
			Comment("Refund amount"),
		field.String("currency").
			Default("KES").
			MaxLen(3),
		field.Enum("status").
			Values("pending", "processing", "succeeded", "failed", "cancelled").
			Default("pending"),
		field.Enum("reason").
			Values("customer_request", "order_cancelled", "duplicate", "fraudulent", "product_unavailable", "other").
			Comment("Refund reason"),
		field.Text("reason_notes").
			Optional().
			Comment("Additional notes about refund reason"),
		field.Enum("provider").
			Values("mpesa", "stripe", "paystack", "flutterwave", "manual").
			Comment("Payment provider"),
		field.String("provider_refund_id").
			MaxLen(255).
			Optional().
			Comment("Provider's refund ID"),
		field.String("provider_reference").
			MaxLen(255).
			Optional().
			Comment("Provider's refund reference"),
		field.UUID("requested_by", uuid.UUID{}).
			Optional().
			Nillable().
			Comment("User who requested the refund"),
		field.UUID("approved_by", uuid.UUID{}).
			Optional().
			Nillable().
			Comment("User who approved the refund"),
		field.Text("error_message").
			Optional().
			Comment("Error message if failed"),
		field.String("error_code").
			MaxLen(100).
			Optional().
			Comment("Provider error code"),
		field.JSON("provider_response", map[string]any{}).
			Optional().
			Comment("Raw provider response"),
		field.JSON("metadata", map[string]any{}).
			Optional().
			Comment("Additional metadata"),
		field.Time("requested_at").
			Default(time.Now).
			Comment("When refund was requested"),
		field.Time("approved_at").
			Optional().
			Nillable().
			Comment("When refund was approved"),
		field.Time("processed_at").
			Optional().
			Nillable().
			Comment("When refund was processed"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the Refund.
func (Refund) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("payment", Payment.Type).
			Ref("refunds").
			Field("payment_id").
			Unique().
			Required(),
	}
}

// Annotations of the Refund.
func (Refund) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table: "refunds",
		},
	}
}

// Indexes of the Refund.
func (Refund) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "order_id"),
		index.Fields("tenant_id", "payment_id"),
		index.Fields("provider", "provider_refund_id"),
		index.Fields("status"),
		index.Fields("created_at"),
	}
}
