package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// AuditLog stores immutable audit events for compliance and security tracking.
type AuditLog struct {
	ent.Schema
}

// Fields of the AuditLog.
func (AuditLog) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.UUID("tenant_id", uuid.UUID{}).
			Optional().
			Comment("Tenant context for multi-tenancy"),
		field.UUID("user_id", uuid.UUID{}).
			Optional().
			Comment("User who performed the action"),
		field.String("action").
			NotEmpty().
			Comment("Action performed (e.g., CREATE, UPDATE, DELETE)"),
		field.String("resource_type").
			Optional().
			Comment("Type of resource affected (e.g., Order, Cart, Payment)"),
		field.String("resource_id").
			Optional().
			Comment("ID of the resource affected"),
		field.String("http_method").
			Optional().
			Comment("HTTP method used (GET, POST, PUT, PATCH, DELETE)"),
		field.String("path").
			Optional().
			Comment("Request path"),
		field.Int("status_code").
			Optional().
			Comment("HTTP response status code"),
		field.String("ip_address").
			Optional().
			MaxLen(45). // IPv6 max length
			Comment("Client IP address"),
		field.String("user_agent").
			Optional().
			MaxLen(512).
			Comment("Client User-Agent header"),
		field.JSON("request_body", map[string]any{}).
			Optional().
			Comment("Sanitized request body for mutations"),
		field.JSON("context", map[string]any{}).
			Optional().
			Comment("Additional context data"),
		field.Int64("duration_ms").
			Optional().
			Comment("Request duration in milliseconds"),
		field.Time("occurred_at").
			Default(time.Now).
			Immutable().
			Comment("When the event occurred"),
	}
}

// Indexes of the AuditLog.
func (AuditLog) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("tenant_id", "occurred_at"),
		index.Fields("user_id", "occurred_at"),
		index.Fields("resource_type", "resource_id"),
		index.Fields("action", "occurred_at"),
	}
}
