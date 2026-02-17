# Architecture Overview

## Layered Design

- **Transport Layer (`internal/http`)**
  - chi router with shared middleware (request ID, logging, recovery, CORS)
  - HTTP handlers delegate to application services via interfaces
  - Future gRPC/Connect server mounted under `internal/grpc`

- **Application Layer (`internal/app`)**
  - Bootstraps configuration, infrastructure adapters, and HTTP server lifecycle
  - Coordinates graceful shutdown, dependency cleanup, and instrumentation

- **Domain Layer (`internal/modules`)**
  - 13 business modules: identity, catalog, ordering, payments, fulfilment, notifications, SLA, analytics, compliance, loyalty, promotions, reviews, settings
  - Use case orchestration, business rules, Ent-generated entity definitions
  - Interacts with repositories via interfaces for persistence/queue/cache

- **Infrastructure Layer (`internal/platform`)**
  - Concrete adapters: PostgreSQL (pgx), Redis cache, NATS JetStream
  - Shared telemetry clients (Prometheus, OpenTelemetry) and background workers

## Cross-Cutting Concerns

- **Configuration:** envconfig + `.env` support with `FOOD_DELIVERY_` prefix
- **Logging:** zap-based structured logging with request ID correlation
- **Observability:** Prometheus metrics endpoint (`/metrics`), OTEL exporters planned
- **Security:** TLS termination at ingress, mTLS service-to-service, OAuth2 introspection, tenant-aware RBAC
- **Validation:** go-playground/validator for request DTO validation; domain invariants enforced in use cases

## Integration Points

- **Inventory Service:** REST for stock availability checks, reservations (create/release/consume), and direct consumption (`internal/platform/inventory/client.go`)
- **Logistics Service:** NATS event `ordering.order.ready` triggers delivery task creation; REST for task tracking and proof of delivery (`internal/platform/logistics/client.go`)
- **Treasury Service:** REST for M-Pesa STK Push, payment status, refunds; webhooks for payment callbacks (`internal/platform/treasury/client.go`)
- **Notifications Service:** REST for sending templated notifications (order confirmation, delivery updates) (`internal/platform/notifications/client.go`)
- **Auth Service:** NATS event subscriptions (`auth.user.*`, `auth.tenant.*`) for identity sync (`internal/modules/identity/events.go`)
- **Maps & Geo:** External APIs (Mapbox/Google) abstracted behind `internal/platform/geo`
- **Analytics:** Outbox pattern feeding data warehouse / analytics pipelines

## Deployment

- Containerised (Docker multi-stage) → Helm chart → ArgoCD GitOps
- Horizontal Pod Autoscaling (CPU/RPS), Pod Disruption Budgets
- Feature flags handled via treasury-managed config service (future)
