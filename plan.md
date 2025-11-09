# Food Delivery Backend Delivery Plan

## Vision & Context
- Build a resilient, Busia-first logistics platform that powers customer ordering, cafe operations, rider fulfilment, and supervisory analytics across web and mobile clients.
- Reuse insights from `Urban Loft Logistics App` proposal and `Urban Cafe Food Delivery System Inception Report` to align with localized branding, dual-language support, and multi-channel payments.
- Backend orchestrates ordering, payments, loyalty, promotions, and notification flows while exposing consistent APIs to internal clients and partner microservices (`notifications-app`, `treasury-app`).

## Technical Foundations
- **Core Stack:** Go 1.22+, Clean/Hexagonal architecture, chi HTTP router, OpenAPI-first contracts, gRPC (ConnectRPC) gateway for high-throughput integrations.
- **Service Layout:** Multi-module Go project with explicit domain packages (`auth`, `catalog`, `orders`, `riders`, `cafes`, `payments`, `reports`, `notifications`). Use dependency inversion (interfaces + adapters) to keep domain logic isolated.
- **Data Layer:** PostgreSQL (pgx/v5) with migration toolkit (Atlas or Goose), Redis for caching and ephemeral state, Event backbone via NATS JetStream or Kafka, S3-compatible storage for media assets.
- **Deployment:** Containerised with multi-stage Docker builds, Helm charts, GitHub Actions CI/CD, ArgoCD for GitOps, Terraform modules for infrastructure.
- **Realtime Transport:** Server-Sent Events/WebSockets via chi middleware with Redis Pub/Sub fan-out; eventual migration to gRPC streams.
- **Observability:** OpenTelemetry instrumentation, Prometheus metrics, structured logging with zap, distributed tracing via Tempo/Jaeger.

## Domain Modules & Feature Scope
1. **Identity & Access Management (Priority 1)**
   - Multi-tenant cafe support, RBAC (customer, rider, cafe staff, super admin).
   - OAuth2/OIDC via external IdP, phone-based OTP fallback using notifications service.
   - Device/session management, refresh tokens, passwordless roadmap.
2. **Catalog & Menu Management (Priority 2)**
   - Menu categories, items, pricing, availability scheduling, dietary tags.
   - Image CDN integration, translation metadata (EN/SW) for localization.
3. **Ordering & Checkout (Priority 3)**
   - Cart persistence, guest checkout, promo code validation, loyalty engine (points accrual/redemption).
   - Address management, geocoding, delivery instructions, order orchestration state machine.
4. **Payments & Treasury Integration (Priority 3)**
   - MPesa STK Push (C2B), MPesa Express, card tokenization via treasury service (Stripe, PayPal).
   - Refunds, payouts (B2C), supplier settlements (B2B) using treasury event API.
   - Reconciliation jobs, idempotent transaction handling, audit trails.
5. **Fulfilment & Logistics (Priority 4)**
   - Rider onboarding, availability slots, dispatch algorithm (proximity, performance score).
   - Live tracking updates, delivery confirmation codes/photo proof, fallback manual assignment.
6. **Cafe Operations (Priority 4)**
   - Kitchen display queue, prep status transitions, stock-out workflows, substitution handling.
   - Shift schedules, capacity throttling, SLA monitoring.
7. **Notifications & Engagement (Priority 5)**
   - Event bridge to `notifications-app` for email/SMS/push; templated events (order placed, driver assigned, delivery complete, loyalty balance).
   - Marketing campaigns, segmented broadcasts, fallback SMS when push fails.
8. **Analytics & Reporting (Priority 6)**
   - Operational dashboards (orders, revenue, rider performance), exportable CSV/PDF.
   - Real-time incident alerts (unassigned orders, delayed deliveries).
9. **Support & Compliance (Priority 6)**
   - Customer support ticketing hooks, GDPR/Kenya DPA data subject requests (export/delete).
   - Activity logging, configurable retention policies.

## Cross-Cutting Concerns
- **Testing:** Go test suites with table-driven tests, Testcontainers for integration, Pact for contract tests, k6 for performance.
- **Observability:** Structured logging (zap), tracing via OpenTelemetry, metrics exported via Prometheus.
- **Security:** OWASP ASVS baseline, TLS everywhere, secrets via Vault/Parameter Store, rate limiting & anomaly detection middleware.
- **Localization:** Tenant-aware locale headers translating menu/catalog content.
- **Scalability:** Stateless HTTP layer, background workers via NATS/Redis streams, sharded database readiness.

## External Integrations & Dependencies
- **`notifications-app`:** Event-driven integration (NATS JetStream or HTTP webhooks) with retries, template catalogue sync, opt-out management.
- **`treasury-app`:** Secure REST/gRPC channel for collection/disbursement flows; webhook listeners for payment status, reconciliation scheduler.
- **Mapping & Geo Services:** Mapbox or Google Maps for geocoding, distance matrix, route ETA.
- **Identity Providers:** OAuth2 for Google/Microsoft, SMS OTP via notifications service.

## Data Management
- PostgreSQL schemas managed via migrations (Atlas/Goose), adhering to multi-tenant constraints.
- Outbox pattern for reliable domain events. CDC streaming for analytics warehouse (future).
- Redis for cached menus, session tokens, rate limiting counters.
- Backup & retention strategy (daily snapshots, PITR), encryption at rest & in transit.

## API & Protocol Strategy
- REST-first with versioned routes (`/v1/{tenant}/orders`), documented via OpenAPI.
- gRPC (ConnectRPC) for high-throughput internal service communication.
- Webhooks for payment/notification callbacks; SSE/WebSockets for live tracking.
- Idempotency keys, correlation IDs, and distributed tracing context propagation.

## Compliance & Risk Controls
- Align with Kenya Data Protection Act: explicit consent flows, user data export/delete endpoints, audit logging.
- PCI scope reduction by delegating card handling to treasury providers.
- Fraud prevention: velocity checks, device fingerprinting, anomaly scoring.
- Disaster recovery playbook, RTO/RPO targets (<1 hour).

## Sprint Roadmap (Priority-Ordered)
1. **Sprint 0 – Foundation (Week 1)**
   - Go project scaffolding, CI/CD pipeline, configuration management, observability baseline.
   - Define domain models, ERD, API guidelines, service interface contracts.
2. **Sprint 1 – Identity & Access (Weeks 2-3)**
   - Tenant-aware auth, RBAC, refresh tokens, OAuth2 integration, audit log baseline.
3. **Sprint 2 – Catalog & Localization (Weeks 4-5)**
   - Menu CRUD, category hierarchy, image handling, localization fields, public menu API.
4. **Sprint 3 – Orders & Cart (Weeks 6-7)**
   - Cart service, checkout workflow, promo engine MVP, order state machine, idempotent order creation.
5. **Sprint 4 – Payments Core (Weeks 8-9)**
   - Treasury integration (MPesa C2B/STK), payment webhook processing, reconciliation logs, retry policies.
6. **Sprint 5 – Fulfilment & Dispatch (Weeks 10-11)**
   - Rider onboarding, availability, dispatch algorithm v1, WebSocket/SSE updates, cafe kitchen queue linkage.
7. **Sprint 6 – Notifications & Ops (Weeks 12-13)**
   - Event pipeline to `notifications-app`, SLA monitoring, issue escalation, support endpoints.
8. **Sprint 7 – Analytics, Compliance & Hardening (Weeks 14-15)**
   - Reporting endpoints, data export/delete tooling, performance tuning, penetration testing, release readiness.
9. **Sprint 8 – Launch & Handover (Week 16)**
   - Production deployment, chaos drills, documentation handover, post-launch monitoring & backlog triage.

## Backlog & Future Enhancements
- AI-assisted order recommendations (collaborative filtering) via separate service.
- Dynamic pricing & surge controls, rider incentive programs, multi-cafe marketplace support.
- Inventory management integration, POS sync, franchise reporting.

## Non-Functional Goals
- Availability 99.95%, financial accuracy with ACID guarantees, P99 latency < 800ms for payment initiation.
- Tenant isolation verified through automated security tests, configurable data residency by organisation, and branch-level segregation policies.

## Runtime Ports & Environments
- **Local development:** backend runs on port **4000**, with `treasury-app` on **4001** and `notifications-app` on **4002** to simplify side-by-side testing.
- **Cloud deployment:** all backend services listen on **port 4000** for consistency behind ingress controllers. Environment variables (`FOOD_DELIVERY_HTTP_PORT`, `TREASURY_HTTP_PORT`, `NOTIFICATIONS_HTTP_PORT`) drive the override during deployment.

---
**Next Steps:** Align with frontend, notifications, and treasury teams on API contracts; finalize sprint staffing; set up shared Postman/Stoplight collections; prepare Go service templates for new domain modules.

