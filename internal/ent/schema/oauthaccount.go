package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// OAuthAccount holds the schema definition for the OAuthAccount entity.
type OAuthAccount struct {
	ent.Schema
}

// Fields of the OAuthAccount.
func (OAuthAccount) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.String("provider").
			NotEmpty(),
		field.String("provider_account_id").
			NotEmpty(),
		field.String("access_token").
			Optional(),
		field.String("refresh_token").
			Optional(),
		field.Time("expires_at").
			Optional(),
		field.Strings("scopes").
			Optional(),
		field.JSON("metadata", map[string]any{}).
			Default(map[string]any{}),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the OAuthAccount.
func (OAuthAccount) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("oauth_accounts").
			Unique(),
	}
}

// Indexes of the OAuthAccount.
func (OAuthAccount) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("provider", "provider_account_id").
			Unique(),
	}
}
