# Ordering-Backend: Schema vs ERD/Architecture Alignment

**Last updated:** March 2026

This document compares `internal/ent/schema/*.go` with `docs/erd.md` and `shared-docs/CROSS-SERVICE-DATA-OWNERSHIP.md` and records gaps and fixes.

---

## 1. Removed Entities (Already Done)

Per data ownership, the following **do not exist** in the schema folder (correctly removed):

| Entity | Owner | Status |
|--------|-------|--------|
| payment_intents, payments, payment_methods, refunds, treasury_events | treasury-api | **Removed** — Order has `payment_intent_id` only |
| notification_templates, notification_events, notification_subscriptions | notifications-api | **Removed** — use StubRepository / notifications-api |
| proof_of_delivery, logistics_events | logistics-api | **Removed** — OrderAssignment has `logistics_task_id`, `rider_id` refs only |

---

## 2. Missing Schema vs ERD

| ERD Table | In Schema? | Action |
|-----------|------------|--------|
| **cafes** | **No** | **Add** — erd and data ownership say ordering owns "Cafe/outlet context". Currently `cafe_id` is a raw UUID on Order, CatalogItem, MenuCategory, Cart, PromoCode with no Cafe entity. |
| **menu_items.recipe_id** | **No** | **Add** — Plan and CROSS-SERVICE-DATA-OWNERSHIP: ordering stores `recipe_id` (reference to inventory-api recipe). |
| group_orders, group_participants, group_contributions | No | Aspirational in erd; not implemented. |
| delivery_zones, zone_schedules, availability_checks, zone_waitlists | No | Aspirational; not implemented. |
| report_jobs, support_tickets, support_ticket_events | No | Aspirational; not implemented. |
| backup_jobs, backup_restores, security_policies | No | Aspirational; not implemented. |
| integration_settings, integration_webhooks, api_clients, api_tokens | No | Aspirational; not implemented. |
| media_assets | No | Aspirational; not implemented. |

---

## 3. Schemas Present but Marked Deprecated / Legacy in ERD

| Schema | ERD / Notes |
|--------|-------------|
| **Session** | erd: "DEPRECATED: Session and MFA artefacts live in auth-service." Kept for migration compatibility. |
| **Device** | Auth/device binding; typically owned by auth-service. |
| **OAuthAccount** | Auth; owned by auth-service. |
| **BackupCode** | MFA; owned by auth-service. |
| **TwoFactorSetting** | MFA; owned by auth-service. |

These remain in schema for migration compatibility; identity/session operations are delegated to auth-service. No removal without explicit migration plan.

---

## 4. Implemented Alignments

- **Order**: `payment_intent_id` (optional UUID), `payment_status` — matches erd and data ownership.
- **OrderAssignment**: `logistics_task_id`, `rider_id` (refs only); no ProofOfDelivery edge — matches erd.
- **User**: no PaymentMethod edge — matches data ownership (treasury owns payment methods).
- **Tenant, TenantSetting, TenantSyncEvent, User, UserProfile, UserPreference, Role, Permission**: present and used for ordering-scoped identity/settings.

---

## 5. Summary of Schema Fixes to Apply

1. **Add Cafe schema** — table `cafes` with fields per erd (id, tenant_id, tenant_slug, name, slug, description, contact_email, phone, location, latitude, longitude, opening_hours, status, created_at, updated_at). Add edges from Order, CatalogItem, MenuCategory, Cart, PromoCode to Cafe (or keep cafe_id as reference only; if Cafe is synced from elsewhere, document that).
2. **Add CatalogItem.recipe_id** — optional UUID, comment "Reference to inventory-api recipe; get details via inventory client."
3. **Update erd.md** — clarify which tables are "implemented" vs "planned"; add note that Session/Device/OAuth/BackupCode/TwoFactor are legacy for compatibility.
