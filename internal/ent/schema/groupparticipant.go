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

// GroupParticipant holds the schema definition for the GroupParticipant entity.
type GroupParticipant struct {
	ent.Schema
}

// Fields of the GroupParticipant.
func (GroupParticipant) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("group_order_id", uuid.UUID{}).
			Comment("Reference to group order"),
		field.UUID("user_id", uuid.UUID{}).
			Comment("Reference to the participant user"),
		field.String("user_name").
			MaxLen(255).
			Comment("Display name of the participant"),
		field.Time("joined_at").
			Default(time.Now).
			Immutable(),
	}
}

// Edges of the GroupParticipant.
func (GroupParticipant) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("group_order", GroupOrder.Type).
			Ref("participants").
			Field("group_order_id").
			Unique().
			Required(),
	}
}

// Annotations of the GroupParticipant.
func (GroupParticipant) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table: "group_participants",
		},
	}
}

// Indexes of the GroupParticipant.
func (GroupParticipant) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("group_order_id", "user_id").
			Unique(),
	}
}
