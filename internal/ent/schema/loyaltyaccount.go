package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// LoyaltyAccount holds the schema definition for the LoyaltyAccount entity.
type LoyaltyAccount struct {
	ent.Schema
}

// Fields of the LoyaltyAccount.
func (LoyaltyAccount) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("tenant_id", uuid.UUID{}).
			Comment("Reference to tenant"),
		field.UUID("user_id", uuid.UUID{}).
			Comment("Reference to user"),
		field.Int("balance_points").
			Default(0).
			Comment("Current available points"),
		field.Int("lifetime_points").
			Default(0).
			Comment("Total points ever earned"),
		field.Int("redeemed_points").
			Default(0).
			Comment("Total points ever redeemed"),
		field.Enum("tier").
			Values("bronze", "silver", "gold", "platinum").
			Default("bronze").
			Comment("Loyalty tier"),
		field.Int("tier_progress").
			Default(0).
			Comment("Points towards next tier"),
		field.Time("tier_expires_at").
			Optional().
			Nillable().
			Comment("When current tier expires"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the LoyaltyAccount.
func (LoyaltyAccount) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("loyalty_account").
			Field("user_id").
			Unique().
			Required(),
		edge.To("transactions", LoyaltyTransaction.Type),
	}
}

// Indexes of the LoyaltyAccount.
func (LoyaltyAccount) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "user_id").
			Unique(),
		index.Fields("tier"),
	}
}
