# Contributing

Thanks for helping improve the Food Delivery backend. This guide explains how to collaborate safely and consistently.

## Prerequisites

- Go 1.22+
- golangci-lint installed locally
- Docker (optional) for Postgres/Redis/NATS test containers
- Familiarity with clean architecture and domain-driven design patterns

## Workflow

1. Create a feature branch from `main`: `git checkout -b feature/{ticket}`
2. Sync dependencies: `make tidy`
3. Implement changes with corresponding tests (`make test`)
4. Run linting (`make lint`) and ensure gofmt/goimports are clean
5. Update docs (`docs/`) or API contracts when behaviour changes
6. Commit using Conventional Commits: `feat(orders): introduce acceptance flow`
7. Open a pull request with:
   - Summary + rationale
   - Test evidence (logs, coverage report)
   - Rollout/rollback notes and feature flag identifiers

## Coding Standards

- Keep business rules in domain packages; handlers should remain thin
- Prefer context-aware functions (`ctx context.Context`) to propagate deadlines
- Use `errors.Join`/`fmt.Errorf` with wrapping to preserve stack context
- Avoid global state; rely on explicit dependency injection via constructors
- Instrument code with structured logs and metrics before shipping

## Testing Expectations

- Table-driven unit tests with clear input/output
- Mock or use Testcontainers for external services (DB, cache, messaging)
- Keep coverage ≥ 80%; highlight risk areas that lack tests in PRs
- Property-based tests encouraged for pricing/dispatch algorithms

## Documentation

- Update `docs/architecture.md` or `docs/api-contracts.md` when service boundaries or contracts evolve
- Reflect new operational runbooks in `docs/development-workflow.md`
- Ensure `docs/documentation-guide.md` references newly added files

By contributing you agree to the [Code of Conduct](CODE_OF_CONDUCT.md).
