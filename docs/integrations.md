# Ordering Backend - Integration Guide

## Overview

This document provides detailed integration information for all external services and systems integrated with the Ordering backend, including internal Codevertex microservices and external third-party services.

**Last Updated**: March 2026

---

## Implementation Status Summary

| Service | Integration Type | Status | Implementation |
|---------|-----------------|--------|----------------|
| **Auth Service** | REST + Events + JWT | ✅ Implemented | `shared-auth-client` library, event handlers |
| **Treasury Service** | REST + Webhooks | ✅ Implemented | `internal/platform/treasury/client.go`, webhook handlers |
| **Logistics Service** | REST + Webhooks | ✅ Implemented | `internal/platform/logistics/client.go`, webhook handlers |
| **Notifications Service** | REST + Events | ✅ Implemented | `internal/platform/notifications/client.go`, local module |
| **Inventory Service** | REST + Events | ✅ Implemented | `internal/platform/inventory/client.go` (inventory-api MVP live with 8 endpoints) |
| **POS Service** | Events | ✅ Implemented | Event publisher with catalog sync and pickup handoff |
| **NATS JetStream** | Events | ✅ Implemented | Publisher and subscriber in `internal/platform/events/` |
| **Redis** | Cache | ✅ Implemented | `internal/platform/cache/redis.go` |
| **PostgreSQL** | Database | ✅ Implemented | Ent ORM with **versioned Atlas migrations** at startup (`schema.WithDir(migrate.Dir)`) |

### Current Integration Implementations

**Fully Implemented:**
- Treasury client with M-Pesa STK Push, Stripe, Paystack, Flutterwave support
- Treasury webhook handler with HMAC signature verification
- Logistics client with task creation, tracking, and fleet member queries
- Logistics webhook handler with full task lifecycle events
- Auth JWT validation via shared-auth-client
- User sync from auth-service events
- Inventory client with stock availability, reservations, and consumption tracking
- Notifications client with multi-channel support (email, SMS, push)
- Event publisher for order lifecycle events (created, ready, cancelled, completed)
- Event publisher for payment events (initiated, completed, failed)
- Event publisher for catalog sync (for POS integration)
- Event publisher for pickup order handoff (ordering → POS)

**Platform Clients:**
- `internal/platform/treasury/client.go` - Treasury service REST client
- `internal/platform/logistics/client.go` - Logistics service REST client
- `internal/platform/inventory/client.go` - Inventory service REST client
- `internal/platform/notifications/client.go` - Notifications service REST client
- `internal/platform/events/publisher.go` - NATS event publisher
- `internal/platform/events/subscriber.go` - NATS event subscriber

**Webhook Endpoints:**
- `POST /api/v1/webhooks/treasury` - Treasury payment events
- `POST /api/v1/webhooks/mpesa/*` - M-Pesa callbacks
- `POST /api/v1/webhooks/logistics` - Logistics task events

**Events Published:**
- `cafe.order.created` - Order placed (→ inventory, notifications)
- `cafe.order.ready` - Order ready for delivery (→ logistics, notifications)
- `cafe.order.cancelled` - Order cancelled (→ inventory, notifications)
- `cafe.order.completed` - Order delivered (→ inventory, notifications)
- `cafe.order.status.changed` - Status updates (→ notifications)
- `cafe.payment.initiated` - Payment started (→ treasury)
- `cafe.payment.completed` - Payment successful (→ notifications)
- `cafe.payment.failed` - Payment failed (→ notifications)
- `cafe.loyalty.points_awarded` - Loyalty points earned (→ notifications, POS)
- `ordering.catalog.updated` - Catalog changes (→ POS sync)
- `ordering.order.for_pickup` - Pickup order handoff (→ POS)

---

## Table of Contents

1. [Implementation Status Summary](#implementation-status-summary)
2. [Internal Codevertex Service Integrations](#internal-codevertex-service-integrations)
3. [External Third-Party Integrations](#external-third-party-integrations)
4. [Integration Patterns](#integration-patterns)
5. [Two-Tier Configuration Management](#two-tier-configuration-management)
6. [Event-Driven Architecture](#event-driven-architecture)
7. [Integration Security](#integration-security)
8. [Error Handling & Resilience](#error-handling--resilience)
9. [Media storage (menu images and uploads)](#media-storage-menu-images-and-uploads)

---

## Media storage (menu images and uploads)

**Local development**
- `ORDERING_MEDIA_ROOT` defaults to `./media`. Create `./media/menu` and place or symlink menu images for local file serving at `GET /media/*`.
- Seed script stores menu item `image_url` as paths (e.g. `/images/menu/espresso.jpg`) for frontend use; cafe-website serves these from its own `public/images/menu`.

**Production (Kubernetes)**
- Set `ORDERING_MEDIA_ROOT=/media` and mount a PVC at `/media` (see devops-k8s `persistence` in ordering-backend values).
- Init container creates `.../menu`, `.../products`, etc., and sets permissions to 775; app runs as UID 1000 with `fsGroup: 1000` so the process can read/write the volume.
- Uploaded menu images or assets can be stored under `/media/menu` and referenced by full URL (e.g. `https://orderingapi.codevertexafrica.com/media/menu/xyz.jpg`) or set `ORDERING_MEDIA_URL_BASE` for absolute URLs in API responses.

**Ingress**
- Ensure ingress allows `proxy-body-size` and routing for `/media/*` so uploads and static media are served correctly.

**Order creation**
- Backend supports (1) cart + `POST /checkout` and (2) direct `POST /api/v1/{tenant}/orders` with body `{ outletId, items, deliveryAddress, paymentMethod, ... }`. `outletId` must be the cafe/outlet UUID (see catalog or outlets API).

---

## Internal Codevertex Service Integrations

### Auth Service

**Integration Type**: OAuth2/OIDC + Events + REST + Service-to-Service Auth

**Production URL**: `https://sso.codevertexafrica.com/`

**Default Tenant**: The ordering service uses `urban-cafe` as the default tenant slug. All users created without a custom `tenant_slug` will be assigned to the `urban-cafe` tenant. The tenant is created with slug `urban-cafe` during seeding.

**Use Cases**:
- User authentication and authorization (all login/registration proxied to auth-service)
- JWT token validation via JWKS
- User identity synchronization via events
- Tenant/outlet discovery via events
- MFA enforcement (managed by auth-service)
- Superuser detection and RBAC bypass
- Service-to-service authentication via API keys

**Architecture**:
- **Authentication Flow**: All login/registration requests proxy to auth-service endpoints
  - Login: `POST https://sso.codevertexafrica.com/api/v1/auth/login` with `{email, password, tenant_slug}`
  - Registration: `POST https://sso.codevertexafrica.com/api/v1/auth/register` with `{email, password, tenant_slug, profile}`
  - Returns: `{access_token, refresh_token, session_id, tenant, user}` from auth-service
- **JWT Validation**: Uses `shared/auth-client` library for token validation
  - JWKS endpoint: `https://sso.codevertexafrica.com/api/v1/.well-known/jwks.json`
  - JWKS cache with configurable TTL and refresh interval
  - All protected `/api/v1` routes require valid Bearer tokens from auth-service
- **User Sync**: Local user table stores `auth_service_user_id` reference
  - Identity data (email, phone, status) synced from auth-service via events
  - Ordering-specific data (preferences, loyalty points) stored locally
  - Sync status tracked via `sync_status` and `sync_at` fields

**REST API Usage**:
- `POST /api/v1/auth/login` - Proxy to auth-service login (requires `tenant_slug`)
- `POST /api/v1/auth/register` - Proxy to auth-service registration (requires `tenant_slug`)
- `POST /api/v1/auth/refresh` - Proxy to auth-service token refresh
- `GET /api/v1/users/{id}` - Get user details from auth-service (for identity sync)
- `GET /api/v1/tenants/{id}` - Get tenant details from auth-service
- `GET /api/v1/tenants/by-slug/{slug}` - Get tenant by slug from auth-service
- `GET /api/v1/.well-known/jwks.json` - JWKS for token validation

**Events Consumed**:
- `auth.user.created` - Create local user with app-specific defaults, store `auth_service_user_id`
- `auth.user.updated` - Update local user identity fields (email, phone, status)
- `auth.user.deactivated` - Deactivate local user
- `auth.tenant.created` - Initialize tenant in ordering system
- `auth.tenant.updated` - Update tenant metadata
- `auth.tenant.synced` - Sync tenant metadata
- `auth.outlet.created` - Create outlet reference
- `auth.outlet.updated` - Update outlet metadata

**Events Published**: None (auth-service is publisher)

**Configuration**:
- Auth-service base URL: `AUTH_SERVICE_URL=https://sso.codevertexafrica.com` (environment variable)
- JWKS endpoint: `https://sso.codevertexafrica.com/api/v1/.well-known/jwks.json`
- Issuer: `https://sso.codevertexafrica.com` (from JWT claims)
- Audience: `codevertex` (from JWT claims)
- JWKS cache TTL: `AUTH_JWKS_CACHE_TTL=3600s` (default)
- JWKS refresh interval: `AUTH_JWKS_REFRESH_INTERVAL=300s` (default)
- API key auth enabled: `AUTH_ENABLE_API_KEY_AUTH=true` (for service-to-service)

**User Synchronization**:
- Local `users` table stores:
  - `auth_service_user_id` (UUID, UNIQUE) - Reference to auth-service user
  - Ordering-specific data: preferences, loyalty points, rider profiles
  - Sync metadata: `sync_status`, `sync_at`
- Identity data synced from auth-service:
  - Email, phone, status, last_login_at
  - Tenant membership and roles from auth-service
- Sync triggers:
  - On `auth.user.created` event: Create local user with defaults
  - On `auth.user.updated` event: Update identity fields
  - On login: Verify sync status, update if needed
  - **On Google OAuth**: ordering-backend (or the app using it) fetches Google profile → calls auth-service SyncUser (`POST /api/v1/users/sync`) → updates local user with returned `auth_service_user_id`.

**Superuser Handling**:
- Superusers from auth-service (role: `superuser`) bypass all RBAC/permission checks
- Superuser detection from JWT claims (`roles` array contains `superuser`)
- Superuser can access all services without restrictions
- Superuser sync: Default superuser from auth-service seed synced to all services

**Service-to-Service Authentication**:
- API key authentication for inter-service communication
- API keys managed by auth-service
- Service accounts for automated operations
- API key validation via auth-service endpoints

**Tenant Handling**:
- Tenant slug is required for all authentication operations (no default tenant)
- All requests must include `tenant_slug` parameter for login/registration
- Tenant ID extracted from JWT claims (`tenant_id` field)
- Multi-tenant isolation enforced via tenant_id in all queries
- Tenant metadata synced from auth-service via events
- Tenant auto-discovery: If tenant doesn't exist in auth-service, it's automatically created from local database

**Tenant Auto-Discovery and Sync**:
- **Auto-Discovery**: When a user attempts to register or login with a `tenant_slug` that doesn't exist in auth-service, the cafe service automatically pulls full tenant details from the local database and creates the tenant in auth-service with the **same UUID and slug** before proceeding with the authentication operation.
- **Tenant ID Matching**: Tenant IDs (UUIDs) and slugs must match across all services. When syncing a tenant to auth-service, the cafe service uses the same UUID from its local database to ensure consistency.
- **No Authentication Required**: Tenant creation in auth-service is a public operation (no authentication required) via `POST /api/v1/tenants` to enable seamless tenant auto-discovery across all services.
- **Billing Plan Independence**: Unlike other services (inventory, POS, logistics) that may require proper billing plans before tenant sync, auth-service is accessible in all plans (free or paid), making tenant auto-discovery always available.
- **Sync Flow**:
  1. User requests registration/login with `tenant_slug`
  2. Cafe service checks if tenant exists in auth-service via `GET /api/v1/tenants/by-slug/{slug}`
  3. If tenant doesn't exist:
     - Cafe service queries local database for full tenant details using `FindTenantBySlug()`
     - If tenant exists locally, uses the same UUID, slug, name, contact info, and metadata
     - If tenant doesn't exist locally, generates a new UUID and uses defaults
     - Creates tenant in auth-service via `POST /api/v1/tenants` with `id` field set to the UUID
  4. Registration/login proceeds normally
- **Error Handling**: If tenant sync fails (network issues, etc.), the operation continues anyway - auth-service may create the tenant automatically during registration, or the operation will fail with a clear error message.
- **Tenant Metadata**: When auto-creating a tenant, the cafe service includes:
  - `id`: Tenant UUID (must match across all services)
  - `slug`: Tenant identifier (must match across all services)
  - `name`: Tenant display name (from local database or derived from slug)
  - `contact_email`: Contact email (from local database or default, stored in metadata)
  - `contact_phone`: Contact phone (from local database or default, stored in metadata)
  - `metadata.source`: Set to `"cafe-service"` to indicate origin
  - `metadata.auto_created`: Set to `true` to indicate auto-discovery
  - `metadata.synced_at`: Timestamp of sync operation

### Notifications Service

**Integration Type**: Events (NATS) + REST API

**Use Cases**:
- Order confirmation notifications
- Order status updates
- Delivery ETA notifications
- Loyalty point notifications
- Marketing campaigns

**REST API Usage**:
- `POST /v1/{tenantId}/notifications/messages` - Send notification
- `GET /v1/{tenantId}/templates` - Get notification templates
- `GET /v1/{tenantId}/preferences` - Get user notification preferences

**Events Published**:
- `cafe.order.created` - Trigger order confirmation notification
- `cafe.order.status.changed` - Trigger status update notification
- `cafe.order.ready` - Notify customer order ready
- `cafe.loyalty.points_awarded` - Send loyalty notification

**Events Consumed**:
- `notifications.delivery.completed` - Track notification delivery
- `notifications.delivery.failed` - Handle delivery failures

**Configuration**:
- Notifications service base URL: `NOTIFICATIONS_SERVICE_BASE_URL` (environment variable)
- Event transport: NATS JetStream
- Retry policy: Exponential backoff (3 retries)

### Treasury App

**Integration Type**: REST API + Events (NATS) + Webhooks

**Use Cases**:
- Payment processing (M-Pesa STK Push, card payments)
- Payment intent creation
- Refund processing
- Payout tracking
- Settlement reconciliation
- Subscription invoicing

**REST API Usage**:
- `POST /api/v1/{tenant}/payments/intents` - Create payment intent (use `payment_method: "pending"` for invoice-only; then redirect user to shared pay page). See [payment-workflow.md](../../../shared-docs/payment-workflow.md).
- `POST /api/v1/{tenant}/payments/intents/{id}/initiate` - Initiate payment for existing intent (paystack, mpesa, cash, manual/till). Staff STK at handover sends `mpesa` with no gateway, which treasury resolves to the outlet's Daraja account first, then PayHero.
- `GET /api/v1/pay/{tenant}/gateways` - Enabled gateways for `GET /payment-methods` (`gatewayDisplay` labels them). Since 2026-10-05 PayHero is listed as `payhero` (its rails in `payhero_methods`) and `mpesa` means the outlet's own Daraja paybill or till. A checkout choice of `payhero` is stored as `payment_method=mpesa` with `metadata.payment_gateway=payhero` (`resolvePaymentMethod`; the enum is unchanged and the order still confirms through the treasury callback).
- `POST /api/v1/{tenant}/payments/intents/{id}/confirm-manual` - Mark intent paid when user paid at till/agent.
- Refund/confirm flows as per treasury-api docs.
- `GET /api/v1/payouts/{id}` - Get payout status
- `GET /api/v1/settlements` - Get settlement data
- `POST /api/v1/invoices` - Create subscription invoice

**Webhooks Consumed**:
- `treasury.payment.success` - Update order payment status
- `treasury.payment.failed` - Handle payment failure
- `treasury.refund.completed` - Update refund status
- `treasury.payout.completed` - Update payout status
- `treasury.settlement.generated` - Process settlement

**Events Published**:
- `cafe.payment.initiated` - Payment intent created
- `cafe.payout.requested` - Payout request for rider/cafe

**Configuration**:
- Treasury service base URL: `TREASURY_SERVICE_BASE_URL` (environment variable)
- Webhook secret: Stored encrypted (Tier 1)
- M-Pesa configuration: Short code, consumer key/secret (Tier 1)
- **Default payment provider**: Supported providers are M-Pesa, Stripe, Paystack, Flutterwave, Manual. For Paystack as default, configure treasury-api with Paystack keys and set default provider (e.g. tenant config or `CreatePaymentIntent` request with `Provider: paystack`) when creating intents.

### Logistics Service

**Delivery pricing (2026-10-10).** Logistics-api is the only source of delivery areas, geofencing and delivery fees. Ordering calls `POST /api/v1/s2s/zones/{tenant}/quote` (X-API-Key) from `internal/platform/logistics/quote.go`, cached in Redis per tenant, outlet and pin (about 11 m) for two minutes, and `GET /api/v1/s2s/zones/{tenant}/coverage` for outlet listings. The storefront calls logistics directly for the public coverage, quote preview and place search (`/zones/coverage`, `/zones/quote`, `/routing/geocode/search|reverse`). Ordering never computes distances or point-in-zone checks itself.


**Integration Type**: REST API + Events (NATS) + WebSockets/SSE

**Use Cases**:
- Delivery task creation
- Rider assignment (query riders from logistics-service)
- Task status updates
- Live driver tracking
- Proof of delivery handling
- Rider onboarding and management

**CRITICAL - Entity Ownership**: 
- **All rider, driver, fleet, delivery task, shift, telemetry, and proof-of-delivery data is owned by `logistics-service`**
- Ordering-backend stores **ONLY** `rider_id` references in `order_assignments` table
- **DO NOT** store rider profiles, fleet data, or delivery task details in ordering-backend
- **DO NOT** use the legacy `riderprofile` and `riderdocument` Ent schemas (they exist but are unused)
- All rider/fleet queries must go to logistics-service APIs: `GET /v1/{tenant}/fleet-members`, `GET /v1/{tenant}/tasks`

**Rider User Management**:
- **Rider Data Storage**: All rider user data (profiles, documents, KYC, vehicle info, shifts, earnings) is stored in `logistics-service`
- **Rider Creation Flow**:
  1. **Tenant Service Availability Check**: Before creating a rider, verify tenant exists in logistics-service and has logistics service enabled in their subscription plan
     - Check: `GET /v1/{tenant}/status` or `GET /api/v1/tenants/{tenant_id}/services` (from auth-service or subscription service)
     - If tenant doesn't have logistics service enabled, show error: "Logistics service not available for this tenant. Please upgrade your plan or contact support."
  2. **Rider Creation Options**:
     - **Option A - API Push**: If tenant has logistics service enabled, ordering-backend can push rider creation to logistics-service:
       - `POST /v1/{tenant}/fleet-members` with rider details
       - Logistics-service creates rider user in auth-service (if not exists) and stores rider profile
       - Returns `rider_id` which ordering-backend stores as reference in `order_assignments`
     - **Option B - UI Redirect**: Redirect user to logistics-service UI for self-onboarding:
       - Redirect to: `https://logistics.codevertexafrica.com/{tenant_slug}/riders/onboard?return_url={cafe_url}`
       - User authenticates with existing auth-service credentials (SSO)
       - User completes rider onboarding in logistics-service UI
       - Logistics-service redirects back to cafe service with `rider_id` in query params
  3. **Rider Authentication**: Riders authenticate via auth-service (same SSO as other users)
     - Rider user created in auth-service with role `rider` and tenant membership
     - Logistics-service stores rider-specific data (vehicle, documents, KYC) locally
  4. **Standalone Logistics Service**: If a tenant only uses logistics-service (no cafe service):
     - Rider onboarding happens directly in logistics-service UI
     - All rider management (profile, documents, shifts, earnings) in logistics-service
     - No ordering-backend involvement needed

**REST API Usage**:
- `GET /v1/{tenant}/status` - Check if tenant has logistics service enabled
- `POST /v1/{tenant}/fleet-members` - Create rider (requires tenant service availability check)
- `GET /v1/{tenant}/fleet-members` - Query riders (for assignment)
- `GET /v1/{tenant}/fleet-members/{id}` - Get rider details
- `POST /v1/{tenant}/tasks` - Create delivery task
- `GET /v1/{tenant}/tasks/{id}` - Get task details

**Events Consumed**:
- `logistics.task.assigned` - Update order with rider assignment
- `logistics.task.accepted` - Rider accepted task
- `logistics.task.en_route` - Update order status to "en route"
- `logistics.task.completed` - Mark order as delivered
- `logistics.task.cancelled` - Handle task cancellation
- `logistics.task.unassigned` - Rider declined or was taken off before pickup; order flagged `needs_rider`
- `logistics.route.updated` - Update ETA
- `logistics.rider.created` - Rider created (if using API push)
- `logistics.rider.onboarded` - Rider onboarding completed

**Events Published**:
- `cafe.order.ready` - Order ready for delivery (triggers task creation)

**WebSocket/SSE**:
- Connect to logistics-service streams for live driver location
- Real-time ETA updates

**Configuration**:
- Logistics service base URL: `LOGISTICS_SERVICE_BASE_URL` (environment variable)
- WebSocket URL: `LOGISTICS_WS_URL` (environment variable)
- Logistics service UI URL: `LOGISTICS_UI_URL` (for redirects)

**Data Ownership**:
- Ordering-backend stores only `rider_id` reference in `order_assignments` table
- No rider profiles, fleet data, or delivery task details stored locally
- All rider queries go to logistics-service APIs

**Tenant Service Availability Pattern**:
This pattern applies to ALL services (logistics, inventory, POS, notifications, treasury):
1. Before creating/referencing data in another service, check if tenant has that service enabled
2. Check tenant subscription plan features or service availability via auth-service/subscription service
3. If service not available: show error message or redirect to upgrade plan
4. If service available: proceed with API push or UI redirect based on user flow

### Inventory Service

**Integration Type**: REST API + Events (NATS)

**Use Cases**:
- Stock availability queries
- Stock reservation for orders
- Recipe consumption tracking
- Low-stock alerts

**REST API Usage**:
- `GET /api/v1/inventory/items/{sku}` - Get stock availability
- `POST /api/v1/inventory/reservations` - Reserve stock for order
- `GET /api/v1/inventory/recipes/{id}` - Get recipe details

**Events Consumed**:
- `inventory.stock.updated` - Update menu item availability
- `inventory.stock.low` - Handle low-stock alerts
- `inventory.reservation.confirmed` - Confirm stock reservation

**Events Published**:
- `cafe.order.placed` - Trigger stock reservation
- `cafe.order.cancelled` - Release stock reservation

**Configuration**:
- Inventory service base URL: `INVENTORY_SERVICE_BASE_URL` (environment variable)

**Data Ownership**:
- Ordering-backend references inventory SKUs in `menu_items` table
- No inventory balances or stock data stored locally

### POS Service

**Integration Type**: Events (NATS) + REST (S2S for order detail fetch)

**Scope Clarification**:
- **POS Service Handles**: Walk-in orders, dine-in orders, kitchen display (KDS), cash management, table management, hotel folios
- **Ordering Service Handles**: Online orders (delivery and pickup) placed by customers via the ordering app or third-party channels (Uber Eats, Glovo)
- **Integration Points**: (1) Online dine-in / pickup orders → POS KDS, (2) Catalog changes → ordering storefront sync

---

#### KDS Integration (Hospitality — Critical Gap)

When a hospitality order (dine-in or pickup) placed via ordering-backend transitions to `confirmed` or `preparing`, pos-api must create KDS tickets so the kitchen sees the order without manual re-entry by a waiter.

**Current state:** ordering-backend publishes `ordering.order.status.changed` on every status transition but pos-api does NOT yet subscribe. This means online hospitality orders are invisible to the KDS. See [pos-api Sprint 13](../../../pos-service/pos-api/docs/sprints/sprint-13-ordering-kds-integration.md).

**Required from ordering-backend (already implemented):**
- `ordering.order.status.changed` NATS event published on every status transition
- Event payload MUST include `fulfillment_type` and, for dine-in, `table_reference`
- Event payload SHOULD include the full `items` array (SKU, name, quantity, notes, modifiers) so pos-api does not need a follow-up REST call

**Recommended enhancement:** Ensure the `ordering.order.status.changed` event payload is enriched with full item details when `new_status IN (confirmed, preparing)` — this avoids a synchronous REST call from pos-api back to ordering-backend.

**If item enrichment is not added to the event**, pos-api will call:
- `GET /api/v1/{tenant}/orders/{order_id}` with `X-API-Key: {INTERNAL_SERVICE_KEY}` (S2S)
- This endpoint must be accessible without a user Bearer token; add it under the ordering-backend S2S routes

**Events Published (relevant to POS)**:
- `ordering.order.status.changed` — pos-api subscribes (filter: `confirmed`, `preparing`, dine_in/pickup)
- `ordering.order.for_pickup` — pos-api consumes for pickup order handoff (existing)
- `ordering.catalog.updated` — pos-api consumes to refresh catalog projection (existing)

**Events Consumed from POS**:
- `pos.kds.order.ready` — (planned, Phase 2) pos-api publishes when all KDS tickets for an order are marked ready; ordering-backend may auto-transition order to `ready`
- `pos.pickup.completed` — customer picked up order; close the ordering record

**S2S API Key**:
- ordering-backend accepts `X-API-Key: {INTERNAL_SERVICE_KEY}` on S2S order-detail routes (same shared platform key)
- pos-api env var: `ORDERING_SERVICE_URL`, `INTERNAL_SERVICE_KEY`

**Note**: Cash drawers, table management, and hotel folios are NOT in ordering-backend scope. Those belong entirely to pos-api.

---

#### Storefront promotions/reports S2S proxies (public, read-only)

ordering-backend never owns discount or sales data — it thin-proxies pos-api's own S2S endpoints
so the public storefront homepage/catalog can render Flash Sales, Top Deals, and a Brands row
without a browser ever calling pos-api directly.

| ordering-backend route (public, no auth) | Proxies | Client | Used for |
|---|---|---|---|
| `GET /{tenant}/promotions/banners?use_case=` | `GET /api/v1/s2s/{tenant}/discounts/banners` | `internal/platform/posdiscounts` | Hero marketing banner carousel |
| `GET /{tenant}/promotions/deals` | `GET /api/v1/s2s/{tenant}/discounts?status=active` | `internal/platform/posdiscounts` | Flash Sales rail (discount-driven; falls back to any item with a discount price set when nothing is flagged `is_flash_sale`) |
| `GET /{tenant}/promotions/top-sellers` | `GET /api/v1/s2s/{tenant}/pos/sales/by-sku?from=&to=` | `internal/platform/posreports` (new, 2026-09-14) | Top Deals rail (real best-sellers by units sold, trailing 90 days — inventory-api's menu-engineering/variance reports already consume the same pos-api endpoint the same way) |

All three: `X-API-Key: {INTERNAL_SERVICE_KEY}` header, best-effort (return an empty array on any
pos-api failure — a promotions/reporting hiccup must never break the storefront homepage), and
cached 2 minutes via `internal/platform/cache` (`promobanner.Handler`, one struct backing all
three routes). `GET /{tenant}/catalog/brands` (`ProxyService.ListBrands`) is the equivalent proxy
over inventory-api's `ItemBrand` master data, for the Top Brands row — no pos-api involved there.

---

## External Third-Party Integrations

### M-Pesa (via Treasury App)

**Purpose**: Mobile money payments (STK Push, C2B, B2C)

**Configuration** (Tier 1 - Developer Only):
- Consumer Key: Stored encrypted at rest
- Consumer Secret: Stored encrypted at rest
- Passkey: Stored encrypted at rest
- Short Code: Configured per tenant (Tier 2)

**Flow**:
1. Customer initiates payment
2. Ordering backend creates payment intent via treasury-api
3. Treasury-api initiates M-Pesa STK Push
4. Customer confirms payment
5. Treasury-api sends webhook to ordering backend
6. Ordering backend updates order payment status

**Integration**: Handled via treasury-api, not directly

### Self-Hosted Routing (Valhalla + TileServer-GL)

**Purpose**: Geocoding, distance matrix, route ETA, map tile rendering

**Architecture**: All mapping and routing is handled by the self-hosted stack, replacing Google Maps / Mapbox:
- **Routing engine**: Valhalla at `https://routing.codevertexafrica.com` (internal: `http://valhalla.logistics.svc.cluster.local:8002`)
- **Map tiles**: TileServer-GL at `https://tiles.codevertexafrica.com` (internal: `http://tileserver.logistics.svc.cluster.local:8080`)
- **Frontend library**: `@bengo-hub/maps` (MapLibre GL JS wrapper, shared NPM package)
- **Data source**: OpenStreetMap Kenya extract from Geofabrik, auto-refreshed weekly

**Configuration**:
- No API keys required (self-hosted, no per-request charges)
- Routing accessed via logistics-api wrapper: `GET /api/v1/{tenant}/routing/*`
- logistics-api adds Redis caching + rate limiting on top of Valhalla

**Use Cases**:
- Address geocoding (via Valhalla + PostGIS)
- Delivery distance calculation
- Route optimization
- ETA estimation
- Distance matrix (batch)
- Isochrone (reachability polygons)

### S3-Compatible Storage

**Purpose**: Media asset storage (menu images, receipts, documents)

**Configuration** (Tier 1):
- Access Key ID: Stored encrypted
- Secret Access Key: Stored encrypted
- Bucket Name: Configured per tenant (Tier 2)
- Region: Environment variable

**Use Cases**:
- Menu item images
- Receipt storage
- Document attachments
- Brand assets

---

## Integration Patterns

### 1. REST API Pattern (Synchronous)

**Use Case**: Immediate data retrieval, payment processing

**Implementation**:
- HTTP client with retry logic
- Circuit breaker pattern
- Request timeout (5 seconds default)
- Idempotency keys for mutations

### 2. Event-Driven Pattern (Asynchronous)

**Use Case**: Order status updates, notifications, inventory changes

**Transport**: NATS JetStream

**Flow**:
1. Service publishes event to NATS
2. Subscriber services consume event
3. Process event and update local state
4. Publish response events if needed

**Reliability**:
- At-least-once delivery
- Event deduplication via event_id
- Retry on failure
- Dead letter queue for failed events

### 3. Webhook Pattern (Callbacks)

**Use Case**: Payment status, delivery updates, settlement notifications

**Implementation**:
- Webhook endpoints in ordering-backend
- Signature verification (HMAC-SHA256)
- Retry logic for failed deliveries
- Idempotency handling

### 4. WebSocket/SSE Pattern (Real-Time)

**Use Case**: Live driver tracking, order status updates

**Implementation**:
- WebSocket connection to logistics-service
- Server-Sent Events for order updates
- Automatic reconnection on failure
- Message queuing for offline clients

---

## Two-Tier Configuration Management

### Tier 1: Developer/Superuser Configuration

**Visibility**: Only developers and superusers

**Configuration Items**:
- API keys and secrets (treasury, notifications, logistics, inventory, POS)
- OAuth client secrets
- Database credentials
- Encryption keys
- Webhook signing secrets
- S3 access keys

**Storage**:
- Encrypted at rest in database (AES-256-GCM)
- K8s secrets for runtime
- Vault for production secrets

**Management**:
- Admin API endpoints (superuser only)
- Key rotation every 90 days

### Tier 2: Business User Configuration

**Visibility**: Normal system users (tenant admins)

**Configuration Items**:
- M-Pesa short code
- S3 bucket name
- Notification preferences
- Feature toggles
- Brand settings (colors, logos)
- Webhook URLs (non-sensitive)

**Storage**:
- Plain text in database (non-sensitive)
- Tenant-specific configuration tables

**Management**:
- Self-service API endpoints
- Tenant admin UI

---

## Event-Driven Architecture

### Event Catalog

#### Outbound Events (Published by Ordering-Backend)

**cafe.order.created**
```json
{
  "event_id": "uuid",
  "event_type": "cafe.order.created",
  "tenant_id": "tenant-uuid",
  "timestamp": "2024-12-05T10:30:00Z",
  "data": {
    "order_id": "order-uuid",
    "customer_id": "user-uuid",
    "cafe_id": "cafe-uuid",
    "total_amount": 1500.00,
    "currency": "KES"
  }
}
```

**cafe.order.ready**
```json
{
  "event_id": "uuid",
  "event_type": "cafe.order.ready",
  "tenant_id": "tenant-uuid",
  "timestamp": "2024-12-05T10:30:00Z",
  "data": {
    "order_id": "order-uuid",
    "cafe_id": "cafe-uuid",
    "delivery_address": {...}
  }
}
```

**cafe.loyalty.points_awarded**
```json
{
  "event_id": "uuid",
  "event_type": "cafe.loyalty.points_awarded",
  "tenant_id": "tenant-uuid",
  "timestamp": "2024-12-05T10:30:00Z",
  "data": {
    "user_id": "user-uuid",
    "order_id": "order-uuid",
    "points": 150
  }
}
```

#### Inbound Events (Consumed by Ordering-Backend)

**logistics.task.assigned**
```json
{
  "event_id": "uuid",
  "event_type": "logistics.task.assigned",
  "tenant_id": "tenant-uuid",
  "timestamp": "2024-12-05T10:30:00Z",
  "data": {
    "task_id": "task-uuid",
    "order_id": "order-uuid",
    "rider_id": "rider-uuid"
  }
}
```

**treasury.payment.success**
```json
{
  "event_id": "uuid",
  "event_type": "treasury.payment.success",
  "tenant_id": "tenant-uuid",
  "timestamp": "2024-12-05T10:30:00Z",
  "data": {
    "payment_id": "payment-uuid",
    "order_id": "order-uuid",
    "amount": 1500.00,
    "provider_reference": "mpesa-ref-123"
  }
}
```

---

## Integration Security

### Authentication

**JWT Tokens**:
- Validated via `shared/auth-client` library
- JWKS from auth-service
- Token claims include tenant_id for scoping

**API Keys** (Service-to-Service):
- Stored in K8s secrets
- Rotated quarterly
- Per-tenant API keys for external integrations

### Authorization

**Tenant Isolation**:
- All requests scoped by tenant_id
- Provider credentials isolated per tenant
- Data isolation enforced at database level

**RBAC**:
- Service-level roles (admin, operator, viewer)
- Tenant admin roles
- Fine-grained permissions per operation

### Secrets Management

**Encryption**:
- Secrets encrypted at rest (AES-256-GCM)
- Decrypted only when used
- Key rotation every 90 days

**Access Control**:
- Tier 1 secrets: Superuser only
- Tier 2 configuration: Tenant admin access
- Audit logging for all secret access

### Webhook Security

**Signature Verification**:
- HMAC-SHA256 signatures
- Secret shared via K8s secret
- Timestamp validation (5-minute window)
- Nonce validation (prevent replay attacks)

---

## Error Handling & Resilience

### Retry Policies

**Exponential Backoff**:
- Initial delay: 1 second
- Max delay: 30 seconds
- Max retries: 3

**Circuit Breaker**:
- Opens after 5 consecutive failures
- Half-open after 60 seconds
- Closes on successful request

### Fallback Strategies

**Service Unavailable**:
- Return 503 Service Unavailable
- Log error for monitoring
- Alert operations team
- Queue requests for retry

**Event Delivery Failure**:
- Retry with exponential backoff
- Dead letter queue after max retries
- Manual reconciliation interface

### Status transitions and duplicate events

Every status change (admin, POS S2S, pos.* and logistics.task.* consumers) goes through
`OrderService.UpdateOrderStatus`, and cancellations through `CancelOrder`. Both decide the
transition's effects first (`planTransition`: COD paid flag and treasury settlement, loyalty and
stock consumption, prepaid refund, reservation release), write the status with a compare-and-set
on the previous status, and run the effects only when that write won. Logistics publishes both
`task.delivered` and `task.completed` for one drop-off, and JetStream redelivers; only one handler
ever finalizes the order. A rejection through `PUT /admin/orders/{id}/status` with `cancelled` is
handled by `CancelOrder`, so it also publishes `ordering.order.cancelled` and returns redeemed points.

Rules enforced on the API:
- Customers can cancel their own order only while it is `pending` or `confirmed` (409 after the
  kitchen starts).
- `DELETE /admin/orders/{id}` works only for finished orders and pending orders never shown to the
  outlet (409 otherwise; reject the order instead).

### Logistics calls and task events (2026-10-06)

| Need | How |
|---|---|
| Create a task (manual "arrange delivery", rider assign fallback) | `POST /api/v1/s2s/dispatch/{tenant_uuid}/tasks` with `X-API-Key: INTERNAL_SERVICE_KEY` |
| Assign a rider | `POST /api/v1/s2s/dispatch/{tenant_uuid}/tasks/{id}/assign` (service key) |
| Live tracking for the customer | `GET /api/v1/s2s/dispatch/{tenant_uuid}/tasks/{id}/tracking` (service key). Returns status, `rider_location`, `rider_name`, `rider_phone`, `eta_minutes`, `eta_at`, `distance_km`. Rider details are only filled while a rider works the task. |
| Cancel a task | Not called. logistics-api closes the order's task itself when it consumes `ordering.order.cancelled`; dispatchers cancel from the logistics board. The old `POST /orders/{id}/delivery/cancel-task` route was removed. |

`GET /orders/{id}/delivery/tracking` and the `rider_location` event of the order SSE stream
(`/orders/{id}/track`, every 30 s while out for delivery) carry the rider's position, name, phone,
ETA and distance. ETA and distance are left out until logistics has computed them. When logistics
does not answer, the stored assignment is returned instead.

Every `logistics.task.*` consumer skips (acknowledges) events whose `source_service` is set and is
not `ordering`. POS till deliveries publish `source_service: pos` with a bare POS order id as the
reference; before this they failed, redelivered and left stray assignment rows.

`logistics.task.unassigned` (durable `ord-logistics-task-unassigned`) arrives when a rider declines
before pickup or a dispatcher takes the job back. The assignment returns to `pending` with no rider
and `metadata.needs_rider`, and the order's metadata gets `delivery_status: needs_rider` with
`rider_id`, `rider_name` and `rider_phone` cleared. Both writes are conditional: a newer rider who
was already stamped is kept, an order that is out for delivery or finished is left alone, and a
replayed event writes nothing. The next `task.assigned` fills the rider in again.

`ordering.order.ready` always carries `outlet_location` with the outlet's name, address and phone;
`latitude` and `longitude` are added only when the outlet has both. Before, an outlet without
coordinates sent no pickup details at all.

### Online payment retry window (2026-10-06)

An online-payment order (M-Pesa STK, PayHero, Paystack) whose payment is declined, cancelled or
times out is not cancelled straight away. It stays `pending` with its stock held until its retry
window closes, and the customer can pay again from the order page.

- Window: tenant service config `orders.payment_retry_window_minutes` (default 30, clamped to
  5 minutes .. 24 hours), counted from `placed_at`. Checkout stamps the deadline on the order as
  `metadata.payment_retry_until`, so the poller never reads config per order. Orders without the
  stamp use `placed_at` + 30 minutes.
- Each failed, cancelled or expired attempt is recorded once per intent: `payment_attempts`,
  `payment_last_failure_reason`, `payment_last_attempt_at`, `payment_last_failed_intent`.
- Every intent created for the order is listed in `metadata.payment_intent_ids`; the current one
  is also written to the `payment_intent_id` column (it was never persisted before, so the poller
  could not check treasury and timed orders out on age alone, and staff payment prompts always
  answered "order has no payment intent").
- Order reads carry `paymentRetry` (`open`, `until`, `attempts`, `retries`, `lastFailureReason`,
  `lastAttemptAt`) while an online-payment order is unpaid. Admin list rows carry it too.

Retry endpoints: `POST /{tenant}/orders/{orderId}/payment/retry` (the order's customer, or staff
with orders.manage) and `POST /{tenant}/orders/guest/{orderId}/payment/retry` (public order page;
the order id is the capability, a `session_id` query must match when sent). The current intent is
checked first: succeeded confirms the order through the normal paid path and answers 409; still
processing answers 409 (a prompt is open); pending (never completed) is returned again; failed,
cancelled or expired is recorded and a fresh intent is created with its own reference
(`payref.BuildAttempt`, `ORD-...-R2`), because treasury returns the existing intent for a reused
reference and refuses to initiate a closed one. A retry slot is claimed in one compare-and-set
metadata write: at most 5 retries per order, 20 seconds apart (429), only while the window is open
(410). The response is `{paymentIntentId, initiateUrl, amount, currency, retryUntil, reused}`, the
same intent and initiate URL checkout returns, and the storefront opens the shared treasury payment
modal with it.

Payment poller (every 2 minutes, one replica per period via `ClaimPeriod`): keyset pages of 50 on
`(placed_at, id)` over the partial index `order_stale_payment_placed_at`, at most 10 pages per
sweep. For each order it checks every intent (current first, at most 6). A success on any intent,
including an earlier attempt, confirms the order. A failed current intent is recorded. Only when the
window has closed is the order cancelled (`CancelOrder`: stock hold released with tenant context,
`ordering.order.cancelled` tells the customer), and never while a status check failed or while the
last prompt is still processing within 10 minutes past the deadline.

Late and duplicate successes: the treasury consumer resolves the order from `entity_id`, so a
success on an older intent confirms the order exactly once (`UpdatePaymentStatus` compare-and-set).
The paying intent is stamped (`payment_paid_intent_id`, and `payment_intent_id` for refunds). A
second success after the order is already paid is listed in `payment_extra_paid_intents` and logged
for a refund. A success after the window closed and the order was cancelled is not fulfilled: the
order stays cancelled, payment_status becomes paid and a "needs reconciliation" warning is logged,
the existing late-payment handling; the refund is manual.

### Background jobs and reporting queries

| Job / query | Guard | Bounded by |
|---|---|---|
| Scheduled hand-off (1 min) | `ClaimPeriod` | partial index `order_scheduled_handoff_due`, handed-off orders filtered in SQL, batch 200 |
| Payment poller (2 min) | `ClaimPeriod` | partial index `order_stale_payment_placed_at`, keyset pages of 50, at most 10 pages per sweep; cancels only after the retry window; manual M-Pesa orders excluded (the outlet confirms them) |
| `GET /admin/orders/summary` | per request | SQL `GROUP BY` (status and currency, day, item); revenue counts paid orders that were not cancelled, refunded or timed out |
| `GET /admin/orders/counts` | per request | `GROUP BY status` over open statuses, served by the tenant and status indexes |
| Order lists | per request | page size from the shared pagination lib; items, delivery address and customer loaded per page, not per order |

### Monitoring

**Metrics**:
- API call latency (p50, p95, p99)
- API call success/failure rates
- Event publishing success rates
- Webhook delivery success rates

**Alerts**:
- High failure rate (>5%)
- Service unavailability
- Event delivery failures
- Rate limit exceeded

---

## References

- [Cross-Service Data Ownership](../../../shared-docs/CROSS-SERVICE-DATA-OWNERSHIP.md) — Canonical data ownership and reference-only pattern across services
- [Auth Service Integration](../auth-service/auth-service/docs/integrations.md)
- [Treasury App Integration](../finance-service/treasury-api/docs/integrations.md)
- [Logistics Service Integration](../logistics-service/logistics-api/docs/integrations.md)
- [Notifications Service Integration](../notifications-service/notifications-api/docs/integrations.md)

