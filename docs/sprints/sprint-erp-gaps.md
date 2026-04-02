# Sprint: ERP E-commerce Gaps — ordering-backend

**Created:** April 2026
**Status:** Planning
**Goal:** Close feature gaps identified from ERP ecommerce module audit before ERP module deletion (Phase 1/5)

---

## Context

The ERP `ecommerce/order/` module contains return/RMA workflows, inventory consumption wiring, and analytics models that do not yet exist in ordering-backend. These must be implemented here (or confirmed deferred) before the ERP module can be removed.

---

## Gap 1: Return / RMA Request Workflow

**ERP source:** `ecommerce/order/` — `ReturnRequest`, `ReturnLine` models
**Priority:** P1
**Status:** Pending

### Current State

Ordering-backend supports refund-only flows (via treasury-api `POST /refunds`). There is no return/exchange request lifecycle — customers cannot request a return, staff cannot approve/reject, and there is no line-level return tracking.

### Required

- [ ] **ORD-ERP-01:** Add `ReturnRequest` Ent schema
  - Fields: `id`, `order_id` (FK), `customer_id`, `reason`, `status` (requested/approved/rejected/completed), `type` (return/exchange), `requested_at`, `resolved_at`, `notes`
  - Edges: order, return_lines
- [ ] **ORD-ERP-02:** Add `ReturnLine` Ent schema
  - Fields: `id`, `return_request_id` (FK), `order_item_id` (FK), `quantity`, `reason`, `condition` (unopened/defective/damaged/other), `refund_amount`
- [ ] **ORD-ERP-03:** Generate Atlas migration for return schemas
- [ ] **ORD-ERP-04:** Add return request handlers
  - `POST /api/v1/{tenant}/orders/{order_id}/returns` — create return request
  - `GET /api/v1/{tenant}/orders/{order_id}/returns` — list returns for order
  - `GET /api/v1/{tenant}/returns/{id}` — get return details
  - `PATCH /api/v1/{tenant}/returns/{id}` — approve/reject return
- [ ] **ORD-ERP-05:** Add return service logic
  - Validate return window (configurable per tenant, e.g. 14 days)
  - On approval: trigger treasury-api refund (`POST /refunds`) for return lines
  - On approval: publish `ordering.return.approved` event for inventory-api to process stock return
  - On completion: update order item status

### Events

- `ordering.return.requested` — notify staff
- `ordering.return.approved` — trigger refund + inventory restock
- `ordering.return.rejected` — notify customer
- `ordering.return.completed` — final state

---

## Gap 2: Inventory Consumption Wiring on Order Completion

**ERP source:** `ecommerce/stockinventory/` — stock consumption triggered on order fulfillment
**Priority:** P0
**Status:** Pending

### Current State

Ordering-backend publishes `ordering.order.completed` but the inventory-api consumption is not yet wired end-to-end. The event is published; inventory-api has the consumer endpoint (`POST /consumption`) but the integration has not been verified in production.

### Required

- [ ] **ORD-ERP-06:** Verify `ordering.order.completed` event payload includes all fields needed by inventory-api consumption endpoint
  - Required: `order_id`, `tenant_id`, `items[]` (each with `sku`, `quantity`, `warehouse_id`)
- [ ] **ORD-ERP-07:** Add integration test: order completion -> inventory consumption
  - Mock inventory-api, verify event published with correct payload
  - Verify idempotency (re-publishing same event does not double-consume)
- [ ] **ORD-ERP-08:** Add fallback: if inventory-api is unavailable, queue consumption for retry (outbox pattern already in place; verify retry behavior)
- [ ] **ORD-ERP-09:** Document the event contract in `docs/integrations.md`

---

## Gap 3: Advanced Analytics Models

**ERP source:** `ecommerce/analytics/` — `CustomerCohort`, `RFMSegment`, `ConversionFunnel`, `SalesAnalytics`, `ProductPerformance`
**Priority:** P2 (deferred)
**Status:** Deferred to Superset

### Decision

Advanced analytics (cohort analysis, RFM segmentation, conversion funnel analysis) will **not** be implemented in ordering-backend. These are reporting/BI concerns and will be handled by Superset dashboards querying the ordering-backend database (read replica) directly.

### Action Items

- [ ] **ORD-ERP-10:** Document Superset connection requirements in `docs/superset-integration.md`
  - Read replica connection string
  - Key tables for analytics: `orders`, `order_items`, `carts`, `promo_codes`, `loyalty_transactions`
- [ ] **ORD-ERP-11:** Create Superset dashboard templates for:
  - Customer cohort analysis (first-order month cohorts, retention)
  - RFM segmentation (recency, frequency, monetary scoring)
  - Conversion funnel (cart -> checkout -> payment -> confirmed)
  - Product performance (top sellers, revenue by category)

---

## References

- [ERP Module Removal Plan](../../../erp/erp-api/docs/module-removal-plan.md)
- [Cross-Service Data Ownership](../../../shared-docs/CROSS-SERVICE-DATA-OWNERSHIP.md)
- [Ordering Integrations](../integrations.md)
