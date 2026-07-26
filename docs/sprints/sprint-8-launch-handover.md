# Sprint 8 - Launch & Handover

**Duration**: Week 16  
**Status**: ⏳ Not Started

---

## Overview

Sprint 8 focuses on production deployment, chaos engineering drills, documentation handover, and post-launch monitoring setup.

---

## Objectives

1. Production deployment
2. Chaos engineering drills
3. Documentation handover
4. Post-launch monitoring
5. Backlog triage
6. Production readiness review
7. **Critical Infrastructure Fixes** (from January 2026 audit)

---

## Critical Pre-Launch Fixes (January 2026 Audit)

### CRITICAL-1: Implement Outbox Background Publisher
**Priority**: CRITICAL - Events may be lost without this
**Status**: ✅ Implemented (January 2026)

**Files Created/Updated**:
- [x] `internal/platform/events/outbox_adapter.go` - NATS publisher adapter for outbox
- [x] `internal/app/app.go` - Wired outbox publisher to app lifecycle with graceful shutdown
- [x] `internal/config/config.go` - Added OutboxEnabled, OutboxPollPeriod, OutboxBatchSize config

### CRITICAL-2: Fix CORS Configuration
**Priority**: HIGH - Browsers reject current configuration
**Status**: ✅ Implemented (January 2026)

**Files Updated**:
- [x] `internal/config/config.go` - Added AllowedOrigins with production URLs as defaults
- [x] `internal/http/router/router.go` - Uses configurable AllowedOrigins from config

**Production Origins Configured**:
- `https://ordering.codevertexafrica.com`
- `https://theurbanloftcafe.com`
- `https://pos.codevertexafrica.com`
- `https://accounts.codevertexafrica.com`

### CRITICAL-3: Implement Subscription Feature Gating
**Priority**: HIGH - Premium features not protected
**Status**: ✅ Implemented (January 2026)

**Files Updated**:
- [x] `internal/http/handlers/analytics/handler.go` - Added `authclient.RequirePlan("PROFESSIONAL")` middleware

Premium features (analytics) now require PROFESSIONAL subscription plan.

### MEDIUM-1: Audit Logging Middleware
**Priority**: MEDIUM - Compliance requirement
**Status**: ✅ Implemented (January 2026)

**Files Created/Updated**:
- [x] `internal/ent/schema/auditlog.go` - AuditLog entity schema with indexes
- [x] `internal/modules/audit/logger.go` - Audit logger with async recording
- [x] `internal/modules/audit/middleware.go` - MutationAudit middleware
- [x] `internal/http/router/router.go` - Wired audit middleware after auth
- [x] `internal/app/app.go` - Initialized audit logger

**Features**:
- Logs all mutation operations (POST, PUT, PATCH, DELETE)
- Captures user ID, tenant ID from JWT claims
- Sanitizes sensitive fields in request body (passwords, tokens, etc.)
- Extracts resource type and ID from URL path
- Records request duration, IP address, user agent
- Asynchronous logging to avoid blocking responses
- Skips health checks, webhooks, and auth endpoints

---

## Technology Stack

### Deployment
- **Containerization**: Docker multi-stage builds
- **Orchestration**: Kubernetes (via devops-k8s)
- **CI/CD**: GitHub Actions → ArgoCD
- **GitOps**: ArgoCD application manifests

### Monitoring
- **Metrics**: Prometheus + Grafana
- **Logging**: Centralized logging (ELK/Seq)
- **Tracing**: Jaeger distributed tracing
- **Alerting**: AlertManager

### Chaos Engineering
- **Tools**: Chaos Mesh or similar
- **Scenarios**: Network failures, pod failures, database failures

---

## User Stories

### US-8.1: Production Deployment
**As a** DevOps engineer  
**I want** to deploy the service to production  
**So that** users can access the platform

**Acceptance Criteria**:
- [ ] Production environment configured
- [ ] Database migrations executed
- [ ] Secrets configured
- [ ] Health checks passing
- [ ] Zero-downtime deployment

### US-8.2: Chaos Drills
**As a** system administrator  
**I want** to run chaos engineering drills  
**So that** I can verify system resilience

**Acceptance Criteria**:
- [ ] Network failure scenarios
- [ ] Pod failure scenarios
- [ ] Database failure scenarios
- [ ] Recovery procedures documented
- [ ] Runbook created

### US-8.3: Documentation
**As a** developer  
**I want** comprehensive documentation  
**So that** I can maintain and extend the system

**Acceptance Criteria**:
- [ ] API documentation (OpenAPI/Swagger)
- [ ] Architecture documentation
- [ ] Deployment guide
- [ ] Runbook for operations
- [ ] Troubleshooting guide

### US-8.4: Monitoring Setup
**As a** operations team member  
**I want** comprehensive monitoring  
**So that** I can track system health

**Acceptance Criteria**:
- [ ] Prometheus metrics configured
- [ ] Grafana dashboards created
- [ ] Alert rules configured
- [ ] Log aggregation setup
- [ ] Distributed tracing enabled

### US-8.5: Production Readiness
**As a** product owner  
**I want** production readiness verification  
**So that** I can launch with confidence

**Acceptance Criteria**:
- [ ] Security audit completed
- [ ] Performance benchmarks met
- [ ] Load testing completed
- [ ] Disaster recovery tested
- [ ] Compliance verified

---

## Deployment Checklist

### Pre-Deployment
- [ ] All tests passing
- [ ] Security scan completed
- [ ] Performance benchmarks met
- [ ] Database migrations tested
- [ ] Secrets configured
- [ ] Environment variables set
- [ ] Health checks implemented
- [ ] Monitoring configured

### Deployment
- [ ] Docker image built and pushed
- [ ] Kubernetes manifests updated
- [ ] ArgoCD application synced
- [ ] Database migrations executed
- [ ] Service health verified
- [ ] Smoke tests passed

### Post-Deployment
- [ ] Monitoring dashboards verified
- [ ] Alert rules tested
- [ ] Log aggregation verified
- [ ] Performance metrics baseline established
- [ ] Documentation updated

---

## Chaos Engineering Scenarios

### Network Failures
- **Scenario**: Simulate network partition
- **Expected**: Service degrades gracefully, recovers automatically
- **Runbook**: Document recovery procedures

### Pod Failures
- **Scenario**: Random pod termination
- **Expected**: Kubernetes restarts pods, service continues
- **Runbook**: Document pod restart procedures

### Database Failures
- **Scenario**: Database connection loss
- **Expected**: Service handles errors gracefully, retries with backoff
- **Runbook**: Document database recovery procedures

### High Load
- **Scenario**: Traffic spike simulation
- **Expected**: Auto-scaling triggers, service handles load
- **Runbook**: Document scaling procedures

---

## Monitoring Setup

### Metrics

**Application Metrics**:
- Request rate (requests/second)
- Request latency (p50, p95, p99)
- Error rate (errors/second)
- Active connections

**Business Metrics**:
- Orders per minute
- Revenue per hour
- Active users
- Cart abandonment rate

**Infrastructure Metrics**:
- CPU usage
- Memory usage
- Disk I/O
- Network I/O

### Dashboards

**Operational Dashboard**:
- Service health overview
- Request metrics
- Error rates
- Infrastructure metrics

**Business Dashboard**:
- Order metrics
- Revenue metrics
- User metrics
- Performance metrics

### Alerts

**Critical Alerts**:
- Service down
- High error rate (>5%)
- Database connection failures
- Payment processing failures

**Warning Alerts**:
- High latency (p95 > 1s)
- High CPU usage (>80%)
- High memory usage (>80%)
- Low disk space (<20%)

---

## Documentation Deliverables

### Technical Documentation
- [ ] API documentation (OpenAPI/Swagger)
- [ ] Architecture diagrams
- [ ] Database schema documentation
- [ ] Integration guides
- [ ] Deployment guide

### Operational Documentation
- [ ] Runbook for common operations
- [ ] Troubleshooting guide
- [ ] Incident response procedures
- [ ] Disaster recovery plan
- [ ] Backup and restore procedures

### User Documentation
- [ ] User guide
- [ ] Admin guide
- [ ] API usage examples
- [ ] FAQ

---

## Post-Launch Activities

### Week 1
- [ ] Monitor system health
- [ ] Review error logs
- [ ] Analyze performance metrics
- [ ] Collect user feedback
- [ ] Address critical issues

### Week 2-4
- [ ] Performance optimization
- [ ] Bug fixes
- [ ] Feature enhancements
- [ ] Documentation updates
- [ ] Team training

### Ongoing
- [ ] Regular security audits
- [ ] Performance monitoring
- [ ] Capacity planning
- [ ] Feature backlog prioritization

---

## Deliverables

- [ ] Production deployment completed
- [ ] Chaos engineering drills executed
- [ ] Documentation handover completed
- [ ] Monitoring dashboards configured
- [ ] Alert rules configured
- [ ] Runbook created
- [ ] Production readiness review completed
- [ ] Post-launch monitoring plan
- [ ] Backlog triage completed

---

## Dependencies

- devops-k8s for Kubernetes deployment
- Prometheus and Grafana for monitoring
- Chaos engineering tools
- Security audit tools

---

## Success Criteria

- [ ] Zero-downtime deployment achieved
- [ ] All health checks passing
- [ ] Performance benchmarks met
- [ ] Security audit passed
- [ ] Documentation complete
- [ ] Team trained on operations
- [ ] Monitoring and alerting functional
- [ ] Disaster recovery tested

---

## Next Steps

- Post-launch monitoring and optimization
- Feature backlog prioritization
- User feedback collection and analysis
- Continuous improvement cycles

