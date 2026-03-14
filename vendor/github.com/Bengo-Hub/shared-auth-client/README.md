# Shared Auth Client Library

A Go library for validating JWT tokens issued by auth-service using JWKS. This library is shared across all BengoBox microservices for consistent authentication and authorization.

**Repository:** `github.com/Bengo-Hub/shared-auth-client`

## Installation

### Production (Recommended)

Import as a Go module in your service:

```go
require (
    github.com/Bengo-Hub/shared-auth-client v0.1.0
)
```

### Local Development (Go Workspace)

When developing locally, clone all repositories into a parent directory (e.g., `BengoBox/`) and use `go.work`:

```bash
# Clone repositories
cd BengoBox/
git clone https://github.com/Bengo-Hub/shared-auth-client.git shared/auth-client
git clone https://github.com/Bengo-Hub/ordering-backend.git ordering-service/ordering-backend
git clone https://github.com/Bengo-Hub/notifications-api.git notifications-service/notifications-api
# ... clone other services

# Create go.work at BengoBox root
cd BengoBox/
go work init ./shared/auth-client ./ordering-service/ordering-backend ./notifications-service/notifications-api
```

See [DEPLOYMENT.md](./DEPLOYMENT.md) for detailed deployment strategies and CI/CD configuration.

## Usage

### Chi Router

```go
import (
    authclient "github.com/Bengo-Hub/shared-auth-client"
)

// Initialize validator
config := authclient.DefaultConfig(
    "https://sso.codevertexitsolutions.com/api/v1/.well-known/jwks.json",
    "https://sso.codevertexitsolutions.com",
    "codevertex",
)
validator, err := authclient.NewValidator(config)
if err != nil {
    log.Fatal(err)
}
defer validator.Stop()

// Create middleware
authMiddleware := authclient.NewAuthMiddleware(validator)

// Apply to routes
router.Route("/api/v1", func(r chi.Router) {
    r.Use(authMiddleware.RequireAuth)
    // Protected routes...
})

// Extract claims in handler
claims, ok := authclient.ClaimsFromContext(r.Context())
if !ok {
    http.Error(w, "unauthorized", http.StatusUnauthorized)
    return
}

userID, _ := claims.UserID()
tenantID, _ := claims.TenantUUID()
```

### Gin Router

```go
import (
    authclient "github.com/Bengo-Hub/shared-auth-client"
    "github.com/gin-gonic/gin"
)

// Initialize validator (same as above)
validator, _ := authclient.NewValidator(config)
authMiddleware := authclient.NewAuthMiddleware(validator)

// Apply middleware
router.Use(authclient.GinMiddleware(authMiddleware))

// Extract claims
claims, ok := authclient.ClaimsFromGinContext(c)
if !ok {
    c.AbortWithStatusJSON(401, gin.H{"error": "unauthorized"})
    return
}

// Require specific scopes
router.Use(authclient.GinRequireScope("read:orders", "write:orders"))
```

### API Key Authentication (Fallback)

Services can optionally enable API key authentication as a fallback when JWT tokens are not provided:

```go
import (
    authclient "github.com/Bengo-Hub/shared-auth-client"
)

// Initialize JWT validator
validator, _ := authclient.NewValidator(config)

// Initialize API key validator (optional)
apiKeyValidator := authclient.NewAPIKeyValidator("https://auth.codevertex.local:4101", nil)

// Create middleware with API key fallback
authMiddleware := authclient.NewAuthMiddlewareWithAPIKey(validator, apiKeyValidator)

// Apply middleware (will accept both JWT Bearer tokens and X-API-Key headers)
router.Use(authclient.GinMiddleware(authMiddleware))
```

**API Key Format:**
- API keys are generated via `POST /api/v1/admin/api-keys` in auth-service
- Clients send API keys in the `X-API-Key` header
- API keys are validated against auth-service and cached for 5 minutes
- API keys return synthetic claims with `tenant_id` and `scopes` from the key configuration

## Features

- ✅ JWKS fetching and caching with automatic refresh
- ✅ RS256 signature validation
- ✅ Issuer and audience validation
- ✅ Scope checking helpers (`HasScope`, `HasAnyScope`, `HasAllScopes`)
- ✅ HTTP middleware for chi and gin routers
- ✅ Production-ready with error handling and observability hooks
- ✅ Thread-safe key caching with singleflight deduplication
- ✅ `SyncUser` method for syncing identities across services

## Configuration

```go
config := authclient.Config{
    JWKSUrl:         "https://sso.codevertexitsolutions.com/api/v1/.well-known/jwks.json",
    Issuer:          "https://sso.codevertexitsolutions.com",
    Audience:        "codevertex",
    CacheTTL:        1 * time.Hour,        // How long to cache JWKS
    RefreshInterval: 5 * time.Minute,      // Background refresh interval
    HTTPClient:      &http.Client{Timeout: 10 * time.Second},
}
```

## Deployment

See [DEPLOYMENT.md](./DEPLOYMENT.md) for:
- Production deployment strategies
- CI/CD configuration examples
- Versioning strategy
- Troubleshooting guide

