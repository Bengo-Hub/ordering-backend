# Testing Strategy

## Pyramid

1. **Unit Tests (60%)** – Table-driven Go tests for domain logic, handlers with mocks (stretch: use Testify/GoMock)
2. **Integration Tests (25%)** – Testcontainers for Postgres/Redis/NATS validating repository and messaging flows
3. **Contract Tests (10%)** – Pact/Buf for ensuring API compatibility with frontend, treasury, notifications
4. **Performance Tests (5%)** – k6 scenarios for checkout/fulfilment SLAs

## Tooling

- `go test ./...` with race detector for critical packages
- `github.com/stretchr/testify` for assertions & test suites
- `github.com/testcontainers/testcontainers-go` (planned) for disposable dependencies
- `golangci-lint` enforces vet, staticcheck, gofmt, goimports
- `buf` for gRPC/protobuf linting (once proto modules land)

## Conventions

- Place unit tests alongside source files: `handler_test.go`
- Use dependency injection for repositories and clients to ease mocking
- Favour context-aware APIs to control timeouts in tests
- Keep fixtures under `testdata/` with anonymised sample payloads

## CI Expectations

- Minimum coverage target: 80% statements, 70% branches (enforced via `go test -coverprofile`)
- Performance suites run nightly and before major releases
- Security scans (gosec) integrated into lint pipeline before GA
