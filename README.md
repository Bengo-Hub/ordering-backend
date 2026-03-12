# Ordering Service Backend

**Multi-Business Online Ordering Platform** - Go-based service for **ONLINE DELIVERY/SHIPPING ORDERS ONLY**. Supports multiple business types (food delivery, retail, grocery, pharmacy, e-commerce) with configurable workflows, flexible catalog management, and seamless integration with logistics, treasury, inventory, and auth services.

**Note**: This service handles **online orders for delivery/shipping only**. Walk-in customers, dine-in orders, pickup orders, and POS terminal operations are handled by the **POS Service**.

## Scope

**What This Service Handles**:
- ✅ **Online orders for delivery/shipping** (customers ordering via web/mobile for delivery to their location)
- ✅ Customer shopping cart management with flexible item modifiers
- ✅ Online catalog/menu display with multi-business type support
- ✅ Order tracking and real-time delivery updates
- ✅ Integration with Logistics Service for delivery task creation
- ✅ Integration with Treasury Service for online payments (M-Pesa, cards)
- ✅ Scheduled orders and ASAP delivery
- ✅ Group ordering (office lunches, shared carts)
- ✅ Multi-tenant with outlet-specific catalogs

**What This Service Does NOT Handle** (delegated to other services):
- ❌ Over-the-counter orders (walk-in customers) → **POS Service**
- ❌ Pickup orders (customer picks up from store) → **POS Service**
- ❌ Dine-in orders (table service, restaurant orders) → **POS Service**
- ❌ Cash drawer management → **POS Service**
- ❌ Kitchen ticket printing → **POS Service**
- ❌ Point-of-sale terminal operations → **POS Service**

**Frontend**: Integrates with **Ordering PWA** (Progressive Web App) for customer-facing online ordering.

## Tenant and SSO

- **Default tenant slug** is `urban-loft` (config: `DefaultTenantSlug`). Used as fallback when no tenant is provided (e.g. default org in UI).
- **JIT tenant sync:** On each request with tenant context (from JWT or URL), middleware ensures the tenant exists in the local DB by calling auth-api `GET /api/v1/tenants/by-slug/{slug}` and upserting. This avoids "tenant not found" after SSO login. See auth-api docs for the uniform tenant sync workflow across Go backends.

## Multi-Business Type Support

This service is configurable to support various business types beyond food delivery:

| Business Type | Order Items | Delivery Options | Special Features |
|---------------|-------------|------------------|------------------|
| **Food Delivery** | Meals, beverages | ASAP, scheduled | Item modifiers (size, extras), group orders |
| **Grocery** | Food items, household | Scheduled windows | Substitution preferences, bulk orders |
| **Retail** | Products, apparel | Same-day/next-day | Size/color variants, gift wrapping |
| **Pharmacy** | Medications, OTC | Priority/standard | Prescription upload, age verification |
| **Flowers** | Bouquets, arrangements | Scheduled delivery | Personalized messages, occasion-based |
| **Electronics** | Devices, accessories | Express/standard | Warranty options, installation services |

## Stack Overview


- **Language:** Go 1.22+
- **HTTP:** chi router, structured middleware, OpenAPI-first endpoints
- **Data:** PostgreSQL (pgx/v5), Redis cache, NATS JetStream/event bus
- **Observability:** zap logging, Prometheus metrics, OpenTelemetry ready
- **Build & Tooling:** Makefile, go test, golangci-lint (install separately), Docker multi-stage images

## Getting Started

```bash
cp config/app.env.example .env # populate with credentials
make tidy # go mod tidy
make run # go run ./cmd/api
```

> **Windows without GNU Make:** From PowerShell, you can run the same steps manually:
> ```powershell
> cd D:\Projects\BengoBox\FoodDelivery\food-delivery-backend
> go mod tidy
> go run ./cmd/api
> ```
> Use `Ctrl+C` in that terminal to stop the server. If you accidentally leave it running in the background, terminate it with:
> ```powershell
> Get-Process go | Where-Object { $_.Path -like "*food-delivery-backend*" } | Stop-Process
> ```
> Restarting the server simply means re-running `go run ./cmd/api` in the foreground.

### API Documentation

- Swagger UI: http://localhost:4000/swagger/index.html
- Regenerate the OpenAPI spec after updating handler annotations:
  ```powershell
  cd D:\Projects\BengoBox\FoodDelivery\food-delivery-backend
  swag init -g cmd/api/main.go -o internal/http/docs
  ```

### Database Migrations & Seeding

```powershell
cd D:\Projects\BengoBox\FoodDelivery\food-delivery-backend

# Apply schema migrations (Ent-managed)
go run ./cmd/migrate

# Seed baseline tenant, roles, permissions, and super admin
go run ./cmd/seed
```

> macOS/Linux equivalent:
> ```bash
> cd ~/Projects/BengoBox/FoodDelivery/food-delivery-backend
> go run ./cmd/migrate
> go run ./cmd/seed
> ```

See [`docs/erd.md`](docs/erd.md) for a detailed entity relationship overview backing these migrations. Default bootstrap credentials:

- Email: `superuser@urbancafe.example`
- Password: `ChangeMe123!` (update immediately after first login)

Port mapping:

- Local development runs on **http://localhost:4000** by default.
- Treasury (`treasury-api`) and Notifications (`notifications-api`) services occupy ports **4001** and **4002** respectively when running locally.
- In cloud/cluster deployments, all backend services are configured to listen on **port 4000** for ingress uniformity. Helm values set `HTTP_PORT` to override the default.

### Required Environment Variables

The service uses **standard env keys** (no prefix) aligned with other Go backends. In cluster, `devops-k8s` sets these via Helm.

| Variable | Purpose | Default |
| -------- | ------- | ------- |
| `POSTGRES_URL` | PostgreSQL connection string | `postgres://postgres:postgres@localhost:5432/food_delivery?sslmode=disable` |
| `REDIS_ADDR` | Redis endpoint | `localhost:6379` |
| `EVENTS_NATS_URL` | NATS connection string (event bus) | `nats://localhost:4222` |
| `HTTP_PORT` | HTTP listen port | `8080` |

### Make Targets

| Command | Description |
| ------- | ----------- |
| `make run` | Start the API server (`go run ./cmd/api`) |
| `make test` | Run unit tests (`go test ./...`) |
| `make lint` | Execute `golangci-lint` (ensure binary installed) |
| `make tidy` | Sync Go modules (`go mod tidy`) |
| `make build` | Compile binary to `bin/food-delivery-backend` |

## Project Layout

```
cmd/api/                # Service entry point
internal/
  app/                  # Application bootstrap & lifecycle management
  config/               # Environment-backed configuration loader
  http/                 # Handlers, router, and HTTP transport logic
  platform/             # Infrastructure adapters (database, cache, events)
  shared/               # Cross-cutting logger & middleware utilities
```

Domain packages (auth, orders, catalog, riders, payments, etc.) will live under `internal/domain/` in subsequent iterations, each with command handlers, repositories, and DTOs.

## Documentation

Additional documentation lives in `docs/` and is indexed by [`docs/documentation-guide.md`](docs/documentation-guide.md):

- [`docs/architecture.md`](docs/architecture.md) – service architecture, module boundaries, integration points
- [`docs/development-workflow.md`](docs/development-workflow.md) – local setup, branching, CI/CD expectations
- [`docs/testing-strategy.md`](docs/testing-strategy.md) – testing pyramid, tooling, coverage goals

## Community & Governance

- [`CONTRIBUTING.md`](CONTRIBUTING.md) – contribution workflow, coding standards
- [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md) – inclusive collaboration expectations
- [`SECURITY.md`](SECURITY.md) – vulnerability disclosure process
- [`SUPPORT.md`](SUPPORT.md) – support and escalation channels
- [`CHANGELOG.md`](CHANGELOG.md) – semantic versioning log for releases

## Roadmap Next Steps

- Define proto/OpenAPI contracts for `/v1/orders`, `/v1/payments`
- Implement domain modules with test-first approach and repository interfaces
- Add telemetry instrumentation (OTEL exporter) and request metrics
- Integrate background worker infrastructure for fulfillment and notifications
