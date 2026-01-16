# Ordering Service - Next Sprint Tasks

**Date**: January 2026
**Current Sprint**: Sprint 1 (70% Complete) → Sprint 2 (Catalog & Localization)
**Priority**: HIGH - Required for cafe-website menu integration

---

## Executive Summary

The Ordering Service is the **highest dependency** for the cafe-website. Sprint 2 (Catalog & Localization) must be completed to enable:
- Public menu browsing on cafe-website
- Localized content (EN/SW)
- Menu filtering and search
- "Order Now" redirect functionality

---

## Sprint 1 Remaining Items (Low Priority - Defer to Post-MVP)

These items can be completed after Sprint 2-4 are done:

| Task | Priority | Status | Notes |
|------|----------|--------|-------|
| Device management | Low | Not Started | Post-MVP |
| Invitation workflows | Medium | Not Started | Post-MVP |
| Audit logging | Medium | Not Started | Post-MVP |
| OAuth2 migration | Medium | Partial | Post-MVP |
| Subscription entitlements | High | Not Started | Sprint 2.5 |

---

## Sprint 2: Catalog & Localization (NEXT PRIORITY)

**Status**: ⏳ Not Started
**Duration**: 2 weeks
**Blocks**: Cafe-website menu display

### Sprint 2 Tasks

#### Week 1: Menu Categories & Items

| # | Task | Priority | Dependencies |
|---|------|----------|--------------|
| 2.1 | Create Ent schemas for menu_categories | Critical | Sprint 1 complete |
| 2.2 | Create Ent schemas for menu_items | Critical | 2.1 |
| 2.3 | Create Ent schemas for menu_item_variants | Critical | 2.2 |
| 2.4 | Implement category CRUD endpoints | Critical | 2.1 |
| 2.5 | Implement menu item CRUD endpoints | Critical | 2.2, 2.3 |
| 2.6 | Create public menu API (no auth) | Critical | 2.4, 2.5 |
| 2.7 | Run Ent migrations | Critical | 2.1-2.3 |

#### Week 2: Localization & Dietary Tags

| # | Task | Priority | Dependencies |
|---|------|----------|--------------|
| 2.8 | Create Ent schemas for translations | High | 2.2 |
| 2.9 | Create Ent schemas for dietary_tags | High | 2.2 |
| 2.10 | Implement translation CRUD endpoints | High | 2.8 |
| 2.11 | Implement dietary tag endpoints | High | 2.9 |
| 2.12 | Add locale parameter to public API | High | 2.6, 2.8 |
| 2.13 | Implement search functionality | High | 2.6 |
| 2.14 | Implement category filtering | High | 2.6 |

### Sprint 2 API Endpoints to Implement

```
Public (No Auth):
GET  /api/v1/{tenant}/menu/categories
GET  /api/v1/{tenant}/menu/items
GET  /api/v1/{tenant}/menu/items/{id}

Admin (Auth Required):
POST /api/v1/{tenant}/catalog/categories
PUT  /api/v1/{tenant}/catalog/categories/{id}
DELETE /api/v1/{tenant}/catalog/categories/{id}
POST /api/v1/{tenant}/catalog/items
PUT  /api/v1/{tenant}/catalog/items/{id}
DELETE /api/v1/{tenant}/catalog/items/{id}
POST /api/v1/{tenant}/catalog/items/{id}/variants
POST /api/v1/{tenant}/catalog/items/{id}/translations
```

### Sprint 2 Database Tables

```sql
-- Run Ent migrations for:
CREATE TABLE menu_categories (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    cafe_id UUID NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    display_order INTEGER DEFAULT 0,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ
);

CREATE TABLE menu_items (
    id UUID PRIMARY KEY,
    tenant_id UUID NOT NULL,
    cafe_id UUID NOT NULL,
    category_id UUID NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    base_price DECIMAL(10,2) NOT NULL,
    currency VARCHAR(3) DEFAULT 'KES',
    is_available BOOLEAN DEFAULT true,
    lead_time_minutes INTEGER,
    image_url VARCHAR(500),
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ
);

CREATE TABLE menu_item_variants (
    id UUID PRIMARY KEY,
    menu_item_id UUID NOT NULL,
    name VARCHAR(255) NOT NULL,
    price_delta DECIMAL(10,2),
    is_available BOOLEAN DEFAULT true,
    sku VARCHAR(100),
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ
);

CREATE TABLE menu_item_translations (
    menu_item_id UUID NOT NULL,
    locale VARCHAR(5) NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ,
    PRIMARY KEY (menu_item_id, locale)
);

CREATE TABLE dietary_tags (
    code VARCHAR(50) PRIMARY KEY,
    label VARCHAR(100) NOT NULL,
    description TEXT,
    icon_url VARCHAR(500)
);
```

---

## Sprint 3: Orders & Cart (After Sprint 2)

**Status**: ⏳ Not Started
**Duration**: 2 weeks
**Blocks**: Checkout flow, order management

### Sprint 3 Key Tasks

| # | Task | Priority |
|---|------|----------|
| 3.1 | Cart service with Redis persistence | Critical |
| 3.2 | Checkout workflow | Critical |
| 3.3 | Order state machine | Critical |
| 3.4 | Promo code engine | High |
| 3.5 | Loyalty points system | High |
| 3.6 | Address management | High |

---

## Sprint 4: Payments Core (After Sprint 3)

**Status**: ⏳ Not Started
**Duration**: 2 weeks
**Blocks**: Payment processing, order completion

### Sprint 4 Key Tasks

| # | Task | Priority |
|---|------|----------|
| 4.1 | Treasury service integration | Critical |
| 4.2 | M-Pesa STK Push implementation | Critical |
| 4.3 | Payment webhook processing | Critical |
| 4.4 | Payment reconciliation | High |
| 4.5 | Refund handling | Medium |

---

## Integration Points for Cafe-Website

### After Sprint 2 Completion

Cafe-website can integrate:
```typescript
// Public menu API - no auth required
GET /api/v1/urban-cafe/menu/categories
GET /api/v1/urban-cafe/menu/items?category_id={id}&locale=en
GET /api/v1/urban-cafe/menu/items/{id}
```

### After Sprint 3 Completion

Cafe-website can redirect to:
```
https://ordersapp.codevertexitsolutions.com/menu?item_id={id}&action=add-to-cart
```

### After Sprint 4 Completion

Full ordering flow available:
```
Menu → Cart → Checkout → Payment → Order Tracking
```

---

## Recommended Execution Order

1. **Sprint 2 Week 1**: Ent schemas + migrations + CRUD endpoints
2. **Sprint 2 Week 2**: Localization + dietary tags + public API
3. **Sprint 3 Week 1**: Cart + checkout workflow
4. **Sprint 3 Week 2**: Order state machine + promo codes
5. **Sprint 4 Week 1**: Treasury integration + M-Pesa
6. **Sprint 4 Week 2**: Webhooks + reconciliation

---

## Resources Needed

- 1 Backend Developer (Full-time)
- Access to Treasury service staging environment
- M-Pesa sandbox credentials
- S3 bucket for image uploads

---

## Success Criteria

Sprint 2 is complete when:
- [ ] Menu categories can be created/listed via API
- [ ] Menu items can be created/listed via API
- [ ] Localized content returned based on `Accept-Language` header
- [ ] Public menu API accessible without authentication
- [ ] Search and filtering working

---

## References

- [Sprint 2 Detailed Plan](./sprints/sprint-2-catalog-localization.md)
- [Sprint 3 Detailed Plan](./sprints/sprint-3-orders-cart.md)
- [Sprint 4 Detailed Plan](./sprints/sprint-4-payments-core.md)
- [Service Communication Guide](./SERVICE-COMMUNICATION.md)
- [API Contracts](./api-contracts.md)
