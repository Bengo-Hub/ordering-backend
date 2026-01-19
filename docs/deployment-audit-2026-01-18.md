# Ordering Backend Deployment Audit
**Date**: 2026-01-18
**Status**: Issues Identified & Documented

---

## Executive Summary

Sprint 7 (Analytics, Compliance & Hardening) has been successfully completed and committed. The deployment pipeline and Kubernetes configuration have been audited, revealing several issues that need attention. Superset is healthy and running, but the ordering-backend deployment is failing due to Docker registry authentication issues.

---

## Completed Work

### 1. Sprint 7 Implementation ✅
- **Security Hardening**:
  - Redis-based rate limiting with sliding window algorithm
  - OWASP-compliant security headers
  - SQL injection and XSS pattern detection
  - Request size limiting and Content-Type validation
  - Tenant ID format validation

- **Performance Optimization**:
  - Database indexes added to User, Order, Payment, NotificationEvent schemas
  - Connection pooling made fully configurable
  - Redis caching service with TTL configuration

- **Analytics & Compliance**:
  - Superset client integration
  - Analytics module and handlers
  - Compliance module (GDPR/DPA)
  - Data export and deletion services

### 2. Git Status ✅
- **Commit**: `2cb3b724` - "feat(sprint-7): implement security hardening and performance optimization"
- **Pushed**: Successfully to `main` branch
- **Changed Files**: 73 files (22,547 insertions, 271 deletions)

---

## Current Deployment Status

### Infrastructure Overview

| Service | Namespace | Status | Replicas | Notes |
|---------|-----------|--------|----------|-------|
| Superset | `infra` | ✅ Healthy | 1/1 | Running v3.1.0 |
| Superset Worker | `infra` | ⚠️ Disabled | 0/0 | Not scaled up |
| Superset Beat | `infra` | ⚠️ Disabled | 0/0 | Not scaled up |
| Redis | `infra` | ✅ Healthy | - | Shared instance |
| PostgreSQL | `infra` | ✅ Healthy | - | Shared instance |
| ordering-backend | `ordering` | ❌ Failed | 0/1 | ErrImagePull |
| ordering-frontend | `ordering` | ❌ Failed | 0/1 | CrashLoopBackOff |
| cafe-website | `cafe` | ✅ Healthy | 1/1 | Running fine |

### No Legacy Resources
✅ No old `cafe-backend` resources found in the cluster

---

## Issues Identified

### Issue 1: Docker Registry Authentication ❌ CRITICAL - **ROOT CAUSE FOUND**
**Status**: Blocking deployments - **Solution Identified**

**Problem**:
```
unauthorized: access token has insufficient scopes
```

**Details**:
- GitHub Actions successfully builds the Docker image
- Image push fails during the "Run production deployment" step
- Build SHA: `2cb3b724`
- Target image: `docker.io/codevertex/ordering-backend:2cb3b724`
- **auth-api and erp-api deployments work fine with same credentials**

**Root Cause Identified** ✅:
**The Docker Hub repository `codevertex/ordering-backend` does not exist!**

Verification:
```bash
# ordering-backend - NOT FOUND
$ curl -s "https://hub.docker.com/v2/repositories/codevertex/ordering-backend/"
{"message":"object not found","errinfo":{}}

# auth-api - EXISTS (253 pulls, last updated 2026-01-18)
$ curl -s "https://hub.docker.com/v2/repositories/codevertex/auth-api/"
{"name":"auth-api","namespace":"codevertex","status":1}
```

**Why auth-api and erp-api succeed**:
- Their Docker Hub repositories already exist
- The `REGISTRY_PASSWORD` token can push to existing repos

**Why ordering-backend fails**:
- Repository doesn't exist (first deployment)
- Token lacks "create repository" permission
- Docker Hub returns "insufficient scopes" when pushing to non-existent repo

**Impact**:
- Cannot deploy ordering-backend until repository is created
- Current cluster has 0/1 ordering-backend pods

**Fix Required** (SIMPLE - 5 minutes):
1. **Log into Docker Hub as `codevertex` user**
2. **Create repository**: https://hub.docker.com/repository/create
   - Name: `ordering-backend`
   - Visibility: Public (match auth-api)
3. **Re-run GitHub Actions workflow**: `gh run rerun 21113419024`

**Detailed Fix Documentation**: See [deployment-fix-docker-hub.md](./deployment-fix-docker-hub.md)

**Location**:
- Docker Hub: https://hub.docker.com/repository/create
- GitHub Workflow: https://github.com/Bengo-Hub/ordering-backend/actions/runs/21113419024

---

### Issue 2: ordering-backend Pod - ImagePullBackOff ❌
**Status**: Cannot start

**Current State**:
```bash
$ kubectl get pods -n ordering
NAME                                 READY   STATUS             RESTARTS   AGE
ordering-backend-7cd7f8d97d-s8knr    0/1     ErrImagePull       0          3m
```

**Error Details**:
```
Failed to pull image "docker.io/codevertex/ordering-backend:latest":
rpc error: code = NotFound desc = failed to pull and unpack image:
docker.io/codevertex/ordering-backend:latest: not found
```

**Root Cause**:
- Image `docker.io/codevertex/ordering-backend:latest` does not exist in Docker Hub
- Previous deployments failed to push the `:latest` tag
- ArgoCD deployment references `:latest` in values.yaml

**Fix Required**:
1. Fix Docker registry authentication (Issue 1)
2. Re-run GitHub Actions to build and push image
3. Wait for ArgoCD to sync and pull the new image

---

### Issue 3: ordering-frontend - CrashLoopBackOff ❌
**Status**: Application crashing

**Current State**:
```bash
NAME                                 READY   STATUS             RESTARTS      AGE
ordering-frontend-6478c4647c-fzclv   0/1     CrashLoopBackOff   313           19h
```

**Analysis Needed**:
- Application has been crashing for 19 hours (313 restarts)
- Need to check application logs to identify crash reason
- Possible causes:
  - Backend API connection issues (ordering-backend is down)
  - Environment variable misconfiguration
  - Missing dependencies or build issues

**Next Steps**:
```bash
# Check logs
kubectl logs -n ordering ordering-frontend-6478c4647c-fzclv --tail=100

# Check environment variables
kubectl describe pod -n ordering ordering-frontend-6478c4647c-fzclv

# Check if frontend is dependent on backend being available
```

---

### Issue 4: Superset Workers/Beat Not Scaled ⚠️
**Status**: Non-critical (analytics backend)

**Current State**:
- `superset-worker`: 0/0 replicas
- `superset-celerybeat`: 0/0 replicas

**Impact**:
- Asynchronous query execution disabled
- Scheduled reports/cache warming disabled
- Only synchronous queries work

**Decision Needed**:
- Are async queries and scheduled reports needed now?
- If yes, scale workers:
  ```bash
  kubectl scale deployment superset-worker -n infra --replicas=2
  kubectl scale deployment superset-celerybeat -n infra --replicas=1
  ```

---

## Superset Integration Status

### Deployment Health ✅
- **Pod**: `superset-6bd6966ddb-dhw4b` - Running (1/1)
- **Image**: `apache/superset:3.1.0`
- **Uptime**: 13 days (restarted once on 2026-01-05)
- **Health Checks**: All passing
  - Startup probe: ✅
  - Readiness probe: ✅
  - Liveness probe: ✅

### Configuration ✅
- **Database**: PostgreSQL (shared instance in `infra` namespace)
- **Cache**: Redis (shared instance in `infra` namespace)
- **Ingress**: `superset.codevertexitsolutions.co.ke` (needs proper DNS/host config)
- **Resources**:
  - Requests: 200m CPU, 512Mi memory
  - Limits: 1000m CPU, 2Gi memory

### Integration with ordering-backend
**Requirements** (from [superset/client.go:1-95](../internal/platform/superset/client.go#L1-L95)):
```go
type Config struct {
    BaseURL        string        // Superset instance URL
    Username       string        // Service account username
    Password       string        // Service account password
    Provider       string        // Auth provider (default: "db")
    AccessTokenTTL time.Duration // Token TTL
    RefreshTokenTTL time.Duration // Refresh token TTL
}
```

**Environment Variables Needed**:
```bash
SUPERSET_BASE_URL=http://superset.infra.svc.cluster.local:8088
SUPERSET_USERNAME=ordering_service
SUPERSET_PASSWORD=<secure-password>
SUPERSET_AUTH_PROVIDER=db
```

**Next Steps for Integration**:
1. Create Superset service account for ordering-backend
2. Configure database connection in Superset for ordering PostgreSQL
3. Set up dashboard for ordering analytics
4. Add environment variables to `ordering-backend-secrets`
5. Test embed dashboard API endpoints

---

## Database Schema & Vector Columns

### Current Schema Status
The ordering-backend uses Ent ORM with the following schemas:

**Core Entities** (with indexes added in Sprint 7):
- `users` - 9 indexes (tenant, email, status, role, auth sync)
- `orders` - 11 indexes (status, time-based, analytics)
- `payments` - 7 indexes (provider, status, time-based)
- `notification_events` - 3 indexes (retry queue, status)

**Vector Columns**:
Currently, the schema does NOT include vector columns. However, the Superset database setup script enables the `pgvector` extension:

```sql
-- From create-superset-database.sh
CREATE EXTENSION IF NOT EXISTS vector;
```

### Future Analytics Enhancements
**Potential Vector Use Cases** (for future sprints):
1. **Menu Item Embeddings**:
   - Store vector embeddings of menu item descriptions
   - Enable semantic search for similar menu items
   - Recommend items based on similarity

2. **Customer Behavior Vectors**:
   - Encode customer ordering patterns
   - Cluster customers by behavior
   - Personalized recommendations

3. **Order Pattern Analysis**:
   - Time-series embeddings for order trends
   - Anomaly detection in order volumes
   - Predictive analytics for demand forecasting

**Implementation Note**:
Vector columns would require:
- Adding `vector` field type to Ent schemas
- Installing `pgvector` extension on ordering database
- Implementing embedding generation logic
- Updating analytics queries to use vector operations

---

## DevOps Configuration Audit

### ArgoCD Application ✅
**Location**: `devops-k8s/apps/ordering-backend/app.yaml`

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: ordering-backend
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://github.com/Bengo-Hub/devops-k8s.git
    targetRevision: main
    path: charts/app
    helm:
      valueFiles:
        - ../../apps/ordering-backend/values.yaml
  destination:
    server: https://kubernetes.default.svc
    namespace: ordering
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
```

**Status**: Configuration is correct ✅

### Helm Values ✅
**Location**: `devops-k8s/apps/ordering-backend/values.yaml`

**Key Configurations**:
- Image: `docker.io/codevertex/ordering-backend:latest`
- Port: 4000
- Health checks: `/healthz` endpoint
- Migrations: Enabled (runs `migrate` binary)
- Seeding: Enabled (runs `seed` binary)
- Autoscaling: 1 min, 1 max (resource-constrained)

**Issues Found**:
- ⚠️ Using `:latest` tag (should use specific SHA or version)
- ⚠️ Autoscaling min/max both set to 1 (effectively disabled)

**Recommended Changes**:
```yaml
image:
  tag: "2cb3b724"  # Use specific SHA instead of :latest

autoscaling:
  enabled: true
  minReplicas: 2  # For HA
  maxReplicas: 4  # Allow scaling under load
```

### GitHub Actions Workflow ✅
**Location**: `.github/workflows/deploy.yml`

**Pipeline Steps**:
1. ✅ Checkout code
2. ✅ Set up Docker Buildx
3. ✅ Install DevOps Tools
4. ✅ Set deployment variables
5. ❌ Run production deployment (FAILS at docker push)
6. ⏭️ Tag and push :latest (SKIPPED)
7. ⏭️ Check ArgoCD status (SKIPPED)
8. ⏭️ Verify API health (SKIPPED)

**Issue**: Step 5 fails due to Docker registry authentication

---

## Action Items

### Immediate (P0) - Fix Deployments
1. **Update Docker Hub Credentials**
   - [ ] Generate new Docker Hub access token with push permissions
   - [ ] Update `REGISTRY_PASSWORD` in GitHub repository secrets
   - [ ] Re-run failed GitHub Actions workflow

2. **Fix ordering-backend Deployment**
   - [ ] Wait for successful image build and push
   - [ ] Verify image exists: `docker pull docker.io/codevertex/ordering-backend:2cb3b724`
   - [ ] Monitor ArgoCD sync
   - [ ] Check pod status: `kubectl get pods -n ordering`

3. **Investigate ordering-frontend Crashes**
   - [ ] Get crash logs: `kubectl logs -n ordering ordering-frontend-6478c4647c-fzclv --tail=200`
   - [ ] Check environment variables and backend connection config
   - [ ] Fix configuration issues
   - [ ] Restart deployment if needed

### Short-term (P1) - Improve Configuration
4. **Update Helm Values**
   - [ ] Change image tag from `:latest` to specific SHA
   - [ ] Enable proper autoscaling (min: 2, max: 4)
   - [ ] Add security environment variables for Sprint 7 features

5. **Complete Superset Integration**
   - [ ] Create Superset service account for ordering-backend
   - [ ] Add Superset credentials to `ordering-backend-secrets`
   - [ ] Configure ordering database connection in Superset
   - [ ] Create initial analytics dashboards
   - [ ] Test embed API endpoints

6. **Scale Superset Workers** (if async queries needed)
   - [ ] Scale workers: `kubectl scale deployment superset-worker -n infra --replicas=2`
   - [ ] Scale beat: `kubectl scale deployment superset-celerybeat -n infra --replicas=1`
   - [ ] Monitor resource usage

### Medium-term (P2) - Documentation & Monitoring
7. **Update Documentation**
   - [ ] Document Sprint 7 security configuration
   - [ ] Add Superset integration guide
   - [ ] Update deployment runbook with troubleshooting steps

8. **Add Monitoring & Alerts**
   - [ ] Set up Prometheus alerts for ordering-backend health
   - [ ] Add rate limiting metrics dashboard
   - [ ] Monitor database query performance

### Long-term (P3) - Future Enhancements
9. **Vector Analytics** (Sprint 8+)
   - [ ] Evaluate vector embedding use cases
   - [ ] Add pgvector extension to ordering database
   - [ ] Implement vector columns in Ent schemas
   - [ ] Build semantic search and recommendation features

---

## Security Configuration (Sprint 7)

### Environment Variables Required
Add these to `ordering-backend-secrets`:

```bash
# Rate Limiting (Sprint 7)
RATE_LIMIT_ENABLED=true
RATE_LIMIT_REQUESTS_PER_MINUTE=60
RATE_LIMIT_REQUESTS_PER_HOUR=1000
RATE_LIMIT_AUTH_PER_MINUTE=10
RATE_LIMIT_PAYMENT_PER_MINUTE=20
RATE_LIMIT_BURST_MULTIPLIER=1.5
RATE_LIMIT_KEY_PREFIX=rl:ordering:

# Security Headers (Sprint 7)
SECURITY_HEADERS_ENABLED=true
INPUT_VALIDATION_ENABLED=true
MAX_REQUEST_BODY_SIZE=10485760

# Connection Pooling (Sprint 7)
POSTGRES_MAX_OPEN_CONNS=20
POSTGRES_MAX_IDLE_CONNS=10
POSTGRES_CONN_MAX_LIFETIME=30m

# Superset Integration
SUPERSET_BASE_URL=http://superset.infra.svc.cluster.local:8088
SUPERSET_USERNAME=ordering_service
SUPERSET_PASSWORD=<to-be-generated>
SUPERSET_AUTH_PROVIDER=db
```

---

## Testing & Verification

### Pre-Deployment Checklist
- [x] Code builds successfully locally
- [x] All tests pass
- [x] Database migrations generated
- [x] Documentation updated
- [ ] Docker image builds in CI
- [ ] Docker image pushed to registry
- [ ] ArgoCD can pull image
- [ ] Pod starts successfully
- [ ] Health checks pass
- [ ] API endpoints respond

### Post-Deployment Verification
```bash
# 1. Check pod status
kubectl get pods -n ordering

# 2. Check logs
kubectl logs -n ordering -l app=ordering-backend --tail=100

# 3. Test health endpoint
kubectl port-forward -n ordering svc/ordering-backend 4000:4000
curl http://localhost:4000/healthz

# 4. Test rate limiting
for i in {1..70}; do
  curl -s -o /dev/null -w "%{http_code}\n" http://localhost:4000/api/v1/status
done
# Should see 429 (Too Many Requests) after 60 requests

# 5. Verify security headers
curl -I http://localhost:4000/api/v1/status | grep -E "(X-|Content-Security)"

# 6. Check database indexes
kubectl exec -n infra <postgres-pod> -- psql -U postgres ordering -c "\d+ users"
```

---

## Conclusion

Sprint 7 implementation is complete and provides production-ready security hardening and performance optimizations. The main deployment blocker is Docker registry authentication, which needs immediate attention. Once resolved, the ordering-backend should deploy successfully with all Sprint 7 features active.

Superset is healthy and ready for integration. Complete the service account setup and database connection configuration to enable analytics dashboards for the ordering service.

**Next Sprint Focus**: Launch & Handover (Sprint 8)
- Production deployment stabilization
- Monitoring and alerting setup
- Documentation handover
- Chaos engineering tests

---

## Post-Audit Implementation (January 2026)

### Critical Fixes Implemented ✅

The following critical and high-priority gaps identified during the audit have been addressed:

#### 1. CRITICAL: Outbox Background Publisher ✅
**Status**: Implemented
**Files Created/Modified**:
- `internal/platform/events/outbox_adapter.go` - NATS publisher adapter bridging outbox module to NATS
- `internal/app/app.go` - Wired outbox publisher to app lifecycle with graceful shutdown
- `internal/config/config.go` - Added `OutboxEnabled`, `OutboxPollPeriod`, `OutboxBatchSize` config

**Features**:
- Background worker polls outbox table and publishes to NATS
- Configurable batch size and poll period
- Graceful shutdown handling

#### 2. CRITICAL: CORS Configuration ✅
**Status**: Fixed across ALL Go microservices
**Files Modified** (all services):
- `internal/config/config.go` - Added `AllowedOrigins` with production URLs as defaults
- `internal/http/router/router.go` - Uses configurable origins from config

**Services Updated**:
| Service | Production Origins |
|---------|-------------------|
| ordering-backend | ordersapp, cafe, pos, accounts |
| logistics-api | ordersapp, pos, accounts |
| inventory-api | pos, ordersapp, accounts |
| pos-api | pos, ordersapp, accounts |
| treasury-api | books, ordersapp, pos, accounts |
| notifications-api | notifications, ordersapp, accounts |
| subscriptions-api | accounts, sso |

**K8s Values Updated** (devops-k8s):
- Added `HTTP_ALLOWED_ORIGINS` env var to all services' values.yaml files

#### 3. HIGH: Subscription Feature Gating ✅
**Status**: Implemented (Partial - Analytics)
**Files Modified**:
- `internal/http/handlers/analytics/handler.go` - Added `authclient.RequirePlan("PROFESSIONAL")` middleware

**Note**: Full feature gating (group_ordering) pending auth-service Sprint 11 JWT enrichment

#### 4. MEDIUM: Audit Logging Middleware ✅
**Status**: Fully Implemented
**Files Created**:
- `internal/ent/schema/auditlog.go` - AuditLog entity schema with indexes
- `internal/modules/audit/logger.go` - Audit logger with async recording
- `internal/modules/audit/middleware.go` - MutationAudit middleware for POST/PUT/PATCH/DELETE

**Files Modified**:
- `internal/http/router/router.go` - Wired audit middleware after auth middleware
- `internal/app/app.go` - Initialized audit logger

**Features**:
- Logs all mutation operations (POST, PUT, PATCH, DELETE)
- Captures user ID, tenant ID from JWT claims
- Sanitizes sensitive fields (passwords, tokens, API keys, CVV, etc.)
- Extracts resource type and ID from URL paths
- Records request duration, IP address, user agent
- Asynchronous logging to avoid blocking responses
- Skips health checks, webhooks, and auth endpoints

#### 5. HIGH: Cafe-Website SSO Integration ✅
**Status**: Fully Implemented
**Files Created/Modified**:
- `src/lib/auth/config.ts` - SSO URLs with production defaults, NextAuth OIDC provider
- `src/hooks/use-auth.ts` - SSO login/logout hooks with proper session clearing
- `src/app/signup/page.tsx` - Redirects to SSO signup with return URL
- `src/app/staff/layout.tsx` - Staff portal with SSO logout integration

**SSO Features**:
- OIDC provider integration with auth-service
- JWT token validation via JWKS
- Access token refresh flow
- SSO logout (clears NextAuth session + redirects to SSO logout endpoint)
- Production URLs as defaults (`https://sso.codevertexitsolutions.com`)
- Return URL support for post-login/signup redirects

#### 6. HIGH: Auth-API CORS for masterspace.co.ke ✅
**Status**: Implemented
**Files Modified**:
- `auth-service/auth-api/internal/httpapi/router.go` - Added dynamic origin checking for `*.masterspace.co.ke` subdomains

### Documentation Updated ✅

- `ordering-backend/docs/plan.md` - Architecture Patterns Migration Status table updated
- `ordering-backend/docs/sprints/sprint-8-launch-handover.md` - All critical gaps marked as completed
- `cafe-website/docs/plan.md` - SSO Integration section added
- `cafe-website/docs/sprints/sprint-4-auth-tracking.md` - Logout flow marked as completed

### Remaining Gaps

| Gap | Priority | Status | Notes |
|-----|----------|--------|-------|
| Docker Registry Auth | CRITICAL | ❌ Pending | Create repo on Docker Hub manually |
| Redis Session Storage | MEDIUM | ⏳ Optional | Recommended for production |
| Booking Service | MEDIUM | ❌ Not Started | Use contact forms as mitigation |
| Group Ordering Feature Gate | MEDIUM | ⏳ Pending | Blocked on auth-service Sprint 11 |
| Superset Service Account | MEDIUM | ⏳ Pending | Create credentials in Superset |
