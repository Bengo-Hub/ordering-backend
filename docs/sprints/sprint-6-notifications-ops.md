# Sprint 6 - Notifications & Ops

**Duration**: Weeks 12-13  
**Status**: ⏳ Not Started

---

## Overview

Sprint 6 focuses on integrating with the notifications service for event-driven messaging, implementing SLA monitoring, issue escalation, and support endpoints.

---

## Objectives

1. Event pipeline to notifications service
2. SLA monitoring
3. Issue escalation workflows
4. Support ticket endpoints
5. Notification template management
6. User preference management

---

## Technology Stack

### Event Publishing
- **Event Bus**: NATS JetStream
- **Event Types**: Order events, payment events, delivery events

### Monitoring
- **Metrics**: Prometheus for SLA tracking
- **Alerts**: AlertManager for threshold violations

### Support
- **Ticketing**: Internal support ticket system
- **Escalation**: Automated escalation rules

---

## User Stories

### US-6.1: Order Notifications
**As a** customer  
**I want** to receive notifications about my order  
**So that** I stay informed about order status

**Acceptance Criteria**:
- [ ] Order confirmation notification
- [ ] Order ready notification
- [ ] Driver assigned notification
- [ ] Delivery ETA updates
- [ ] Delivery completion notification

### US-6.2: SLA Monitoring
**As a** cafe administrator  
**I want** to monitor order SLA compliance  
**So that** I can ensure timely delivery

**Acceptance Criteria**:
- [ ] SLA calculation per order
- [ ] SLA violation alerts
- [ ] SLA dashboard
- [ ] SLA reports

### US-6.3: Support Tickets
**As a** customer  
**I want** to create support tickets  
**So that** I can get help with issues

**Acceptance Criteria**:
- [ ] Ticket creation endpoint
- [ ] Ticket status tracking
- [ ] Ticket assignment
- [ ] Ticket history

### US-6.4: Issue Escalation
**As a** system administrator  
**I want** automatic issue escalation  
**So that** critical issues are handled promptly

**Acceptance Criteria**:
- [ ] Escalation rules configuration
- [ ] Automatic escalation triggers
- [ ] Escalation notifications
- [ ] Escalation history

### US-6.5: Notification Preferences
**As a** user  
**I want** to manage my notification preferences  
**So that** I receive only relevant notifications

**Acceptance Criteria**:
- [ ] Preference management endpoints
- [ ] Channel selection (email, SMS, push)
- [ ] Event type subscriptions
- [ ] Preference persistence

---

## API Endpoints

### Notifications

**GET /api/v1/{tenant}/notifications**
- List user's notifications
- Query params: `unread_only`, `type`, `date_from`
- Pagination support

**PUT /api/v1/{tenant}/notifications/{id}/read**
- Mark notification as read

**PUT /api/v1/{tenant}/notifications/read-all**
- Mark all notifications as read

### Notification Preferences

**GET /api/v1/{tenant}/notification-preferences**
- Get user's notification preferences

**PUT /api/v1/{tenant}/notification-preferences**
- Update notification preferences
- Request: `{ "email": true, "sms": false, "push": true, "event_types": [...] }`

### Support Tickets

**GET /api/v1/{tenant}/support-tickets**
- List support tickets
- Query params: `status`, `priority`, `assigned_to`
- Pagination support

**POST /api/v1/{tenant}/support-tickets**
- Create support ticket
- Request: `{ "category": "...", "subject": "...", "description": "...", "priority": "normal" }`

**GET /api/v1/{tenant}/support-tickets/{id}**
- Get ticket details

**PUT /api/v1/{tenant}/support-tickets/{id}**
- Update ticket (status, assignment, etc.)

**POST /api/v1/{tenant}/support-tickets/{id}/comments**
- Add comment to ticket

### SLA Monitoring

**GET /api/v1/{tenant}/sla/metrics**
- Get SLA metrics
- Query params: `date_from`, `date_to`, `cafe_id`
- Response: SLA compliance rates, average times

**GET /api/v1/{tenant}/sla/violations**
- Get SLA violations
- Query params: `date_from`, `date_to`, `severity`

---

## Database Schema

### Notifications Module

**notification_templates**
- `id` (UUID, PK)
- `tenant_id` (UUID, FK → tenants)
- `channel` (VARCHAR)
- `event_key` (VARCHAR)
- `locale` (VARCHAR)
- `subject` (VARCHAR)
- `body` (TEXT)
- `data_schema` (JSONB)
- `is_active` (BOOLEAN, default: true)
- `created_at`, `updated_at` (TIMESTAMPTZ)

**notification_events**
- `id` (UUID, PK)
- `tenant_id` (UUID, FK → tenants)
- `event_key` (VARCHAR)
- `payload` (JSONB)
- `status` (VARCHAR, default: 'pending')
- `attempts` (INTEGER, default: 0)
- `last_attempt_at` (TIMESTAMPTZ)
- `created_at` (TIMESTAMPTZ)

**notification_subscriptions**
- `id` (UUID, PK)
- `tenant_id` (UUID, FK → tenants)
- `user_id` (UUID, FK → users)
- `channel` (VARCHAR)
- `event_key` (VARCHAR)
- `is_subscribed` (BOOLEAN, default: true)
- `updated_at` (TIMESTAMPTZ)

### Support Module

**support_tickets**
- `id` (UUID, PK)
- `tenant_id` (UUID, FK → tenants)
- `user_id` (UUID, FK → users)
- `category` (VARCHAR)
- `priority` (VARCHAR, default: 'normal')
- `status` (VARCHAR, default: 'open')
- `subject` (VARCHAR)
- `description` (TEXT)
- `assigned_to` (UUID, FK → users)
- `created_at`, `updated_at`, `closed_at` (TIMESTAMPTZ)

**support_ticket_events**
- `id` (UUID, PK)
- `ticket_id` (UUID, FK → support_tickets)
- `event_type` (VARCHAR)
- `payload` (JSONB)
- `actor_user_id` (UUID, FK → users)
- `occurred_at` (TIMESTAMPTZ)

### SLA Module

**sla_metrics**
- `id` (UUID, PK)
- `tenant_id` (UUID, FK → tenants)
- `order_id` (UUID, FK → orders)
- `metric_type` (VARCHAR)
- `target_seconds` (INTEGER)
- `actual_seconds` (INTEGER)
- `status` (VARCHAR)
- `measured_at` (TIMESTAMPTZ)

---

## Code Structure

### Module Organization

**Notifications Module** (`internal/modules/notifications/`):
- `event.go` - Notification event domain models and service
- `template.go` - Template domain models and service
- `preference.go` - Preference domain models and service

**Support Module** (`internal/modules/support/`):
- `ticket.go` - Ticket domain models and service
- `escalation.go` - Escalation service

**SLA Module** (`internal/modules/sla/`):
- `metric.go` - SLA metric domain models and service
- `monitor.go` - SLA monitoring service

**Notifications Client** (`internal/platform/notifications/`):
- `client.go` - Notifications service REST API client
- `events.go` - Event publisher for notifications

---

## Integration Points

### Notifications Service
- **REST API**: Send notification, get templates, manage preferences
- **Events Published**:
  - `cafe.order.created` - Order confirmation
  - `cafe.order.ready` - Order ready notification
  - `cafe.order.status.changed` - Status update
  - `cafe.loyalty.points_awarded` - Loyalty notification

**Events Consumed**:
- `notifications.delivery.completed` - Track notification delivery
- `notifications.delivery.failed` - Handle delivery failures

---

## Testing Strategy

### Unit Tests
- Notification event publishing tests
- SLA calculation tests
- Escalation rule tests
- Support ticket workflow tests

### Integration Tests
- End-to-end notification flow
- SLA monitoring and alerting
- Support ticket creation and assignment
- Event publishing to notifications service

---

## Deliverables

- [ ] Event pipeline to notifications service
- [ ] Notification template management
- [ ] User preference management
- [ ] Support ticket system
- [ ] Issue escalation workflows
- [ ] SLA monitoring and metrics
- [ ] SLA violation alerts
- [ ] Database migrations
- [ ] Integration tests

---

## Dependencies

- Notifications service for message delivery
- Prometheus for metrics
- AlertManager for alerts

---

## Next Steps

- Sprint 7: Analytics, Compliance & Hardening
  - Reporting endpoints
  - Data export/delete tooling
  - Performance tuning
  - Security hardening

