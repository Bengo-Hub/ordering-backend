# Sprint 1 - Identity & Access Hardening

**Duration**: Weeks 2-3  
**Status**: 🚧 Partially Implemented

---

## Overview

Sprint 1 focuses on hardening identity and access management by persisting identity data to PostgreSQL, enforcing tenant scoping, implementing device management, invitation workflows, and establishing an audit log baseline.

---

## Objectives

1. Persist identity data to Postgres via Ent repositories
2. Enforce tenant scoping on all operations
3. Implement device management
4. Create invitation workflows
5. Establish audit log baseline
6. Deliver subscription entitlement service scaffolding

---

## Technology Stack

### Data Persistence
- **ORM**: Ent (schema-as-code migrations)
- **Database**: PostgreSQL 16+
- **Migrations**: Ent migration tooling

### State Management
- **Session Storage**: Redis for ephemeral session data
- **Device Tracking**: Database table for device registration

### Event Publishing
- **Event Bus**: NATS JetStream
- **Outbox Pattern**: Reliable event publishing

---

## User Stories

### US-1.1: Persist Identity Data
**As a** system administrator  
**I want** user identity data persisted in PostgreSQL  
**So that** user data survives service restarts

**Acceptance Criteria**:
- [x] User entities persisted via Ent repositories
- [ ] **Auth-Service Integration**: All login/registration requests proxy to auth-service (`https://sso.codevertexitsolutions.com/`)
- [ ] **User Sync**: `auth_service_user_id` field added to user schema, user sync via events
- [ ] **Superuser Handling**: Superuser detection from JWT claims with RBAC bypass
- [ ] Session data managed by auth-service (no local session tables)
- [ ] Token refresh proxied to auth-service
- [ ] User profile data synced with auth-service via `auth.user.*` events

### US-1.2: Tenant Scoping
**As a** system administrator  
**I want** all operations scoped by tenant  
**So that** multi-tenant data isolation is enforced

**Acceptance Criteria**:
- [x] Tenant ID extracted from JWT claims (via auth-service middleware)
- [ ] **Tenant Slug Required**: All login/registration requests require `tenant_slug` parameter
- [ ] All database queries filtered by tenant_id (partial - identity module uses tenant_id, but not enforced across all operations)
- [ ] Tenant validation middleware
- [ ] Cross-tenant access prevented
- [ ] Tenant metadata synced from auth-service via `auth.tenant.*` events

### US-1.3: Device Management
**As a** user  
**I want** to manage my registered devices  
**So that** I can see and revoke device access

**Acceptance Criteria**:
- [ ] Device registration on login
- [ ] Device list endpoint
- [ ] Device revocation endpoint
- [ ] Device metadata tracking (OS, browser, IP)

### US-1.4: Invitation Workflows
**As a** tenant administrator  
**I want** to invite users to my organization  
**So that** I can onboard new team members

**Acceptance Criteria**:
- [ ] Invitation creation endpoint
- [ ] Invitation email via notifications service
- [ ] Invitation acceptance flow
- [ ] Invitation expiration handling

### US-1.5: Audit Log Baseline
**As a** compliance officer  
**I want** audit logs for all user actions  
**So that** I can track system access and changes

**Acceptance Criteria**:
- [ ] Audit log entity definition
- [ ] Audit middleware for HTTP requests
- [ ] Audit log query endpoints
- [ ] Log retention policies

### US-1.6: Subscription Entitlements
**As a** system administrator  
**I want** subscription entitlement service  
**So that** I can gate feature access by plan

**Acceptance Criteria**:
- [ ] Entitlement service interface
- [ ] Plan-to-feature mapping
- [ ] Feature flag checking
- [ ] Usage tracking for overage

---

## API Endpoints

### Device Management

**GET /api/v1/devices**
- Returns list of user's registered devices
- Response: Array of device objects with metadata

**DELETE /api/v1/devices/{id}**
- Revokes device access
- Invalidates all sessions for device

### Invitations

**POST /api/v1/invitations**
- Creates invitation for new user
- Request: `{ "email": "...", "role": "...", "expires_in": 86400 }`
- Publishes invitation event to notifications service

**GET /api/v1/invitations/{token}**
- Validates invitation token
- Returns invitation details

**POST /api/v1/invitations/{token}/accept**
- Accepts invitation and creates user
- Request: `{ "password": "...", "full_name": "..." }`

### Audit Logs

**GET /api/v1/audit-logs**
- Query audit logs with filters
- Query params: `user_id`, `action`, `resource_type`, `date_from`, `date_to`
- Pagination support

---

## Database Schema

### Device Management

**devices**
- `id` (UUID, PK)
- `user_id` (UUID, FK → users)
- `tenant_id` (UUID, FK → tenants)
- `device_id` (VARCHAR, UNIQUE)
- `device_name` (VARCHAR)
- `device_type` (VARCHAR)
- `os` (VARCHAR)
- `browser` (VARCHAR)
- `ip_address` (VARCHAR)
- `last_used_at` (TIMESTAMPTZ)
- `created_at`, `updated_at` (TIMESTAMPTZ)

### Invitations

**invitations**
- `id` (UUID, PK)
- `tenant_id` (UUID, FK → tenants)
- `email` (VARCHAR)
- `role_code` (VARCHAR, FK → roles)
- `token` (VARCHAR, UNIQUE)
- `invited_by` (UUID, FK → users)
- `status` (VARCHAR, default: 'pending')
- `expires_at` (TIMESTAMPTZ)
- `accepted_at` (TIMESTAMPTZ)
- `created_at`, `updated_at` (TIMESTAMPTZ)

### Audit Logs

**audit_logs**
- `id` (UUID, PK)
- `tenant_id` (UUID, FK → tenants)
- `user_id` (UUID, FK → users)
- `action` (VARCHAR)
- `resource_type` (VARCHAR)
- `resource_id` (UUID)
- `metadata` (JSONB)
- `ip_address` (VARCHAR)
- `user_agent` (VARCHAR)
- `occurred_at` (TIMESTAMPTZ)

### Subscription Entitlements

**subscription_entitlements**
- `id` (UUID, PK)
- `tenant_id` (UUID, FK → tenants)
- `feature_code` (VARCHAR)
- `enabled` (BOOLEAN)
- `limits_json` (JSONB)
- `created_at`, `updated_at` (TIMESTAMPTZ)

---

## Code Structure

### Module Organization

**Identity Module** (`internal/modules/identity/`):
- `device.go` - Device domain models and service
- `invitation.go` - Invitation domain models and service
- `audit.go` - Audit log domain models and service
- `entitlement.go` - Subscription entitlement service

**Middleware** (`internal/http/middleware/`):
- `tenant.go` - Tenant scoping middleware
- `audit.go` - Audit logging middleware
- `device.go` - Device registration middleware

---

## Integration Points

### Notifications Service
- **Event**: `cafe.invitation.created` - Trigger invitation email
- **Event**: `cafe.invitation.accepted` - Notify inviter

### Auth Service
- **Production URL**: `https://sso.codevertexitsolutions.com/`
- **Login Proxy**: All login requests proxy to `POST /api/v1/auth/login` with `{email, password, tenant_slug}`
- **Registration Proxy**: All registration requests proxy to `POST /api/v1/auth/register` with `{email, password, tenant_slug, profile}`
- **User Sync**: Consume `auth.user.created`, `auth.user.updated`, `auth.user.deactivated` events
- **Tenant Sync**: Consume `auth.tenant.created`, `auth.tenant.updated`, `auth.outlet.created` events
- **Superuser Sync**: Default superuser from auth-service seed synced to all services
- **Service-to-Service**: API key authentication for inter-service calls

---

## Testing Strategy

### Unit Tests
- Service layer tests with mocked repositories
- Middleware tests with test HTTP requests
- Validation logic tests

### Integration Tests
- End-to-end invitation flow
- Device registration and revocation
- Audit log creation and querying
- Tenant scoping enforcement

---

## Deliverables

- [x] Identity data persistence via Ent (users, sessions, roles, permissions)
- [ ] Tenant scoping middleware and enforcement (partial - tenant_id in JWT but not fully enforced)
- [ ] Device management endpoints (not implemented)
- [ ] Invitation workflow implementation (not implemented)
- [ ] Audit log baseline with query endpoints (not implemented)
- [ ] Subscription entitlement service scaffolding (not implemented)
- [x] Database migrations for identity entities (users, sessions, roles, permissions, tenants)
- [ ] Integration tests for all features

## Implementation Notes

**Completed:**
- User and session persistence via Ent ORM
- ✅ **Auth-Service Login Integration**: Login/registration proxies to auth-service (`https://sso.codevertexitsolutions.com/`)
- ✅ **JWT Token Validation**: Token validation via auth-service JWKS using `shared/auth-client` library
- ✅ **User Synchronization**: `auth_service_user_id` field added to user schema, user sync on login implemented
- ✅ **Tenant Slug Handling**: Login/registration requires `tenant_slug` parameter
- ✅ **User Profile and Preferences Management**: Cafe-specific user data management
- ✅ **Basic RBAC**: Roles and permissions with cafe-specific roles merged with auth-service roles
- ✅ **Tenant Entity Support**: Tenant scoping via JWT claims
- ✅ **Service-to-Service Auth**: API key authentication support via `shared/auth-client`

**Partially Implemented:**
- ⚠️ **OAuth2 (Google)**: Still uses local OAuth flow (needs migration to auth-service OAuth)
- ✅ **User Sync via Events**: Event listeners for `auth.user.*` events implemented in `internal/modules/identity/events.go` and wired in `app.go`
- ⚠️ **Superuser Handling**: Superuser detection logic partially implemented - need to verify HasScope("superuser") in all permission checks

**Not Implemented:**
- ❌ Device management (device registration, tracking, revocation)
- ❌ Invitation workflows (user invitations, acceptance flows)
- ❌ Audit logging (compliance logging for user actions)
- ❌ Subscription entitlements (feature flag service)
- ❌ Full tenant scoping enforcement across all operations

---

## Current Sprint Status Summary (January 2026)

**Overall Progress**: ~70% Complete

| Task | Status | Priority | Notes |
|------|--------|----------|-------|
| User persistence via Ent | ✅ Complete | Critical | Working |
| Auth-service login integration | ✅ Complete | Critical | Login/registration proxy working |
| JWT token validation | ✅ Complete | Critical | Using shared-auth-client |
| User sync on login | ✅ Complete | Critical | auth_service_user_id stored |
| Tenant slug handling | ✅ Complete | Critical | Required for all auth operations |
| User profile management | ✅ Complete | High | Cafe-specific data |
| Basic RBAC | ✅ Complete | High | Roles and permissions |
| Event listeners | ✅ Complete | High | auth.user.* events consumed |
| OAuth2 migration | ⚠️ Partial | Medium | Local OAuth needs migration |
| Device management | ❌ Not Started | Low | Defer to post-MVP |
| Invitation workflows | ❌ Not Started | Medium | Defer to post-MVP |
| Audit logging | ❌ Not Started | Medium | Defer to post-MVP |
| Subscription entitlements | ❌ Not Started | High | Needed for feature gates |

**Recommendation**: Proceed to Sprint 2 (Catalog & Localization) as the core auth/RBAC is functional. Device management, invitations, and audit logging can be added incrementally post-MVP.

**Blocking Items for Sprint 2**:
- None - Sprint 1 core deliverables are complete

---

## Dependencies

- Ent ORM migrations
- NATS JetStream for events
- Notifications service for invitation emails
- Auth service for user sync

---

## Next Steps

- Sprint 2: Catalog & Localization
  - Menu CRUD operations
  - Category hierarchy
  - Image handling
  - Localization fields (EN/SW)

