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

// ProofOfDelivery holds the schema definition for the ProofOfDelivery entity.
// This stores proof of delivery artifacts received from the logistics service.
type ProofOfDelivery struct {
	ent.Schema
}

// Fields of the ProofOfDelivery.
func (ProofOfDelivery) Fields() []ent.Field {
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
		field.String("logistics_task_id").
			MaxLen(255).
			Comment("Task ID from logistics service"),
		field.Enum("type").
			Values("signature", "photo", "otp", "pin", "contactless", "recipient_name").
			Comment("Type of proof"),
		field.String("signature_url").
			MaxLen(500).
			Optional().
			Comment("URL to signature image"),
		field.JSON("photo_urls", []string{}).
			Optional().
			Comment("URLs to delivery photos"),
		field.Bool("otp_verified").
			Default(false).
			Comment("Whether OTP was verified"),
		field.String("otp_code").
			MaxLen(10).
			Optional().
			Sensitive().
			Comment("OTP code used (for audit)"),
		field.String("recipient_name").
			MaxLen(255).
			Optional().
			Comment("Name of person who received"),
		field.String("recipient_relation").
			MaxLen(100).
			Optional().
			Comment("Relation of recipient (self, family, etc.)"),
		field.Float("delivery_latitude").
			Optional().
			Nillable().
			Comment("Latitude where delivery was made"),
		field.Float("delivery_longitude").
			Optional().
			Nillable().
			Comment("Longitude where delivery was made"),
		field.String("rider_notes").
			MaxLen(1000).
			Optional().
			Comment("Notes from rider"),
		field.String("customer_rating").
			MaxLen(10).
			Optional().
			Comment("Customer rating (1-5)"),
		field.String("customer_feedback").
			MaxLen(1000).
			Optional().
			Comment("Customer feedback"),
		field.Bool("is_verified").
			Default(false).
			Comment("Whether PoD was verified"),
		field.String("verified_by").
			MaxLen(255).
			Optional().
			Comment("Who verified the PoD"),
		field.JSON("metadata", map[string]any{}).
			Optional().
			Comment("Additional metadata from logistics"),
		field.Time("delivered_at").
			Comment("When delivery was completed"),
		field.Time("verified_at").
			Optional().
			Nillable().
			Comment("When PoD was verified"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the ProofOfDelivery.
func (ProofOfDelivery) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("assignment", OrderAssignment.Type).
			Ref("proof_of_delivery").
			Field("assignment_id").
			Unique().
			Required(),
	}
}

// Annotations of the ProofOfDelivery.
func (ProofOfDelivery) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table: "proof_of_delivery",
		},
	}
}

// Indexes of the ProofOfDelivery.
func (ProofOfDelivery) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "order_id"),
		index.Fields("assignment_id").Unique(),
		index.Fields("logistics_task_id"),
		index.Fields("delivered_at"),
		index.Fields("is_verified"),
	}
}
