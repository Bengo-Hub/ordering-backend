package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// User holds the schema definition for the User entity.
type User struct {
	ent.Schema
}

// Fields of the User.
func (User) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("tenant_id", uuid.UUID{}),
		field.String("email").
			NotEmpty(),
		field.String("password_hash").
			Optional(),
		field.String("full_name").
			NotEmpty(),
		field.String("phone").
			Optional(),
		field.String("status").
			Default("active"),
		field.String("primary_role").
			Optional(),
		field.String("locale").
			Default("en"),
		field.String("timezone").
			Default("Africa/Nairobi"),
		field.Time("last_login_at").
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

// Edges of the User.
func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("tenant", Tenant.Type).
			Ref("users").
			Field("tenant_id").
			Required().
			Unique(),
		edge.To("roles", Role.Type),
		edge.To("sessions", Session.Type),
		edge.To("devices", Device.Type),
		edge.To("oauth_accounts", OAuthAccount.Type),
		edge.From("two_factor_settings", TwoFactorSetting.Type).
			Ref("user").
			Unique(),
		edge.From("backup_codes", BackupCode.Type).
			Ref("user"),
		edge.To("preferences", UserPreference.Type).Unique(),
		edge.To("profile", UserProfile.Type).Unique(),
		edge.To("rider_profile", RiderProfile.Type).Unique(),
		edge.To("reviewed_documents", RiderDocument.Type),
	}
}

// Indexes of the User.
func (User) Indexes() []ent.Index {
	return nil
}
