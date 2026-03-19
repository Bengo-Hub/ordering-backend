package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// Tenant holds the schema definition for the Tenant entity in ordering-backend.
//
// This schema is IDENTICAL to auth-api internal/ent/schema/tenant.go by design.
// All services that integrate with SSO must maintain a local tenant copy synced
// from auth-api using the same UUID. This ensures cross-service tenant consistency.
//
// Sync pattern: GET /api/v1/tenants/by-slug/{slug} on auth-api → upsert locally with SetID(authAPIUUID).
type Tenant struct {
	ent.Schema
}

// Fields of the Tenant.
// IMPORTANT: Keep in sync with auth-api internal/ent/schema/tenant.go.
func (Tenant) Fields() []ent.Field {
	return []ent.Field{
		// ── Identity ─────────────────────────────────────────────────────────
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable(),
		field.String("name").
			NotEmpty().
			Comment("Display name of the organisation"),
		field.String("slug").
			NotEmpty().
			Unique().
			Comment("URL-safe identifier; used by all services to key tenant rows"),
		field.String("status").
			Default("active").
			Comment("active | inactive | suspended"),

		// ── Contact & Branding ────────────────────────────────────────────────
		field.String("contact_email").
			Optional().
			Comment("Primary billing and alerts email for this organisation"),
		field.String("contact_phone").
			Optional().
			Comment("Primary contact phone (E.164 format)"),
		field.String("logo_url").
			Optional().
			Comment("URL to the organisation's logo"),
		field.String("website").
			Optional().
			Comment("Organisation's public website"),
		field.String("country").
			Optional().
			Default("KE").
			Comment("ISO 3166-1 alpha-2 country code"),
		field.String("timezone").
			Optional().
			Default("Africa/Nairobi").
			Comment("IANA timezone for this tenant"),
		field.JSON("brand_colors", map[string]any{}).
			Optional().
			Comment("Brand palette: { primary, secondary, accent } — used by notification templates and UI theming"),

		// ── Organisation Profile ──────────────────────────────────────────────
		// Collected during multi-step registration to drive subscription recommendation.
		field.String("org_size").
			Optional().
			Comment("Staff count band: 1-5 | 6-20 | 21-100 | 100+"),
		field.String("use_case").
			Optional().
			Nillable().
			Comment("Primary business use case: hospitality | retail | quick_service | manufacturing | warehousing | services | e_commerce | other"),

		// ── Subscription Tier Cache ────────────────────────────────────────────
		// Denormalized from subscription-api for fast JWT enrichment and auth checks.
		// Updated via background sync whenever the subscription changes.
		field.String("subscription_plan").
			Optional().
			Comment("Active plan code: STARTER | GROWTH | PROFESSIONAL"),
		field.String("subscription_status").
			Optional().
			Comment("ACTIVE | TRIAL | EXPIRED | CANCELLED"),
		field.Time("subscription_expires_at").
			Optional().
			Nillable().
			Comment("UTC expiry of the current subscription period"),
		field.String("subscription_id").
			Optional().
			Comment("UUID of TenantSubscription record in subscription-api"),

		// ── Tier Limit Cache ───────────────────────────────────────────────────
		// JSON blob mirroring subscription-api tierLimitsJSON so this service can
		// enforce limits (max_orders_per_day, max_outlets, max_riders) without a round-trip.
		field.JSON("tier_limits", map[string]any{}).
			Optional().
			Comment("Denormalized tier limits: max_members, max_admins, max_outlets, max_riders, max_orders_per_day, etc."),

		// ── Arbitrary Metadata ────────────────────────────────────────────────
		field.JSON("metadata", map[string]any{}).
			Optional().
			Default(map[string]any{}).
			Comment("Free-form key-value store for org-specific configuration"),

		// ── Timestamps ───────────────────────────────────────────────────────
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the Tenant.
func (Tenant) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("settings", TenantSetting.Type).
			Unique(),
		edge.To("users", User.Type),
		edge.To("outlets", Outlet.Type),
		edge.To("sync_events", TenantSyncEvent.Type),
	}
}

// Indexes of the Tenant.
func (Tenant) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("slug").Unique(),
		index.Fields("status"),
		index.Fields("subscription_plan"),
	}
}
