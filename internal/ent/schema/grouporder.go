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

// GroupOrder holds the schema definition for the GroupOrder entity.
type GroupOrder struct {
	ent.Schema
}

// Fields of the GroupOrder.
func (GroupOrder) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("tenant_id", uuid.UUID{}).
			Comment("Reference to tenant"),
		field.UUID("host_user_id", uuid.UUID{}).
			Comment("The user who created the group order"),
		field.UUID("cart_id", uuid.UUID{}).
			Comment("Reference to the shared cart"),
		field.String("invite_code").
			MaxLen(6).
			Comment("6-character invite code for joining"),
		field.Enum("status").
			Values("open", "locked", "checked_out", "expired").
			Default("open"),
		field.Int("max_participants").
			Default(10).
			Comment("Maximum number of participants allowed"),
		field.Time("expires_at").
			Comment("When the group order session expires"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the GroupOrder.
func (GroupOrder) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("participants", GroupParticipant.Type),
	}
}

// Annotations of the GroupOrder.
func (GroupOrder) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table: "group_orders",
		},
	}
}

// Indexes of the GroupOrder.
func (GroupOrder) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("invite_code").
			Unique(),
		index.Fields("tenant_id", "host_user_id"),
		index.Fields("status"),
		index.Fields("expires_at"),
	}
}
