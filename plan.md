# Cafe Backend - Implementation Plan

## Executive Summary

**System Purpose**: Cloud-hosted cafe platform enabling customer ordering, cafe operations, and supervisory analytics. The service orchestrates ordering, payments, loyalty, promotions, and notification flows while exposing consistent APIs to internal clients and partner microservices.

**Key Capabilities**:
- Multi-tenant cafe support with outlet management
- Menu catalog with localization (EN/SW)
- Order management with state machine
- Cart and checkout workflows
- Loyalty points accrual and redemption
- Promo code engine
- Kitchen display queue
- Subscription and licensing management
- Integration with treasury, logistics, notifications, inventory, and POS services

**Entity Ownership**: This service owns cafe-specific entities: cafe orders, menu items (references inventory SKUs), cart, loyalty points, and cafe promotions. 

**Cafe does NOT own**:
- **Riders/drivers/fleets**: All rider, driver, fleet, delivery task, shift, and telemetry data is owned by `logistics-service`. Cafe backend stores only `rider_id` references in `order_assignments` table. All rider/fleet queries must go to logistics-service APIs. **Rider creation**: Check tenant has logistics service enabled, then either push to logistics-service API or redirect to logistics-service UI.
- **Catalog items**: References inventory-service SKUs, no duplication. **Inventory operations**: Check tenant has inventory service enabled before creating/updating items.
- **Payment processing**: Uses treasury-app for all payment operations. **Payment operations**: Check tenant has treasury service enabled before processing payments.
- **Inventory balances**: Queries inventory-service, no local stock data
- **Users**: References auth-service via `auth_service_user_id` field, local table stores only cafe-specific extensions (preferences, cafe roles, loyalty points). Identity data synced from auth-service via events.

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

### 2. Catalog & Menu Management

**Cafe-Specific Features**:
- Menu categories and hierarchy
- Menu items with variants (size, flavor)
- Pricing and availability scheduling
- Dietary tags (vegan, gluten-free, etc.)
- Image CDN integration
- Translation metadata (EN/SW) for localization
- Menu item schedules (time-based availability)

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

### 3. Subscription & Licensing

**Cafe-Specific Features**:
- Tiered plan catalogue (Starter/Growth/Professional)
- Feature toggles (loyalty, multi-outlet, POS access, support levels)
- Tenant subscription lifecycle (trials, activation, proration, overage tracking)
- License renewal automation
- Entitlements service for runtime feature flags

**Entities Owned**:
- `subscription_plans` - Plan catalogue
- `subscription_features` - Feature entitlements
- `tenant_subscriptions` - Active subscription state
- `subscription_invoices` - Billing history
- `subscription_usages` - Aggregated usage for overage fees
- `license_renewals` - Renewal activity log

**Integration Points**:
- **treasury-app**: Invoicing and receipting
- **notifications-app**: Renewal reminders

### 4. Ordering & Checkout

**Cafe-Specific Features**:
- Cart persistence
- Guest checkout support
- Promo code validation
- Loyalty engine (points accrual/redemption)
- Address management
- Geocoding and delivery instructions
- Order orchestration state machine

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

**Cafe-Specific Features**:
- Payment method tokenization
- Payment intent management
- Refund processing
- Payout tracking (riders/cafes)
- Settlement reconciliation

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

### 6. POS & External Sales Integrations

**Cafe-Specific Features**:
- Abstract POS integration service
- Sync adapters for external POS microservice
- Catalog sync, ticket import, settlement exports
- Connection health monitoring

**Entities Owned**:
- `pos_providers` - Registry of supported POS ecosystems
- `pos_connections` - Tenant-specific credentials
- `pos_locations` - Mapping between POS outlets and cafes
- `pos_sync_jobs` - Audit of imports/exports
- `pos_order_links` - Bridge table linking internal orders to external POS

**Integration Points**:
- **pos-service**: POS device/session data (references only)
- **treasury-app**: Settlement reconciliation
- **notifications-app**: Operational alerts

### 7. Order Fulfilment & Logistics Integration

**Cafe-Specific Features**:
- Order readiness notification
- Task status consumption
- Rider reference queries
- Live tracking integration
- Proof of delivery handling

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

### 8. Cafe Operations

**Cafe-Specific Features**:
- Kitchen display queue
- Prep status transitions
- Stock-out workflows
- Substitution handling
- Shift schedules
- Capacity throttling
- SLA monitoring

**Entities Owned**:
- `kitchen_tickets` - Kitchen production tracking
- `ticket_events` - Ticket workflow history
- `capacity_rules` - Throttling controls
- `shift_schedules` - Staff rostering

**Integration Points**:
- **inventory-service**: Stock availability queries (no duplication)

### 9. Notifications & Engagement

**Cafe-Specific Features**:
- Event bridge to notifications-app
- Templated events (order placed, driver assigned, delivery complete, loyalty balance)
- Marketing campaigns
- Segmented broadcasts

**Entities Owned**:
- `notification_templates` - Messaging templates (cafe-specific)
- `notification_events` - Pending notifications for async dispatch
- `notification_subscriptions` - Opt-in/opt-out preferences

**Integration Points**:
- **notifications-app**: Channel delivery guarantees, template rendering, audit trails

### 10. Analytics & Reporting

**Cafe-Specific Features**:
- Operational dashboards (orders, revenue, rider performance)
- Exportable CSV/PDF reports
- Real-time incident alerts

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
- Sprint 0: Foundation ✅ **Completed** (Nov 2025)
- Sprint 1: Identity & Access Hardening 🚧 **Partially Implemented** (identity persistence done, device/invitations/audit pending)
- Sprint 2: Catalog & Localization ⏳ **Not Started**
- Sprint 3: Orders & Cart ⏳ **Not Started**
- Sprint 4: Payments Core ⏳ **Not Started**
- Sprint 5: Order Fulfilment & Logistics Integration ⏳ **Not Started**
- Sprint 6: Notifications & Ops ⏳ **Not Started**
- Sprint 7: Analytics, Compliance & Hardening ⏳ **Not Started**
- Sprint 8: Launch & Handover ⏳ **Not Started**

## Current Implementation Status

**Implemented Modules:**
- ✅ Identity & Access Management (OAuth2, JWT, user profiles, preferences, RBAC, Google SSO Sync)
- ✅ Basic tenant support (tenant entity exists, scoping not fully enforced)
- ✅ Session management (database persistence)
- ✅ Health checks and observability baseline

**Not Yet Implemented:**
- ❌ Catalog & Menu Management
- ❌ Ordering & Checkout
- ❌ Payments Integration
- ❌ Logistics Integration (delivery task creation)
- ❌ Notifications Integration
- ❌ Analytics & Reporting
- ❌ Subscription Management
- ❌ POS Integration

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
