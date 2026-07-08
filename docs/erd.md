# Ordering Service – Entity Relationship Overview

This document provides a textual ERD for the Codevertex Ordering Service (online delivery/shipping orders only).
The schema supports **all business types** — food/hospitality, retail, grocery, pharmacy, electronics, e-commerce, etc. — through a flexible catalog projection model backed by `inventory-api` as the single source of truth for item master data.
The database structure is defined via Ent schemas — the Go source of truth that powers code generation and migrations.

> **Conventions**
>
> - All primary keys are UUIDs unless noted.
> - `tenant_id` enforces multi-tenant isolation on every table.
> - Timestamps use `TIMESTAMPTZ`.
> - JSON metadata columns allow forward-compatible extension.
> - `catalog_item_id` (not `menu_item_id`) is the canonical reference for order items.

---

## Identity & Access Management

| Table | Key Columns | Purpose / Relationships |
|-------|-------------|-------------------------|
| `tenants` | `id`, `slug`, `name`, `status`, `use_case`, `subscription_plan`, `tier_limits`, `metadata`, `created_at`, `updated_at` | Master record synced from auth-service. `slug` shared across all microservices. `use_case` drives UI hints (hospitality, retail, e_commerce, etc.). Default demo tenant slug: `urban-loft`. |
| `tenant_settings` | `tenant_id (PK)`, `brand_palette`, `locales`, `features`, `updated_at` | JSON configuration (colors, localisation, feature toggles). |
| `users` | `id`, `tenant_id`, `auth_service_user_id` (UUID UNIQUE), `email`, `full_name`, `phone`, `status`, `sync_status`, `sync_at`, `last_login_at`, `created_at`, `updated_at` | Core user profile. `auth_service_user_id` references auth-service user. Synced via `auth.user.*` events. Unique `(tenant_id, email)`. |
| `roles` | `code (PK)`, `name`, `description`, `scope`, `system_role` | Legacy role catalogue: `customer`, `rider`, `staff`, `admin`, `superuser`. Kept for backward compatibility. |
| `permissions` | `code (PK)`, `name`, `module`, `description` | Legacy permission catalogue. Kept for backward compatibility. |
| `role_legacy_permissions` | `(role_id, permission_id) PK` | Legacy role-permission M2M (renamed from `role_permissions`). |
| `ordering_permissions` | `id` (UUID PK), `permission_code` (UNIQUE), `name`, `module`, `action`, `resource`, `description`, `created_at` | New RBAC permission catalogue using `ordering.{module}.{action}` codes. Modules: orders, catalog, outlets, promotions, delivery_zones, delivery_windows, loyalty, analytics, config, users. |
| `ordering_roles` | `id` (UUID PK), `tenant_id`, `role_code`, `name`, `description`, `is_system_role`, `created_at`, `updated_at` | Tenant-scoped RBAC roles. System roles: `ordering_admin`, `store_manager`, `kitchen_staff`, `cashier`, `delivery_coordinator`, `viewer`. Unique `(tenant_id, role_code)`. |
| `role_permissions` | `id` (PK), `role_id` (UUID FK), `permission_id` (UUID FK) | Junction table: ordering_role to ordering_permission. Unique `(role_id, permission_id)`. |
| `user_role_assignments` | `id` (UUID PK), `tenant_id`, `user_id` (FK users), `role_id` (FK ordering_roles), `assigned_by`, `assigned_at`, `expires_at` | User-to-role assignments with optional expiry. Unique `(tenant_id, user_id, role_id)`. |
| `user_preferences` | `user_id (PK)`, `theme`, `language`, `notify_email`, `notify_sms`, `notify_push`, `timezone` | Personalisation settings. |
| `rate_limit_configs` | `id` (UUID PK), `service_name`, `key_type`, `endpoint_pattern`, `requests_per_window`, `window_seconds`, `burst_multiplier`, `is_active`, `description` | Database-driven rate limit configuration. Unique `(service_name, key_type, endpoint_pattern)`. |
| `service_configs` | `id` (UUID PK), `tenant_id` (nullable), `config_key`, `config_value`, `config_type`, `description`, `is_secret` | Service-level configuration key-value pairs. Nil `tenant_id` = platform default; non-nil = tenant override. Unique `(tenant_id, config_key)`. |
| _SSO Integration_ | — | Auth delegated to auth-service (OIDC). JWT claims are **source of truth** for roles and permissions — local DB used for extensions only. `IsSuperuser` and `IsAdmin` bypass all RBAC checks. |
| _Rider Data_ | — | **NOT OWNED HERE**: All rider, fleet, delivery task, and PoD data owned by `logistics-service`. Ordering stores only `rider_id` references. |

---

## Catalog Management (Use-Case Agnostic)

The catalog is a **tenant-scoped projection** of inventory-api master data. It is **not** limited to food/menu items — it adapts to any business type: hospitality (dishes), retail (products), grocery (packaged goods), pharmacy (medications), electronics, etc.

| Table | Key Columns | Description |
|-------|-------------|-------------|
| `outlets` | `id`, `tenant_id`, `tenant_slug`, `name`, `slug`, `description`, `contact_email`, `phone`, `location`, `latitude`, `longitude`, `opening_hours`, `status`, `created_at`, `updated_at` | Physical or virtual outlet (store, restaurant, warehouse, etc.) under a tenant. **Projection** — master data owned by inventory-api (Warehouse entity) and auth-service. |
| `catalog_categories` | `id`, `tenant_id`, `outlet_id` (optional), `name`, `slug`, `description`, `image_url`, `parent_id` (optional), `display_order`, `is_active`, `created_at`, `updated_at` | Hierarchical item categories scoped to tenant/outlet. `slug` is a URL-safe identifier. Use-case examples: Beverages/Food/Pastries (café), Electronics/Accessories (retail), Fresh Produce/Dairy (grocery). Synced from inventory-api or created locally. |
| `catalog_items` | `id`, `tenant_id`, `outlet_id` (optional), `inventory_item_id` (ref to inventory-api), `category_id`, `recipe_id` (optional, ref to inventory-api recipe), `sku`, `name`, `description`, `image_url`, `base_price`, `currency`, `display_order`, `is_available`, `is_featured`, `lead_time_minutes`, `metadata`, `created_at`, `updated_at` | Fulfillment-ready projection of an inventory item. SKU links to `inventory-api` for stock checks and reservations. `inventory_item_id` is the master reference. Works for **any item type**: food dishes, grocery products, electronic goods, apparel, pharmacy items, etc. |
| `dietary_tags` | `code (PK)`, `label`, `description` | Optional use-case tags — e.g., `vegan`, `gluten_free`, `organic`, `halal`. Applicable to hospitality/food items; ignored for other use cases. |
| `catalog_item_dietary_tags` | `(catalog_item_id, dietary_code) PK` | Many-to-many link for dietary/attribute tagging. |
| `catalog_item_assets` | `id`, `catalog_item_id`, `asset_type`, `url`, `display_order`, `is_primary`, `metadata`, `created_at` | CDN media assets per catalog item (images, videos). |
| `catalog_item_schedules` | `id`, `catalog_item_id`, `day_of_week`, `time_start`, `time_end`, `is_active`, `created_at` | Time-based availability windows (e.g., lunch specials, seasonal promotions). |

### Catalog Item Reference Chain

```
OrderItem.catalog_item_id
    ↓
CatalogItem (local projection per tenant/outlet)
    ├── name, description, image_url, base_price  (display data)
    ├── sku                                         (stock reference key)
    ├── inventory_item_id                           (FK ref to inventory-api)
    └── recipe_id (optional)                        (FK ref to inventory-api recipe)

For stock ops: CatalogItem.sku / inventory_item_id → inventory-api REST API
```

### Multi-Use-Case Examples

| Use Case | Catalog Category Examples | Catalog Item Examples |
|----------|--------------------------|----------------------|
| Hospitality (café/restaurant) | Hot Beverages, Mains, Desserts | Cappuccino, Club Sandwich, Cheesecake |
| Grocery / Convenience | Fresh Produce, Dairy, Snacks | Avocado, Milk 1L, Doritos |
| Electronics / Retail | Phones, Accessories, Laptops | iPhone 15, USB-C Cable, MacBook |
| Pharmacy | OTC Medication, Supplements | Paracetamol 500mg, Vitamin C |
| Flowers / Gifts | Arrangements, Gift Sets | Red Roses Bouquet, Birthday Hamper |

---

## Shopping Cart & Checkout

| Table | Key Columns | Description |
|-------|-------------|-------------|
| `customer_addresses` | `id`, `tenant_id`, `user_id`, `label`, `address_line1`, `city`, `county`, `postal_code`, `latitude`, `longitude`, `plus_code`, `instructions`, `contact_name`, `contact_phone`, `is_default`, `is_verified`, `created_at`, `updated_at` | Saved delivery addresses per customer. |
| `carts` | `id`, `tenant_id`, `user_id`, `session_id` (guest), `status`, `currency`, `subtotal`, `discount_total`, `tax_total`, `delivery_fee`, `loyalty_points_redeemed`, `expires_at`, `created_at`, `updated_at` | Active shopping carts. Supports guest checkout via `session_id`. Status: `active`, `checked_out`, `abandoned`, `expired`. |
| `cart_items` | `id`, `cart_id`, `catalog_item_id`, `variant_id` (optional), `name_snapshot`, `unit_price`, `quantity`, `total_price`, `notes`, `modifiers` (JSON), `metadata`, `created_at`, `updated_at` | Cart line items. `catalog_item_id` references the catalog projection. `name_snapshot` captured at add-to-cart time. |

---

## Orders

| Table | Key Columns | Description |
|-------|-------------|-------------|
| `orders` | `id`, `tenant_id`, `outlet_id`, `customer_id`, `cart_id`, `order_number`, `status`, `payment_status`, `payment_intent_id` (ref to treasury-api), `currency`, `subtotal`, `discount_total`, `tax_total`, `delivery_fee`, `tip_total`, `grand_total`, `loyalty_points_earned`, `loyalty_points_redeemed`, `delivery_address_id`, `promo_code_id`, `instructions`, `channel`, `idempotency_key`, `placed_at`, `confirmed_at`, `ready_at`, `delivered_at`, `completed_at`, `cancelled_at`, `cancellation_reason`, `rating` (1-5), `rating_comment`, `rated_at`, `metadata`, `created_at`, `updated_at` | Canonical online delivery order. `outlet_id` is a reference (not FK) to the fulfilling outlet. `payment_intent_id` references treasury-api. |
| `order_items` | `id`, `order_id`, `catalog_item_id`, `variant_id` (optional), `name_snapshot`, `variant_name_snapshot`, `unit_price`, `quantity`, `total_price`, `notes`, `modifiers` (JSON), `metadata` | Order line items. `catalog_item_id` is the canonical item reference (replaces legacy `menu_item_id`). `name_snapshot` and `unit_price` captured at order time. |
| `order_events` | `id`, `order_id`, `event_type`, `payload`, `actor_user_id`, `occurred_at` | Audit trail for order lifecycle transitions. |
| `order_assignments` | `id`, `order_id`, `logistics_task_id` (ref to logistics-api), `rider_id` (string ref — NOT FK), `status`, `assigned_at`, `accepted_at`, `picked_up_at`, `completed_at`, `cancelled_at`, `metadata` | Delivery dispatch. `rider_id` references logistics-service fleet member. |
| `delivery_windows` | `id`, `order_id`, `assignment_id`, `eta_start`, `eta_end`, `actual_arrival`, `actual_dropoff`, `eta_minutes`, `distance_km`, `route_info` (JSON), `source`, `is_current` | Customer-facing ETA from logistics task updates. |

### Order Status Flow

```
pending → confirmed → preparing → ready → out_for_delivery → delivered → completed
                                       ↘ cancelled
                                       ↘ refunded
```

### Payment Status Flow

```
pending → authorized → paid → completed
                    ↘ failed
                    ↘ refunded / partially_refunded
```

---

## Promotions & Loyalty

| Table | Key Columns | Description |
|-------|-------------|-------------|
| `promo_codes` | `id`, `tenant_id`, `outlet_id` (optional), `code`, `description`, `discount_type` (percentage/fixed_amount/free_delivery/free_item), `discount_value`, `max_discount_amount`, `min_subtotal`, `max_uses`, `max_uses_per_user`, `first_order_only`, `eligible_categories` (JSON), `eligible_items` (JSON), `starts_at`, `ends_at`, `metadata`, `created_at`, `updated_at` | Promotion catalogue. |
| `promo_redemptions` | `id`, `promo_code_id`, `order_id`, `user_id`, `redeemed_at`, `discount_amount` | Historical redemption log. |
| `loyalty_accounts` | `id`, `tenant_id`, `user_id` (UNIQUE), `balance_points`, `lifetime_points`, `redeemed_points`, `tier`, `tier_progress`, `tier_expires_at`, `created_at`, `updated_at` | Per-customer loyalty balance with tier progression (bronze/silver/gold/platinum). |
| `loyalty_transactions` | `id`, `account_id`, `order_id`, `points`, `transaction_type` (earn/burn/adjust), `description`, `occurred_at`, `metadata` | Earn/burn ledger. |

---

## Group Orders

| Table | Key Columns | Description |
|-------|-------------|-------------|
| `group_orders` | `id`, `tenant_id`, `organizer_id`, `outlet_id`, `share_link`, `share_code`, `status`, `deadline`, `max_participants`, `split_method`, `delivery_address_id`, `created_at`, `updated_at`, `finalized_at` | Collaborative ordering sessions (e.g., office lunch). `split_method`: equal, by_item, custom. |
| `group_participants` | `id`, `group_order_id`, `user_id`, `participant_name`, `status`, `joined_at`, `items_count`, `contribution_amount` | Participants (registered or guest). |
| `group_contributions` | `id`, `group_order_id`, `participant_id`, `order_id`, `amount`, `payment_status`, `paid_at` | Split payment contributions. |

---

## Delivery Zones & Availability

| Table | Key Columns | Description |
|-------|-------------|-------------|
| `delivery_zones` | `id`, `tenant_id`, `outlet_id`, `name`, `zone_polygon` (GeoJSON), `delivery_fee`, `minimum_order`, `estimated_time_minutes`, `is_active`, `created_at`, `updated_at` | Geographic delivery service areas per outlet. |
| `zone_schedules` | `id`, `zone_id`, `day_of_week`, `time_start`, `time_end`, `is_available` | Time-based zone availability. |
| `availability_checks` | `id`, `tenant_id`, `latitude`, `longitude`, `zone_id`, `is_serviceable`, `checked_at`, `user_id`, `session_id` | Audit log of delivery availability checks. |

---

## Payments & Treasury (Reference-Only)

**Data ownership**: Payment intents, transactions, refunds, and webhook events are **owned by treasury-api**. Ordering-backend stores only `payment_intent_id` reference and `payment_status`.

| Table | Owner | Integration |
|-------|-------|-------------|
| `payment_intents`, `transactions`, `refunds` | `treasury-api` | Referenced via `orders.payment_intent_id`. |

---

## Fulfilment & Logistics (Reference-Only)

**Data ownership**: Delivery tasks, PoD, and fleet data are **owned by logistics-api**. Ordering stores only task/rider references.

| Table | Owner | Integration |
|-------|-------|-------------|
| `delivery_tasks`, `proof_of_delivery` | `logistics-api` | Referenced via `order_assignments.logistics_task_id`. |

---

## Analytics, Support & Compliance

| Table | Key Columns | Description |
|-------|-------------|-------------|
| `audit_logs` | `id`, `tenant_id`, `user_id`, `resource_type`, `resource_id`, `action`, `metadata`, `ip_address`, `occurred_at` | Compliance audit trail. |
| `data_subject_requests` | `id`, `tenant_id`, `user_id`, `request_type`, `status`, `submitted_at`, `processed_at`, `notes` | GDPR/DPA workflows. |
| `data_deletion_jobs` | `id`, `tenant_id`, `user_id`, `status`, `scheduled_at`, `completed_at` | Scheduled data deletion jobs. |
| `data_export_jobs` | `id`, `tenant_id`, `user_id`, `status`, `result_url`, `requested_at`, `completed_at` | Data portability exports. |

---

## Cross-Cutting Infrastructure

| Table | Key Columns | Description |
|-------|-------------|-------------|
| `outbox_events` | `id`, `tenant_id`, `aggregate_type`, `aggregate_id`, `event_type`, `payload`, `status`, `attempts`, `last_attempt_at`, `published_at`, `error_message`, `created_at` | Transactional outbox for reliable NATS event publishing. Status: PENDING → PUBLISHED / FAILED. |
| `tenant_sync_events` | `id`, `tenant_id`, `tenant_slug`, `destination_service`, `payload`, `synced_at`, `status` | Tracks outbound tenant/outlet provisioning to downstream services. |

---

## Key Relationships

```
tenants.id ← users.tenant_id
tenants.id ← outlets.tenant_id
tenants.id ← catalog_categories.tenant_id
tenants.id ← catalog_items.tenant_id
tenants.id ← orders.tenant_id
users.id ← orders.customer_id
orders.id ← order_items.order_id
catalog_items.id ← order_items.catalog_item_id          ← PRIMARY ITEM REFERENCE
catalog_items.inventory_item_id → inventory-api items   ← MASTER DATA REFERENCE
catalog_items.sku → inventory-api stock checks
orders.id ← order_assignments.order_id
order_assignments.logistics_task_id → logistics-api     ← REFERENCE ONLY
orders.payment_intent_id → treasury-api                 ← REFERENCE ONLY
```

---

## Seed Data Overview

- **Demo Tenant**: `urban-loft` (slug) — "Urban Loft Cafe" — synced from auth-service via UUID
- **Roles**: `customer`, `rider`, `staff`, `admin`, `superuser`
- **Permissions**: Django-style per entity — `{entity}:{add|read|read_own|change|change_own|delete|manage|manage_own}` for: `orders`, `catalog`, `payments`, `loyalty`, `notifications`, `analytics`, `support`, `logistics`
- **Catalog items**: **NOT seeded here** — catalog items and categories are seeded in `inventory-api` (single source of truth). Ordering-backend catalog is populated from inventory-api via sync events or admin UI.
- **Default admin**: Synced from auth-service via tenant sync on first login (JIT provisioning).

> Refer to `cmd/seed/main.go` for roles/permissions seed and `inventory-api/cmd/seed/main.go` for item/category seed.
