# Sprint 7 - Analytics, Compliance & Hardening

**Duration**: Weeks 14-15  
**Status**: ⏳ Not Started

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
- [ ] Data export endpoint
- [ ] JSON/CSV export format
- [ ] Complete user data export
- [ ] Export job tracking

### US-7.3: Data Deletion
**As a** user  
**I want** to delete my account and data  
**So that** I can exercise my right to be forgotten

**Acceptance Criteria**:
- [ ] Account deletion endpoint
- [ ] Soft delete with retention period
- [ ] Data anonymization
- [ ] Deletion confirmation

### US-7.4: Performance Optimization
**As a** system administrator  
**I want** optimized database queries  
**So that** the system performs well under load

**Acceptance Criteria**:
- [ ] Query optimization
- [ ] Database indexes
- [ ] Connection pooling optimization
- [ ] Caching strategy

### US-7.5: Security Hardening
**As a** security officer  
**I want** the system to be secure  
**So that** user data is protected

**Acceptance Criteria**:
- [ ] OWASP ASVS compliance
- [ ] Rate limiting implementation
- [ ] Input validation
- [ ] SQL injection prevention
- [ ] XSS prevention

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
- `policy.go` - Security policy service
- `rate_limit.go` - Rate limiting service
- `validation.go` - Input validation service

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

- [ ] Reporting endpoints
- [ ] Data export tooling
- [ ] Data deletion tooling
- [ ] Performance optimizations
- [ ] Security hardening
- [ ] Subscription invoicing hardening
- [ ] Penetration testing report
- [ ] Database migrations
- [ ] Integration tests
- [ ] Performance test results

---

## Dependencies

- Apache Superset for advanced analytics
- S3-compatible storage for backups
- Security audit tools

---

## Next Steps

- Sprint 8: Launch & Handover
  - Production deployment
  - Chaos drills
  - Documentation handover
  - Post-launch monitoring

