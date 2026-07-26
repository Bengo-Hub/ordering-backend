# Ordering-Backend – Apache Superset Integration

## Overview

The ordering-backend integrates with the centralized Apache Superset instance for BI dashboards, analytics, and reporting. Superset is deployed as a centralized service accessible to all Codevertex services.

---

## Architecture

### Service Configuration

**Environment Variables**:
- `SUPERSET_BASE_URL` - Superset service URL (e.g., `https://superset.codevertexafrica.com`)
- `SUPERSET_ADMIN_USERNAME` - Admin username (K8s secret)
- `SUPERSET_ADMIN_PASSWORD` - Admin password (K8s secret)
- `SUPERSET_API_VERSION` - API version (default: v1)

**Authentication**:
- Admin credentials used for backend-to-Superset communication
- User authentication via JWT tokens passed to Superset for SSO
- Guest tokens generated for embedded dashboards

---

## Integration Methods

### 1. REST API Client

Backend uses Go HTTP client configured for Superset REST API calls.

**Base Configuration**:
- Base URL: `SUPERSET_BASE_URL/api/v1`
- Default headers: `Content-Type: application/json`
- Authentication: Bearer token from Superset login endpoint
- Retry policy: Exponential backoff (3 retries)
- Circuit breaker: Opens after 5 consecutive failures

**HTTP Client Setup**:
- Go HTTP client with 30-second timeout
- Token management with expiration tracking
- Base URL configuration from environment
- Automatic token refresh before expiry

**Key API Endpoints**:

**Authentication**:
- `POST /api/v1/security/login` - Login with admin credentials
- `POST /api/v1/security/refresh` - Refresh access token
- `POST /api/v1/security/guest_token/` - Generate guest token for embedding

**Data Sources**:
- `GET /api/v1/database/` - List all data sources
- `POST /api/v1/database/` - Create new data source
- `PUT /api/v1/database/{id}` - Update data source
- `DELETE /api/v1/database/{id}` - Delete data source

**Dashboards**:
- `GET /api/v1/dashboard/` - List all dashboards
- `POST /api/v1/dashboard/` - Create new dashboard
- `PUT /api/v1/dashboard/{id}` - Update dashboard
- `GET /api/v1/dashboard/{id}` - Get dashboard details
- `POST /api/v1/dashboard/{id}/copy` - Copy dashboard

**Charts**:
- `GET /api/v1/chart/` - List all charts
- `POST /api/v1/chart/` - Create new chart
- `PUT /api/v1/chart/{id}` - Update chart
- `GET /api/v1/chart/{id}` - Get chart details

**Datasets**:
- `GET /api/v1/dataset/` - List all datasets
- `POST /api/v1/dataset/` - Create new dataset
- `PUT /api/v1/dataset/{id}` - Update dataset

### 2. Database Direct Connection

Superset connects directly to PostgreSQL database via read-only user for data access.

**Connection Configuration**:
- Database type: PostgreSQL
- Connection string: Provided to Superset via data source API
- Read-only user: `superset_readonly` (created in PostgreSQL)
- Permissions: SELECT only on all tables, no write access
- SSL: Required for production connections

**Read-Only User Setup**:
- Create `superset_readonly` role in PostgreSQL
- Grant CONNECT on database
- Grant USAGE on schema
- Grant SELECT on all tables
- Set default privileges for future tables

**Connection String** (for Superset):
```
postgresql://superset_readonly:password@postgresql.infra.svc.cluster.local:5432/cafe_db?sslmode=require
```

**Data Source Creation**:
- Data source created programmatically on application startup
- Connection tested before marking as active
- Data source updated if connection parameters change

### 3. Frontend Embedding

**React/TypeScript Component**:
- Component props: `dashboardId`, `tenantId`
- State management for embed URL and loading status
- Effect hook to fetch embed URL from backend
- Automatic token refresh before expiry (every 4 minutes)
- Iframe rendering of Superset dashboard
- Error handling for failed requests

---

## Pre-Built Dashboards

### 1. Order Analytics Dashboard

**Charts**:
- Orders by status (pie chart)
- Daily order volume (line chart)
- Revenue by cafe (bar chart)
- Average order value over time (line chart)
- Top menu items (table)

**Filters**:
- Date range
- Cafe selection
- Order status

**Data Source**: `orders`, `order_items`, `cafes` tables

### 2. Revenue Dashboard

**Charts**:
- Revenue by period (line chart)
- Revenue by cafe (bar chart)
- Payment method breakdown (pie chart)
- Refund rate (metric)
- Average transaction value (metric)

**Filters**:
- Date range
- Cafe selection
- Payment status

**Data Source**: `orders`, `payments`, `refunds` tables

### 3. Customer Analytics Dashboard

**Charts**:
- Customer acquisition over time (line chart)
- Customer lifetime value (bar chart)
- Loyalty points distribution (histogram)
- Repeat customer rate (metric)
- Top customers by orders (table)

**Filters**:
- Date range
- Customer segment

**Data Source**: `orders`, `loyalty_accounts`, `loyalty_transactions` tables

### 4. Operations Dashboard

**Charts**:
- Kitchen ticket status (pie chart)
- Average prep time (metric)
- Orders by delivery window (bar chart)
- Capacity utilization (line chart)
- Shift performance (table)

**Filters**:
- Date range
- Cafe selection
- Shift selection

**Data Source**: `kitchen_tickets`, `orders`, `shift_schedules` tables

### 5. Subscription & Licensing Dashboard

**Charts**:
- Active subscriptions by plan (pie chart)
- Subscription revenue (line chart)
- Usage vs limits (bar chart)
- Renewal rate (metric)
- Overage fees (metric)

**Filters**:
- Date range
- Plan type

**Data Source**: `tenant_subscriptions`, `subscription_invoices`, `subscription_usages` tables

---

## Implementation Details

### Initialization

On application startup:
1. Authenticate with Superset using admin credentials
2. Create/update data sources pointing to PostgreSQL
3. Create/update dashboard definitions for each module
4. Maintain dashboard-to-module mapping in local database

**Initialization Process**:
1. Authenticate with Superset using admin credentials
2. Create/update data source pointing to PostgreSQL
3. Create/update dashboards for each module:
   - Order Analytics
   - Revenue Dashboard
   - Customer Analytics
   - Operations Dashboard
   - Subscription Dashboard
4. Log warnings for dashboard creation failures (non-blocking)

### Dashboard Bootstrap

**Backend Endpoint**: `GET /api/v1/dashboards/{module}/embed`

**Process**:
1. Extract tenant ID from context
2. Get dashboard ID for module from Superset
3. Generate guest token with Row-Level Security (RLS) clause filtering by tenant_id
4. Construct embed URL with dashboard ID and guest token
5. Return embed URL with expiration time (5 minutes)

**Frontend Request**:
- Frontend requests dashboard URL from backend
- Backend generates secure embedded URL with user authentication token
- URL includes dashboard ID, filters, and time range
- Frontend renders dashboard using Superset SDK iframe

### Row-Level Security (RLS)

**Implementation**:
- Guest tokens include RLS clauses
- RLS filters data by `tenant_id`
- Each tenant sees only their data

**RLS Configuration**:
- RLS clause filters data by `tenant_id`
- Each tenant sees only their data
- Guest token includes RLS configuration

---

## Error Handling

### Retry Logic

**Retry Policy**:
- Maximum 3 retry attempts
- Exponential backoff (1s, 2s, 4s delays)
- Retry on 5xx errors or network failures
- Return response on success or after max retries

### Circuit Breaker

**Implementation**:
- Opens after 5 consecutive failures
- Half-open after 60 seconds
- Closes on successful request

### Fallback Strategies

**Superset Unavailable**:
- Return cached dashboard URLs (if available)
- Show static dashboard images
- Log error for monitoring
- Alert operations team

---

## Monitoring

### Metrics

**Integration-Specific Metrics**:
- Superset API call latency (p50, p95, p99)
- Dashboard creation/update success rates
- Guest token generation latency
- Data source connection health

**Prometheus Metrics**:
- `superset_api_call_duration_seconds` - Histogram of API call durations (labeled by endpoint, status)
- `superset_dashboard_views_total` - Counter of dashboard views (labeled by dashboard, tenant)

### Alerts

**Alert Conditions**:
- Superset service unavailability
- High API call failure rate (>5%)
- Dashboard creation failures
- Data source connection failures

---

## Security Considerations

### Authentication & Authorization

- Admin credentials stored in K8s secrets
- Guest tokens expire after 5 minutes
- RLS ensures tenant data isolation
- JWT tokens validated for SSO

### Data Privacy

- Read-only database user
- RLS filters enforce tenant isolation
- Sensitive data masked in logs
- PII data excluded from dashboards (if applicable)

---

## References

- [Apache Superset REST API Documentation](https://superset.apache.org/docs/api)
- [Superset Deployment Guide](../../devops-k8s/docs/superset-deployment.md)
- [TruLoad Superset Integration](../TruLoad/truload-backend/docs/integration.md#apache-superset-integration)

