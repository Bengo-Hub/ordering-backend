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
  - 14 business modules: identity, catalog, ordering, payments, fulfilment, notifications, SLA, analytics, compliance, loyalty, promotions, reviews, settings, **rbac**
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
- **RBAC Authorization:** Full RBAC module (`internal/modules/rbac`) following the treasury-api pattern. Entities: `OrderingPermission` (permission codes `ordering.{module}.{action}`), `OrderingRole` (tenant-scoped with system roles), `RolePermission` (junction), `UserRoleAssignment` (user-role with expiry). Service provides `EnsureUserFromToken` (JIT provisioning), `HasPermission`, `HasRole`, `AssignRole`, `RevokeRole`. HTTP endpoints at `/rbac/assignments`, `/rbac/roles`, `/rbac/permissions`. Database-driven `RateLimitConfig` and `ServiceConfig` tables for runtime configuration.
- **Validation:** go-playground/validator for request DTO validation; domain invariants enforced in use cases

## Integration Points

- **Auth Service:** NATS event subscriptions (`auth.user.*`, `auth.tenant.*`) for identity sync (`internal/modules/identity/events.go`). Local authentication artifacts (sessions, MFA) have been removed in favor of SSO validation.
- **Inventory & POS Services:** Ordering acts as a **Front-end Projector**, consuming the catalog from POS/Inventory to present a fulfillment-optimized view. It does not own master product data or variants.
- **Maps & Geo:** External APIs (Mapbox/Google) abstracted behind `internal/platform/geo`
- **Analytics:** Outbox pattern feeding data warehouse / analytics pipelines

## Deployment

- Containerised (Docker multi-stage) → Helm chart → ArgoCD GitOps
- Horizontal Pod Autoscaling (CPU/RPS), Pod Disruption Budgets
- Feature flags handled via treasury-managed config service (future)
