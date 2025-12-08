# Ordering Backend - Integration Guide

## Overview

This document provides detailed integration information for all external services and systems integrated with the Ordering backend, including internal BengoBox microservices and external third-party services.

---

## Table of Contents

1. [Internal BengoBox Service Integrations](#internal-bengobox-service-integrations)
2. [External Third-Party Integrations](#external-third-party-integrations)
3. [Integration Patterns](#integration-patterns)
4. [Two-Tier Configuration Management](#two-tier-configuration-management)
5. [Event-Driven Architecture](#event-driven-architecture)
6. [Integration Security](#integration-security)
7. [Error Handling & Resilience](#error-handling--resilience)

---

## Internal BengoBox Service Integrations

### Auth Service

**Integration Type**: OAuth2/OIDC + Events + REST + Service-to-Service Auth

**Production URL**: `https://sso.codevertexitsolutions.com/`

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
  - Login: `POST https://sso.codevertexitsolutions.com/api/v1/auth/login` with `{email, password, tenant_slug}`
  - Registration: `POST https://sso.codevertexitsolutions.com/api/v1/auth/register` with `{email, password, tenant_slug, profile}`
  - Returns: `{access_token, refresh_token, session_id, tenant, user}` from auth-service
- **JWT Validation**: Uses `shared/auth-client` library for token validation
  - JWKS endpoint: `https://sso.codevertexitsolutions.com/api/v1/.well-known/jwks.json`
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
- Auth-service base URL: `AUTH_SERVICE_URL=https://sso.codevertexitsolutions.com` (environment variable)
- JWKS endpoint: `https://sso.codevertexitsolutions.com/api/v1/.well-known/jwks.json`
- Issuer: `https://auth.bengobox.local` (from JWT claims, may need update)
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
  - **On Google OAuth**: `cafe-backend` fetches Google profile -> calls `auth-service` SyncUser (`POST /api/v1/users/sync`) -> updates local user with returned `auth_service_user_id`.

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
- `POST /api/v1/payments/intents` - Create payment intent
- `POST /api/v1/payments/confirm` - Confirm payment
- `POST /api/v1/payments/refund` - Process refund
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

### Logistics Service

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
- Cafe backend stores **ONLY** `rider_id` references in `order_assignments` table
- **DO NOT** store rider profiles, fleet data, or delivery task details in cafe-backend
- **DO NOT** use the legacy `riderprofile` and `riderdocument` Ent schemas (they exist but are unused)
- All rider/fleet queries must go to logistics-service APIs: `GET /v1/{tenant}/fleet-members`, `GET /v1/{tenant}/tasks`

**Rider User Management**:
- **Rider Data Storage**: All rider user data (profiles, documents, KYC, vehicle info, shifts, earnings) is stored in `logistics-service`
- **Rider Creation Flow**:
  1. **Tenant Service Availability Check**: Before creating a rider, verify tenant exists in logistics-service and has logistics service enabled in their subscription plan
     - Check: `GET /v1/{tenant}/status` or `GET /api/v1/tenants/{tenant_id}/services` (from auth-service or subscription service)
     - If tenant doesn't have logistics service enabled, show error: "Logistics service not available for this tenant. Please upgrade your plan or contact support."
  2. **Rider Creation Options**:
     - **Option A - API Push**: If tenant has logistics service enabled, cafe backend can push rider creation to logistics-service:
       - `POST /v1/{tenant}/fleet-members` with rider details
       - Logistics-service creates rider user in auth-service (if not exists) and stores rider profile
       - Returns `rider_id` which cafe backend stores as reference in `order_assignments`
     - **Option B - UI Redirect**: Redirect user to logistics-service UI for self-onboarding:
       - Redirect to: `https://logistics.codevertexitsolutions.com/{tenant_slug}/riders/onboard?return_url={cafe_url}`
       - User authenticates with existing auth-service credentials (SSO)
       - User completes rider onboarding in logistics-service UI
       - Logistics-service redirects back to cafe service with `rider_id` in query params
  3. **Rider Authentication**: Riders authenticate via auth-service (same SSO as other users)
     - Rider user created in auth-service with role `rider` and tenant membership
     - Logistics-service stores rider-specific data (vehicle, documents, KYC) locally
  4. **Standalone Logistics Service**: If a tenant only uses logistics-service (no cafe service):
     - Rider onboarding happens directly in logistics-service UI
     - All rider management (profile, documents, shifts, earnings) in logistics-service
     - No cafe-backend involvement needed

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
- Cafe backend stores only `rider_id` reference in `order_assignments` table
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
- Cafe backend references inventory SKUs in `menu_items` table
- No inventory balances or stock data stored locally

### POS Service

**Integration Type**: Events (NATS) - Minimal integration

**Scope Clarification**: 
- **POS Service Handles**: Over-the-counter orders (walk-in customers), pickup orders, dine-in orders, kitchen tickets, cash management
- **Ordering Service Handles**: Online delivery orders only
- **Integration Point**: Customer places online order for pickup → order transitions from ordering-service to POS-service for fulfillment

**Use Cases for Ordering Service**:
- Catalog sync (catalog changes published, POS subscribes)
- Online-for-pickup orders (created in ordering-service, fulfilled by POS-service)

**Events Published by Ordering Service**:
- `ordering.catalog.updated` - Notify POS of menu changes (for catalog consistency)
- `ordering.order.for_pickup` - Online order placed for pickup (POS takes over fulfillment)

**Events Consumed from POS**:
- `pos.order.ready` - Pickup order ready for customer (if initiated from ordering)
- `pos.pickup.completed` - Customer picked up order (close ordering record)

**Note**: Direct POS operations (cash drawers, kitchen tickets, table management) are NOT in ordering-service scope. Those belong entirely to POS-service.

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
2. Cafe backend creates payment intent via treasury-app
3. Treasury-app initiates M-Pesa STK Push
4. Customer confirms payment
5. Treasury-app sends webhook to cafe backend
6. Cafe backend updates order payment status

**Integration**: Handled via treasury-app, not directly

### Mapbox / Google Maps

**Purpose**: Geocoding, distance matrix, route ETA

**Configuration** (Tier 1):
- API Key: Stored encrypted at rest

**Use Cases**:
- Address geocoding
- Delivery distance calculation
- Route optimization
- ETA estimation

**API Usage**:
- Geocoding: Convert address to coordinates
- Distance Matrix: Calculate delivery distance
- Directions: Get route for delivery

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
- Webhook endpoints in cafe backend
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

#### Outbound Events (Published by Cafe Backend)

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

#### Inbound Events (Consumed by Cafe Backend)

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

- [Cross-Service Data Ownership](../CROSS-SERVICE-DATA-OWNERSHIP.md) - Architecture pattern for data ownership and user management across services
- [Auth Service Integration](../auth-service/auth-service/docs/integrations.md)
- [Treasury App Integration](../finance-service/treasury-app/docs/integrations.md)
- [Logistics Service Integration](../logistics-service/logistics-api/docs/integrations.md)
- [Notifications Service Integration](../notifications-service/notifications-api/docs/integrations.md)

