# Tagging and Versioning Guide

## Repository Structure

**Important:** `shared-auth-client` is an **independent GitHub repository** (`github.com/Bengo-Hub/shared-auth-client`) in the Bengo-Hub organization. Each BengoBox service (ordering-backend, notifications-api, etc.) is also an independent repository. The `BengoBox` folder is just a local root directory where developers clone repositories - it is **not** a monorepo.

## Tagging the Library

### Step 1: Tag the Repository

Tag the `shared-auth-client` repository:

```bash
# In the shared-auth-client repository
cd shared-auth-client/
git tag v0.1.0
git push origin v0.1.0
```

### Step 2: Update Service go.mod Files

Each service should import the library without `replace` directives:

```go
require (
    github.com/Bengo-Hub/shared-auth-client v0.1.0
)
```

**Note:** For local development, developers can use `go.work` at the `BengoBox` root to link all cloned repositories together.

### Step 3: Verify

```bash
# In any service repository
go mod tidy
go build ./cmd/api
```

## Local Development Setup

When developing locally, clone all repositories into a parent directory:

```bash
# Create parent directory
mkdir -p BengoBox
cd BengoBox/

# Clone all repositories
git clone https://github.com/Bengo-Hub/ordering-backend.git ordering-service/ordering-backend
git clone https://github.com/Bengo-Hub/notifications-api.git notifications-service/notifications-api
git clone https://github.com/Bengo-Hub/treasury-api.git finance-service/treasury-api
git clone https://github.com/Bengo-Hub/inventory-api.git inventory-service/inventory-api
git clone https://github.com/Bengo-Hub/pos-api.git pos-service/pos-api
git clone https://github.com/Bengo-Hub/logistics-api.git logistics-service/logistics-api
git clone https://github.com/Bengo-Hub/shared-auth-client.git shared/auth-client

# Create go.work at BengoBox root
cd BengoBox/
go work init \
  ./ordering-service/ordering-backend \
  ./notifications-service/notifications-api \
  ./finance-service/treasury-api \
  ./inventory-service/inventory-api \
  ./pos-service/pos-api \
  ./logistics-service \
  ./shared/auth-client
```

This allows local development without needing to fetch from GitHub each time.

## Production Deployment

In production and CI/CD:

1. **No `replace` directives** - services import directly from GitHub
2. **Tagged versions** - always use specific versions (e.g., `v0.1.0`)
3. **Private module access** - configure `GOPRIVATE` and git credentials

## Versioning Strategy

- **v0.MAJOR.MINOR** for pre-1.0 releases
- **v1.0.0** for stable API
- Increment MAJOR for breaking changes
- Increment MINOR for new features (backward compatible)
- Increment PATCH for bug fixes

## Current Version

**v0.1.0** - Initial release with JWKS validation, middleware, and production-ready features.

## Updating Services to Use New Versions

When releasing a new version:

1. **Tag the new version:**
   ```bash
   cd shared-auth-client/
   git tag v0.2.0
   git push origin v0.2.0
   ```

2. **Update each service:**
   ```bash
   cd cafe-backend/
   go get github.com/Bengo-Hub/shared-auth-client@v0.2.0
   go mod tidy
   ```

3. **Test and deploy** each service independently
