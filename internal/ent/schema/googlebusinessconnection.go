package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// GoogleBusinessConnection holds a tenant's (optionally outlet-scoped) Google Business
// Profile OAuth connection. The OAuth token JSON is stored AES-256-GCM encrypted in
// encrypted_tokens (see internal/modules/googlebusiness/crypto.go). One connection per
// (tenant_id, outlet_id) pair — a NULL outlet_id is the tenant-wide default connection.
type GoogleBusinessConnection struct {
	ent.Schema
}

// Fields of the GoogleBusinessConnection.
func (GoogleBusinessConnection) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("tenant_id", uuid.UUID{}).
			Comment("Owning tenant"),
		field.UUID("outlet_id", uuid.UUID{}).
			Optional().
			Nillable().
			Comment("Nil = tenant-wide connection; set = outlet-scoped connection"),
		field.String("account_id").
			Optional().
			Comment("Google Business Profile account resource id, e.g. accounts/123"),
		field.String("location_id").
			Optional().
			Comment("Google Business Profile location resource name, e.g. accounts/123/locations/456"),
		field.String("place_id").
			Optional().
			Comment("Google Maps Place ID used to build the public writereview deep link"),
		field.String("location_name").
			Optional().
			Comment("Human-readable location/business name for display"),
		field.Text("encrypted_tokens").
			Optional().
			Sensitive().
			Comment("AES-256-GCM encrypted OAuth token JSON {access_token,refresh_token,expiry,scope}"),
		field.Time("token_expiry").
			Optional().
			Nillable().
			Comment("Access token expiry; used to decide when to refresh"),
		field.String("status").
			Default("disconnected").
			Comment("Connection status: disconnected, connected, needs_location"),
		field.Time("connected_at").
			Optional().
			Nillable().
			Comment("When the OAuth connection was established"),
		field.String("connected_by").
			Optional().
			Comment("User id/email that performed the connection"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Indexes of the GoogleBusinessConnection.
func (GoogleBusinessConnection) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id"),
		// One connection per tenant/outlet pair. A NULL outlet_id row is the tenant-wide
		// connection; Postgres treats NULLs as distinct so we additionally guard the
		// tenant-wide row in application logic.
		index.Fields("tenant_id", "outlet_id").Unique(),
	}
}
