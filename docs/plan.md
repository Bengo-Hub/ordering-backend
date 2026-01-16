# Ordering Service - Implementation Plan

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
- Each service owns and manages all data related to its domain
- Other services reference data via IDs and tenant mapping
- Before creating/referencing data in another service:
  1. Check tenant has that service enabled in their subscription plan
  2. Verify tenant exists in target service (if tenant-specific)
  3. Either push data via API or redirect to target service UI
  4. Store only reference IDs locally, never duplicate data

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
- **auth-service** (Production: `https://sso.codevertexitsolutions.com/`):
  - **Authentication**: All login/registration requests proxy to auth-service `/api/v1/auth/login` and `/api/v1/auth/register`
  - **Default Tenant**: If `tenant_slug` is not provided in login/registration requests, defaults to `urban-cafe`
  - **JWT Validation**: Token validation via JWKS (`/api/v1/.well-known/jwks.json`) using `shared/auth-client` library
  - **User Identity Sync**: Local user table stores `auth_service_user_id` reference to auth-service user
  - **User Events**: Consume `auth.user.created`, `auth.user.updated`, `auth.user.deactivated` events to sync user data
  - **Tenant Events**: Consume `auth.tenant.created`, `auth.tenant.updated`, `auth.outlet.created` events
  - **Superuser Handling**: Superusers from auth-service bypass all RBAC/permission checks across all services
  - **MFA Enforcement**: MFA state managed by auth-service, cafe backend respects MFA requirements
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
  - **Important**: Cafe backend stores only `rider_id` references, never rider profiles or fleet data
- **inventory-service**: Stock availability queries (references inventory SKUs, no duplication)
- **treasury-app**: Payment processing (payment intents, webhooks, refunds)

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
- `payouts` - Payouts to riders/cafes (references treasury-app)
- `settlements` - Periodic accounting for cafes
- `treasury_events` - Webhook ingestion for treasury systems

**Integration Points**:
- **treasury-app**: Payment processing, ledgering, invoicing, payout orchestration, financial compliance exports
- **M-Pesa**: STK Push (C2B), Express payments via treasury-app



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

**Note**: Cafe backend does NOT store rider profiles, fleet data, or delivery task details. Only references.



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
- **notifications-app**: Channel delivery guarantees, template rendering, audit trails

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
- **Authentication**: All authentication delegated to auth-service (`https://sso.codevertexitsolutions.com/`)
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
- Sprint 1: Business Configuration 🚧 **In Progress** (Multi-business type support, workflow templates)
- Sprint 2: Flexible Catalog ⏳ **Planned** (Item types, variants, modifiers, categories)
- Sprint 3: Shopping Cart ⏳ **Planned** (Cart persistence, Redis caching, real-time calculations)
- Sprint 4: Delivery Options ⏳ **Planned** (Zones, scheduling, dropoff methods, availability checks)
- Sprint 5: Order Management ⏳ **Planned** (Order placement, state machine, tracking)
- Sprint 6: Group Orders ⏳ **Planned** (Collaborative ordering, split payments)
- Sprint 7: Payments Integration ⏳ **Planned** (Treasury integration, payment methods, webhooks)
- Sprint 8: Logistics Integration ⏳ **Planned** (Delivery tasks, live tracking, POD)
- Sprint 9: Promo & Loyalty ⏳ **Planned** (Promo codes, loyalty points, rewards)
- Sprint 10: Notifications ⏳ **Planned** (Order status notifications, marketing)
- Sprint 11: Analytics & PWA ⏳ **Planned** (Dashboards, Ordering PWA development)
- Sprint 12: Launch & Hardening ⏳ **Planned** (Load testing, security audit, production deployment)

## Current Implementation Status

**Implemented Modules:**
- ✅ Foundation (server bootstrap, health checks, observability baseline)
- ✅ Auth integration (JWT validation via auth-service, session management)
- ✅ Basic multi-tenancy support

**In Progress:**
- 🚧 Business configuration module
- 🚧 Flexible catalog schema design

**Not Yet Implemented:**
- ❌ Shopping cart
- ❌ Delivery options and zone management
- ❌ Order management and state machine
- ❌ Group ordering
- ❌ Payments integration (treasury-service)
- ❌ Logistics integration (delivery tasks, tracking)
- ❌ Promo codes and loyalty
- ❌ Notifications integration
- ❌ Analytics and reporting
- ❌ Ordering PWA frontend

---

## Runtime Ports & Environments

- **Local development**: Backend runs on port **4000**, with `treasury-app` on **4001** and `notifications-app` on **4002**
- **Cloud deployment**: All backend services listen on **port 4000** for consistency behind ingress controllers

---

## References

- [Integration Guide](docs/integrations.md)
- [Entity Relationship Diagram](docs/erd.md)
- [Superset Integration](docs/superset-integration.md)
- [Sprint Plans](docs/sprints/)

## Google Auth Sync Workflow
For Google OAuth, the `cafe-backend` handles the callback, exchanges the code for a token/profile, and then **synchronously calls auth-service** (`SyncUser` endpoint) to ensure the user identity exists in the central auth system. The local `users` table is then updated/created with the `auth_service_user_id`.
