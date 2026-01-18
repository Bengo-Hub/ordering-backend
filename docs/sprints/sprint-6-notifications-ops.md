# Sprint 6 - Notifications & Ops

**Duration**: Weeks 12-13
**Status**: ✅ Complete (January 2026)

---

## Sprint Progress (Updated January 2026)

| Task | Status | Notes |
|------|--------|-------|
| Ent schema: NotificationEvent | ✅ Complete | `internal/ent/schema/notificationevent.go` |
| Ent schema: NotificationTemplate | ✅ Complete | `internal/ent/schema/notificationtemplate.go` |
| Ent schema: NotificationSubscription | ✅ Complete | `internal/ent/schema/notificationsubscription.go` |
| Ent schema: SLAMetric | ✅ Complete | `internal/ent/schema/slametric.go` |
| Run Ent code generation | ✅ Complete | `go generate ./internal/ent` |
| Notifications module domain | ✅ Complete | `internal/modules/notifications/domain.go` |
| Notifications service implementation | ✅ Complete | `internal/modules/notifications/service.go` |
| Notifications repository (Ent) | ✅ Complete | `internal/modules/notifications/repository_ent.go` |
| SLA module domain | ✅ Complete | `internal/modules/sla/domain.go` |
| SLA service implementation | ✅ Complete | `internal/modules/sla/service.go` |
| SLA repository (Ent) | ✅ Complete | `internal/modules/sla/repository_ent.go` |
| Notifications HTTP handlers | ✅ Complete | `internal/http/handlers/notifications/handler.go` |
| SLA HTTP handlers | ✅ Complete | `internal/http/handlers/sla/handler.go` |
| Router integration | ✅ Complete | `internal/http/router/router.go` |
| App initialization | ✅ Complete | `internal/app/app.go` |
| Event publisher wired to order service | ✅ Complete | `OrderService.SetEventPublisher()` in `app.go` |
| Order created event publishing | ✅ Complete | `publishOrderCreated()` in `order_service.go` |
| Order status changed event publishing | ✅ Complete | `publishOrderStatusChanged()` in `order_service.go` |
| Order ready event publishing | ✅ Complete | `publishOrderReady()` in `order_service.go` |
| Order completed event publishing | ✅ Complete | `publishOrderCompleted()` in `order_service.go` |
| Order cancelled event publishing | ✅ Complete | `publishOrderCancelled()` in `order_service.go` |
| Support ticket system | ⏳ Pending | Deferred - handled by ticketing-service |
| Issue escalation workflows | ⏳ Pending | Deferred to future sprint |

---

## Overview

Sprint 6 focuses on integrating with the notifications service for event-driven messaging, implementing SLA monitoring, issue escalation, and support endpoints.

---

## Objectives

1. Event pipeline to notifications service
2. SLA monitoring
3. Issue escalation workflows
4. Support ticket endpoints (deferred to ticketing-service)
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
- [x] Order confirmation notification (via `PublishOrderCreated`)
- [x] Order ready notification (via `PublishOrderReady`)
- [x] Driver assigned notification (via `PublishDriverAssigned`)
- [ ] Delivery ETA updates (requires event from logistics)
- [x] Delivery completion notification (via `PublishDeliveryComplete`)

### US-6.2: SLA Monitoring
**As a** cafe administrator
**I want** to monitor order SLA compliance
**So that** I can ensure timely delivery

**Acceptance Criteria**:
- [x] SLA calculation per order (via `SLA.Service.StartOrderTracking`)
- [x] SLA metric types: order_to_confirm, confirm_to_ready, ready_to_pickup, pickup_to_delivery, end_to_end
- [x] SLA violation detection (via `GetBreachedMetrics`)
- [x] SLA dashboard endpoint (`GET /sla/stats`)
- [x] SLA reports endpoint (`GET /sla/metrics`)

### US-6.3: Support Tickets
**As a** customer
**I want** to create support tickets
**So that** I can get help with issues

**Acceptance Criteria**:
- [ ] Ticket creation endpoint - **Deferred to ticketing-service**
- [ ] Ticket status tracking - **Deferred to ticketing-service**
- [ ] Ticket assignment - **Deferred to ticketing-service**
- [ ] Ticket history - **Deferred to ticketing-service**

**Note**: Support ticket functionality is now handled by the dedicated `ticketing-service` microservice.

### US-6.4: Issue Escalation
**As a** system administrator
**I want** automatic issue escalation
**So that** critical issues are handled promptly

**Acceptance Criteria**:
- [ ] Escalation rules configuration (future sprint)
- [ ] Automatic escalation triggers (future sprint)
- [ ] Escalation notifications (future sprint)
- [ ] Escalation history (future sprint)

### US-6.5: Notification Preferences
**As a** user
**I want** to manage my notification preferences
**So that** I receive only relevant notifications

**Acceptance Criteria**:
- [x] Preference management endpoints (`GET/PUT /notifications/preferences`)
- [x] Channel selection (email, SMS, push) - via `NotificationChannel` enum
- [x] Event type subscriptions - via `NotificationSubscription` entity
- [x] Preference persistence - via Ent repository

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

- [x] Notification event domain model and service (`internal/modules/notifications/`)
- [x] Notification template management (`NotificationTemplate` entity + handlers)
- [x] User preference management (`NotificationSubscription` entity + handlers)
- [x] Order notification helpers (`PublishOrderCreated`, `PublishOrderReady`, etc.)
- [x] NATS event publisher (`internal/platform/events/publisher.go`)
- [x] Event publisher wired to order service (`OrderService.SetEventPublisher()`)
- [x] Order lifecycle events (created, status changed, ready, completed, cancelled)
- [x] Payment events (initiated, completed, failed)
- [x] POS integration events (catalog updated, order for pickup)
- [ ] Support ticket system - **Deferred to ticketing-service**
- [ ] Issue escalation workflows - **Deferred to future sprint**
- [x] SLA monitoring and metrics (`internal/modules/sla/`)
- [x] SLA metric tracking per order (`SLAMetric` entity)
- [x] SLA violation detection (`GetBreachedMetrics`)
- [x] SLA statistics endpoints (`GET /sla/stats`, `GET /sla/breached`)
- [x] Database migrations (via Ent schema auto-migrate)
- [ ] Integration tests - **Ongoing**

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

