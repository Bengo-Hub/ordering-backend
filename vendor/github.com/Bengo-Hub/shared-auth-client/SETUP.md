# Setup Guide for Shared Auth Client

## Repository Structure

**Important:** `shared-auth-client` is an **independent GitHub repository** that must be created in the `Bengo-Hub` organization.

**Repository URL:** `https://github.com/Bengo-Hub/shared-auth-client`

## Initial Setup Steps

### Step 1: Create GitHub Repository

1. Go to the Bengo-Hub organization on GitHub
2. Create a new repository named `shared-auth-client`
3. Make it private (recommended) or public
4. **Do not** initialize with README, .gitignore, or license (we already have these)

### Step 2: Push Code to Repository

```bash
# In the shared/auth-client directory
cd shared/auth-client

# Initialize git if not already done
git init
git add .
git commit -m "Initial commit: shared-auth-client v0.1.0"

# Add remote and push
git remote add origin https://github.com/Bengo-Hub/shared-auth-client.git
git branch -M main
git push -u origin main
```

### Step 3: Tag the First Version

```bash
# Tag v0.1.0
git tag v0.1.0
git push origin v0.1.0
```

### Step 4: Update Service Repositories

Once the repository is created and tagged, update each service:

1. **Remove replace directives** from `go.mod`:
   ```bash
   # In each service directory
   go mod edit -dropreplace=github.com/Bengo-Hub/shared-auth-client
   ```

2. **Verify imports work:**
   ```bash
   go mod tidy
   go build ./cmd/api
   ```

## Local Development Workflow

### Option A: Using Go Workspace (Recommended)

1. Clone all repositories into a parent directory:
   ```bash
   mkdir -p BengoBox
   cd BengoBox/
   
   git clone https://github.com/Bengo-Hub/shared-auth-client.git shared/auth-client
   git clone https://github.com/Bengo-Hub/ordering-backend.git ordering-service/ordering-backend
   git clone https://github.com/Bengo-Hub/notifications-api.git notifications-service/notifications-api
   git clone https://github.com/Bengo-Hub/treasury-api.git finance-service/treasury-api
   git clone https://github.com/Bengo-Hub/inventory-api.git inventory-service/inventory-api
   git clone https://github.com/Bengo-Hub/pos-api.git pos-service/pos-api
   git clone https://github.com/Bengo-Hub/logistics-api.git logistics-service/logistics-api
   ```

2. Create `go.work` at BengoBox root:
   ```bash
   cd BengoBox/
   go work init \
     ./shared/auth-client \
     ./ordering-service/ordering-backend \
     ./notifications-service/notifications-api \
     .finance-service/treasury-api \
     ./inventory-service/inventory-api \
     ./pos-service/pos-api \
     ./logistics-service/logistics-api
   ```

3. **Keep replace directives** in service `go.mod` files for local development

4. `go.work` will automatically use local paths

### Option B: Using Replace Directives Only

1. Clone repositories as above
2. Keep `replace` directives in each service's `go.mod`
3. Works without `go.work` but less convenient

### Option C: Direct Import (Production)

1. Remove `replace` directives from `go.mod`
2. Services fetch from GitHub directly
3. Requires repository to exist and be tagged
4. Use in CI/CD and production deployments

## CI/CD Configuration

For production builds, configure private module access:

```yaml
- name: Configure private modules
  run: |
    git config --global url."https://${{ secrets.GITHUB_TOKEN }}@github.com/".insteadOf "https://github.com/"
    export GOPRIVATE=github.com/Bengo-Hub/*
    export GONOPROXY=github.com/Bengo-Hub/*
    export GONOSUMDB=github.com/Bengo-Hub/*
```

## Current Status

- ✅ Module path updated: `github.com/Bengo-Hub/shared-auth-client`
- ✅ All service imports updated
- ⏭️ **TODO:** Create GitHub repository `Bengo-Hub/shared-auth-client`
- ⏭️ **TODO:** Push code and tag as `v0.1.0`
- ⏭️ **TODO:** Remove replace directives from service `go.mod` files

## Next Steps

1. Create the GitHub repository
2. Push the code
3. Tag as `v0.1.0`
4. Remove replace directives from all services
5. Test builds in CI/CD

