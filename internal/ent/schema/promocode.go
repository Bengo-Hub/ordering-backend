package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// PromoCode holds the schema definition for the PromoCode entity.
type PromoCode struct {
	ent.Schema
}

// Fields of the PromoCode.
func (PromoCode) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("tenant_id", uuid.UUID{}).
			Comment("Reference to tenant"),
		field.UUID("outlet_id", uuid.UUID{}).
			Optional().
			Nillable().
			Comment("Specific outlet, null for all outlets"),
		field.String("code").
			MaxLen(50).
			Comment("Promo code (case-insensitive)"),
		field.String("name").
			MaxLen(255).
			Comment("Internal name for the promo"),
		field.Text("description").
			Optional().
			Comment("Customer-facing description"),
		field.Enum("discount_type").
			Values("percentage", "fixed_amount", "free_delivery", "free_item").
			Comment("Type of discount"),
		field.Float("discount_value").
			Comment("Discount amount or percentage"),
		field.Float("max_discount_amount").
			Optional().
			Nillable().
			Comment("Maximum discount for percentage discounts"),
		field.Float("min_subtotal").
			Default(0).
			Comment("Minimum order subtotal required"),
		field.Int("max_uses").
			Optional().
			Nillable().
			Comment("Maximum total uses"),
		field.Int("max_uses_per_user").
			Optional().
			Nillable().
			Comment("Maximum uses per user"),
		field.Int("usage_count").
			Default(0).
			Comment("Current usage count"),
		field.Bool("is_active").
			Default(true),
		field.Bool("first_order_only").
			Default(false).
			Comment("Only valid for first orders"),
		field.Time("starts_at").
			Optional().
			Nillable().
			Comment("When promo becomes active"),
		field.Time("ends_at").
			Optional().
			Nillable().
			Comment("When promo expires"),
		field.JSON("eligible_categories", []uuid.UUID{}).
			Optional().
			Comment("Specific categories this applies to"),
		field.JSON("eligible_items", []uuid.UUID{}).
			Optional().
			Comment("Specific items this applies to"),
		field.JSON("metadata", map[string]any{}).
			Optional(),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the PromoCode.
func (PromoCode) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("redemptions", PromoRedemption.Type),
	}
}

// Indexes of the PromoCode.
func (PromoCode) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "code").
			Unique(),
		index.Fields("is_active"),
		index.Fields("starts_at", "ends_at"),
	}
}
