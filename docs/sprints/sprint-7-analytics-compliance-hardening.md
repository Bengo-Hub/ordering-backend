# Sprint 7 - Analytics, Compliance & Hardening

**Duration**: Weeks 14-15
**Status**: 🚧 In Progress (January 2026)

---

## Sprint Progress (Updated January 2026)

| Task | Status | Notes |
|------|--------|-------|
| Superset client scaffolding | ✅ Complete | `internal/platform/superset/client.go` |
| Analytics module scaffolding | ✅ Complete | `internal/modules/analytics/` |
| Analytics HTTP handlers | ✅ Complete | `internal/http/handlers/analytics/handler.go` |
| Dashboard embed endpoints | ✅ Complete | `/api/v1/{tenant}/analytics/dashboards/{module}/embed` |
| Superset config | ✅ Complete | `internal/config/config.go` (SupersetConfig) |
| Compliance module | ✅ Complete | `internal/modules/compliance/` |
| Compliance HTTP handlers | ✅ Complete | `internal/http/handlers/compliance/handler.go` |
| Compliance Ent schemas | ✅ Complete | `data_subject_requests`, `data_export_jobs`, `data_deletion_jobs` |
| Router/App wiring | ✅ Complete | Analytics and compliance handlers registered |
| Performance optimization | ✅ Complete | Database indexes, connection pooling, Redis caching service |
| Security hardening | ✅ Complete | Rate limiting, security headers, input validation middleware |

---

## Overview

Sprint 7 focuses on building analytics and reporting capabilities, implementing compliance features (GDPR/DPA), and hardening the system for production.

---

## Objectives

1. Reporting endpoints
2. Data export/delete tooling
3. Performance tuning
4. Security hardening
5. Subscription invoicing hardening
6. Penetration testing

---

## Technology Stack

### Analytics
- **Reporting**: Custom report generation
- **Export**: CSV, PDF export
- **Caching**: Redis for report caching

### Compliance
- **Data Export**: JSON/CSV export
- **Data Deletion**: Soft delete with retention policies
- **Audit Logs**: Comprehensive audit trail

### Performance
- **Caching**: Redis for hot data
- **Database**: Query optimization, indexes
- **Connection Pooling**: Optimized pool sizes

### Security
- **OWASP**: ASVS baseline compliance
- **Rate Limiting**: Per-tenant rate limits
- **Input Validation**: Comprehensive validation
- **SQL Injection**: Parameterized queries

---

## User Stories

### US-7.1: Analytics Reports
**As a** cafe administrator  
**I want** to generate analytics reports  
**So that** I can understand business performance

**Acceptance Criteria**:
- [ ] Order analytics reports
- [ ] Revenue reports
- [ ] Customer analytics
- [ ] Export to CSV/PDF
- [ ] Scheduled report generation

### US-7.2: Data Export
**As a** user
**I want** to export my data
**So that** I can comply with data portability requirements

**Acceptance Criteria**:
- [x] Data export endpoint
- [x] JSON/CSV export format
- [x] Complete user data export
- [x] Export job tracking

### US-7.3: Data Deletion
**As a** user
**I want** to delete my account and data
**So that** I can exercise my right to be forgotten

**Acceptance Criteria**:
- [x] Account deletion endpoint
- [x] Soft delete with retention period
- [x] Data anonymization
- [x] Deletion confirmation

### US-7.4: Performance Optimization
**As a** system administrator
**I want** optimized database queries
**So that** the system performs well under load

**Acceptance Criteria**:
- [x] Query optimization
- [x] Database indexes (User, Order, Payment, NotificationEvent schemas)
- [x] Connection pooling optimization (configurable via environment variables)
- [x] Caching strategy (Redis caching service with TTL configuration)

### US-7.5: Security Hardening
**As a** security officer
**I want** the system to be secure
**So that** user data is protected

**Acceptance Criteria**:
- [x] OWASP ASVS compliance (security headers middleware)
- [x] Rate limiting implementation (Redis-based sliding window, per-IP, per-tenant, per-endpoint)
- [x] Input validation (SQL injection and XSS pattern detection middleware)
- [x] SQL injection prevention (pattern detection in query parameters and headers)
- [x] XSS prevention (pattern detection and security headers)

---

## API Endpoints

### Reports

**GET /api/v1/{tenant}/reports/orders**
- Generate order analytics report
- Query params: `date_from`, `date_to`, `cafe_id`, `format` (csv/pdf)
- Response: Report file or download URL

**GET /api/v1/{tenant}/reports/revenue**
- Generate revenue report
- Query params: Same as orders

**GET /api/v1/{tenant}/reports/customers**
- Generate customer analytics report
- Query params: Same as orders

**POST /api/v1/{tenant}/reports/jobs**
- Create report generation job
- Request: `{ "report_type": "...", "parameters": {...}, "format": "csv" }`
- Response: Job ID

**GET /api/v1/{tenant}/reports/jobs/{id}**
- Get report job status
- Response: Job status and result URL

### Data Export

**POST /api/v1/{tenant}/data-export**
- Request data export
- Response: Export job ID

**GET /api/v1/{tenant}/data-export/{id}**
- Get export job status
- Response: Export file URL when ready

### Data Deletion

**POST /api/v1/{tenant}/data-deletion**
- Request account deletion
- Request: `{ "reason": "...", "confirm": true }`
- Response: Deletion job ID

**GET /api/v1/{tenant}/data-deletion/{id}**
- Get deletion job status

### Compliance

**GET /api/v1/{tenant}/audit-logs**
- Query audit logs
- Query params: `user_id`, `action`, `resource_type`, `date_from`, `date_to`
- Pagination support

**GET /api/v1/{tenant}/data-subject-requests**
- List data subject requests
- Query params: `status`, `type`

**POST /api/v1/{tenant}/data-subject-requests**
- Create data subject request
- Request: `{ "request_type": "export|delete", "description": "..." }`

---

## Database Schema

### Analytics Module

**report_jobs**
- `id` (UUID, PK)
- `tenant_id` (UUID, FK → tenants)
- `report_type` (VARCHAR)
- `status` (VARCHAR, default: 'pending')
- `requested_by` (UUID, FK → users)
- `parameters` (JSONB)
- `result_url` (VARCHAR)
- `requested_at` (TIMESTAMPTZ)
- `completed_at` (TIMESTAMPTZ)
- `error_message` (TEXT)

### Compliance Module

**data_subject_requests**
- `id` (UUID, PK)
- `tenant_id` (UUID, FK → tenants)
- `user_id` (UUID, FK → users)
- `request_type` (VARCHAR)
- `status` (VARCHAR, default: 'pending')
- `submitted_at` (TIMESTAMPTZ)
- `processed_at` (TIMESTAMPTZ)
- `notes` (TEXT)

**backup_jobs**
- `id` (UUID, PK)
- `tenant_id` (UUID, FK → tenants)
- `job_type` (VARCHAR)
- `status` (VARCHAR, default: 'pending')
- `requested_by` (UUID, FK → users)
- `storage_url` (VARCHAR)
- `requested_at` (TIMESTAMPTZ)
- `completed_at` (TIMESTAMPTZ)
- `error_message` (TEXT)

**backup_restores**
- `id` (UUID, PK)
- `backup_job_id` (UUID, FK → backup_jobs)
- `initiated_by` (UUID, FK → users)
- `restore_point` (TIMESTAMPTZ)
- `status` (VARCHAR, default: 'pending')
- `started_at` (TIMESTAMPTZ)
- `completed_at` (TIMESTAMPTZ)
- `notes` (TEXT)

**security_policies**
- `id` (UUID, PK)
- `tenant_id` (UUID, FK → tenants)
- `password_policy_json` (JSONB)
- `session_policy_json` (JSONB)
- `created_at`, `updated_at` (TIMESTAMPTZ)

---

## Code Structure

### Module Organization

**Analytics Module** (`internal/modules/analytics/`):
- `report.go` - Report domain models and service
- `generator.go` - Report generation service
- `exporter.go` - Export service (CSV, PDF)

**Compliance Module** (`internal/modules/compliance/`):
- `export.go` - Data export service
- `deletion.go` - Data deletion service
- `request.go` - Data subject request service

**Security Module** (`internal/modules/security/`):
- `rate_limiter.go` - Redis-based rate limiting service with sliding window algorithm
- `middleware.go` - Security middleware (headers, input validation, XSS/SQL injection detection)

---

## Integration Points

### Apache Superset
- **Integration**: Pre-built dashboards for analytics
- **Data Source**: Read-only PostgreSQL connection
- **Dashboards**: Order analytics, revenue, customer analytics

### Backup Service
- **Integration**: Scheduled backup jobs
- **Storage**: S3-compatible storage
- **Retention**: Configurable retention policies

---

## Testing Strategy

### Unit Tests
- Report generation tests
- Data export tests
- Data deletion tests
- Security policy tests

### Integration Tests
- End-to-end report generation
- Data export and import
- Data deletion workflow
- Security hardening validation

### Performance Tests
- Load testing
- Stress testing
- Database query performance
- API response time benchmarks

### Security Tests
- Penetration testing
- SQL injection tests
- XSS tests
- Rate limiting tests

---

## Deliverables

- [x] Reporting endpoints (Analytics dashboard embed)
- [x] Data export tooling
- [x] Data deletion tooling
- [x] Database migrations (Ent schemas for compliance)
- [x] Performance optimizations
  - [x] Database indexes (User, Order, Payment, NotificationEvent)
  - [x] Connection pooling (configurable via env vars)
  - [x] Redis caching service (`internal/platform/cache/service.go`)
- [x] Security hardening
  - [x] Rate limiting middleware (Redis-based sliding window)
  - [x] Security headers (OWASP compliance)
  - [x] Input validation middleware (SQL injection and XSS detection)
  - [x] Content-Type validation
  - [x] Request size limiting
  - [x] Tenant ID format validation
- [ ] Subscription invoicing hardening
- [ ] Penetration testing report
- [ ] Integration tests
- [ ] Performance test results

---

## Dependencies

- Apache Superset for advanced analytics
- S3-compatible storage for backups
- Security audit tools

---

## Security Implementation Details

### Rate Limiting
**Location**: `internal/modules/security/rate_limiter.go`

Features:
- Redis-based sliding window algorithm for accurate rate limiting
- Multiple rate limit strategies:
  - Global IP-based rate limiting (60 req/min default)
  - Tenant-based rate limiting
  - User-based rate limiting
  - Endpoint-specific rate limiting
- Stricter limits for sensitive endpoints:
  - Auth endpoints: 10 req/min (configurable via `RATE_LIMIT_AUTH_PER_MINUTE`)
  - Payment endpoints: 20 req/min (configurable via `RATE_LIMIT_PAYMENT_PER_MINUTE`)
- Configurable burst multiplier for traffic spikes
- Rate limit headers in responses: `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset`, `Retry-After`

Environment Variables:
```bash
RATE_LIMIT_ENABLED=true
RATE_LIMIT_REQUESTS_PER_MINUTE=60
RATE_LIMIT_REQUESTS_PER_HOUR=1000
RATE_LIMIT_AUTH_PER_MINUTE=10
RATE_LIMIT_PAYMENT_PER_MINUTE=20
RATE_LIMIT_BURST_MULTIPLIER=1.5
RATE_LIMIT_KEY_PREFIX=rl:ordering:
```

### Security Middleware
**Location**: `internal/modules/security/middleware.go`

Features:
1. **Security Headers** (OWASP compliance):
   - `X-Content-Type-Options: nosniff`
   - `X-XSS-Protection: 1; mode=block`
   - `X-Frame-Options: DENY`
   - `Referrer-Policy: strict-origin-when-cross-origin`
   - `Content-Security-Policy: default-src 'none'; frame-ancestors 'none'`
   - `Strict-Transport-Security: max-age=31536000; includeSubDomains`
   - `Permissions-Policy: geolocation=(), microphone=(), camera=()`

2. **Input Validation**:
   - SQL injection pattern detection (UNION SELECT, OR 1=1, etc.)
   - XSS pattern detection (<script>, javascript:, onerror=, etc.)
   - Validation of query parameters, headers, and request bodies
   - Automatic rejection of malicious patterns with 400 Bad Request

3. **Request Size Limiting**:
   - Default: 10MB max request body size (configurable via `MAX_REQUEST_BODY_SIZE`)
   - Uses `http.MaxBytesReader` for proper handling

4. **Content-Type Validation**:
   - Enforces `application/json` for POST/PUT/PATCH requests
   - Returns 415 Unsupported Media Type for invalid content types

5. **Tenant ID Validation**:
   - Validates UUID format: `xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx`
   - Validates slug format: `lowercase-alphanumeric-with-hyphens`

Environment Variables:
```bash
SECURITY_HEADERS_ENABLED=true
INPUT_VALIDATION_ENABLED=true
MAX_REQUEST_BODY_SIZE=10485760
```

### Database Performance Optimizations

Added indexes to critical schemas for improved query performance:

1. **User Schema** ([user.go:94-105](internal/ent/schema/user.go#L94-L105)):
   - `(tenant_id, email)` - Unique index for user lookup
   - `(tenant_id, auth_service_user_id)` - Auth service sync
   - `(tenant_id, status)` - Active user queries
   - `(tenant_id, primary_role)` - Role-based queries
   - Additional indexes on email, phone, sync_status, last_login_at, created_at

2. **Order Schema** ([order.go:155-177](internal/ent/schema/order.go#L155-L177)):
   - `(tenant_id, status, created_at)` - Time-based filtering
   - `(tenant_id, cafe_id, status)` - Cafe order queries
   - `(tenant_id, customer_id, status)` - Customer order history
   - `(tenant_id, payment_status)` - Payment tracking
   - Time-based indexes on placed_at, created_at, completed_at, delivered_at
   - Indexes on delivery_address_id and channel for analytics

3. **Payment Schema** ([payment.go:112-123](internal/ent/schema/payment.go#L112-L123)):
   - `(tenant_id, status, created_at)` - Payment reporting
   - `(provider, provider_reference)` - Provider reconciliation
   - Indexes on mpesa_transaction_id, mpesa_phone_number for M-Pesa queries
   - Time-based indexes on processed_at and captured_at

4. **NotificationEvent Schema** ([notificationevent.go:96-106](internal/ent/schema/notificationevent.go#L96-L106)):
   - `(status, created_at)` - Retry queue queries
   - `(status, attempts, last_attempt_at)` - Failed notification retry logic

### Connection Pooling Configuration

**Location**: [app.go:101-114](internal/app/app.go#L101-L114)

Configurable connection pool settings:
```bash
POSTGRES_MAX_OPEN_CONNS=20        # Maximum open connections
POSTGRES_MAX_IDLE_CONNS=10        # Maximum idle connections
POSTGRES_CONN_MAX_LIFETIME=30m    # Connection max lifetime
```

Connection pool is now fully configurable via environment variables instead of hardcoded values.

### Redis Caching Service

**Location**: `internal/platform/cache/service.go`

Features:
- Generic caching service with JSON serialization
- `Get`, `Set`, `Delete` operations with TTL support
- `GetOrSet` pattern for cache-aside strategy
- Pattern-based cache invalidation with `DeletePattern`
- Distributed locking with `SetNX`
- Counter operations with `Increment`
- Predefined cache key patterns for:
  - Menu items and categories
  - User profiles
  - Loyalty accounts
  - Promo codes
  - Shopping carts
  - Active carts

Configurable TTLs:
- Menu items: 30 minutes
- Categories: 1 hour
- User profiles: 10 minutes
- Loyalty accounts: 5 minutes
- Promo codes: 15 minutes

---

## Next Steps

- Sprint 8: Launch & Handover
  - Production deployment
  - Chaos drills
  - Documentation handover
  - Post-launch monitoring

