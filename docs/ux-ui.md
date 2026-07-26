# Ordering Backend – API Consumer UX Patterns

**Last Updated**: March 6, 2026

This document defines the API patterns that frontend consumers (ordering-frontend, cafe-website, POS) must follow when integrating with `orderingapi.codevertexafrica.com`.

---

## Tenant-Scoped Routing

Every API call is scoped by tenant slug in the URL path:

```
GET  /v1/{tenant}/menu-items
POST /v1/{tenant}/orders
```

The default tenant slug for MVP is `urban-loft`. Frontends must read the slug from the URL route parameter (`[orgSlug]`) and forward it to every API call.

---

## Authentication Flow

### Login / Registration

All auth operations proxy through the ordering backend to auth-service:

```
POST /v1/{tenant}/auth/login    → proxies to sso.codevertexafrica.com
POST /v1/{tenant}/auth/register → proxies to sso.codevertexafrica.com
POST /v1/{tenant}/auth/refresh  → proxies to sso.codevertexafrica.com
```

Frontends receive `{ access_token, refresh_token, session_id, tenant, user }` and must:

1. Store `access_token` in memory (Zustand store or React context)
2. Store `refresh_token` in `httpOnly` cookie or secure storage
3. Attach `Authorization: Bearer {access_token}` to all subsequent requests

### Token Refresh

When a request returns `401 Unauthorized`, the frontend must:

1. Call `/v1/{tenant}/auth/refresh` with the stored refresh token
2. Retry the original request with the new access token
3. If refresh fails, redirect to `/[orgSlug]/auth`

The Axios `baseapi` client should implement this as a response interceptor with request queueing to avoid thundering herd on concurrent 401s.

### Role-Based Redirects

After successful auth, inspect JWT claims:

| Role | Redirect |
|------|----------|
| `customer` | Stay in ordering-frontend |
| `staff`, `admin` | Redirect to `NEXT_PUBLIC_CAFE_WEBSITE_URL` |
| `rider` | Redirect to `NEXT_PUBLIC_LOGISTICS_UI_URL` |
| `superuser` | Stay (full access) |

---

## Outlet Selection & Enforcement

### MVP Constraint: Busia Only

For the March 17 launch, only the **Busia** outlet is active under the `urban-loft` tenant. The API returns outlets via:

```
GET /v1/{tenant}/outlets
```

Frontends must:

1. Fetch the outlet list on app init
2. If only one outlet exists, auto-select it (no picker shown)
3. Store the selected `outlet_id` and pass it to cart/order creation
4. Pass `cafe_id` (outlet ID) in order payload

### Multi-Outlet (Post-MVP)

When multiple outlets are active, show an outlet picker in the header. Changing outlet clears the cart (menus differ per outlet).

---

## Menu Browsing Patterns

### Endpoints

```
GET /v1/{tenant}/menu-categories?cafe_id={outlet_id}
GET /v1/{tenant}/menu-items?cafe_id={outlet_id}&category_id={cat_id}
GET /v1/{tenant}/menu-items/{id}
```

### Frontend Expectations

- **Pagination**: List endpoints return `{ data: [], meta: { total, page, per_page } }`. Use cursor or offset pagination as provided.
- **Availability**: Respect `is_available` field. Grey out unavailable items but keep them visible with "Unavailable" badge.
- **Variants**: `menu_items/{id}` returns `variants[]` with `price_delta`. Display variant selector (size, flavour) before add-to-cart.
- **Images**: Use `image_url` with CDN prefix. Implement lazy loading and WebP fallback.
- **Dietary Tags**: Render tag badges (vegan, gluten-free) from `dietary_tags[]`.

### Caching Strategy

Menu data changes infrequently. TanStack Query config:

- `staleTime`: 5 minutes
- `gcTime`: 30 minutes
- Service worker: Stale-While-Revalidate for `/catalog` routes

---

## Cart & Checkout Flow

### Cart Operations

```
POST   /v1/{tenant}/carts                  → create cart
POST   /v1/{tenant}/carts/{id}/items       → add item
PATCH  /v1/{tenant}/carts/{id}/items/{iid} → update quantity
DELETE /v1/{tenant}/carts/{id}/items/{iid} → remove item
GET    /v1/{tenant}/carts/{id}             → get cart with totals
```

Cart is server-side with local Zustand mirror for optimistic updates. On network failure, persist cart to IndexedDB and sync when online.

### Checkout

```
POST /v1/{tenant}/orders
```

Payload must include:

```json
{
  "cart_id": "uuid",
  "cafe_id": "uuid",
  "delivery_address_id": "uuid",
  "payment_method": "mpesa|card|cash",
  "idempotency_key": "client-generated-uuid",
  "instructions": "optional",
  "promo_code": "optional"
}
```

**Idempotency**: The frontend MUST generate a `idempotency_key` (UUIDv4) before submitting. This prevents duplicate orders on retry/double-click. Disable the submit button after first click and re-enable only on error.

### Payment Integration

After order creation, if payment is required:

1. Backend returns `{ order_id, payment_intent_id, payment_url }` (for card) or initiates STK push (for M-Pesa)
2. For M-Pesa: show "Waiting for payment confirmation..." with polling on `GET /v1/{tenant}/orders/{id}` watching `payment_status`
3. For card: redirect to `payment_url`, handle callback redirect
4. Poll interval: 3 seconds for first 30s, then 5 seconds up to 2 minutes, then show "Payment timeout" with retry option

---

## Order Tracking

### Endpoints

```
GET /v1/{tenant}/orders/{id}           → order detail with status
GET /v1/{tenant}/orders/{id}/tracking  → delivery tracking data
```

### Status Flow

```
placed → confirmed → preparing → ready → out_for_delivery → delivered → completed
                                                           ↘ cancelled
```

Frontends should poll order status every 10 seconds while on the tracking page. For live rider location, connect to the logistics-service WebSocket/SSE endpoint using the `logistics_task_id` from the order response.

### Real-Time Updates

The backend publishes `cafe.order.status.changed` events. Frontends that support SSE/WebSocket should listen on:

```
GET /v1/{tenant}/orders/{id}/stream  → SSE endpoint (if available)
```

Fallback: poll `GET /v1/{tenant}/orders/{id}` with TanStack Query `refetchInterval: 10000`.

---

## Error Response Contract

All errors follow a consistent envelope:

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Human-readable message",
    "details": [
      { "field": "phone", "message": "required" }
    ]
  }
}
```

### Frontend Error Handling

| HTTP Status | Frontend Action |
|-------------|-----------------|
| `400` | Show field-level validation errors from `details[]` |
| `401` | Trigger token refresh; if fails, redirect to auth |
| `403` | Show "Permission denied" toast |
| `404` | Show "Not found" page or toast |
| `409` | Conflict — show message (e.g. "Item no longer available") |
| `422` | Business rule violation — show `message` to user |
| `429` | Rate limited — show "Please wait" with retry-after header |
| `500` | Show generic error toast, log to error tracking |
| `503` | Show "Service temporarily unavailable" banner |

---

## Pagination & Filtering

### Standard Query Parameters

```
?page=1&per_page=20           → offset pagination
?sort=created_at&order=desc   → sorting
?search=chapati               → text search
?status=active                → filtering
```

### Response Envelope

```json
{
  "data": [...],
  "meta": {
    "total": 150,
    "page": 1,
    "per_page": 20,
    "total_pages": 8
  }
}
```

Frontends should implement infinite scroll for menu browsing and traditional pagination for dashboard tables.

---

## Request Headers

Every request must include:

| Header | Value | Purpose |
|--------|-------|---------|
| `Authorization` | `Bearer {token}` | Auth (protected routes) |
| `X-Request-ID` | UUIDv4 | Correlation / debugging |
| `Content-Type` | `application/json` | Request body format |
| `Accept-Language` | `en` or `sw` | i18n preference |

The `baseapi` Axios instance should set these automatically via interceptors.

---

## Staff Dashboard API Patterns

Staff/admin routes (used by cafe-website or ordering-frontend staff dashboard):

```
GET    /v1/{tenant}/admin/orders?status=pending&page=1
PATCH  /v1/{tenant}/admin/orders/{id}/status  → { "status": "confirmed" }
GET    /v1/{tenant}/admin/analytics/summary
GET    /v1/{tenant}/admin/menu-items
POST   /v1/{tenant}/admin/menu-items
PATCH  /v1/{tenant}/admin/menu-items/{id}
```

These require `staff` or `admin` role in JWT claims. The frontend must check roles before rendering admin navigation.

---

## Rate Limits

| Endpoint Group | Limit | Window |
|----------------|-------|--------|
| Auth (login/register) | 10 req | 1 min |
| Menu browsing | 120 req | 1 min |
| Cart mutations | 60 req | 1 min |
| Order creation | 5 req | 1 min |
| Admin endpoints | 60 req | 1 min |

Frontends should debounce search inputs (300ms) and disable buttons after submission to stay within limits.
