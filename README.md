# Food Delivery Backend

Go-based service that orchestrates ordering, logistics, payments, and cafe operations for the BengoBox Urban Café ecosystem. Built with clean architecture, contract-first APIs, and event-driven integrations with treasury and notifications services.

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
- Treasury (`treasury-app`) and Notifications (`notifications-app`) services occupy ports **4001** and **4002** respectively when running locally.
- In cloud/cluster deployments, all backend services are configured to listen on **port 4000** for ingress uniformity. Helm values set `FOOD_DELIVERY_HTTP_PORT` to override the default.

### Required Environment Variables

All variables are prefixed with `FOOD_DELIVERY_` to avoid collisions.

| Variable | Purpose | Default |
| -------- | ------- | ------- |
| `FOOD_DELIVERY_POSTGRES_URL` | PostgreSQL connection string | `postgres://postgres:postgres@localhost:5432/food_delivery?sslmode=disable` |
| `FOOD_DELIVERY_REDIS_ADDR` | Redis endpoint | `localhost:6379` |
| `FOOD_DELIVERY_NATS_URL` | NATS connection string | `nats://localhost:4222` |
| `FOOD_DELIVERY_HTTP_PORT` | HTTP listen port | `8080` |

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
