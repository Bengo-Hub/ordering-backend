# Sprint 0 - Foundation

**Duration**: Week 1  
**Status**: ✅ Completed (Nov 2025)

---

## Overview

Sprint 0 establishes the foundational infrastructure, project structure, and core identity management capabilities for the ordering-backend service.

---

## Objectives

1. Set up Go project scaffolding with clean architecture
2. Configure CI/CD pipeline with centralized devops-k8s workflows
3. Implement observability baseline (logging, metrics, tracing)
4. Deliver identity bootstrap with RBAC, OAuth2, and JWT session management
5. Define domain models, ERD, API guidelines, and service interface contracts

---

## Technology Stack

### Core Framework

**Language**: Go 1.22+  
**Architecture Pattern**: Clean/Hexagonal Architecture

**Project Structure**:
```
ordering-backend/
├── cmd/
│   ├── api/              # Main application entry point
│   ├── migrate/          # Database migration tool
│   └── seed/             # Database seeding tool
├── internal/
│   ├── app/              # Application initialization
│   ├── config/           # Configuration management
│   ├── ent/               # Ent ORM generated code
│   ├── http/              # HTTP handlers and routing
│   │   ├── handlers/      # Request handlers
│   │   └── router/        # Route definitions
│   ├── modules/           # Domain modules
│   │   └── identity/      # Identity module
│   ├── platform/          # Platform services
│   │   ├── cache/         # Redis cache
│   │   ├── database/      # PostgreSQL connection
│   │   └── events/        # NATS event bus
│   ├── shared/            # Shared utilities
│   │   ├── logger/        # Structured logging
│   │   └── middleware/    # HTTP middleware
│   └── validators/        # Input validation
├── docs/                  # Documentation
├── config/                # Configuration files
├── go.mod                 # Go module definition
├── go.sum                 # Go module checksums
├── Dockerfile             # Container image definition
└── Makefile               # Build automation
```

**HTTP Framework**: chi router  
**API Documentation**: OpenAPI/Swagger (Swagger UI)

### Data Layer

**Database**: PostgreSQL 16+  
**ORM**: Ent (schema-as-code)  
**Connection Pool**: pgx/v5  
**Migrations**: Ent migrations

**Database Connection**:
- PostgreSQL connection pool using pgx/v5
- Ent ORM client for type-safe database operations
- Connection pooling with configurable limits

**Caching**: Redis 7+
- Redis client for caching and ephemeral state
- Connection pooling and health checks

### Observability

**Logging**: zap (structured logging)  
**Metrics**: Prometheus  
**Tracing**: OpenTelemetry

**Logger Setup**:
- Structured logging with zap
- Environment-based configuration (development vs production)
- JSON output for production, console output for development

**Metrics Setup**:
- Prometheus metrics registration
- HTTP request duration and count metrics
- Database query duration metrics

### Authentication

**JWT Library**: `github.com/golang-jwt/jwt/v5`  
**OAuth2**: `golang.org/x/oauth2`  
**Auth Client**: `shared/auth-client` v0.1.0

**Auth Middleware**:
- JWT token extraction from Authorization header
- Token validation using shared/auth-client library
- Context enrichment with user_id and tenant_id
- Unauthorized response for invalid/missing tokens

---

## User Stories

### US-0.1: Project Scaffolding
**As a** developer  
**I want** a well-structured Go project with clean architecture  
**So that** I can easily navigate and extend the codebase

**Acceptance Criteria**:
- [x] Project structure follows clean/hexagonal architecture
- [x] Go modules properly configured
- [x] Makefile with common tasks (build, test, run, migrate)
- [x] Dockerfile for containerization
- [x] .gitignore configured

### US-0.2: CI/CD Pipeline
**As a** DevOps engineer  
**I want** automated CI/CD pipeline  
**So that** code changes are automatically tested and deployed

**Acceptance Criteria**:
- [x] GitHub Actions workflow configured
- [x] Integration with centralized devops-k8s workflows
- [x] Automated testing on PR
- [x] Docker image build and push
- [x] ArgoCD deployment configuration

### US-0.3: Observability Baseline
**As a** operations team member  
**I want** structured logging, metrics, and tracing  
**So that** I can monitor and debug the service

**Acceptance Criteria**:
- [x] Structured logging with zap
- [x] Prometheus metrics endpoint
- [x] OpenTelemetry tracing configured
- [x] Health check endpoint
- [x] Request ID middleware

### US-0.4: Identity Bootstrap
**As a** user  
**I want** to authenticate using OAuth2 (Google)  
**So that** I can access the cafe platform

**Acceptance Criteria**:
- [x] OAuth2 initiation endpoint (legacy - still uses local OAuth, needs migration to auth-service)
- [x] OAuth2 callback handler (legacy - still uses local OAuth, needs migration to auth-service)
- [x] **JWT token issuance**: ✅ Implemented via auth-service (`https://sso.codevertexitsolutions.com/`). Login/registration proxy to auth-service and return tokens from auth-service.
- [x] **Session management**: ✅ Sessions managed by auth-service. Local session table deprecated. JWT tokens validated via JWKS from auth-service.
- [ ] **Token refresh**: Should proxy to auth-service, currently using local refresh (needs migration)

### US-0.5: RBAC Implementation
**As a** system administrator  
**I want** role-based access control  
**So that** users have appropriate permissions

**Acceptance Criteria**:
- [x] Role and permission entities defined
- [x] Role-permission mappings
- [x] User-role assignments
- [x] Permission checking middleware
- [x] Seed data for default roles

### US-0.6: User Profile Management
**As a** user  
**I want** to manage my profile and preferences  
**So that** I can customize my experience

**Acceptance Criteria**:
- [x] Profile endpoints (GET, PUT)
- [x] Preferences endpoints (GET, PUT)
- [x] Security settings endpoints
- [x] Profile data validation

---

## API Endpoints

### Authentication

**POST /api/v1/auth/login** (Should proxy to auth-service)
- Request: `{ "email": "user@example.com", "password": "password", "tenant_slug": "urban-cafe" }` (tenant_slug defaults to "urban-cafe" if omitted)
- Proxies to: `POST https://sso.codevertexitsolutions.com/api/v1/auth/login`
- Response: `{ "access_token": "...", "refresh_token": "...", "session_id": "...", "tenant": {...}, "user": {...}, "expires_in": 899 }`

**POST /api/v1/auth/oauth/{provider}**
- Providers: `google`, `microsoft`, `github`
- Redirects to OAuth provider

**GET /api/v1/auth/oauth/{provider}/callback**
- OAuth callback handler
- Exchanges code for tokens

**POST /api/v1/auth/refresh**
- Request: `{ "refresh_token": "..." }`
- Response: `{ "access_token": "...", "expires_in": 3600 }`

**POST /api/v1/auth/logout**
- Invalidates refresh token

### User Profile

**GET /api/v1/profile**
- Returns current user profile

**PUT /api/v1/profile**
- Updates user profile
- Request: `{ "full_name": "...", "phone": "..." }`

**GET /api/v1/preferences**
- Returns user preferences

**PUT /api/v1/preferences**
- Updates user preferences
- Request: `{ "theme": "dark", "language": "en" }`

**GET /api/v1/security**
- Returns security settings

**PUT /api/v1/security**
- Updates security settings

---

## Database Schema

### Identity Module

**tenants**
- `id` (UUID, PK)
- `slug` (VARCHAR, UNIQUE)
- `name` (VARCHAR)
- `status` (VARCHAR, default: 'active')
- `created_at`, `updated_at` (TIMESTAMPTZ)

**users**
- `id` (UUID, PK)
- `tenant_id` (UUID, FK → tenants)
- `auth_service_user_id` (UUID, UNIQUE)
- `email` (VARCHAR, UNIQUE per tenant)
- `full_name` (VARCHAR)
- `phone` (VARCHAR)
- `status` (VARCHAR, default: 'active')
- `primary_role` (VARCHAR)
- `sync_status` (VARCHAR, default: 'synced')
- `sync_at` (TIMESTAMPTZ)
- `last_login_at` (TIMESTAMPTZ)
- `created_at`, `updated_at` (TIMESTAMPTZ)

**roles**
- `code` (VARCHAR, PK)
- `name` (VARCHAR)
- `description` (TEXT)
- `scope` (VARCHAR)
- `system_role` (BOOLEAN, default: false)
- `created_at`, `updated_at` (TIMESTAMPTZ)

**permissions**
- `code` (VARCHAR, PK)
- `name` (VARCHAR)
- `module` (VARCHAR)
- `description` (TEXT)
- `created_at` (TIMESTAMPTZ)

**role_permissions**
- `role_code` (VARCHAR, FK → roles)
- `permission_code` (VARCHAR, FK → permissions)
- Composite PK: (role_code, permission_code)

**user_roles**
- `user_id` (UUID, FK → users)
- `role_code` (VARCHAR, FK → roles)
- `assigned_by` (UUID, FK → users)
- `assigned_at` (TIMESTAMPTZ)
- Composite PK: (user_id, role_code)

---

## Code Structure

### Module Organization

**Identity Module** (`internal/modules/identity/`):
- `domain.go` - Domain models and types
- `service.go` - Business logic layer
- `repository.go` - Repository interface definition
- `repository_ent.go` - Ent ORM implementation
- `errors.go` - Domain-specific errors

**Service Pattern**:
- Service struct with repository and auth client dependencies
- Business logic methods (GetProfile, UpdateProfile, etc.)
- Error handling and validation

**Repository Pattern**:
- Repository interface for data access abstraction
- Ent implementation for PostgreSQL operations
- Type-safe queries using Ent ORM

---

## Testing Strategy

### Unit Tests

**Test Structure**:
- Table-driven tests for service methods
- Test cases for success and error scenarios
- Mock repository for isolation
- Assertions for expected behavior

### Integration Tests

**Testcontainers Setup**:
- PostgreSQL container for database tests
- Redis container for cache tests
- Cleanup after test completion
- Real database operations for integration validation

---

## Deliverables

- [x] Go project structure with clean architecture
- [x] CI/CD pipeline with GitHub Actions
- [x] Observability baseline (logging, metrics, tracing)
- [x] Identity bootstrap with OAuth2 and JWT
- [x] RBAC implementation
- [x] User profile management endpoints
- [x] Database schema with Ent
- [x] API documentation (Swagger)
- [x] Health check endpoint
- [x] Seed data for default roles and permissions

---

## Dependencies

- Go 1.22+
- PostgreSQL 16+
- Redis 7+
- Ent ORM
- chi router
- zap logger
- Prometheus client
- OpenTelemetry SDK
- shared/auth-client v0.1.0

---

## Next Steps

- Sprint 1: Identity & Access Hardening
  - Persist identity data to Postgres
  - Enforce tenant scoping
  - Device management
  - Invitation workflows
  - Audit log baseline

