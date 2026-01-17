# Sprint 3 - Orders & Cart

**Duration**: Weeks 6-7
**Status**: 🚧 In Progress (January 2026)

---

## Sprint Progress (Updated January 2026)

| Task | Status | Notes |
|------|--------|-------|
| Ent schema: Cart | ✅ Complete | `internal/ent/schema/cart.go` |
| Ent schema: CartItem | ✅ Complete | `internal/ent/schema/cartitem.go` |
| Ent schema: Order | ✅ Complete | `internal/ent/schema/order.go` |
| Ent schema: OrderItem | ✅ Complete | `internal/ent/schema/orderitem.go` |
| Ent schema: OrderEvent | ✅ Complete | `internal/ent/schema/orderevent.go` |
| Ent schema: CustomerAddress | ✅ Complete | `internal/ent/schema/customeraddress.go` |
| Ent schema: PromoCode | ✅ Complete | `internal/ent/schema/promocode.go` |
| Ent schema: PromoRedemption | ✅ Complete | `internal/ent/schema/promoredemption.go` |
| Ent schema: LoyaltyAccount | ✅ Complete | `internal/ent/schema/loyaltyaccount.go` |
| Ent schema: LoyaltyTransaction | ✅ Complete | `internal/ent/schema/loyaltytransaction.go` |
| User schema edges updated | ✅ Complete | Added carts, orders, addresses, loyalty_account edges |
| Run Ent code generation | ⏳ Pending | `go generate ./ent` |
| Cart service implementation | ⏳ Pending | |
| Cart HTTP handlers | ⏳ Pending | |
| Order service implementation | ⏳ Pending | |
| Order state machine | ⏳ Pending | |
| Checkout workflow | ⏳ Pending | |
| PromoCode validation | ⏳ Pending | |

**Next Steps**:
1. Run `go generate ./ent` to generate Ent code
2. Implement Cart module (service, repository, handlers)
3. Implement Order module with state machine
4. Implement checkout workflow

---

## Overview

Sprint 3 focuses on building the ordering system with cart persistence, checkout workflow, promo code validation, and order state machine.

---

## Objectives

1. Cart service with persistence
2. Checkout workflow implementation
3. Promo code engine
4. Order state machine
5. Idempotent order creation
6. Loyalty points integration

---

## Technology Stack

### State Management
- **Cart Storage**: Redis for active carts, PostgreSQL for persistence
- **Session Management**: Cart tied to user session

### Business Logic
- **State Machine**: Order status transitions
- **Validation**: Promo code rules engine
- **Calculation**: Price calculation with discounts and taxes

### Event Publishing
- **Event Bus**: NATS JetStream
- **Order Events**: Order lifecycle events

---

## User Stories

### US-3.1: Shopping Cart
**As a** customer  
**I want** to add items to my cart  
**So that** I can prepare my order

**Acceptance Criteria**:
- [ ] Add items to cart
- [ ] Update item quantities
- [ ] Remove items from cart
- [ ] Cart persistence across sessions
- [ ] Cart expiration handling

### US-3.2: Checkout Process
**As a** customer  
**I want** to checkout my cart  
**So that** I can place an order

**Acceptance Criteria**:
- [ ] Checkout initiation
- [ ] Address selection/entry
- [ ] Payment method selection
- [ ] Order summary display
- [ ] Order confirmation

### US-3.3: Promo Codes
**As a** customer  
**I want** to apply promo codes  
**So that** I can get discounts

**Acceptance Criteria**:
- [ ] Promo code validation
- [ ] Discount calculation
- [ ] Usage limits enforcement
- [ ] Expiration date checking

### US-3.4: Order Management
**As a** cafe administrator  
**I want** to manage orders  
**So that** I can track order status

**Acceptance Criteria**:
- [ ] Order status transitions
- [ ] Order cancellation
- [ ] Order history
- [ ] Order search and filtering

### US-3.5: Loyalty Points
**As a** customer  
**I want** to earn and redeem loyalty points  
**So that** I can get rewards

**Acceptance Criteria**:
- [ ] Points accrual on order completion
- [ ] Points redemption at checkout
- [ ] Points balance display
- [ ] Points transaction history

---

## API Endpoints

### Cart

**GET /api/v1/{tenant}/carts/current**
- Get current user's active cart
- Response: Cart object with items

**POST /api/v1/{tenant}/carts/items**
- Add item to cart
- Request: `{ "menu_item_id": "...", "variant_id": "...", "quantity": 2 }`

**PUT /api/v1/{tenant}/carts/items/{id}**
- Update cart item quantity
- Request: `{ "quantity": 3 }`

**DELETE /api/v1/{tenant}/carts/items/{id}**
- Remove item from cart

**DELETE /api/v1/{tenant}/carts/current**
- Clear entire cart

### Checkout

**POST /api/v1/{tenant}/checkout**
- Initiate checkout
- Request: `{ "delivery_address_id": "...", "payment_method_id": "...", "promo_code": "...", "loyalty_points_redeemed": 0 }`
- Response: Order object with payment intent

**POST /api/v1/{tenant}/checkout/validate**
- Validate checkout data before submission
- Request: Same as checkout
- Response: Validation result with errors

### Promo Codes

**POST /api/v1/{tenant}/promo-codes/validate**
- Validate promo code
- Request: `{ "code": "SAVE20" }`
- Response: `{ "valid": true, "discount_amount": 200, "discount_type": "percentage" }`

### Orders

**GET /api/v1/{tenant}/orders**
- List user's orders
- Query params: `status`, `date_from`, `date_to`
- Pagination support

**GET /api/v1/{tenant}/orders/{id}**
- Get order details
- Includes items, status history, payment info

**POST /api/v1/{tenant}/orders/{id}/cancel**
- Cancel order
- Request: `{ "reason": "..." }`

### Loyalty

**GET /api/v1/{tenant}/loyalty/balance**
- Get loyalty points balance

**GET /api/v1/{tenant}/loyalty/transactions**
- Get loyalty transaction history
- Pagination support

---

## Database Schema

### Ordering Module

**customer_addresses**
- `id` (UUID, PK)
- `tenant_id` (UUID, FK → tenants)
- `user_id` (UUID, FK → users)
- `label` (VARCHAR)
- `address_line1` (VARCHAR)
- `address_line2` (VARCHAR)
- `city` (VARCHAR)
- `county` (VARCHAR)
- `postal_code` (VARCHAR)
- `latitude` (DECIMAL)
- `longitude` (DECIMAL)
- `instructions` (TEXT)
- `is_default` (BOOLEAN, default: false)
- `created_at`, `updated_at` (TIMESTAMPTZ)

**carts**
- `id` (UUID, PK)
- `tenant_id` (UUID, FK → tenants)
- `user_id` (UUID, FK → users)
- `status` (VARCHAR, default: 'active')
- `currency` (VARCHAR, default: 'KES')
- `subtotal` (DECIMAL)
- `discount_total` (DECIMAL)
- `tax_total` (DECIMAL)
- `delivery_fee` (DECIMAL)
- `loyalty_points_redeemed` (INTEGER, default: 0)
- `expires_at` (TIMESTAMPTZ)
- `created_at`, `updated_at` (TIMESTAMPTZ)

**cart_items**
- `id` (UUID, PK)
- `cart_id` (UUID, FK → carts)
- `menu_item_id` (UUID, FK → menu_items)
- `variant_id` (UUID, FK → menu_item_variants)
- `name_snapshot` (VARCHAR)
- `quantity` (INTEGER)
- `unit_price` (DECIMAL)
- `total_price` (DECIMAL)
- `notes` (TEXT)
- `metadata` (JSONB)
- `created_at`, `updated_at` (TIMESTAMPTZ)

**orders**
- `id` (UUID, PK)
- `tenant_id` (UUID, FK → tenants)
- `cafe_id` (UUID, FK → cafes)
- `customer_id` (UUID, FK → users)
- `cart_id` (UUID, FK → carts)
- `status` (VARCHAR, default: 'pending')
- `payment_status` (VARCHAR, default: 'pending')
- `currency` (VARCHAR, default: 'KES')
- `subtotal` (DECIMAL)
- `discount_total` (DECIMAL)
- `tax_total` (DECIMAL)
- `delivery_fee` (DECIMAL)
- `tip_total` (DECIMAL)
- `grand_total` (DECIMAL)
- `loyalty_points_earned` (INTEGER, default: 0)
- `loyalty_points_redeemed` (INTEGER, default: 0)
- `delivery_address_id` (UUID, FK → customer_addresses)
- `instructions` (TEXT)
- `channel` (VARCHAR)
- `source` (VARCHAR)
- `placed_at` (TIMESTAMPTZ)
- `ready_at` (TIMESTAMPTZ)
- `delivered_at` (TIMESTAMPTZ)
- `cancelled_at` (TIMESTAMPTZ)
- `cancellation_reason` (TEXT)
- `metadata` (JSONB)
- `created_at`, `updated_at` (TIMESTAMPTZ)

**order_items**
- `id` (UUID, PK)
- `order_id` (UUID, FK → orders)
- `menu_item_id` (UUID, FK → menu_items)
- `variant_id` (UUID, FK → menu_item_variants)
- `name_snapshot` (VARCHAR)
- `quantity` (INTEGER)
- `unit_price` (DECIMAL)
- `total_price` (DECIMAL)
- `notes` (TEXT)
- `metadata` (JSONB)

**order_events**
- `id` (UUID, PK)
- `order_id` (UUID, FK → orders)
- `event_type` (VARCHAR)
- `payload` (JSONB)
- `actor_user_id` (UUID, FK → users)
- `occurred_at` (TIMESTAMPTZ)

**promo_codes**
- `id` (UUID, PK)
- `tenant_id` (UUID, FK → tenants)
- `code` (VARCHAR, UNIQUE)
- `description` (TEXT)
- `discount_type` (VARCHAR)
- `discount_value` (DECIMAL)
- `max_uses` (INTEGER)
- `usage_count` (INTEGER, default: 0)
- `min_subtotal` (DECIMAL)
- `starts_at` (TIMESTAMPTZ)
- `ends_at` (TIMESTAMPTZ)
- `metadata` (JSONB)
- `created_at`, `updated_at` (TIMESTAMPTZ)

**promo_redemptions**
- `id` (UUID, PK)
- `promo_code_id` (UUID, FK → promo_codes)
- `order_id` (UUID, FK → orders)
- `user_id` (UUID, FK → users)
- `redeemed_at` (TIMESTAMPTZ)
- `discount_amount` (DECIMAL)

**loyalty_accounts**
- `id` (UUID, PK)
- `tenant_id` (UUID, FK → tenants)
- `user_id` (UUID, FK → users)
- `balance_points` (INTEGER, default: 0)
- `tier` (VARCHAR)
- `lifetime_points` (INTEGER, default: 0)
- `created_at`, `updated_at` (TIMESTAMPTZ)

**loyalty_transactions**
- `id` (UUID, PK)
- `account_id` (UUID, FK → loyalty_accounts)
- `order_id` (UUID, FK → orders)
- `points` (INTEGER)
- `transaction_type` (VARCHAR)
- `description` (TEXT)
- `occurred_at` (TIMESTAMPTZ)
- `metadata` (JSONB)

---

## Code Structure

### Module Organization

**Ordering Module** (`internal/modules/ordering/`):
- `cart.go` - Cart domain models and service
- `order.go` - Order domain models and service
- `promo.go` - Promo code domain models and service
- `loyalty.go` - Loyalty domain models and service
- `address.go` - Address domain models and service

**State Machine** (`internal/modules/ordering/state_machine.go`):
- Order status transitions
- Validation rules for transitions
- Event publishing on state changes

---

## Integration Points

### Treasury App
- **Event**: `cafe.order.created` - Trigger payment intent creation
- **Webhook**: `treasury.payment.success` - Update order payment status

### Inventory Service
- **Query**: Stock availability before checkout
- **Event**: `cafe.order.placed` - Reserve stock
- **Event**: `cafe.order.cancelled` - Release stock reservation

### Notifications Service
- **Event**: `cafe.order.created` - Send order confirmation
- **Event**: `cafe.order.status.changed` - Send status update

---

## Testing Strategy

### Unit Tests
- Cart service tests
- Order state machine tests
- Promo code validation tests
- Loyalty points calculation tests

### Integration Tests
- End-to-end checkout flow
- Order creation with payment
- Promo code application
- Loyalty points accrual and redemption

---

## Deliverables

- [ ] Cart CRUD endpoints
- [ ] Checkout workflow
- [ ] Promo code engine
- [ ] Order state machine
- [ ] Idempotent order creation
- [ ] Loyalty points system
- [ ] Address management
- [ ] Order history and search
- [ ] Database migrations
- [ ] Integration tests

---

## Dependencies

- Treasury app for payment processing
- Inventory service for stock checks
- Notifications service for order confirmations

---

## Next Steps

- Sprint 4: Payments Core
  - Treasury integration (M-Pesa STK Push)
  - Payment webhook processing
  - Reconciliation logs
  - Retry policies

