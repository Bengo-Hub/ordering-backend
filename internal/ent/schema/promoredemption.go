package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// PromoRedemption holds the schema definition for the PromoRedemption entity.
type PromoRedemption struct {
	ent.Schema
}

// Fields of the PromoRedemption.
func (PromoRedemption) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("promo_code_id", uuid.UUID{}).
			Comment("Reference to promo code"),
		field.UUID("order_id", uuid.UUID{}).
			Comment("Reference to order"),
		field.UUID("user_id", uuid.UUID{}).
			Comment("Reference to user who redeemed"),
		field.Float("discount_amount").
			Comment("Actual discount amount applied"),
		field.Time("redeemed_at").
			Default(time.Now).
			Immutable(),
	}
}

// Edges of the PromoRedemption.
func (PromoRedemption) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("promo_code", PromoCode.Type).
			Ref("redemptions").
			Field("promo_code_id").
			Unique().
			Required(),
	}
}

// Indexes of the PromoRedemption.
func (PromoRedemption) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("promo_code_id"),
		index.Fields("order_id").
			Unique(),
		index.Fields("user_id"),
		index.Fields("promo_code_id", "user_id"),
	}
}
