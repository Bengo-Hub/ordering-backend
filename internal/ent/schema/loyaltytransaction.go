package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// LoyaltyTransaction holds the schema definition for the LoyaltyTransaction entity.
type LoyaltyTransaction struct {
	ent.Schema
}

// Fields of the LoyaltyTransaction.
func (LoyaltyTransaction) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("account_id", uuid.UUID{}).
			Comment("Reference to loyalty account"),
		field.UUID("order_id", uuid.UUID{}).
			Optional().
			Nillable().
			Comment("Reference to related order"),
		field.Int("points").
			Comment("Points earned (positive) or redeemed (negative)"),
		field.Int("balance_after").
			Comment("Account balance after transaction"),
		field.Enum("transaction_type").
			Values("earned", "redeemed", "expired", "adjusted", "bonus", "referral").
			Comment("Type of transaction"),
		field.Text("description").
			Optional().
			Comment("Human-readable description"),
		field.String("reference").
			Optional().
			MaxLen(255).
			Comment("External reference (e.g., order number)"),
		field.JSON("metadata", map[string]any{}).
			Optional(),
		field.Time("occurred_at").
			Default(time.Now).
			Immutable(),
	}
}

// Edges of the LoyaltyTransaction.
func (LoyaltyTransaction) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("account", LoyaltyAccount.Type).
			Ref("transactions").
			Field("account_id").
			Unique().
			Required(),
	}
}

// Indexes of the LoyaltyTransaction.
func (LoyaltyTransaction) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("account_id"),
		index.Fields("order_id"),
		index.Fields("transaction_type"),
		index.Fields("occurred_at"),
	}
}
