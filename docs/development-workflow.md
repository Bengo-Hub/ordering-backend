# Development Workflow

## Prerequisites

- Go 1.22+
- PostgreSQL 14+, Redis 7+, NATS JetStream (optional for local dev)
- golangci-lint (install via `go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest`)

## Local Setup

```bash
cp config/app.env.example .env
make tidy
make run
```

- `FOOD_DELIVERY_POSTGRES_URL` should point to your local database (Docker compose recommended)
- Integration tests will later rely on Testcontainers for ephemeral services

## Branch Strategy

- Trunk-based development with feature branches: `feature/{ticket}`, `fix/{issue}`
- Use Conventional Commits to drive semantic releases (e.g. `feat(orders): add create order use case`)
- Pull requests require:
  - Passing lint + tests (`make lint && make test`)
  - Updated docs if APIs or workflows change
  - Recorded rollout/rollback plan

## CI/CD Stages

1. **Lint & Test:** golangci-lint, go test ./...
2. **Build:** Multi-stage Docker image -> push to registry
3. **Security:** Snyk/Trivy scans, dependency audit
4. **Deploy:** ArgoCD sync to dev/staging/prod clusters

## Tooling Notes

- Use `air` or `templ` for hot reload if desired (add to `tools/` later)
- SQL migrations managed by Atlas or Goose (TBD) under `db/migrations`
- Observability instrumentation added through OpenTelemetry in `internal/platform/telemetry`
