# Sprint 4 - Payments Core

**Duration**: Weeks 8-9
**Status**: ✅ Complete (January 2026)

---

## Sprint Progress (Updated January 2026)

| Task | Status | Notes |
|------|--------|-------|
| Ent schema: PaymentMethod | ✅ Complete | `internal/ent/schema/paymentmethod.go` |
| Ent schema: PaymentIntent | ✅ Complete | `internal/ent/schema/paymentintent.go` |
| Ent schema: Payment | ✅ Complete | `internal/ent/schema/payment.go` |
| Ent schema: Refund | ✅ Complete | `internal/ent/schema/refund.go` |
| Ent schema: TreasuryEvent | ✅ Complete | `internal/ent/schema/treasuryevent.go` |
| Run Ent code generation | ✅ Complete | `go generate ./internal/ent` |
| Treasury client implementation | ✅ Complete | `internal/platform/treasury/client.go` |
| Payment service implementation | ✅ Complete | `internal/modules/payments/` |
| Webhook signature verification | ✅ Complete | `internal/platform/treasury/webhook.go` |
| Payment HTTP handlers | ✅ Complete | `internal/http/handlers/payments/payment_handler.go` |
| Payment Method HTTP handlers | ✅ Complete | `internal/http/handlers/payments/method_handler.go` |
| Webhook HTTP handlers | ✅ Complete | `internal/http/handlers/payments/webhook_handler.go` |
| Router integration | ✅ Complete | `internal/http/router/router.go` |
| App initialization | ✅ Complete | `internal/app/app.go` |

---

## Overview

Sprint 4 focuses on integrating with the treasury service for payment processing, handling payment webhooks, and implementing reconciliation workflows.

---

## Objectives

1. Treasury integration (M-Pesa C2B/STK)
2. Payment webhook processing
3. Reconciliation logs
4. Retry policies
5. Refund processing
6. Payment method management

---

## Technology Stack

### Payment Processing
- **Treasury Service**: REST API integration
- **Webhooks**: HMAC signature verification
- **Idempotency**: Request deduplication

### Retry Logic
- **Policy**: Exponential backoff
- **Circuit Breaker**: Failure handling
- **Dead Letter Queue**: Failed payment processing

---

## User Stories

### US-4.1: Payment Initiation
**As a** customer
**I want** to pay for my order
**So that** I can complete my purchase

**Acceptance Criteria**:
- [x] Payment intent creation
- [x] M-Pesa STK Push initiation
- [x] Payment status tracking
- [x] Payment confirmation

### US-4.2: Payment Webhooks
**As a** system administrator
**I want** payment webhooks to be processed reliably
**So that** order status updates automatically

**Acceptance Criteria**:
- [x] Webhook signature verification
- [x] Idempotent webhook processing
- [x] Order status updates on payment success
- [x] Error handling and retry logic

### US-4.3: Payment Methods
**As a** customer
**I want** to save payment methods
**So that** I can checkout faster

**Acceptance Criteria**:
- [x] Payment method storage (tokenized)
- [x] Default payment method selection
- [x] Payment method deletion
- [x] Payment method list

### US-4.4: Refunds
**As a** cafe administrator
**I want** to process refunds
**So that** I can handle customer complaints

**Acceptance Criteria**:
- [x] Refund initiation
- [x] Refund status tracking
- [x] Partial refund support
- [x] Refund history

### US-4.5: Reconciliation
**As a** finance team member
**I want** payment reconciliation reports
**So that** I can verify financial transactions

**Acceptance Criteria**:
- [ ] Reconciliation log generation (Deferred to Sprint 7)
- [ ] Payment vs order matching (Deferred to Sprint 7)
- [ ] Discrepancy detection (Deferred to Sprint 7)
- [ ] Reconciliation reports (Deferred to Sprint 7)

---

## API Endpoints

### Payment Intents

**POST /api/v1/{tenant}/payments/intents**
- Create payment intent
- Request: `{ "order_id": "...", "amount": 1500, "currency": "KES", "payment_method": "mpesa" }`
- Response: Payment intent with client secret

**GET /api/v1/{tenant}/payments/intents/{id}**
- Get payment intent status

### Payments

**GET /api/v1/{tenant}/payments**
- List payments
- Query params: `order_id`, `status`, `date_from`, `date_to`
- Pagination support

**GET /api/v1/{tenant}/payments/{id}**
- Get payment details

### Payment Methods

**GET /api/v1/{tenant}/payment-methods**
- List user's payment methods

**POST /api/v1/{tenant}/payment-methods**
- Add payment method
- Request: `{ "type": "card", "provider": "stripe", "token": "..." }`

**PUT /api/v1/{tenant}/payment-methods/{id}**
- Update payment method (e.g., set as default)

**DELETE /api/v1/{tenant}/payment-methods/{id}**
- Delete payment method

### Refunds

**POST /api/v1/{tenant}/refunds**
- Initiate refund
- Request: `{ "payment_id": "...", "amount": 500, "reason": "..." }`

**GET /api/v1/{tenant}/refunds**
- List refunds
- Query params: `payment_id`, `status`

**GET /api/v1/{tenant}/refunds/{id}**
- Get refund details

### Webhooks

**POST /api/v1/webhooks/treasury**
- Treasury payment webhook endpoint
- Signature verification required
- Processes payment status updates

### Reconciliation

**GET /api/v1/{tenant}/reconciliation**
- Get reconciliation report
- Query params: `date_from`, `date_to`, `cafe_id`

---

## Database Schema

### Payments Module

**payment_methods**
- `id` (UUID, PK)
- `user_id` (UUID, FK → users)
- `tenant_id` (UUID, FK → tenants)
- `provider` (VARCHAR)
- `type` (VARCHAR)
- `mask` (VARCHAR)
- `exp_month` (INTEGER)
- `exp_year` (INTEGER)
- `is_default` (BOOLEAN, default: false)
- `fingerprint` (VARCHAR)
- `created_at`, `updated_at` (TIMESTAMPTZ)

**payment_intents**
- `id` (UUID, PK)
- `order_id` (UUID, FK → orders)
- `provider` (VARCHAR)
- `client_secret` (VARCHAR)
- `status` (VARCHAR, default: 'pending')
- `amount` (DECIMAL)
- `currency` (VARCHAR)
- `metadata` (JSONB)
- `created_at`, `updated_at` (TIMESTAMPTZ)

**payments**
- `id` (UUID, PK)
- `payment_intent_id` (UUID, FK → payment_intents)
- `order_id` (UUID, FK → orders)
- `amount` (DECIMAL)
- `currency` (VARCHAR)
- `status` (VARCHAR, default: 'pending')
- `provider_reference` (VARCHAR)
- `processed_at` (TIMESTAMPTZ)
- `captured_at` (TIMESTAMPTZ)
- `metadata` (JSONB)

**refunds**
- `id` (UUID, PK)
- `payment_id` (UUID, FK → payments)
- `amount` (DECIMAL)
- `currency` (VARCHAR)
- `status` (VARCHAR, default: 'pending')
- `reason` (TEXT)
- `requested_at` (TIMESTAMPTZ)
- `processed_at` (TIMESTAMPTZ)
- `metadata` (JSONB)

**payouts**
- `id` (UUID, PK)
- `tenant_id` (UUID, FK → tenants)
- `recipient_type` (VARCHAR)
- `recipient_id` (UUID)
- `amount` (DECIMAL)
- `currency` (VARCHAR)
- `status` (VARCHAR, default: 'pending')
- `scheduled_at` (TIMESTAMPTZ)
- `processed_at` (TIMESTAMPTZ)
- `metadata` (JSONB)

**settlements**
- `id` (UUID, PK)
- `tenant_id` (UUID, FK → tenants)
- `cafe_id` (UUID, FK → cafes)
- `period_start` (TIMESTAMPTZ)
- `period_end` (TIMESTAMPTZ)
- `gross_amount` (DECIMAL)
- `net_amount` (DECIMAL)
- `status` (VARCHAR)
- `generated_at` (TIMESTAMPTZ)
- `metadata` (JSONB)

**treasury_events**
- `id` (UUID, PK)
- `external_id` (VARCHAR, UNIQUE)
- `event_type` (VARCHAR)
- `payload` (JSONB)
- `received_at` (TIMESTAMPTZ)
- `processed_at` (TIMESTAMPTZ)
- `status` (VARCHAR, default: 'pending')
- `error_message` (TEXT)

---

## Code Structure

### Module Organization

**Payments Module** (`internal/modules/payments/`):
- `intent.go` - Payment intent domain models and service
- `payment.go` - Payment domain models and service
- `refund.go` - Refund domain models and service
- `method.go` - Payment method domain models and service
- `webhook.go` - Webhook processing service
- `reconciliation.go` - Reconciliation service

**Treasury Client** (`internal/platform/treasury/`):
- `client.go` - Treasury service REST API client
- `webhook.go` - Webhook signature verification

---

## Integration Points

### Treasury App
- **REST API**: Payment intent creation, refund processing
- **Webhook**: Payment status updates, refund confirmations
- **Events**: `treasury.payment.success`, `treasury.payment.failed`, `treasury.refund.completed`

### Order Service
- **Event**: `cafe.payment.initiated` - Payment intent created
- **Event**: `cafe.payment.completed` - Payment successful, update order
- **Event**: `cafe.payment.failed` - Payment failed, handle retry

---

## Testing Strategy

### Unit Tests
- Payment intent creation tests
- Webhook signature verification tests
- Refund processing tests
- Reconciliation calculation tests

### Integration Tests
- End-to-end payment flow
- Webhook processing with treasury service
- Refund flow
- Reconciliation report generation

---

## Deliverables

- [x] Treasury service integration
- [x] Payment intent creation
- [x] M-Pesa STK Push integration
- [x] Payment webhook processing
- [x] Payment method management
- [x] Refund processing
- [ ] Reconciliation logs (deferred to Sprint 7)
- [x] Retry policies and error handling
- [x] Database migrations (via Ent schema)
- [ ] Integration tests (ongoing)

---

## Dependencies

- Treasury app for payment processing
- M-Pesa Daraja API (via treasury app)
- Webhook signature secrets

---

## Next Steps

- Sprint 5: Order Fulfilment & Logistics Integration
  - Logistics service integration
  - Delivery task creation
  - Task status consumption
  - Live driver tracking

