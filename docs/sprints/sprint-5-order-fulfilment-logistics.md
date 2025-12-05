# Sprint 5 - Order Fulfilment & Logistics Integration

**Duration**: Weeks 10-11  
**Status**: ⏳ Not Started

---

## Overview

Sprint 5 focuses on integrating with the logistics service for delivery task creation, task status updates, and live driver tracking. **IMPORTANT**: All rider, driver, fleet, and delivery task data is owned by `logistics-service`. Cafe backend stores only `rider_id` references and consumes logistics-service APIs.

---

## Objectives

1. Logistics service integration
2. Delivery task creation
3. Task status consumption
4. Live driver tracking (WebSocket/SSE)
5. Rider reference queries
6. Proof of delivery handling

---

## Technology Stack

### Real-Time Communication
- **WebSocket**: Live driver location updates
- **SSE**: Server-Sent Events for order status
- **Event Bus**: NATS JetStream for task events

### Integration
- **REST API**: Logistics service API client
- **Webhooks**: Task status callbacks

---

## User Stories

### US-5.1: Delivery Task Creation
**As a** cafe administrator  
**I want** to create delivery tasks when orders are ready  
**So that** riders can pick up and deliver orders

**Acceptance Criteria**:
- [ ] Automatic task creation on order ready
- [ ] Manual task creation option
- [ ] Task details include order and delivery address
- [ ] Task status tracking

### US-5.2: Task Status Updates
**As a** customer  
**I want** to see real-time order status updates  
**So that** I know when my order will arrive

**Acceptance Criteria**:
- [ ] Order status updates from logistics events
- [ ] Status timeline display
- [ ] ETA updates
- [ ] Delivery confirmation

### US-5.3: Live Driver Tracking
**As a** customer  
**I want** to see my driver's location in real-time  
**So that** I can track my delivery

**Acceptance Criteria**:
- [ ] WebSocket connection to logistics service
- [ ] Real-time location updates
- [ ] Map display with driver location
- [ ] ETA calculation based on location

### US-5.4: Rider Information
**As a** customer  
**I want** to see rider information  
**So that** I can contact the rider if needed

**Acceptance Criteria**:
- [ ] Query rider details from logistics-service API (`GET /v1/{tenant}/fleet-members/{id}`)
- [ ] Rider contact information (from logistics-service)
- [ ] Rider rating display (from logistics-service)
- [ ] Rider photo display (from logistics-service)
- **Note**: All rider data comes from logistics-service, cafe backend only stores `rider_id` reference

### US-5.5: Proof of Delivery
**As a** cafe administrator  
**I want** to receive proof of delivery  
**So that** I can confirm order completion

**Acceptance Criteria**:
- [ ] PoD artifacts (signature, photo, OTP)
- [ ] PoD storage and retrieval
- [ ] PoD display in order details
- [ ] PoD validation

---

## API Endpoints

### Delivery Tasks

**POST /api/v1/{tenant}/orders/{id}/create-delivery-task**
- Create delivery task for order
- Request: `{ "priority": "normal", "instructions": "..." }`
- Response: Task reference from logistics service

**GET /api/v1/{tenant}/orders/{id}/delivery-task**
- Get delivery task details
- Response: Task status, rider info, ETA

**POST /api/v1/{tenant}/orders/{id}/cancel-delivery-task**
- Cancel delivery task
- Request: `{ "reason": "..." }`

### Live Tracking

**GET /api/v1/{tenant}/orders/{id}/tracking**
- Get real-time tracking data
- Response: Driver location, ETA, status

**WebSocket /ws/orders/{id}/tracking**
- WebSocket connection for live updates
- Streams: Location updates, status changes, ETA updates

---

## Database Schema

### Fulfilment Module

**order_assignments**
- `id` (UUID, PK)
- `order_id` (UUID, FK → orders)
- `rider_id` (VARCHAR) - Reference from logistics service
- `logistics_task_id` (VARCHAR) - Reference from logistics service
- `status` (VARCHAR, default: 'pending')
- `assigned_at` (TIMESTAMPTZ)
- `accepted_at` (TIMESTAMPTZ)
- `picked_up_at` (TIMESTAMPTZ)
- `completed_at` (TIMESTAMPTZ)
- `rejected_reason` (TEXT)
- `metadata` (JSONB)

**delivery_windows**
- `id` (UUID, PK)
- `order_id` (UUID, FK → orders)
- `eta_start` (TIMESTAMPTZ)
- `eta_end` (TIMESTAMPTZ)
- `actual_arrival` (TIMESTAMPTZ)
- `actual_dropoff` (TIMESTAMPTZ)

**proof_of_delivery**
- `id` (UUID, PK)
- `order_id` (UUID, FK → orders)
- `logistics_task_id` (VARCHAR)
- `signature_url` (VARCHAR)
- `photo_urls` (JSONB)
- `otp_verified` (BOOLEAN)
- `delivered_at` (TIMESTAMPTZ)
- `metadata` (JSONB)

---

## Code Structure

### Module Organization

**Fulfilment Module** (`internal/modules/fulfilment/`):
- `task.go` - Delivery task domain models and service
- `tracking.go` - Live tracking service
- `pod.go` - Proof of delivery domain models and service

**Logistics Client** (`internal/platform/logistics/`):
- `client.go` - Logistics service REST API client
- `websocket.go` - WebSocket client for live tracking
- `events.go` - Event handler for logistics events

---

## Integration Points

### Logistics Service
**Entity Ownership**: All rider, driver, fleet, delivery task, shift, telemetry, and proof-of-delivery data is owned by `logistics-service`.

**REST API Usage**:
- Task creation: `POST /v1/{tenant}/tasks`
- Rider queries: `GET /v1/{tenant}/fleet-members` (all rider data from logistics-service)
- Task status: `GET /v1/{tenant}/tasks/{id}`
- **DO NOT** store rider profiles or fleet data in cafe-backend

**WebSocket/SSE**: Live driver location stream from logistics-service

**Events Consumed**:
  - `logistics.task.assigned` - Rider assigned (stores only `rider_id` reference)
  - `logistics.task.accepted` - Rider accepted
  - `logistics.task.en_route` - Rider en route
  - `logistics.task.completed` - Delivery completed
  - `logistics.task.cancelled` - Task cancelled
  - `logistics.route.updated` - ETA updated

**Events Published**:
  - `cafe.order.ready` - Order ready for delivery (triggers task creation in logistics-service)

**Events Published**:
- `cafe.order.ready` - Order ready for delivery (triggers task creation)

### Order Service
- **Integration**: Order status updates from task events
- **Status Mapping**: Task status → Order status

---

## Testing Strategy

### Unit Tests
- Task creation service tests
- Event handler tests
- WebSocket client tests

### Integration Tests
- End-to-end delivery flow
- WebSocket connection and message handling
- Event processing from logistics service
- Proof of delivery handling

---

## Deliverables

- [ ] Logistics service REST API client
- [ ] Delivery task creation
- [ ] Task status event handlers
- [ ] WebSocket client for live tracking
- [ ] Rider information queries (via logistics-service API, not stored locally)
- [ ] Proof of delivery handling
- [ ] Order status updates from logistics
- [ ] Database migrations
- [ ] Integration tests

---

## Dependencies

- Logistics service for all rider/driver/fleet logic (cafe backend only stores `rider_id` references)
- WebSocket support in logistics service
- NATS JetStream for events

---

## Next Steps

- Sprint 6: Notifications & Ops
  - Event pipeline to notifications service
  - SLA monitoring
  - Issue escalation
  - Support endpoints

