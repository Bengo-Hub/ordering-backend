# Sprint 9 – MVP Launch (March 17, 2026)

**Progress (March 2026)**: Backend scan done. Seed has urban-loft tenant and Busia (via menu categories/items). GET /api/v1/{tenant}/cafes returns outlets; treasury webhook with HMAC and cart/order endpoints present. CORS updated to include ordersapp and theurbanloftcafe.com. Inventory reservation on order placement not yet wired. **Tenant/brand**: Public GET /api/v1/{tenant}/config added; returns tenant display name, logo_url (from metadata), primary_color, secondary_color from TenantSetting.BrandPalette; auth skipped for /config, /cafes, /menu so tenant-scoped catalog remains public. Frontend may alternatively use auth-api GET /api/v1/tenants/by-slug/{slug} for tenant display info.

**RBAC**: Ordering-backend has its own RBAC: roles and permissions in DB (ent Permission, Role, User with M2M). Authentication relies on auth-api JWT (validated by middleware); user sync from auth-service via NATS events. Seed includes roles (customer, rider, staff, admin, superuser) and permissions including semantic (e.g. orders:view, catalog:manage) and CRUD-style (add, read, read_own, change, change_own, delete, manage, manage_own) for orders and catalog. **Redis**: Redis is in use for rate limiting and auth config (e.g. JWKS). Permission/session cache is not implemented in the identity module; **Redis cache for permissions is recommended** for high traffic (e.g. cache user permissions by user ID with TTL). **Events/background jobs**: Outbox publisher (Transactional Outbox) runs in background; NATS used for order events (order.created, order.status.changed, order.ready, order.completed, order.cancelled), auth user sync, and logistics/fulfilment. Event publishing is used in order service; background outbox drains to NATS.

**Duration**: March 6 – March 17, 2026 (10 working days)  
**Status**: 🔴 In Progress  
**Goal**: Ship a working E2E customer ordering flow for the Busia outlet under the `urban-loft` tenant.

---

## Hard Deadline Constraints

- **March 17**: Public launch of `ordersapp.codevertexitsolutions.com` and `orderapi.codevertexitsolutions.com`
- **Tenant**: `urban-loft` only
- **Outlet**: Busia only (remove or deactivate all mock outlets including Kiambu)
- **Scope**: Customer ordering flow only. Admin/staff dashboards are best-effort.

---

## Critical Path Tasks

### CP-1: Busia Outlet Data Enforcement

**Priority**: P0 — blocks everything  
**Owner**: Backend

- [x] Verify `urban-loft` tenant exists in DB seed with correct UUID
- [ ] Verify Busia outlet (`cafes` table) has correct address, coordinates, opening hours, contact info
- [x] Remove or deactivate Kiambu mock outlet and any other test data
- [x] Ensure `GET /v1/urban-loft/outlets` returns only Busia (API: `GET /api/v1/{tenant}/cafes`; seed has only Busia menu data)
- [ ] Verify outlet data syncs correctly with auth-service tenant/outlet events
- [x] Seed at least 15 real menu items with images, categories, prices, and variants for Busia

### CP-2: Menu → Inventory SKU Linkage

**Priority**: P0 — blocks checkout  
**Owner**: Backend

- [ ] Verify `menu_item_variants.sku` maps to real SKUs in inventory-service
- [ ] Test `GET /api/v1/inventory/items/{sku}` returns stock data for each menu item
- [ ] Ensure order placement triggers stock reservation via `POST /api/v1/inventory/reservations`
- [ ] Ensure order cancellation releases reservation
- [ ] Handle inventory-service unavailability gracefully (allow order with warning, not hard block)

### CP-3: E2E Customer Flow

**Priority**: P0  
**Owner**: Backend + Frontend

The full happy path must work:

1. **Auth**: Customer registers/logs in at `/urban-loft/auth`
2. **Menu**: Browse menu at `/urban-loft/menu`, view item details, select variants
3. **Cart**: Add items, modify quantities, see totals
4. **Checkout**: Enter delivery address, select M-Pesa payment, submit order
5. **Payment**: M-Pesa STK push fires, payment webhook confirms
6. **Confirmation**: Order status moves to `confirmed`, customer sees confirmation
7. **Tracking**: Customer can view order status at `/urban-loft/track/{orderId}`

Specific backend tasks:

- [x] Verify cart creation and item addition endpoints work E2E
- [x] Verify order creation endpoint exists (`POST /api/v1/{tenant}/orders`)
- [ ] Verify order creation with `idempotency_key` prevents duplicates
- [x] Verify treasury integration: STK push → webhook → payment_status update (webhook endpoint implemented)
- [ ] Verify order status transitions: `placed → confirmed → preparing → ready->in-transit->delivered`
- [ ] Verify event publishing: `cafe.order.created`, `cafe.order.status.changed`
- [ ] Verify notifications-service receives order events and sends confirmation SMS/push

### CP-4: Payment Webhook Reliability

**Priority**: P0 — payments must not be lost  
**Owner**: Backend

- [x] Verify treasury webhook endpoint `POST /v1/webhooks/treasury` with HMAC validation (path: `POST /api/v1/{tenant}/webhooks/treasury`; HMAC-SHA256 in `internal/platform/treasury/webhook.go`)
- [x] Verify M-Pesa callback endpoint `POST /v1/webhooks/mpesa/*` handles STK push result
- [ ] Implement payment status polling fallback (in case webhook is delayed)
- [ ] Test: successful payment, failed payment, timeout, duplicate webhook
- [ ] Verify outbox publisher is running and processing payment events

---

## High Priority Tasks

### HP-1: Atlas Migration Transition

**Priority**: P1 — technical debt, blocks future schema changes  
**Owner**: Backend

- [ ] Install Atlas CLI in dev environment and CI pipeline
- [ ] Generate initial Atlas migration from current Ent schema (`atlas migrate diff`)
- [ ] Create `atlas.hcl` config pointing to production DB
- [ ] Test migration apply on staging DB
- [ ] Update Dockerfile to run Atlas migrations on startup instead of Ent auto-migrate
- [ ] Document rollback procedure
- [ ] **Decision**: Run Atlas in CI only or also on app boot? (Recommend CI-only for production)

### HP-2: Outbox Publisher Verification

**Priority**: P1 — events may be lost without this  
**Owner**: Backend

- [ ] Verify outbox background publisher is running (implemented in Sprint 8)
- [ ] Test: create order → verify `cafe.order.created` event published to NATS
- [ ] Test: payment confirmed → verify `cafe.payment.completed` event published
- [ ] Monitor outbox table for unpublished events (should drain within `OutboxPollPeriod`)
- [ ] Add health check: `/healthz` includes outbox publisher status

### HP-3: CORS & Security Headers

**Priority**: P1  
**Owner**: Backend

- [x] Verify CORS allows `https://ordersapp.codevertexitsolutions.com`
- [x] Verify CORS allows `https://theurbanloftcafe.com`
- [ ] Test preflight requests from browser (OPTIONS)
- [ ] Verify `X-Request-ID` propagation through all handlers
- [ ] Verify rate limiting is active on auth endpoints

### HP-4: Staff Dashboard API Wiring

**Priority**: P1 — best effort for launch  
**Owner**: Backend

- [ ] `GET /v1/{tenant}/admin/orders` returns orders with filtering (status, date range)
- [ ] `PATCH /v1/{tenant}/admin/orders/{id}/status` allows staff to advance order status
- [ ] `GET /v1/{tenant}/admin/analytics/summary` returns basic metrics (orders today, revenue, avg order value)
- [ ] Verify admin endpoints require `staff` or `admin` role
- [ ] Test: staff user can see and manage orders, customer user gets 403

---

## Medium Priority Tasks

### MP-1: Multi-Tenant Verification

**Priority**: P2

- [ ] Verify tenant isolation: requests to non-existent tenant slug return 404
- [ ] Verify data isolation: tenant A cannot see tenant B's orders/menu
- [ ] Verify JWT tenant_id claim matches URL tenant slug
- [ ] Test with a second test tenant (don't expose in production)

### MP-2: Notification Templates

**Priority**: P2

- [ ] Verify order confirmation notification template exists in notifications-service
- [ ] Verify order status update templates (preparing, ready, out_for_delivery, delivered)
- [ ] Test SMS delivery to real phone number
- [ ] Test push notification delivery

### MP-3: SLA Monitoring

**Priority**: P2

- [ ] Verify Prometheus metrics endpoint (`/metrics`) exposes request latency, error rate, order count
- [ ] Create basic Grafana dashboard: orders/min, p95 latency, error rate, active carts
- [ ] Set up critical alerts: 5xx rate >5%, order creation failures, payment webhook failures

---

## Out of Scope (Post-MVP)

- Group ordering
- Loyalty tier progression
- Promo code engine (basic codes work, advanced rules deferred)
- Multiple delivery partners
- gRPC/ConnectRPC endpoints
- Multi-language API responses (Swahili)
- Superset analytics embed
- POS integration beyond event publishing

---

## Deployment Checklist

### Pre-Launch (March 14-16)

- [ ] Run full seed on production DB (tenant, outlet, menu items, categories)
- [ ] Verify all environment variables set in K8s secrets
- [ ] Verify NATS JetStream streams and consumers created
- [ ] Verify Redis cache connectivity
- [ ] Verify treasury-service webhook secrets configured
- [ ] Run `atlas migrate apply` on production DB (if Atlas transition complete)
- [ ] Smoke test all critical path endpoints on staging
- [ ] Load test: 50 concurrent orders (verify no deadlocks or timeouts)

### Launch Day (March 17)

- [ ] Deploy final image via ArgoCD
- [ ] Verify `/healthz` returns 200
- [ ] Place one real test order through full flow
- [ ] Verify payment webhook fires and order status updates
- [ ] Monitor error rate for first 2 hours
- [ ] Keep rollback image tagged and ready

### Post-Launch (March 18-21)

- [ ] Monitor order success rate
- [ ] Review error logs for unexpected 5xx
- [ ] Check outbox table is draining (no stuck events)
- [ ] Collect first customer feedback
- [ ] Triage any blocking bugs as hotfixes

---

## Risk Register

| Risk | Impact | Mitigation |
|------|--------|------------|
| Treasury webhook fails silently | Orders stuck in `payment_pending` | Payment status polling fallback; alert on orders >5min unpaid |
| Inventory-service down during checkout | Orders blocked | Graceful degradation: allow order, log warning, reconcile later |
| Auth-service token validation fails | All requests 401 | JWKS cache with TTL; fallback to last known good keys |
| NATS JetStream unavailable | Events lost | Outbox pattern ensures events persist in DB; publisher retries |
| Database migration breaks schema | App crash | Atlas versioned migrations with rollback; test on staging first |

---

## Success Criteria

- [ ] A customer can complete an order from menu browse to delivery confirmation
- [ ] M-Pesa payment works end-to-end
- [ ] Order status updates are visible in real-time
- [ ] Staff can view and manage orders via admin endpoints
- [ ] Zero data leaks between tenants
- [ ] p95 API latency < 500ms under normal load
- [ ] Error rate < 1% for critical path endpoints
