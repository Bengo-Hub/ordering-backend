# Docker Hub Repository Issue - Root Cause Analysis
**Date**: 2026-01-18
**Status**: Issue Identified - Action Required

---

## Problem Statement

The `ordering-backend` deployment fails with:
```
unauthorized: access token has insufficient scopes
```

Meanwhile, `auth-api` and `erp-api` deployments succeed using the **same** `REGISTRY_PASSWORD` secret.

---

## Root Cause

**The Docker Hub repository `codevertex/ordering-backend` does not exist.**

### Verification

```bash
# ordering-backend repository - NOT FOUND
$ curl -s "https://hub.docker.com/v2/repositories/codevertex/ordering-backend/"
{"message":"object not found","errinfo":{}}

# auth-api repository - EXISTS
$ curl -s "https://hub.docker.com/v2/repositories/codevertex/auth-api/"
{
  "user": "codevertex",
  "name": "auth-api",
  "namespace": "codevertex",
  "repository_type": "image",
  "status": 1,
  "status_description": "active",
  "is_private": false,
  "pull_count": 253,
  "last_updated": "2026-01-18T14:30:03.300411Z"
}
```

### Why This Matters

Docker Hub's authentication behavior:
- **Existing repositories**: Token with push access can push images
- **New repositories**: Token needs **create repository** permission to push to non-existent repos
- The `REGISTRY_PASSWORD` token has push access to existing repos but cannot create new ones
- Docker returns "insufficient scopes" when trying to push to a non-existent repo

### Why auth-api and erp-api Work

1. `codevertex/auth-api` - Created on 2025-12-10, has 253 pulls
2. `codevertex/erp-api` - Exists (not checked but deployment succeeds)
3. Both repositories exist, so the token can push without needing create permissions

### Why ordering-backend Fails

1. Repository doesn't exist yet (this is the first deployment)
2. Token attempts to push → Docker Hub checks if repo exists → Repo not found
3. Token doesn't have permission to create repos → Returns "insufficient scopes"

---

## Solution Options

### Option 1: Create Repository Manually ✅ RECOMMENDED

**Steps**:
1. Log into Docker Hub: https://hub.docker.com/
   - Username: `codevertex`
   - Use the account credentials (not the CI token)

2. Create new repository:
   - Go to: https://hub.docker.com/repository/create
   - **Name**: `ordering-backend`
   - **Visibility**: Public (or match auth-api visibility)
   - **Description**: "BengoBox Ordering Service - Online Delivery Orders"

3. Re-run the GitHub Actions workflow:
   ```bash
   gh run rerun 21113419024 -R Bengo-Hub/ordering-backend
   ```

**Advantages**:
- Quick fix (5 minutes)
- No need to update secrets
- No impact on other services
- More secure (token has limited permissions)

**Time to Fix**: ~5 minutes

---

### Option 2: Update Access Token with Create Permissions

**Steps**:
1. Log into Docker Hub as `codevertex`

2. Generate new access token with extended permissions:
   - Go to: https://hub.docker.com/settings/security
   - Click "New Access Token"
   - **Description**: `github-actions-ci-with-create`
   - **Permissions**:
     - ✅ Read
     - ✅ Write
     - ✅ Delete
     - ✅ **Create repositories** ← IMPORTANT

3. Copy the new token

4. Update GitHub secret in ALL repositories:
   ```bash
   # Update for ordering-backend
   gh secret set REGISTRY_PASSWORD -R Bengo-Hub/ordering-backend

   # Also update in other repos if using the same token
   gh secret set REGISTRY_PASSWORD -R Bengo-Hub/auth-api
   gh secret set REGISTRY_PASSWORD -R Bengo-Hub/erp-api
   # ... etc for all repos
   ```

5. Re-run the workflow

**Advantages**:
- Fully automated deployments for new services
- Token can create repos on-demand
- No manual repo creation needed in future

**Disadvantages**:
- More powerful token (security consideration)
- Need to update secret in multiple repos
- Takes longer (~15-20 minutes)

**Time to Fix**: ~20 minutes

---

## Recommended Action Plan

### Phase 1: Immediate Fix (Use Option 1)
1. **Create `codevertex/ordering-backend` repository manually** ← DO THIS NOW
2. Re-run GitHub Actions workflow
3. Verify deployment succeeds
4. Monitor pod startup and health checks

### Phase 2: Document for Future Services
1. Add to onboarding checklist: "Create Docker Hub repo before first deploy"
2. Update deployment documentation
3. Consider Option 2 for long-term automation

---

## Implementation Steps (Option 1)

### Step 1: Create Docker Hub Repository
```
1. Navigate to: https://hub.docker.com/repository/create
2. Fill in:
   - Repository Name: ordering-backend
   - Namespace: codevertex (should be pre-selected)
   - Description: BengoBox Ordering Service - Online Delivery Orders
   - Visibility: Public (or match your other repos)
3. Click "Create"
```

### Step 2: Verify Repository Created
```bash
curl -s "https://hub.docker.com/v2/repositories/codevertex/ordering-backend/" | grep -E '(name|status)'
# Should return: "name":"ordering-backend","status":1
```

### Step 3: Re-run GitHub Actions
```bash
# From ordering-backend repository
gh run rerun 21113419024

# Or trigger new run by pushing a commit
git commit --allow-empty -m "chore: trigger deployment after Docker Hub repo creation"
git push
```

### Step 4: Monitor Deployment
```bash
# Watch the workflow
gh run watch

# Once image is pushed, check ArgoCD sync
kubectl get application ordering-backend -n argocd

# Check pods
kubectl get pods -n ordering -w

# Check logs when pod starts
kubectl logs -n ordering -l app=ordering-backend -f
```

---

## Verification Checklist

After creating the repository and re-running the workflow:

- [ ] GitHub Actions "Run production deployment" step succeeds
- [ ] Docker image pushed: `docker.io/codevertex/ordering-backend:2cb3b724`
- [ ] Latest tag pushed: `docker.io/codevertex/ordering-backend:latest`
- [ ] ArgoCD syncs successfully
- [ ] Pod starts: `kubectl get pods -n ordering`
- [ ] Health check passes: `/healthz` returns OK
- [ ] API responds: Test endpoint with `curl`

---

## Prevention for Future Services

### Deployment Checklist for New Services

Before running first deployment:

1. **Check if Docker Hub repo exists**:
   ```bash
   curl -s "https://hub.docker.com/v2/repositories/codevertex/<service-name>/" | grep name
   ```

2. **If not exists, create it manually**:
   - Go to https://hub.docker.com/repository/create
   - Name: `<service-name>`
   - Visibility: Match existing repos
   - Create

3. **Then run deployment**:
   ```bash
   git push  # Triggers GitHub Actions
   ```

### Alternative: Use Organization-Level Token

If you frequently deploy new services, consider:
- Creating an organization-level token with create permissions
- Store as a separate secret: `REGISTRY_PASSWORD_WITH_CREATE`
- Use for new service deployments
- Switch to read-only token after repo is created

---

## Related Documentation

- [Deployment Audit 2026-01-18](./deployment-audit-2026-01-18.md)
- [Docker Hub API Documentation](https://docs.docker.com/docker-hub/api/latest/)
- [GitHub Actions Docker Login](https://github.com/docker/login-action)

---

## Status Update

**Current Status**: Waiting for repository creation

**Next Action**: Create `codevertex/ordering-backend` repository in Docker Hub (5 minutes)

**Expected Result**: Deployment will succeed, image will be available at:
- `docker.io/codevertex/ordering-backend:2cb3b724`
- `docker.io/codevertex/ordering-backend:latest`

**Timeline**:
- Create repo: 5 minutes
- Re-run workflow: 3-4 minutes (build + push)
- ArgoCD sync: 1-2 minutes
- Pod startup: 1-2 minutes
- **Total**: ~10-15 minutes to fully deployed
