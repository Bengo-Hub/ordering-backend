# Production Deployment Guide

## Overview

The `shared-auth-client` library is used across all BengoBox microservices for JWT validation and authentication. This library is published as an independent GitHub repository (`github.com/Bengo-Hub/shared-auth-client`) in the Bengo-Hub organization.

## Architecture Context

**Important:** Each BengoBox service (ordering-backend, notifications-api, treasury-api, inventory-api, pos-api, logistics-api) is an **independent GitHub repository** in the `Bengo-Hub` organization. The `BengoBox` folder is just a local root directory where developers clone all repositories for local development - it is **not** a monorepo.

## Module Structure

```
shared-auth-client/  (Independent GitHub repository)
├── go.mod                    # Module: github.com/Bengo-Hub/shared-auth-client
├── claims.go                 # JWT claims structure
├── validator.go              # JWKS-based token validation
├── middleware.go             # HTTP middleware (chi)
├── gin.go                    # Gin-specific middleware
└── README.md                 # Usage documentation
```

## Deployment Strategies

### Option 1: Go Workspace (Local Development) ✅ Recommended for Local

When developing locally, clone all repositories into a parent directory (e.g., `BengoBox/`) and use `go.work`:

```bash
# Clone all repositories
cd BengoBox/
git clone https://github.com/Bengo-Hub/ordering-backend.git ordering-service/ordering-backend
git clone https://github.com/Bengo-Hub/notifications-api.git notifications-service/notifications-api
git clone https://github.com/Bengo-Hub/shared-auth-client.git shared/auth-client
# ... clone other services

# Create go.work at BengoBox root
cd BengoBox/
go work init ./ordering-service/ordering-backend ./notifications-service/notifications-api ./shared/auth-client
```

**Pros:**
- No code changes needed
- Works seamlessly with `go build`, `go test`, `go run`
- All modules see each other automatically
- Perfect for local development

**Cons:**
- Only works when all repos are cloned in the same parent directory
- Not used in production/CI/CD

### Option 2: Git-Based Import (Production) ✅ Recommended for Production

Since all services are in the same GitHub organization (`Bengo-Hub`), use git-based imports with tagged versions:

#### Setup Steps:

1. **Tag the library repository:**
   ```bash
   cd shared-auth-client/
   git tag v0.1.0
   git push origin v0.1.0
   ```

2. **Service go.mod** (no replace directive needed):
   ```go
   require (
       github.com/Bengo-Hub/shared-auth-client v0.1.0
   )
   ```

3. **Configure Git credentials** (for private repos in CI/CD):
   ```bash
   git config --global url."https://${GITHUB_TOKEN}@github.com/".insteadOf "https://github.com/"
   ```

4. **In CI/CD pipelines**, set:
   ```bash
   export GOPRIVATE=github.com/Bengo-Hub/*
   export GONOPROXY=github.com/Bengo-Hub/*
   export GONOSUMDB=github.com/Bengo-Hub/*
   ```

5. **For local development**, if you don't use `go.work`, you can still use:
   ```bash
   go get github.com/Bengo-Hub/shared-auth-client@v0.1.0
   ```

## CI/CD Configuration

### GitHub Actions Example

```yaml
- name: Setup Go
  uses: actions/setup-go@v4
  with:
    go-version: '1.24'
  
- name: Configure private modules
  run: |
    git config --global url."https://${{ secrets.GITHUB_TOKEN }}@github.com/".insteadOf "https://github.com/"
    export GOPRIVATE=github.com/Bengo-Hub/*
    export GONOPROXY=github.com/Bengo-Hub/*
    export GONOSUMDB=github.com/Bengo-Hub/*

- name: Build
  run: go build ./cmd/api
```

### Dockerfile Example

```dockerfile
FROM golang:1.24-alpine AS builder

# Configure private modules
ARG GITHUB_TOKEN
RUN git config --global url."https://${GITHUB_TOKEN}@github.com/".insteadOf "https://github.com/"
ENV GOPRIVATE=github.com/Bengo-Hub/*
ENV GONOPROXY=github.com/Bengo-Hub/*
ENV GONOSUMDB=github.com/Bengo-Hub/*

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o api ./cmd/api

FROM alpine:latest
COPY --from=builder /app/api /api
CMD ["/api"]
```

## Versioning Strategy

- **Semantic Versioning**: Follow `v0.MAJOR.MINOR` for pre-1.0 releases
- **Breaking Changes**: Increment MAJOR version
- **New Features**: Increment MINOR version
- **Bug Fixes**: Increment PATCH version

Example:
- `v0.1.0` - Initial release
- `v0.1.1` - Bug fixes
- `v0.2.0` - New features (backward compatible)
- `v1.0.0` - Stable API

## Service Integration Checklist

Each service integrating `shared-auth-client` should:

- [ ] Add proper version constraint in `go.mod`: `github.com/Bengo-Hub/shared-auth-client v0.1.0`
- [ ] Remove any `replace` directives (for production)
- [ ] Configure CI/CD for private module access
- [ ] Test authentication flow end-to-end
- [ ] Document auth-service URL and JWKS endpoint
- [ ] Set up monitoring for JWKS fetch failures
- [ ] Configure proper caching (JWKS cache TTL)

## Environment Variables

Each service needs these environment variables:

```bash
# Auth Service Configuration
AUTH_SERVICE_URL=https://sso.codevertexitsolutions.com
AUTH_ISSUER=https://sso.codevertexitsolutions.com
AUTH_AUDIENCE=codevertex
AUTH_JWKS_URL=https://sso.codevertexitsolutions.com/api/v1/.well-known/jwks.json

# Optional: Cache configuration
AUTH_JWKS_CACHE_TTL=3600s
AUTH_JWKS_REFRESH_INTERVAL=300s
```

## Monitoring & Observability

Monitor these metrics:
- JWKS fetch success/failure rate
- JWKS cache hit/miss ratio
- Token validation latency
- Token validation failure reasons
- JWKS refresh interval compliance

## Troubleshooting

### Issue: `cannot find module providing package`
**Solution**: Ensure `GOPRIVATE` is set and git credentials are configured. Verify the repository exists at `github.com/Bengo-Hub/shared-auth-client`.

### Issue: `401 Unauthorized` when fetching JWKS
**Solution**: Verify auth-service is accessible and JWKS endpoint is public.

### Issue: `key not found in JWKS`
**Solution**: Check if auth-service has rotated keys. JWKS should auto-refresh, but verify refresh interval.

### Issue: `invalid issuer` or `invalid audience`
**Solution**: Verify `AUTH_ISSUER` and `AUTH_AUDIENCE` match token claims.

## Migration Path

### From Local Replace to Production Module

1. **Tag the library:**
   ```bash
   cd shared-auth-client
   git tag v0.1.0
   git push origin v0.1.0
   ```

2. **Update each service:**
   ```bash
   # Remove replace directive
   # Update require to use version
   go mod edit -require=github.com/Bengo-Hub/shared-auth-client@v0.1.0
   go mod edit -dropreplace=github.com/Bengo-Hub/shared-auth-client
   go mod tidy
   ```

3. **Test locally** (use go.work for development if needed)

4. **Deploy** with CI/CD configured for private modules

## Best Practices

1. **Always use tagged versions** in production (never `@latest` or `@main`)
2. **Pin versions** in `go.mod` for reproducible builds
3. **Test locally** with `go.work` before deploying
4. **Monitor JWKS** fetch failures and cache performance
5. **Document breaking changes** in CHANGELOG.md
6. **Keep backward compatibility** within MAJOR versions
