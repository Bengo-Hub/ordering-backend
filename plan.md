# Food Delivery Backend Delivery Plan

## Vision & Context
- Build a resilient, Busia-first logistics platform that powers customer ordering, cafe operations, rider fulfilment, and supervisory analytics across web and mobile clients.
- Reuse insights from `Urban Loft Logistics App` proposal and `Urban Cafe Food Delivery System Inception Report` to align with localized branding, dual-language support, and multi-channel payments.
- Backend orchestrates ordering, payments, loyalty, promotions, and notification flows while exposing consistent APIs to internal clients and partner microservices (`notifications-app`, `treasury-app`).

## Technical Foundations
- **Core Stack:** Go 1.22+, Clean/Hexagonal architecture, chi HTTP router, OpenAPI-first contracts, gRPC (ConnectRPC) gateway for high-throughput integrations.
- **Service Layout:** Multi-module Go project with explicit domain packages (`auth`, `catalog`, `orders`, `riders`, `cafes`, `payments`, `reports`, `notifications`). Use dependency inversion (interfaces + adapters) to keep domain logic isolated.
- **Data Layer:** PostgreSQL (pgx/v5) with Ent ORM (schema-as-code migrations), Redis for caching and ephemeral state, Event backbone via NATS JetStream or Kafka, S3-compatible storage for media assets.
- **Deployment:** Containerised with multi-stage Docker builds, Helm charts, GitHub Actions CI/CD, ArgoCD for GitOps, Terraform modules for infrastructure.
- **Realtime Transport:** Server-Sent Events/WebSockets via chi middleware with Redis Pub/Sub fan-out; eventual migration to gRPC streams.
- **Observability:** OpenTelemetry instrumentation, Prometheus metrics, structured logging with zap, distributed tracing via Tempo/Jaeger.

## Domain Modules & Feature Scope
1. **Identity & Access Management (Priority 1)**
   - Multi-tenant cafe support, RBAC (customer, rider, cafe staff, super admin) using a shared `tenant_slug` and outlet registry consistent with `auth-service`, `logistics-service`, `inventory-service`, and `pos-service`.
   - OAuth2/OIDC via external IdP, phone-based OTP fallback using notifications service, with token issuance owned by `auth-service` and this backend consuming claims (no duplicate identity storage).
   - Device/session management, refresh tokens, two-factor auth (TOTP/SMS) and backup codes rely on `auth-service`; only tenant-level preferences are cached locally.
   - Rider KYC verification flow emits onboarding events to `logistics-service`, which remains the source of truth for rider profiles, documents, and status transitions. If the tenant/outlet metadata has not yet been replicated, a discovery webhook is sent to `logistics-service` before the rider record is persisted.
   - During login, `auth-service` emits tenant/outlet discovery callbacks so this backend can hydrate metadata prior to handling user-specific workflows.
   - Tenant sync events are persisted locally (`tenant_sync_events`) to fan-out webhook notifications to logistics, inventory, POS, treasury, and notifications services—ensuring downstream services receive canonical tenant/outlet data without polling.
   - _Progress:_ Sprint 0 delivered in-memory RBAC + OAuth2 flows with JWT sessions, profile/preferences/security endpoints, and customer order summaries (now aligned to the shared identity architecture).
2. **Catalog & Menu Management (Priority 2)**
   - Menu categories, items, pricing, availability scheduling, dietary tags.
   - Image CDN integration, translation metadata (EN/SW) for localization.
3. **Subscription & Licensing (Priority 2)**
   - Tiered plan catalogue (Starter/Growth/Professional) with feature toggles for loyalty, multi-outlet, POS access, and support levels.
   - Tenant subscription lifecycle: trials, activation, proration, overage tracking (extra riders/orders), and in-app upgrade/downgrade workflows.
   - License renewal automation: billing schedules, reminders via `notifications-app`, integration with `treasury-app` for invoicing and receipting.
   - Entitlements service exposing runtime feature flags (e.g. POS integration, advanced analytics) consumed by frontend/admin clients.
4. **Ordering & Checkout (Priority 3)**
   - Cart persistence, guest checkout, promo code validation, loyalty engine (points accrual/redemption).
   - Address management, geocoding, delivery instructions, order orchestration state machine.
5. **Payments & Treasury Integration (Priority 3)**
   - MPesa STK Push (C2B), MPesa Express, card tokenization via treasury service (Stripe, PayPal).
   - Refunds, payouts (B2C), supplier settlements (B2B) using treasury event API.
   - Reconciliation jobs, idempotent transaction handling, audit trails.
   - Federated with `treasury-app` for ledgering, invoicing, payout orchestration, and financial compliance exports.
6. **POS & External Sales Integrations (Priority 3)**
   - Abstract POS integration service supporting cafe/bar, ecommerce, kitchen, and general POS scenarios with unified order ingest.
   - Sync adapters for external POS microservice (catalog sync, ticket import, settlement exports) gated by subscription entitlements.
   - Treasury + notifications hooks to reflect POS settlement events, low-stock alerts, and reconciliation discrepancies.
7. **Fulfilment & Logistics (Priority 4)**
   - Rider onboarding forms captured in frontend are forwarded here and synchronised with `logistics-service` under the same tenant/outlet identifiers; this backend stores only rider references.
   - Live tracking updates, delivery confirmation codes/photo proof, fallback manual assignment now consume task/rider streams produced by `logistics-service`.
   - Enforce rider verification states (pending, active, suspended) using data fetched from `logistics-service`.
   - Emits payout and performance events to `treasury-app`, consumes delivery notifications from `notifications-app`, enriches ETAs with external mapping APIs, and hands off routing to `logistics-service` for multi-leg orchestration without duplicating rider or route tables.
8. **Cafe Operations (Priority 4)**
   - Kitchen display queue, prep status transitions, stock-out workflows, substitution handling.
   - Shift schedules, capacity throttling, SLA monitoring.
9. **Notifications & Engagement (Priority 5)**
   - Event bridge to `notifications-app` for email/SMS/push; templated events (order placed, driver assigned, delivery complete, loyalty balance).
   - Marketing campaigns, segmented broadcasts, fallback SMS when push fails.
   - Backend persists audience preferences while `notifications-app` handles channel delivery guarantees, template rendering, and audit trails.
10. **Analytics & Reporting (Priority 6)**
   - Operational dashboards (orders, revenue, rider performance), exportable CSV/PDF.
   - Real-time incident alerts (unassigned orders, delayed deliveries).
11. **Support & Compliance (Priority 6)**
   - Customer support ticketing hooks, GDPR/Kenya DPA data subject requests (export/delete).
   - Activity logging, configurable retention policies.

## Cross-Cutting Concerns
- **Testing:** Go test suites with table-driven tests, Testcontainers for integration, Pact for contract tests, k6 for performance.
- **Observability:** Structured logging (zap), tracing via OpenTelemetry, metrics exported via Prometheus.
- **Security:** OWASP ASVS baseline, TLS everywhere, secrets via Vault/Parameter Store, rate limiting & anomaly detection middleware.
- **Localization:** Tenant-aware locale headers translating menu/catalog content.
- **Scalability:** Stateless HTTP layer, background workers via NATS/Redis streams, sharded database readiness.
- **Brand & System configuration:** Persist tenant-specific look & feel settings (colors, typography, logos) plus API credentials, webhook endpoints, and feature toggles exposed through admin APIs consumed by frontend clients; share outlet metadata with `pos-service`, `inventory-service`, and `logistics-service` instead of duplicating tables.
- **Identity Federation:** All inbound requests validated against `auth-service` tokens (JWKS cache, audience checks) with fallbacks for introspection; MFA enforcement respected per-tenant policy.
- **Data modelling:** Ent schemas act as the single source of truth for database structure, enabling declarative migrations and type-safe repositories across modules. Tenant/outlet discovery webhooks ensure downstream services are up to date before domain records are written.

## External Integrations & Dependencies
- **`notifications-app`:** Event-driven integration (NATS JetStream or HTTP webhooks) with retries, template catalogue sync, opt-out management.
- **`treasury-app`:** Secure REST/gRPC channel for collection/disbursement flows; webhook listeners for payment status, reconciliation scheduler.
- **`pos-gateway` (external microservice):** Bidirectional sync for POS orders, menu updates, and settlement summaries; per-tenant credential management aligned with subscription entitlements.
- **`logistics-service`:** Dispatch requests, rider onboarding, route status callbacks, proof-of-delivery ingestion, and marketplace carrier coordination. All interactions use the shared tenant/outlet registry to prevent divergent data.
- **`inventory-service`:** Stock availability, reservation, and recipe consumption events to drive menu status and fulfilment eligibility.
- **`auth-service`:** OIDC authority for all user/staff/admin identities; provides JWT validation, tenant membership claims, and MFA enforcement for backend APIs.
- **Mapping & Geo Services:** Mapbox or Google Maps for geocoding, distance matrix, route ETA.
- **Identity Providers:** OAuth2 for Google/Microsoft, SMS OTP via notifications service.
- **Webhook Fabric:** All inter-service integrations (treasury settlements, logistics tasks, inventory signals, tenant/outlet discovery) are implemented via callbacks/webhooks with retry policies—polling is avoided to maintain consistency.

## Data Management
- PostgreSQL schemas managed via migrations (Atlas/Goose), adhering to multi-tenant constraints.
- Outbox pattern for reliable domain events. CDC streaming for analytics warehouse (future).
- Redis for cached menus, session tokens, rate limiting counters.
- Backup & retention strategy (daily snapshots, PITR), encryption at rest & in transit. Surface backup jobs and restore requests through admin settings.
- Subscription ledger stored in PostgreSQL with historical invoices, renewal schedules, and feature entitlement snapshots.

## API & Protocol Strategy
- REST-first with versioned routes (`/v1/{tenant}/orders`), documented via OpenAPI.
- gRPC (ConnectRPC) for high-throughput internal service communication.
- Webhooks for payment/notification callbacks, tenant/outlet discovery, logistics task status, and treasury settlements; SSE/WebSockets for live tracking.
- Idempotency keys, correlation IDs, and distributed tracing context propagation.

## Compliance & Risk Controls
- Align with Kenya Data Protection Act: explicit consent flows, user data export/delete endpoints, audit logging.
- PCI scope reduction by delegating card handling to treasury providers.
- Fraud prevention: velocity checks, device fingerprinting, anomaly scoring.
- Disaster recovery playbook, RTO/RPO targets (<1 hour).

## Sprint Roadmap (Priority-Ordered)
1. **Sprint 0 – Foundation (Week 1)**
   - Go project scaffolding, CI/CD pipeline, configuration management, observability baseline.
   - Identity bootstrap: deliver RBAC roles/permissions, OAuth2 (Google) initiation & callback, JWT access/refresh session management, profile/preferences/security endpoints, and customer order summaries powering Sprint 0 dashboards. _Status: ✅ Completed (Nov 2025)._
   - Define domain models, ERD, API guidelines, service interface contracts.
2. **Sprint 1 – Identity & Access Hardening (Weeks 2-3)**
   - Persist identity data to Postgres via Ent repositories (users, sessions, tokens), enforce tenant scoping, device management, invitation workflows, and audit log baseline.
   - Kick off rider verification service design (document schema, review workflow) to unblock frontend onboarding flows.
   - Deliver initial subscription entitlement service scaffolding to gate RBAC/feature access and align JWT claims with `auth-service` contracts.
3. **Sprint 2 – Catalog & Localization (Weeks 4-5)**
   - Menu CRUD, category hierarchy, image handling, localization fields, public menu API.
4. **Sprint 3 – Orders & Cart (Weeks 6-7)**
   - Cart service, checkout workflow, promo engine MVP, order state machine, idempotent order creation.
5. **Sprint 4 – Payments Core (Weeks 8-9)**
   - Treasury integration (MPesa C2B/STK), payment webhook processing, reconciliation logs, retry policies.
6. **Sprint 5 – Fulfilment & Dispatch (Weeks 10-11)**
   - Rider onboarding, availability, dispatch algorithm v1, WebSocket/SSE updates, cafe kitchen queue linkage.
   - Enforce rider status from KYC service before dispatching tasks; expose verification APIs to client apps.
7. **Sprint 6 – Notifications & Ops (Weeks 12-13)**
   - Event pipeline to `notifications-app`, SLA monitoring, issue escalation, support endpoints.
8. **Sprint 7 – Analytics, Compliance & Hardening (Weeks 14-15)**
   - Reporting endpoints, data export/delete tooling, performance tuning, penetration testing, release readiness.
   - Harden subscription invoicing, renewal reminders, and overage metering in tandem with analytics exports.
9. **Sprint 8 – Launch & Handover (Week 16)**
   - Production deployment, chaos drills, documentation handover, post-launch monitoring & backlog triage.

## Backlog & Future Enhancements
- AI-assisted order recommendations (collaborative filtering) via separate service.
- Dynamic pricing & surge controls, rider incentive programs, multi-cafe marketplace support.
- Inventory management integration, POS sync, franchise reporting, and advanced revenue share calculations.
- Tenant theming service (look & feel) with audit trail and preview endpoints for admin UI.

## Non-Functional Goals
- Availability 99.95%, financial accuracy with ACID guarantees, P99 latency < 800ms for payment initiation.
- Tenant isolation verified through automated security tests, configurable data residency by organisation, and branch-level segregation policies.

## Runtime Ports & Environments
- **Local development:** backend runs on port **4000**, with `treasury-app` on **4001** and `notifications-app` on **4002** to simplify side-by-side testing.
- **Cloud deployment:** all backend services listen on **port 4000** for consistency behind ingress controllers. Environment variables (`FOOD_DELIVERY_HTTP_PORT`, `TREASURY_HTTP_PORT`, `NOTIFICATIONS_HTTP_PORT`) drive the override during deployment.

---
**Next Steps:** Align with frontend, notifications, and treasury teams on API contracts; finalize sprint staffing; set up shared Postman/Stoplight collections; prepare Go service templates for new domain modules. Prioritise rider KYC service schema, look & feel configuration APIs, and authentication flows that unblock newly added frontend onboarding surfaces.

