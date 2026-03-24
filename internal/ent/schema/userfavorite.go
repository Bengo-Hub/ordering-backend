package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// UserFavorite stores user-favorited inventory items (by SKU).
// Replaces the old M2M edge User→CatalogItem.
type UserFavorite struct {
	ent.Schema
}

// Fields of the UserFavorite.
func (UserFavorite) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("tenant_id", uuid.UUID{}).
			Comment("Reference to tenant"),
		field.UUID("user_id", uuid.UUID{}).
			Comment("Reference to auth-service user"),
		field.String("inventory_sku").
			NotEmpty().
			MaxLen(100).
			Comment("SKU from inventory-api"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
	}
}

// Indexes of the UserFavorite.
func (UserFavorite) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "user_id", "inventory_sku").Unique(),
		index.Fields("tenant_id", "user_id"),
		index.Fields("inventory_sku"),
	}
}
