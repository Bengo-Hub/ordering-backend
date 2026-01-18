# Ordering Service – Entity Relationship Overview

This document provides a textual ERD for the BengoBox Ordering Service (online delivery orders only).  
The schema supports multiple business types (food, retail, grocery, pharmacy, etc.) with flexible catalog, multi-delivery options, and group ordering.  
The database structure is defined via Ent schemas—the Go source of truth that powers code generation and migrations.

> **Conventions**
>
> - All primary keys are UUIDs unless noted.
> - `tenant_id` enforces multi-tenant isolation.
> - Timestamps use `TIMESTAMPTZ`.
> - JSON metadata columns allow forward-compatible extension.

---

## Identity & Access Management

| Table | Key Columns | Purpose / Relationships |
|-------|-------------|-------------------------|
| `tenants` | `id`, `slug`, `name`, `status`, `created_at`, `updated_at` | Master record for each organisation (e.g. Urban Café HQ); `slug` shared across all Go microservices for multi-tenant routing. Default tenant slug is `urban-cafe`. |
| `tenant_settings` | `tenant_id (PK)`, `brand_palette`, `locales`, `features`, `updated_at` | JSON configuration (colors, localisation, feature toggles, default integration settings). |
| `users` | `id`, `tenant_id`, `auth_service_user_id` (UUID, UNIQUE, FK reference to auth-service), `email`, `password_hash` (deprecated - auth handled by auth-service), `full_name`, `phone`, `status`, `primary_role`, `sync_status`, `sync_at`, `last_login_at`, `created_at`, `updated_at` | Core user profile. `auth_service_user_id` references auth-service user. Identity data (email, phone, status) synced from auth-service. Unique `(tenant_id, email)` and `auth_service_user_id`. |
| `user_profiles` | `user_id (PK)`, `avatar_url`, `bio`, `preferences_json` | Extended profile metadata. |
| `roles` | `code (PK)`, `name`, `description`, `scope`, `system_role` | Canonical role catalogue (`customer`, `rider`, `staff`, `admin`, `superuser`). |
| `permissions` | `code (PK)`, `name`, `module`, `description` | Fine-grained permission catalogue. |
| `role_permissions` | `(role_code, permission_code) PK` | Many-to-many link for capability matrix. |
| `user_roles` | `(user_id, role_code) PK`, `assigned_by`, `assigned_at` | User role assignments. |
| `sessions` | — | **DEPRECATED**: Session and MFA artefacts live in `auth-service`. Cafe backend validates tokens via JWKS from auth-service (`https://sso.codevertexitsolutions.com/api/v1/.well-known/jwks.json`). Local session table exists for migration compatibility but is not used in production. |
| `user_preferences` | `user_id (PK)`, `theme`, `language`, `notify_email`, `notify_sms`, `notify_push`, `timezone`, `created_at`, `updated_at` | Personalisation settings retained for local UX. Identity data (email, phone, status) synced from auth-service via `auth.user.*` events. |
| _Logistics Integration_ | — | **IMPORTANT**: Rider profiles, documents, availability, fleet management, and delivery task logic are centralized in `logistics-service`. Cafe backend stores only `rider_id` references in `order_assignments` table. All rider/fleet/driver data queries go directly to logistics-service APIs.  
**Rider Creation**: Before creating a rider, check tenant has logistics service enabled. Options: (1) Push to logistics-service API `POST /v1/{tenant}/fleet-members`, or (2) Redirect to logistics-service UI for self-onboarding. Riders authenticate via auth-service (SSO), logistics-service stores all rider data.  
**Tenant Service Availability**: Check tenant subscription plan features before creating/referencing data in any service (logistics, inventory, POS, treasury, notifications). |
| _SSO Integration_ | — | **Production Auth-Service**: `https://sso.codevertexitsolutions.com/`  
Authentication and token issuance delegated to auth-service (OIDC authority). All login/registration requests proxy to auth-service. Local tables store domain-specific extensions (profiles, preferences, cafe roles, loyalty points) while trust is established via JWT claims from auth-service.  
**User Sync**: `auth_service_user_id` field references auth-service user. Sync via events: `auth.user.created`, `auth.user.updated`, `auth.user.deactivated`.  
**Superuser Handling**: Superusers from auth-service bypass all RBAC/permission checks.  
**Service-to-Service**: API key authentication for inter-service communication. |
| _Note on Rider Entities_ | — | **REMOVED**: Ent schemas for `riderprofile` and `riderdocument` have been deleted. All rider data belongs to `logistics-service`. Cafe backend should only store `rider_id` references when linking orders to delivery tasks. |

## Catalog & Menu Management

| Table | Key Columns | Description |
|-------|-------------|-------------|
| `cafes` | `id`, `tenant_id`, `tenant_slug`, `name`, `slug`, `description`, `contact_email`, `phone`, `location`, `latitude`, `longitude`, `opening_hours`, `status`, `created_at`, `updated_at` | Individual outlets under a tenant, mirroring the shared outlet registry used by POS, inventory, and logistics services. |
| `menu_categories` | `id`, `tenant_id`, `cafe_id`, `name`, `description`, `display_order`, `is_active`, `created_at`, `updated_at` | Category hierarchy. |
| `menu_items` | `id`, `tenant_id`, `cafe_id`, `category_id`, `name`, `description`, `base_price`, `currency`, `is_available`, `lead_time_minutes`, `image_url`, `nutrition_json`, `created_at`, `updated_at` | Products available for ordering. |
| `menu_item_variants` | `id`, `menu_item_id`, `name`, `price_delta`, `is_available`, `sku`, `created_at`, `updated_at` | Size/flavour variants inheriting from a menu item. |
| `menu_item_translations` | `(menu_item_id, locale) PK`, `name`, `description`, `created_at`, `updated_at` | Localised copy. |
| `dietary_tags` | `code (PK)`, `label`, `description` | e.g. vegan, gluten-free. |
| `menu_item_dietary_tags` | `(menu_item_id, dietary_code) PK` | Many-to-many link. |
| `menu_item_assets` | `id`, `menu_item_id`, `asset_type`, `url`, `metadata`, `created_at` | Additional media / CDN assets. |
| `menu_item_schedules` | `id`, `menu_item_id`, `day_of_week`, `time_start`, `time_end`, `created_at` | Availability windows. |



## Ordering & Checkout

| Table | Key Columns | Description |
|-------|-------------|-------------|
| `customer_addresses` | `id`, `tenant_id`, `user_id`, `label`, `address_line1`, `address_line2`, `city`, `county`, `postal_code`, `latitude`, `longitude`, `instructions`, `is_default`, `created_at`, `updated_at` | Saved delivery addresses. |
| `carts` | `id`, `tenant_id`, `user_id`, `status`, `currency`, `subtotal`, `discount_total`, `tax_total`, `delivery_fee`, `loyalty_points_redeemed`, `expires_at`, `created_at`, `updated_at` | Active shopping carts. |
| `cart_items` | `id`, `cart_id`, `menu_item_id`, `variant_id`, `name_snapshot`, `quantity`, `unit_price`, `total_price`, `notes`, `metadata`, `created_at`, `updated_at` | Line items within a cart. |
| `orders` | `id`, `tenant_id`, `cafe_id`, `customer_id`, `cart_id`, `order_number`, `status`, `payment_status`, `currency`, `subtotal`, `discount_total`, `tax_total`, `delivery_fee`, `tip_total`, `grand_total`, `loyalty_points_earned`, `loyalty_points_redeemed`, `delivery_address_id`, `promo_code_id`, `instructions`, `channel`, `source`, `idempotency_key`, `placed_at`, `confirmed_at`, `ready_at`, `delivered_at`, `completed_at`, `cancelled_at`, `cancellation_reason`, `metadata`, `created_at`, `updated_at` | Canonical order record. `order_number` is human-readable. `idempotency_key` prevents duplicate order creation. |
| `order_items` | `id`, `order_id`, `menu_item_id`, `variant_id`, `name_snapshot`, `quantity`, `unit_price`, `total_price`, `notes`, `metadata` | Order line items (with snapshot of product info). |
| `order_events` | `id`, `order_id`, `event_type`, `payload`, `actor_user_id`, `occurred_at` | Audit events (status changes, notifications). |
| `order_assignments` | `id`, `order_id`, `rider_id`, `status`, `assigned_at`, `accepted_at`, `picked_up_at`, `completed_at`, `rejected_reason`, `metadata` | Rider dispatch workflow. **Note**: `rider_id` is a reference to logistics-service fleet member ID, not a foreign key. All rider data is owned by logistics-service. |
| `delivery_windows` | `id`, `order_id`, `eta_start`, `eta_end`, `actual_arrival`, `actual_dropoff` | Time commitments. |
| `promo_codes` | `id`, `tenant_id`, `code`, `description`, `discount_type`, `discount_value`, `max_uses`, `usage_count`, `min_subtotal`, `starts_at`, `ends_at`, `metadata`, `created_at`, `updated_at` | Promotion catalogue. |
| `promo_redemptions` | `id`, `promo_code_id`, `order_id`, `user_id`, `redeemed_at`, `discount_amount` | Historical redemptions. |
| `loyalty_accounts` | `id`, `tenant_id`, `user_id`, `balance_points`, `lifetime_points`, `redeemed_points`, `tier`, `tier_progress`, `tier_expires_at`, `created_at`, `updated_at` | Customer loyalty balances with tier progression. |
| `loyalty_transactions` | `id`, `account_id`, `order_id`, `points`, `transaction_type`, `description`, `occurred_at`, `metadata` | Earn/burn ledger. |

## Group Orders

| Table | Key Columns | Description |
|-------|-------------|-------------|
| `group_orders` | `id`, `tenant_id`, `organizer_id`, `cafe_id`, `share_link`, `share_code`, `status`, `deadline`, `max_participants`, `split_method`, `delivery_address_id`, `created_at`, `updated_at`, `finalized_at` | Group ordering sessions for collaborative orders (office lunches, events). `split_method` can be 'equal', 'by_item', or 'custom'. |
| `group_participants` | `id`, `group_order_id`, `user_id`, `participant_name`, `status`, `joined_at`, `items_count`, `contribution_amount` | Tracks participants in a group order. Guest participants can join using only a name (no user_id required). |
| `group_contributions` | `id`, `group_order_id`, `participant_id`, `order_id`, `amount`, `payment_status`, `paid_at` | Individual payment contributions for group order split. Links to individual orders or payment records. |

## Delivery Zones & Availability

| Table | Key Columns | Description |
|-------|-------------|-------------|
| `delivery_zones` | `id`, `tenant_id`, `cafe_id`, `name`, `zone_polygon`, `delivery_fee`, `minimum_order`, `estimated_time_minutes`, `is_active`, `created_at`, `updated_at` | Geographic zones for delivery service areas. `zone_polygon` stored as PostGIS geometry (GeoJSON polygon). Different fees and minimums per zone. |
| `zone_schedules` | `id`, `zone_id`, `day_of_week`, `time_start`, `time_end`, `is_available` | Time-based availability for delivery zones (e.g., lunch hours only, closed Sundays). |
| `availability_checks` | `id`, `tenant_id`, `latitude`, `longitude`, `zone_id`, `is_serviceable`, `checked_at`, `user_id`, `session_id` | Audit log of delivery availability checks for analytics and zone expansion planning. |
| `zone_waitlists` | `id`, `tenant_id`, `email`, `phone`, `address`, `latitude`, `longitude`, `subscribed_at`, `notified_at` | Waitlist for customers in areas not yet serviced. Used to prioritize zone expansion. |

## Payments & Treasury

| Table | Key Columns | Description |
|-------|-------------|-------------|
| `payment_methods` | `id`, `user_id`, `tenant_id`, `provider`, `type`, `mask`, `exp_month`, `exp_year`, `is_default`, `fingerprint`, `created_at`, `updated_at` | Tokenised payment instruments. |
| `payment_intents` | `id`, `order_id`, `provider`, `client_secret`, `status`, `amount`, `currency`, `metadata`, `created_at`, `updated_at` | In-flight payment attempts. |
| `payments` | `id`, `payment_intent_id`, `order_id`, `amount`, `currency`, `status`, `provider_reference`, `processed_at`, `captured_at`, `metadata` | Finalised payment records. |
| `refunds` | `id`, `payment_id`, `amount`, `currency`, `status`, `reason`, `requested_at`, `processed_at`, `metadata` | Refund transactions. |
| `payouts` | `id`, `tenant_id`, `recipient_type`, `recipient_id`, `amount`, `currency`, `status`, `scheduled_at`, `processed_at`, `metadata` | Payouts to riders/cafes. |
| `settlements` | `id`, `tenant_id`, `cafe_id`, `period_start`, `period_end`, `gross_amount`, `net_amount`, `status`, `generated_at`, `metadata` | Periodic accounting for cafes. |
| `treasury_events` | `id`, `external_id`, `event_type`, `payload`, `received_at`, `processed_at`, `status`, `error_message` | Webhook ingestion for treasury systems. |
| _External Integration_ | — | The tables above synchronise with `treasury-api` for ledgering, billing documents, payout approvals, and financial reconciliation. |



## Fulfilment & Logistics

| Table | Key Columns | Description |
|-------|-------------|-------------|
| `delivery_windows` | `id`, `order_id`, `eta_start`, `eta_end`, `actual_arrival`, `actual_dropoff` | Customer-facing ETA commitments sourced from logistics task updates. |
| _Logistics Integration_ | — | **Entity Ownership**: All rider, driver, fleet, delivery task, shift, telemetry, proof-of-delivery, and dispatch rule data is owned by `logistics-service`. Cafe backend stores only task references (`logistics_task_id`) and `rider_id` references in `order_assignments`. All rider/fleet queries must go to logistics-service APIs (`GET /v1/{tenant}/fleet-members`, `GET /v1/{tenant}/tasks`). |



## Notifications & Engagement

| Table | Key Columns | Description |
|-------|-------------|-------------|
| `notification_templates` | `id`, `tenant_id`, `channel`, `event_key`, `locale`, `subject`, `body`, `data_schema`, `is_active`, `created_at`, `updated_at` | Messaging templates. |
| `notification_events` | `id`, `tenant_id`, `event_key`, `payload`, `status`, `attempts`, `last_attempt_at`, `created_at` | Pending notifications for async dispatch. |
| `notification_subscriptions` | `id`, `tenant_id`, `user_id`, `channel`, `event_key`, `is_subscribed`, `updated_at` | Opt-in/opt-out preferences (complements `user_preferences`). |
| _External Integration_ | — | Delivery and marketing messaging is delegated to `notifications-api`, which consumes events emitted from these tables and enforces channel delivery guarantees. |

## Analytics, Support & Compliance

| Table | Key Columns | Description |
|-------|-------------|-------------|
| `report_jobs` | `id`, `tenant_id`, `report_type`, `status`, `requested_by`, `parameters`, `result_url`, `requested_at`, `completed_at`, `error_message` | Long-running analytics jobs / exports. |
| `support_tickets` | `id`, `tenant_id`, `user_id`, `category`, `priority`, `status`, `subject`, `description`, `assigned_to`, `created_at`, `updated_at`, `closed_at` | Support case management. |
| `support_ticket_events` | `id`, `ticket_id`, `event_type`, `payload`, `actor_user_id`, `occurred_at` | Ticket history. |
| `audit_logs` | `id`, `tenant_id`, `user_id`, `resource_type`, `resource_id`, `action`, `metadata`, `ip_address`, `user_agent`, `occurred_at` | Compliance logging. |
| `data_subject_requests` | `id`, `tenant_id`, `user_id`, `request_type`, `status`, `submitted_at`, `processed_at`, `notes` | GDPR / DPA workflows. |
| `backup_jobs` | `id`, `tenant_id`, `job_type`, `status`, `requested_by`, `storage_url`, `requested_at`, `completed_at`, `error_message` | Scheduled backups (daily snapshots, PITR exports) and their lifecycle. |
| `backup_restores` | `id`, `backup_job_id`, `initiated_by`, `restore_point`, `status`, `started_at`, `completed_at`, `notes` | Restore activity with approvals and audit trail. |
| `security_policies` | `id`, `tenant_id`, `password_policy_json`, `session_policy_json`, `created_at`, `updated_at` | Tenant-configurable security posture surfaced in admin settings. |

## Integration & API Settings

| Table | Key Columns | Description |
|-------|-------------|-------------|
| `integration_settings` | `id`, `tenant_id`, `service_code`, `status`, `config_json`, `created_at`, `updated_at` | Generic key/value configuration for treasury, notifications, POS, and future integrations. |
| `integration_webhooks` | `id`, `integration_setting_id`, `url`, `auth_type`, `secret`, `event_filter`, `created_at`, `updated_at`, `last_delivery_status` | Outgoing webhook destinations per integration. |
| `api_clients` | `id`, `tenant_id`, `name`, `client_id`, `client_secret_hash`, `scopes`, `last_used_at`, `created_at`, `updated_at`, `revoked_at` | API credentials for partners/internal tooling. |
| `api_tokens` | `id`, `api_client_id`, `token_hash`, `expires_at`, `created_at`, `revoked_at`, `reason` | Rotatable tokens aligned with security policies. |
| `tenant_sync_events` | `id`, `tenant_id`, `tenant_slug`, `destination_service`, `payload`, `synced_at`, `status` | Tracks outbound tenant/outlet discovery callbacks to ensure downstream services hydrate metadata before processing domain data. |

## Cross-Cutting Utilities

| Table | Key Columns | Description |
|-------|-------------|-------------|
| `outbox_events` | `id`, `tenant_id`, `aggregate_type`, `aggregate_id`, `event_type`, `payload`, `status`, `attempts`, `last_attempt_at`, `created_at` | Reliable event publishing pattern. |
| `media_assets` | `id`, `tenant_id`, `owner_type`, `owner_id`, `asset_type`, `url`, `metadata`, `created_at` | Generic file references (e.g., receipts, documents). |
| `tenant_sync_events` | `id`, `tenant_id`, `tenant_slug`, `destination_service`, `payload`, `synced_at`, `status` | Tracks outbound tenant/outlet discovery callbacks so downstream services hydrate metadata before processing domain data. |

---

## Diagram Notes

- Relationships follow standard relational conventions:
  - `users.tenant_id -> tenants.id`
  - `orders.customer_id -> users.id`
  - `order_items.order_id -> orders.id`
  - `menu_items.category_id -> menu_categories.id`
  - `order_assignments.rider_id` - Reference to logistics-service fleet member ID (NOT a foreign key, rider data owned by logistics-service)
  - `payments.payment_intent_id -> payment_intents.id`
  - `support_ticket_events.ticket_id -> support_tickets.id`
- Tenant/outlet discovery relies on webhook flows captured in `tenant_sync_events`, ensuring downstream services are provisioned before related domain records are written (no polling needed).

- Where `metadata` or `payload` columns appear they are stored as `JSONB` for flexible, validated schemas.

- Many tables include `created_at` / `updated_at` audit fields for tracking history even before formal audit logging is enabled.

---

## Seed Data Overview

- **Demo Tenant**: `demo-tenant` (slug) - "Demo Tenant" (name) - for development/testing only
- System roles: `customer`, `rider`, `staff`, `admin`, `superuser`.
- Permissions grouped by module (auth, catalog, orders, payments, logistics, operations, notifications, analytics, support).
- Default super admin account seeded for bootstrap with full permission set, scoped to the demo tenant.
- **Demo Users**: Seeded idempotently via `cmd/seed/main.go` for testing purposes.

**Note**: In production, tenants are created through normal registration flows. The seed data is provided for development/testing purposes only. All authentication operations require a `tenant_slug` parameter - there is no default tenant.

Refer to `cmd/seed` for the initial data set and execution instructions in `README.md`.


