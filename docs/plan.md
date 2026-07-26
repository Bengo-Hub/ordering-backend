# Ordering Service - Implementation Plan
**Overall Backend Progress: ~95% Complete**

**March 20 update (late)**: Delivery zones implemented — `DeliveryZone` Ent schema with zone_polygon (GeoJSON), delivery_fee, minimum_order, estimated_time_minutes. CRUD endpoints at `/zones` with `zones:manage` and `zones:read` permissions. Atlas migration generated. Treasury client aligned with `source_service`, `reference_id`, service charge fields.

**March 20 update**: CatalogCategory schema updated with `name`, `slug`, `description`, `image_url` fields. Atlas migration generated (`20260320023309_add_category_name_description_slug_imageurl.sql`). Public menu API response hydration fixed — `toPublicCatalogItem()` now populates `description`, `imageUrl`, `categoryName`, `currency`. `toPublicCategory()` now returns `name`, `description`, `imageUrl`. `ListOutlets()` queries the actual `outlets` table instead of deriving from category outlet_ids. Catalog item create/update endpoints now accept `name`, `description`, `basePrice`, `currency`, `imageUrl` directly (inventoryItemId made optional). Seed script updated to create outlet (Urban Loft Cafe Busia), 7 categories, and 30 menu items with KES pricing. Cafe-website API client aligned to camelCase response format.

**March 6 update**: Seed creates default tenant by slug (urban-loft) with **DB-generated UUID** (no fixed tenant ID). Public menu routes resolve tenant by slug when X-Tenant-ID is not a valid UUID. Inventory client (`internal/platform/inventory/client.go`) already wired for stock check, reserve, consume, release. Run `go run ./cmd/seed/` after DB is up to populate menu data. **RBAC**: Own roles/permissions in DB (ent); auth via auth-api JWT; user sync from auth-service via NATS. Seed includes CRUD-style permissions (add, read, read_own, change, change_own, delete, manage, manage_own) for orders and catalog. **Redis**: Used for rate limiting and auth config; Redis cache for permissions recommended for high traffic. **Events**: Outbox publisher and NATS used for order events and auth sync.

## Executive Summary

**System Purpose**: Multi-business online ordering platform for **DELIVERY/SHIPPING ORDERS ONLY**. Supports various business types (food delivery, grocery, retail, pharmacy, e-commerce) with configurable workflows, flexible catalog management, and seamless integration with logistics, treasury, inventory, and auth services.

**Key Capabilities**:
- Multi-business type support (food, retail, grocery, pharmacy, flowers, electronics, etc.)
- Online order placement and tracking (delivery to customer location)
- Flexible catalog with item modifiers and variants (size, dosage, color, pack size)
- Multiple delivery options (ASAP, scheduled, dropoff methods, contact preferences)
- Group ordering (office lunches, shared carts, split payments)
- Zone-based delivery availability with geocoding
- Payment processing integration (M-Pesa, cards, wallets, cash on delivery)
- Real-time order tracking and delivery notifications
  - **Live Tracking**: Integrates with `logistics-service` to pull real-time rider coordinates when an order is in the delivery stage.
- Multi-tenant with outlet-specific catalogs and pricing
- Promo codes and loyalty rewards

**Entity Ownership**: This service owns **online delivery/shipping orders ONLY**: shopping cart, online catalog (references inventory SKUs), online orders, delivery addresses, promo codes, loyalty accounts, group orders. 

**Ordering Service does NOT own** (delegated to other services):
- ❌ **Over-the-counter orders** (walk-in customers) → **POS Service**
- ❌ **Pickup orders** (customer picks up from store) → **POS Service**
- ❌ **Dine-in orders** (table service, restaurant orders) → **POS Service**
- ❌ **Cash drawer management** → **POS Service**
- ❌ **Kitchen ticket printing** → **POS Service**
- ❌ **POS terminal operations** → **POS Service**
- ❌ **Riders/drivers/fleets**: All rider, driver, fleet, delivery task, shift, and telemetry data owned by `logistics-service`. Ordering service stores only `rider_id` references. All rider/fleet queries go to logistics-service APIs.
- ❌ **Inventory balances**: All stock data owned by `inventory-service`. Ordering service only references via `inventory_sku`. Stock checks and reservations via inventory-service APIs.
- ❌ **Payment processing**: All payment operations owned by `treasury-service`. Ordering service creates payment intents and receives confirmation webhooks.
- ❌ **Users/tenants/outlets**: All identity data owned by `auth-service`. Ordering service references via `user_id`, `tenant_id`, `outlet_id`. User data synced via events.

**Cross-Service Data Ownership Pattern**:
- Each service owns and manages all data related to its domain.
- Ordering-backend references external entities via IDs (UUIDs).
- Redundant tables (PoD, Payments, Notifications) have been removed from the schema.
- Service availability checks integrated into workflows.
- Single Source of Truth enforced per **shared-docs/CROSS-SERVICE-DATA-OWNERSHIP.md**.
---

## Technology Stack

### Core Framework
- **Language**: Go 1.22+
- **Architecture**: Clean/Hexagonal architecture
- **HTTP Router**: chi
- **API Documentation**: OpenAPI-first contracts
- **gRPC**: ConnectRPC gateway for high-throughput integrations

### Data & Caching
- **Primary Database**: PostgreSQL 16+ (pgx/v5) with Ent ORM
- **Caching**: Redis 7+ for caching and ephemeral state
- **Message Broker**: NATS JetStream or Kafka for event backbone
- **Storage**: S3-compatible storage for media assets

### Supporting Libraries
- **ORM**: Ent (schema-as-code migrations)
- **Validation**: Custom validators
- **Resilience**: Circuit breaker patterns, retry policies
- **Logging**: zap (structured logging)
- **Tracing**: OpenTelemetry instrumentation
- **Metrics**: Prometheus

### DevOps & Observability
- **Containerization**: Multi-stage Docker builds
- **Orchestration**: Kubernetes (via centralized devops-k8s)
- **CI/CD**: GitHub Actions → ArgoCD
- **Monitoring**: Prometheus + Grafana, OpenTelemetry
- **APM**: Jaeger distributed tracing

---

## Domain Modules & Features

### 1. Identity & Access Management

**Cafe-Specific Features**:
- Multi-tenant cafe support with RBAC (customer, rider, cafe staff, admin, superuser)
- Tenant-level preferences and settings
- User profile extensions (cafe-specific preferences)
- Device/session management (references auth-service)

**Entities Owned**:
- `tenants` - Cafe organization records (synced from auth-service via events)
- `tenant_settings` - Brand palette, locales, feature toggles
- `users` - Local user table with `auth_service_user_id` reference to auth-service user
  - Stores cafe-specific data: preferences, cafe roles, loyalty points, rider profiles
  - Identity data (email, phone, status) synced from auth-service
  - Sync status tracked via `sync_status` and `sync_at` fields
- `user_profiles` - Extended profile metadata (cafe-specific)
- `user_preferences` - Cafe-specific preferences (theme, language, notifications)
- `roles` - Cafe-specific role catalogue (`customer`, `rider`, `staff`, `admin`)
- `permissions` - Fine-grained permission catalogue
- `role_permissions` - Role-permission mappings
- `user_roles` - User role assignments (cafe-specific roles, merged with auth-service roles from JWT)

**Default Tenant**: The cafe service uses `urban-cafe` as the default tenant slug. All users created without a custom tenant_slug will be assigned to the `urban-cafe` tenant. The tenant is created with slug `urban-cafe` during seeding.

**Integration Points**:
- **auth-service** (Production: `https://sso.codevertexafrica.com/`):
  - **Authentication**: All login/registration requests proxy to auth-service `/api/v1/auth/login` and `/api/v1/auth/register`
  - **Default Tenant**: If `tenant_slug` is not provided in login/registration requests, defaults to `urban-cafe`
  - **JWT Validation**: Token validation via JWKS (`/api/v1/.well-known/jwks.json`) using `shared/auth-client` library
  - **User Identity Sync**: Local user table stores `auth_service_user_id` reference to auth-service user
  - **User Events**: Consume `auth.user.created`, `auth.user.updated`, `auth.user.deactivated` events to sync user data
  - **Tenant Events**: Consume `auth.tenant.created`, `auth.tenant.updated`, `auth.outlet.created` events
  - **Superuser Handling**: Superusers from auth-service bypass all RBAC/permission checks across all services
  - **MFA Enforcement**: MFA state managed by auth-service, ordering-backend respects MFA requirements
  - **Service-to-Service Auth**: API key authentication for service-to-service calls
  - **Tenant Auto-Discovery**: When a user attempts to register/login with a `tenant_slug` that doesn't exist in auth-service, the cafe service automatically pulls full tenant details from the local database and creates the tenant in auth-service with the **same UUID and slug** before proceeding. This ensures tenant IDs match across all services. This is a public operation (no authentication required) and works for all billing plans (free or paid), unlike other services that may require proper billing plans before tenant sync.
- **Tenant Sync**: Webhook events to logistics, inventory, POS, treasury, notifications services (these services may require proper billing plans before tenant sync)

### 2. Catalog & Item Management

**Multi-Business Features**:
- Flexible item types (meal, product, medication, service, subscription)
- Item variants (size, color, dosage, pack size) - configurable per business type
- Add-ons and modifiers (extra cheese for food, gift wrapping for retail, consultation for pharmacy)
- Category hierarchy (unlimited nesting)
- Pricing models (outlet-specific, time-based, volume-based)
- Multi-image support with CDN integration
- Tag system (dietary, features, occasions, compliance)
- Availability scheduling (time-based, outlet-specific, stock-based)
- Age-restricted items (pharmacy, alcohol)
- Prescription-required items (pharmacy)

**Entities Owned**:
- `cafes` - Individual outlets under a tenant
- `menu_categories` - Category hierarchy
- `menu_items` - Products available for ordering
- `menu_item_variants` - Size/flavor variants
- `menu_item_translations` - Localized copy
- `dietary_tags` - Dietary information tags
- `menu_item_dietary_tags` - Many-to-many link
- `menu_item_assets` - Additional media/CDN assets
- `menu_item_schedules` - Availability windows

**Integration Points**:
- **inventory-service**: Stock availability queries (references inventory SKUs, no duplication)
- **POS Service**: Menu sync for external POS systems

### 9. Group Orders

**Collaborative Ordering Features**:
- Group session creation with shareable links
- Multi-participant cart (no login required for participants)
- Organizer controls (edit, remove items, finalize)
- Deadline enforcement
- Split payment options (equal split, by-item, custom contributions)
- Group chat/notes

**Entities Owned**:
- `group_orders` - Group ordering sessions
- `group_participants` - Participant tracking
- `group_contributions` - Payment split records

**Use Cases**: Office lunches, party catering, household grocery pooling, event supplies

### 4. Ordering & Checkout

**Online Ordering Features** (Delivery/Shipping Only):
- Shopping cart persistence (Redis-cached, PostgreSQL fallback)
- Guest checkout support with cart merging
- Real-time stock validation via inventory-service
- Multiple delivery options:
  - ASAP delivery (ETA from logistics-service)
  - Scheduled delivery (date/time picker)
  - Delivery windows (2-hour slots for grocery)
- Contact preferences (call, text, meet at door, leave at door)
- Dropoff options (hand to me, leave at door, curbside, meet outside)
- Special delivery instructions (free-text)
- Address management with geocoding
- Zone-based delivery availability checks
- Promo code validation and redemption
- Loyalty points accrual and redemption
- Group ordering (shared cart, split payments)
- Payment method selection (M-Pesa, cards, cash on delivery, wallet)
- Order orchestration state machine (pending → confirmed → preparing → en_route → delivered)

**Entities Owned**:
- `customer_addresses` - Saved delivery addresses
- `carts` - Active shopping carts
- `cart_items` - Line items within a cart
- `orders` - Canonical order record
- `order_items` - Order line items (with snapshot)
- `order_events` - Audit events
- `order_assignments` - Rider dispatch workflow (references logistics-service)
- `delivery_windows` - Time commitments
- `promo_codes` - Promotion catalogue
- `promo_redemptions` - Historical redemptions
- `loyalty_accounts` - Customer loyalty balances
- `loyalty_transactions` - Earn/burn ledger

**Integration Points**:
- **logistics-service**: 
  - Delivery task creation: `POST /v1/{tenant}/tasks` or emit `cafe.order.ready` event
  - Rider queries: `GET /v1/{tenant}/fleet-members` (all rider data from logistics-service)
  - Task status: Subscribe to `logistics.task.*` events
  - Live tracking: WebSocket/SSE streams from logistics-service
  - **Important**: Ordering-backend stores only `rider_id` references, never rider profiles or fleet data
- **inventory-service**: Stock availability queries (references inventory SKUs, no duplication)
- **treasury-api**: Payment processing (payment intents, webhooks, refunds)

### 5. Payments & Treasury Integration

**Online Payment Features**:
- Multiple payment methods (M-Pesa, credit/debit cards, digital wallets, cash on delivery)
- Saved payment methods with tokenization
- Payment intent creation via treasury-service
- Payment confirmation webhooks
- Refund processing for cancelled/returned orders
- Split payments for group orders

**Entities Owned**:
- `payment_methods` - Tokenized payment instruments
- `payment_intents` - In-flight payment attempts
- `payments` - Finalized payment records
- `refunds` - Refund transactions
- `payouts` - Payouts to riders/cafes (references treasury-api)
- `settlements` - Periodic accounting for cafes
- `treasury_events` - Webhook ingestion for treasury systems

**Integration Points**:
- **treasury-api**: Payment processing, ledgering, invoicing, payout orchestration, financial compliance exports
- **M-Pesa**: STK Push (C2B), Express payments via treasury-api



### 6. Order Fulfilment & Logistics Integration

**Online Delivery Features**:
- Delivery task creation when order is ready
- Rider assignment tracking (references only)
- Real-time delivery tracking (WebSocket/SSE streams)
- Delivery ETA updates
- Proof of delivery handling
- Customer notifications (rider assigned, en route, arriving, delivered)

**Entities Owned**:
- `order_assignments` - Rider dispatch workflow (stores only `rider_id` reference from logistics-service)
- `delivery_windows` - ETA commitments sourced from logistics task updates

**Integration Points**:
- **logistics-service**: All rider, driver, fleet, and delivery task logic centralized here
  - Task creation: `POST /v1/{tenant}/tasks` or emit `cafe.order.ready` webhook
  - Task status: Subscribe to `logistics.task.*` events
  - Rider queries: `GET /v1/{tenant}/fleet-members`
  - Live tracking: WebSocket/SSE streams from logistics-service
  - Proof of delivery: Receive `logistics.task.completed` events

**Note**: Ordering-backend does NOT store rider profiles, fleet data, or delivery task details. Only references.



### 7. Notifications & Customer Engagement

**Online Ordering Notifications**:
- Order status change notifications (push, SMS, email)
- Delivery tracking updates (rider assigned, en route, arriving)
- Promo code and loyalty alerts
- Marketing campaigns and offers
- Order reminders (scheduled orders)
- Feedback requests post-delivery

**Entities Owned**:
- `notification_templates` - Messaging templates (cafe-specific)
- `notification_events` - Pending notifications for async dispatch
- `notification_subscriptions` - Opt-in/opt-out preferences

**Integration Points**:
- **notifications-api**: Channel delivery guarantees, template rendering, audit trails

### 8. Analytics & Reporting

**Operational Dashboards**:
- Order volume and trends
- Revenue analytics
- Customer behavior (repeat orders, cart abandonment)
- Delivery performance metrics
- Popular items and categories
- Promo code effectiveness

**Entities Owned**:
- `report_jobs` - Long-running analytics jobs/exports

**Integration Points**:
- **Apache Superset**: BI dashboards and analytics (see `docs/superset-integration.md`)

### 11. Support & Compliance

**Cafe-Specific Features**:
- Customer support ticketing
- GDPR/Kenya DPA data subject requests
- Activity logging
- Configurable retention policies

**Entities Owned**:
- `support_tickets` - Support case management
- `support_ticket_events` - Ticket history
- `audit_logs` - Compliance logging
- `data_subject_requests` - GDPR/DPA workflows
- `backup_jobs` - Scheduled backups
- `backup_restores` - Restore activity
- `security_policies` - Tenant-configurable security posture

---

## Cross-Cutting Concerns

### Testing
- Go test suites with table-driven tests
- Testcontainers for integration testing
- Pact for contract tests
- k6 for performance testing

### Observability
- Structured logging (zap)
- Tracing via OpenTelemetry
- Metrics exported via Prometheus
- Distributed tracing via Tempo/Jaeger

### Security
- OWASP ASVS baseline
- TLS everywhere
- Secrets via Vault/Parameter Store
- Rate limiting & anomaly detection middleware
- **Authentication**: All authentication delegated to auth-service (`https://sso.codevertexafrica.com/`)
  - Login/registration proxy to auth-service endpoints
  - JWT validation via JWKS using `shared/auth-client` library
  - Superuser detection from JWT claims with RBAC bypass
  - Service-to-service authentication via API keys

### Localization
- Tenant-aware locale headers
- Menu/catalog content translation (EN/SW)

### Scalability
- Stateless HTTP layer
- Background workers via NATS/Redis streams
- Sharded database readiness

### Brand & System Configuration
- Tenant-specific look & feel settings (colors, typography, logos)
- API credentials, webhook endpoints, feature toggles
- Shared outlet metadata with pos-service, inventory-service, logistics-service

### Data Modelling
- Ent schemas as single source of truth
- Tenant/outlet discovery webhooks
- Outbox pattern for reliable domain events

### Architecture Patterns Migration Status (January 2026)

| Pattern | Status | Library | Notes |
|---------|--------|---------|-------|
| Outbox Pattern | ✅ **Fully Implemented** | `shared-events` v0.1.0 | Schema + repository + background publisher |
| Circuit Breaker | ⏳ **Dependency Ready** | `shared-service-client` v0.1.0 | Import and use in HTTP clients |
| Shared Middleware | ✅ **Completed** | `httpware` v0.1.1 | Migrated to shared package |
| JWT Validation | ✅ Implemented | `shared-auth-client` v0.2.0 | Production |
| Subscription Feature Gating | ✅ **Partial** | `shared-auth-client` v0.2.0 | Analytics gated, group_ordering pending |
| Dual Auth (JWT + API Key) | ✅ Implemented | `shared-auth-client` v0.2.0 | SSO fully supports both |
| CORS Configuration | ✅ **Fixed** | chi/cors | Configurable origins, production defaults |
| Audit Logging | ✅ **Implemented** | `internal/modules/audit` | Mutation endpoints logged asynchronously |

**Migration Checklist:**
- [x] Add `github.com/Bengo-Hub/shared-events` dependency ✅ (Jan 2026)
- [x] Create `outbox_events` Ent schema ✅ (Jan 2026)
- [x] Create `internal/modules/outbox/repository.go` ✅ (Jan 2026)
- [ ] Replace direct NATS publish with `PublishWithOutbox`
- [x] **CRITICAL**: Add background publisher worker ✅ (Jan 2026) - `internal/platform/events/outbox_adapter.go`, wired in `app.go`
- [ ] Add `github.com/Bengo-Hub/shared-service-client` dependency
- [ ] Replace direct HTTP calls with shared client
- [x] Add `github.com/Bengo-Hub/httpware` dependency ✅ (Jan 2026)
- [x] Replace local middleware with shared package ✅ (Jan 2026)
- [x] Upgrade to `shared-auth-client` v0.2.0 ✅ (Jan 2026)
- [x] Add feature gating middleware for premium features ✅ (Jan 2026) - Analytics gated with `RequirePlan("PROFESSIONAL")`
- [ ] Use `authclient.RequireFeature()` middleware for additional subscription-gated routes (group_ordering)
- [x] **CRITICAL**: Fix CORS configuration ✅ (Jan 2026) - Configurable AllowedOrigins in config with production defaults
- [x] Wire audit logging middleware to mutation endpoints ✅ (Jan 2026) - `internal/modules/audit/` module with MutationAudit middleware

### Subscription Feature Gating (Pending auth-service Sprint 11)

Once auth-service embeds subscription data in JWT, apply feature gating:

```go
// In router.go - Gate premium features
r.Route("/group-orders", func(r chi.Router) {
    r.Use(authclient.RequireFeature("group_ordering"))
    r.Post("/", handler.CreateGroupOrder)
    r.Get("/", handler.ListGroupOrders)
})

r.Route("/analytics", func(r chi.Router) {
    r.Use(authclient.RequirePlan("PROFESSIONAL"))
    r.Get("/dashboard", handler.GetAnalyticsDashboard)
})
```

**Features to Gate:**
| Feature | Required Plan | Feature Code |
|---------|---------------|--------------|
| Group Ordering | Growth+ | `group_ordering` |
| Advanced Analytics | Professional | `advanced_analytics` |
| Scheduled Delivery | Growth+ | `scheduled_delivery` |
| Priority Support | Professional | `priority_support` |

---

## API & Protocol Strategy

- **REST-first**: Versioned routes (`/api/v1/{tenant}/orders`), documented via OpenAPI
- **gRPC**: ConnectRPC for high-throughput internal service communication
- **Webhooks**: Payment/notification callbacks, tenant/outlet discovery, logistics task status, treasury settlements
- **SSE/WebSockets**: Live tracking updates
- **Idempotency**: Keys, correlation IDs, distributed tracing context propagation

---

## Compliance & Risk Controls

- Align with Kenya Data Protection Act: explicit consent flows, user data export/delete endpoints, audit logging
- PCI scope reduction by delegating card handling to treasury providers
- Fraud prevention: velocity checks, device fingerprinting, anomaly scoring
- Disaster recovery playbook, RTO/RPO targets (<1 hour)

---

## Sprint Delivery Plan

See `docs/sprints/` folder for detailed sprint plans:
- Sprint 0: Foundation ✅ **Completed** (Foundation, database, auth integration)
- Sprint 1: Identity & Access ✅ **Completed** (Auth-service integration, RBAC, user sync)
- Sprint 2: Catalog & Localization ✅ **Completed** (Categories, items, variants, translations, dietary tags)
- Sprint 3: Orders & Cart ✅ **Completed** (Cart, checkout, orders, promo codes, loyalty, addresses)
- Sprint 4: Payments Core ✅ **Completed** (Treasury integration, payment webhooks, refunds, M-Pesa STK Push)
- Sprint 5: Order Fulfilment & Logistics ✅ **Completed** (Logistics integration, delivery tasks, webhook handlers)
- Sprint 6: Notifications & Ops ✅ **Completed** (Order status notifications, SLA monitoring, event publishing)
- Sprint 7: Analytics, Compliance & Hardening 🚧 **In Progress** (Dashboards, security audit, compliance)
- Sprint 8: Launch & Handover ⏳ **Planned** (Load testing, production deployment)

## Current Implementation Status (January 2026)

**Implemented Modules:**
- ✅ Foundation (server bootstrap, health checks, observability baseline)
- ✅ Auth integration (JWT validation via auth-service, user sync, superuser handling)
- ✅ Multi-tenancy (tenant scoping, tenant sync via events)
- ✅ RBAC (roles, permissions, cafe-specific role assignments)
- ✅ Catalog module (categories, items, variants, translations, dietary tags, schedules)
- ✅ Public menu API (locale-aware, no auth required)
- ✅ Admin catalog API (RBAC-protected CRUD operations)
- ✅ Ordering module (cart, checkout, orders, order state machine)
- ✅ Promo code engine (validation, redemption, usage limits)
- ✅ Loyalty points system (earn/redeem, tiers, transactions)
- ✅ Customer address management (CRUD, default address)
- ✅ Ordering HTTP handlers (cart, orders, promo, loyalty, addresses)
- ✅ Payments module (treasury integration, payment intents, M-Pesa STK Push)
- ✅ Payment method management (CRUD, default method)
- ✅ Webhook processing (treasury webhooks, signature verification)
- ✅ Refund processing (initiate, track, partial refunds)
- ✅ Fulfilment module (logistics integration, task service, webhook handlers)
- ✅ Notifications module (templates, preferences, local event storage)
- ✅ SLA module (metrics, violation detection, statistics endpoints)

**Completed Sprint 5 (Order Fulfilment & Logistics):**
- ✅ Logistics service REST API client (`internal/platform/logistics/client.go`)
- ✅ Delivery task creation (`POST /{tenant}/orders/{id}/delivery/create-task`)
- ✅ Task status consumption (webhook handlers for all logistics events)
- ✅ Fulfilment module (task service, webhook service, repository)
- ✅ Ent schemas (OrderAssignment, DeliveryWindow, ProofOfDelivery, LogisticsEvent)
- ✅ Rider information queries (via logistics service API)
- ✅ Proof of delivery handling (webhook events, storage)
- ✅ Tracking endpoint (`GET /{tenant}/orders/{id}/delivery/tracking`)

**Completed Sprint 6 (Notifications & Ops):**
- ✅ Notifications module (Ent schemas, service, repository, handlers)
- ✅ SLA module (Ent schema, service, repository, handlers)
- ✅ Notification preferences management (GET/PUT endpoints)
- ✅ Notifications REST client (`internal/platform/notifications/client.go`)
- ✅ Inventory REST client (`internal/platform/inventory/client.go`)
- ✅ NATS event publisher (`internal/platform/events/publisher.go`)
- ✅ NATS event subscriber (`internal/platform/events/subscriber.go`)
- ✅ Event publisher for order lifecycle (created, ready, cancelled, completed)
- ✅ Event publisher for payment events (initiated, completed, failed)
- ✅ Event publisher for POS integration (catalog sync, pickup handoff)
- ✅ Event publisher wired to order service (`OrderService.SetEventPublisher()`)
- ⏳ Integration tests (ongoing)

**Current Sprint (Sprint 7 - Analytics, Compliance & Hardening):**
- ✅ Analytics dashboard integration (Superset embed URLs with RLS)
- ✅ Superset integration tests (comprehensive unit tests with mocking)
- ✅ Superset production URL configuration (https://superset.codevertexafrica.com)
- ✅ Data export/delete tooling (GDPR/DPA compliance)
- ✅ Performance optimization (database indexes, connection pooling, caching)
- ✅ Security hardening (rate limiting, headers, input validation, audit logging)
- ⏳ Analytics reports (CSV/PDF generation - domain models defined, implementation pending)
- ⏳ Integration tests (compliance workflow E2E tests)

**Not Yet Implemented:**
- ❌ Delivery options and zone management
- ❌ Group ordering
- ❌ Analytics and reporting
- ❌ Reconciliation logs
- ❌ Image upload/CDN
- ❌ Ordering PWA frontend

**External Service Integrations - 100% Platform Clients Complete:**
| Service | Client | Status |
|---------|--------|--------|
| Auth Service | `shared-auth-client` | ✅ Implemented |
| Treasury Service | `internal/platform/treasury/client.go` | ✅ Implemented |
| Logistics Service | `internal/platform/logistics/client.go` | ✅ Implemented |
| Notifications Service | `internal/platform/notifications/client.go` | ✅ Implemented |
| Inventory Service | `internal/platform/inventory/client.go` | ✅ Implemented |
| NATS Events | `internal/platform/events/publisher.go` | ✅ Implemented |

---

## Runtime Ports & Environments

- **Local development**: Backend runs on port **4000**, with `treasury-api` on **4001** and `notifications-api` on **4002**
- **Cloud deployment**: All backend services listen on **port 4000** for consistency behind ingress controllers

---

## References

- [Integration Guide](docs/integrations.md)
- [Entity Relationship Diagram](docs/erd.md)
- [Superset Integration](docs/superset-integration.md)
- [Sprint Plans](docs/sprints/)

## Google Auth Sync Workflow
For Google OAuth, ordering-backend handles the callback, exchanges the code for a token/profile, and then **synchronously calls auth-service** (`SyncUser` endpoint) to ensure the user identity exists in the central auth system. The local `users` table is then updated/created with the `auth_service_user_id`.
