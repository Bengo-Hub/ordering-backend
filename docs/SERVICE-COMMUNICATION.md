# Ordering Service - Communication Architecture

**Date**: January 2026
**Version**: 1.0
**Purpose**: Define service-to-service communication patterns, integration points, and architectural decisions for the Ordering Service within the BengoBox microservices ecosystem.

---

## Table of Contents

1. [Communication Overview](#communication-overview)
2. [Integration Patterns](#integration-patterns)
3. [Service Dependencies](#service-dependencies)
4. [Event-Driven Architecture](#event-driven-architecture)
5. [REST API Communications](#rest-api-communications)
6. [Real-Time Communications](#real-time-communications)
7. [Security & Authentication](#security--authentication)
8. [Error Handling & Resilience](#error-handling--resilience)
9. [Deployment & Service Discovery](#deployment--service-discovery)

---

## Communication Overview

The Ordering Service is the **core business service** for the Urban Loft Cafe platform. It orchestrates order lifecycle management and integrates with multiple services.

### Communication Matrix

| Service | Pattern | Direction | Use Cases |
|---------|---------|-----------|-----------|
| Auth Service | REST + Events | Consume | SSO, JWT validation, user sync |
| Treasury Service | REST + Events + Webhooks | Both | Payment processing, refunds |
| Logistics Service | REST + Events | Publish | Delivery task creation |
| Inventory Service | REST + Events | Both | Stock checks, reservations |
| Notifications Service | Events | Publish | Order confirmations, alerts |
| POS Service | Events | Publish | Loyalty points sync |
| Cafe Website | REST | Serve | Menu display, order status |

### Architecture Principles

1. **Event-First**: Prefer events for state changes that affect other services
2. **Synchronous for Reads**: Use REST APIs for data queries
3. **Idempotency**: All mutations must be idempotent
4. **Tenant Isolation**: All communications scoped by tenant_id
5. **Eventual Consistency**: Accept eventual consistency across services

---

## Integration Patterns

### When to Use Each Pattern

| Pattern | When to Use | Examples |
|---------|-------------|----------|
| **REST API (Sync)** | Immediate response needed | Stock availability check before checkout |
| **Events (Async)** | State changes that affect others | Order placed → Inventory reserve stock |
| **Webhooks** | External service callbacks | Payment gateway confirmation |
| **WebSocket** | Real-time bidirectional | Order tracking (future) |
| **SSE** | Real-time server-push | Order status updates (future) |

### Pattern Decision Tree

```
Need immediate response?
├── Yes → Use REST API
│   └── Need data? → GET
│   └── Need action? → POST (with idempotency key)
└── No → Use Events
    └── State change? → Publish event to NATS
    └── Notification? → Publish event to NATS
```

---

## Service Dependencies

### Upstream Services (Ordering Service Consumes)

#### 1. Auth Service

**Pattern**: REST API + Events
**Criticality**: CRITICAL

| Integration | Method | Endpoint/Event | Purpose |
|------------|--------|----------------|---------|
| JWT Validation | REST | `/.well-known/jwks.json` | Token validation |
| User Lookup | REST | `/api/v1/users/{id}` | Identity sync |
| User Sync | Event | `auth.user.created` | Create local user ref |
| User Update | Event | `auth.user.updated` | Update user details |
| Tenant Sync | Event | `auth.tenant.created` | Initialize tenant |

**Implementation**:
```go
// Using shared-auth-client library
authClient := authclient.NewClient(config.AuthService)
claims, err := authClient.ValidateToken(ctx, token)

// Event consumer (in app.go)
nats.Subscribe("auth.user.*", func(msg *nats.Msg) {
    identityService.HandleUserEvent(ctx, msg)
})
```

#### 2. Inventory Service

**Pattern**: REST API + Events
**Criticality**: HIGH

| Integration | Method | Endpoint/Event | Purpose |
|------------|--------|----------------|---------|
| Stock Check | REST | `/v1/{tenant}/inventory/items/{sku}` | Pre-checkout availability |
| Reserve Stock | Event | `cafe.order.placed` | Reserve inventory |
| Release Stock | Event | `cafe.order.cancelled` | Release reservation |
| Stock Updates | Event | `inventory.stock.updated` | Update availability cache |

**Implementation**:
```go
// REST API call for stock check
func (s *CatalogService) CheckAvailability(ctx context.Context, sku string) (bool, error) {
    resp, err := s.inventoryClient.Get(ctx, fmt.Sprintf("/v1/%s/inventory/items/%s", tenantID, sku))
    // Handle response
}

// Event publishing for order placed
func (s *OrderService) PlaceOrder(ctx context.Context, order *Order) error {
    // ... create order
    s.eventBus.Publish("cafe.order.placed", OrderPlacedEvent{
        OrderID: order.ID,
        Items:   order.Items,
    })
}
```

### Downstream Services (Ordering Service Serves)

#### 3. Cafe Website

**Pattern**: REST API (public + authenticated)
**Criticality**: HIGH

| Endpoint | Auth | Purpose |
|----------|------|---------|
| `GET /api/v1/{tenant}/catalog/items` | No | Public catalog browsing |
| `GET /api/v1/{tenant}/catalog/categories` | No | Category listing |
| `GET /api/v1/{tenant}/orders/{id}` | Yes | Order details |
| `GET /api/v1/{tenant}/loyalty/balance` | Yes | Loyalty points |

#### 4. Ordering PWA

**Pattern**: REST API (authenticated)
**Criticality**: CRITICAL

All cart, checkout, and order management endpoints.

### Peer Services (Bidirectional)

#### 5. Treasury Service

**Pattern**: REST API + Events + Webhooks
**Criticality**: CRITICAL

| Integration | Method | Endpoint/Event | Purpose |
|------------|--------|----------------|---------|
| Payment Intent | REST | `POST /api/v1/{tenant}/payments/intents` | Create payment |
| Payment Confirm | Webhook | `treasury.payment.success` | Confirm order |
| Payment Failed | Webhook | `treasury.payment.failed` | Handle failure |
| Refund | REST | `POST /api/v1/{tenant}/payments/refund` | Process refund |

**Implementation**:
```go
// Payment intent creation
func (s *CheckoutService) CreatePaymentIntent(ctx context.Context, order *Order) (*PaymentIntent, error) {
    resp, err := s.treasuryClient.Post(ctx, "/api/v1/"+tenantID+"/payments/intents", PaymentIntentRequest{
        Amount:     order.GrandTotal,
        Currency:   order.Currency,
        OrderID:    order.ID,
        IdempotencyKey: order.ID.String(), // Idempotent
    })
    return resp, err
}

// Webhook handler for payment confirmation
func (h *WebhookHandler) HandlePaymentSuccess(w http.ResponseWriter, r *http.Request) {
    // Verify HMAC signature
    if !verifySignature(r) {
        http.Error(w, "Invalid signature", http.StatusUnauthorized)
        return
    }

    var event PaymentSuccessEvent
    json.NewDecoder(r.Body).Decode(&event)

    h.orderService.ConfirmPayment(ctx, event.OrderID, event.PaymentID)
}
```

#### 6. Logistics Service

**Pattern**: REST API + Events
**Criticality**: HIGH

| Integration | Method | Endpoint/Event | Purpose |
|------------|--------|----------------|---------|
| Task Status | REST | `GET /v1/{tenant}/tasks/{id}` | Get delivery status |
| Create Task | Event | `cafe.order.ready` | Create delivery task |
| Task Assigned | Event | `logistics.task.assigned` | Update order status |
| Task Completed | Event | `logistics.task.completed` | Mark delivered |

**Implementation**:
```go
// Publish order ready event
func (s *OrderService) MarkOrderReady(ctx context.Context, orderID uuid.UUID) error {
    order, _ := s.repo.Get(ctx, orderID)
    order.Status = "ready"
    s.repo.Update(ctx, order)

    // Publish event for logistics
    s.eventBus.Publish("cafe.order.ready", OrderReadyEvent{
        OrderID:         order.ID,
        CafeID:          order.CafeID,
        DeliveryAddress: order.DeliveryAddress,
        Items:           order.Items,
    })
    return nil
}

// Subscribe to logistics events
nats.Subscribe("logistics.task.*", func(msg *nats.Msg) {
    switch msg.Subject {
    case "logistics.task.assigned":
        orderService.UpdateDeliveryAssigned(ctx, msg)
    case "logistics.task.completed":
        orderService.MarkDelivered(ctx, msg)
    }
})
```

#### 7. Notifications Service

**Pattern**: Events
**Criticality**: MEDIUM

| Integration | Method | Event | Purpose |
|------------|--------|-------|---------|
| Order Confirmation | Event | `cafe.order.created` | Email/SMS confirmation |
| Order Ready | Event | `cafe.order.ready` | Notify customer |
| Status Change | Event | `cafe.order.status.changed` | Update notifications |

**Implementation**:
```go
// Order lifecycle events trigger notifications
func (s *OrderService) CreateOrder(ctx context.Context, order *Order) error {
    // ... create order logic

    // Publish event for notifications
    s.eventBus.Publish("cafe.order.created", OrderCreatedEvent{
        OrderID:     order.ID,
        CustomerID:  order.CustomerID,
        Email:       order.CustomerEmail,
        Phone:       order.CustomerPhone,
        GrandTotal:  order.GrandTotal,
        Items:       order.Items,
    })
    return nil
}
```

---

## Event-Driven Architecture

### NATS JetStream Configuration

```go
// Stream configuration
stream := &nats.StreamConfig{
    Name:     "CAFE_ORDERS",
    Subjects: []string{"cafe.order.>", "cafe.cart.>"},
    Storage:  nats.FileStorage,
    Replicas: 3,
}

// Consumer configuration for reliable delivery
consumer := &nats.ConsumerConfig{
    Durable:       "ordering-service",
    AckPolicy:     nats.AckExplicitPolicy,
    MaxDeliver:    5,
    AckWait:       30 * time.Second,
    FilterSubject: "cafe.order.>",
}
```

### Event Catalog

#### Published Events

| Event | Trigger | Consumers |
|-------|---------|-----------|
| `cafe.order.created` | Order placed | Treasury, Notifications, Inventory |
| `cafe.order.ready` | Kitchen prepared | Logistics, Notifications |
| `cafe.order.cancelled` | Order cancelled | Inventory, Treasury, Notifications |
| `cafe.order.status.changed` | Any status change | Notifications |
| `cafe.loyalty.accrued` | Points earned | POS |

#### Consumed Events

| Event | Publisher | Action |
|-------|-----------|--------|
| `auth.user.created` | Auth Service | Create user ref |
| `auth.user.updated` | Auth Service | Update user details |
| `auth.tenant.created` | Auth Service | Initialize tenant |
| `treasury.payment.success` | Treasury | Confirm order |
| `treasury.payment.failed` | Treasury | Handle failure |
| `logistics.task.assigned` | Logistics | Update status |
| `logistics.task.completed` | Logistics | Mark delivered |
| `inventory.stock.updated` | Inventory | Update cache |

### Event Structure

```json
{
  "event_id": "uuid",
  "event_type": "cafe.order.created",
  "tenant_id": "tenant-uuid",
  "timestamp": "2026-01-16T10:30:00Z",
  "version": "1.0",
  "data": {
    "order_id": "order-uuid",
    "customer_id": "customer-uuid",
    "cafe_id": "cafe-uuid",
    "grand_total": 1500.00,
    "currency": "KES",
    "items": [...]
  },
  "metadata": {
    "correlation_id": "uuid",
    "source": "ordering-service"
  }
}
```

### Outbox Pattern for Reliability

```go
// Transactional outbox for reliable event publishing
func (s *OrderService) CreateOrder(ctx context.Context, order *Order) error {
    return s.db.WithTx(ctx, func(tx *ent.Tx) error {
        // 1. Create order
        if err := tx.Order.Create().SetOrder(order).Exec(ctx); err != nil {
            return err
        }

        // 2. Write event to outbox (same transaction)
        event := OutboxEvent{
            EventType: "cafe.order.created",
            Payload:   order.ToEventPayload(),
        }
        if err := tx.Outbox.Create().SetEvent(event).Exec(ctx); err != nil {
            return err
        }

        return nil
    })
}

// Background worker publishes from outbox
func (w *OutboxWorker) Run(ctx context.Context) {
    for {
        events := w.repo.GetPendingEvents(ctx, 100)
        for _, event := range events {
            if err := w.nats.Publish(event.EventType, event.Payload); err != nil {
                continue // Will retry
            }
            w.repo.MarkPublished(ctx, event.ID)
        }
        time.Sleep(100 * time.Millisecond)
    }
}
```

---

## REST API Communications

### HTTP Client Configuration

```go
// Standard HTTP client with resilience patterns
func NewServiceClient(baseURL string) *ServiceClient {
    return &ServiceClient{
        client: &http.Client{
            Timeout: 5 * time.Second,
            Transport: &http.Transport{
                MaxIdleConns:        100,
                MaxIdleConnsPerHost: 10,
                IdleConnTimeout:     90 * time.Second,
            },
        },
        baseURL: baseURL,
        retrier: retry.NewExponentialBackoff(3, 1*time.Second, 30*time.Second),
        breaker: circuitbreaker.New(5, 60*time.Second),
    }
}
```

### Request Headers

```go
// Required headers for all inter-service calls
req.Header.Set("Authorization", "Bearer "+token)
req.Header.Set("X-Tenant-ID", tenantID)
req.Header.Set("X-Request-ID", requestID)
req.Header.Set("X-Idempotency-Key", idempotencyKey) // For mutations
req.Header.Set("X-API-Key", apiKey) // For service-to-service
req.Header.Set("Content-Type", "application/json")
```

### Idempotency Implementation

```go
// All mutations use idempotency keys
func (c *ServiceClient) Post(ctx context.Context, path string, body interface{}) (*Response, error) {
    idempotencyKey := ctx.Value("idempotency_key").(string)
    if idempotencyKey == "" {
        return nil, errors.New("idempotency key required for mutations")
    }

    req, _ := http.NewRequestWithContext(ctx, "POST", c.baseURL+path, toJSON(body))
    req.Header.Set("X-Idempotency-Key", idempotencyKey)

    return c.do(req)
}
```

---

## Real-Time Communications

### WebSocket for Order Tracking (Future)

```go
// WebSocket handler for live order updates
func (h *OrderHandler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
    orderID := chi.URLParam(r, "orderID")

    conn, _ := upgrader.Upgrade(w, r, nil)
    defer conn.Close()

    // Subscribe to order updates
    sub := h.orderUpdates.Subscribe(orderID)
    defer sub.Unsubscribe()

    for update := range sub.Updates() {
        conn.WriteJSON(update)
    }
}
```

### Server-Sent Events for Status Updates (Future)

```go
// SSE endpoint for order status stream
func (h *OrderHandler) HandleSSE(w http.ResponseWriter, r *http.Request) {
    orderID := chi.URLParam(r, "orderID")

    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")

    flusher := w.(http.Flusher)

    sub := h.orderUpdates.Subscribe(orderID)
    defer sub.Unsubscribe()

    for update := range sub.Updates() {
        fmt.Fprintf(w, "data: %s\n\n", toJSON(update))
        flusher.Flush()
    }
}
```

---

## Security & Authentication

### JWT Validation

```go
// All protected endpoints validate JWT
func AuthMiddleware(authClient *authclient.Client) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            token := extractToken(r)
            if token == "" {
                http.Error(w, "Unauthorized", http.StatusUnauthorized)
                return
            }

            claims, err := authClient.ValidateToken(r.Context(), token)
            if err != nil {
                http.Error(w, "Invalid token", http.StatusUnauthorized)
                return
            }

            // Add claims to context
            ctx := context.WithValue(r.Context(), "claims", claims)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

### Service-to-Service Authentication

```go
// API key authentication for inter-service calls
func ServiceAuthMiddleware(apiKeyStore APIKeyStore) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            apiKey := r.Header.Get("X-API-Key")
            if apiKey != "" {
                service, err := apiKeyStore.ValidateKey(apiKey)
                if err == nil {
                    ctx := context.WithValue(r.Context(), "service", service)
                    next.ServeHTTP(w, r.WithContext(ctx))
                    return
                }
            }

            // Fall back to JWT auth
            // ...
        })
    }
}
```

### Webhook Signature Verification

```go
// Verify HMAC-SHA256 signature for webhooks
func VerifyWebhookSignature(r *http.Request, secret string) bool {
    signature := r.Header.Get("X-Webhook-Signature")
    timestamp := r.Header.Get("X-Webhook-Timestamp")

    // Check timestamp (5-minute window)
    ts, _ := strconv.ParseInt(timestamp, 10, 64)
    if time.Now().Unix()-ts > 300 {
        return false
    }

    body, _ := io.ReadAll(r.Body)
    r.Body = io.NopCloser(bytes.NewBuffer(body))

    payload := fmt.Sprintf("%s.%s", timestamp, string(body))
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write([]byte(payload))
    expected := hex.EncodeToString(mac.Sum(nil))

    return hmac.Equal([]byte(signature), []byte(expected))
}
```

---

## Error Handling & Resilience

### Retry Policy

```go
// Exponential backoff with jitter
type RetryPolicy struct {
    MaxRetries int
    InitialDelay time.Duration
    MaxDelay time.Duration
}

func (r *RetryPolicy) Execute(fn func() error) error {
    var err error
    delay := r.InitialDelay

    for i := 0; i <= r.MaxRetries; i++ {
        err = fn()
        if err == nil {
            return nil
        }

        if i == r.MaxRetries {
            break
        }

        // Add jitter
        jitter := time.Duration(rand.Float64() * float64(delay) * 0.1)
        time.Sleep(delay + jitter)

        // Exponential backoff
        delay = min(delay*2, r.MaxDelay)
    }

    return err
}
```

### Circuit Breaker

```go
// Circuit breaker for service calls
type CircuitBreaker struct {
    failures int
    threshold int
    state string // closed, open, half-open
    lastFailure time.Time
    timeout time.Duration
}

func (cb *CircuitBreaker) Execute(fn func() error) error {
    if cb.state == "open" {
        if time.Since(cb.lastFailure) > cb.timeout {
            cb.state = "half-open"
        } else {
            return ErrCircuitOpen
        }
    }

    err := fn()
    if err != nil {
        cb.failures++
        cb.lastFailure = time.Now()
        if cb.failures >= cb.threshold {
            cb.state = "open"
        }
        return err
    }

    cb.failures = 0
    cb.state = "closed"
    return nil
}
```

### Fallback Strategies

| Service | Fallback Strategy |
|---------|------------------|
| Auth Service | Return cached JWKS, reject new tokens |
| Inventory Service | Allow order (async validation) |
| Treasury Service | Queue payment, retry |
| Logistics Service | Queue task creation |
| Notifications Service | Queue notification |

---

## Deployment & Service Discovery

### Kubernetes Service Discovery

```yaml
# Service definition
apiVersion: v1
kind: Service
metadata:
  name: ordering-service
  namespace: cafe
spec:
  selector:
    app: ordering-service
  ports:
    - port: 8080
      targetPort: 8080
  type: ClusterIP
```

### Environment Configuration

```env
# Service URLs (resolved via K8s DNS)
AUTH_SERVICE_URL=http://auth-service.auth.svc.cluster.local:8080
TREASURY_SERVICE_URL=http://treasury-service.finance.svc.cluster.local:8080
LOGISTICS_SERVICE_URL=http://logistics-service.logistics.svc.cluster.local:8080
INVENTORY_SERVICE_URL=http://inventory-service.inventory.svc.cluster.local:8080
NOTIFICATIONS_SERVICE_URL=http://notifications-service.notifications.svc.cluster.local:8080

# NATS (standard key used by all Go services)
EVENTS_NATS_URL=nats://nats.messaging.svc.cluster.local:4222

# External URLs (public)
AUTH_SERVICE_PUBLIC_URL=https://sso.codevertexitsolutions.com
```

### Health Checks

```go
// Readiness check includes dependencies
func (h *HealthHandler) Readiness(w http.ResponseWriter, r *http.Request) {
    checks := map[string]bool{
        "database": h.db.Ping() == nil,
        "redis":    h.redis.Ping(r.Context()) == nil,
        "nats":     h.nats.Status() == nats.CONNECTED,
    }

    allHealthy := true
    for _, healthy := range checks {
        if !healthy {
            allHealthy = false
            break
        }
    }

    if !allHealthy {
        w.WriteHeader(http.StatusServiceUnavailable)
    }
    json.NewEncoder(w).Encode(checks)
}
```

---

## References

- [Microservices Architecture](../../../../docs/microservice-architecture.md)
- [Cross-Service Data Ownership](../../../../docs/CROSS-SERVICE-DATA-OWNERSHIP.md)
- [Auth Integration Guide](../../../../docs/AUTH-INTEGRATION-GUIDE.md)
- [TRINITY Authorization Pattern](../../../../docs/TRINITY-AUTHORIZATION-PATTERN.md)
- [Ordering Service Integrations](./integrations.md)
