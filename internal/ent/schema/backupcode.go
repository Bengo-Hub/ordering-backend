package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// BackupCode holds the schema definition for the BackupCode entity.
type BackupCode struct {
	ent.Schema
}

// Fields of the BackupCode.
func (BackupCode) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.String("code_hash").
			NotEmpty(),
		field.Time("used_at").
			Optional(),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
	}
}

// Edges of the BackupCode.
func (BackupCode) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("user", User.Type).
			Required().
			Unique(),
	}
}
