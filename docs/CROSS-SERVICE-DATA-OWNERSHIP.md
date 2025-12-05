# Cross-Service Data Ownership & User Management

## Overview

This document defines the architecture pattern for data ownership and user management across microservices in the BengoBox platform. Each service owns and manages all data related to its domain, while other services reference data via IDs and tenant mapping.

---

## Core Principles

1. **Single Source of Truth**: Each service owns and manages all data related to its domain
2. **Reference Only**: Other services store only reference IDs, never duplicate data
3. **Tenant Service Availability**: Check tenant subscription plan before creating/referencing data in another service (except auth-service, which is always available)
4. **SSO Authentication**: All users authenticate via auth-service (SSO), service-specific data stored locally
5. **Service Independence**: Services can operate standalone or in combination based on tenant subscription
6. **Tenant Auto-Discovery**: Services can auto-sync tenants to auth-service (public operation, no authentication required, works for all billing plans)

---

## Data Ownership by Service

### Auth-Service
**Owns**:
- User identity (email, password, phone, status)
- Tenant membership and roles
- Sessions and MFA
- OAuth accounts

**Other Services Reference**:
- `auth_service_user_id` (UUID) - Reference to auth-service user
- Identity data synced via events: `auth.user.created`, `auth.user.updated`, `auth.user.deactivated`

### Logistics-Service
**Owns**:
- Rider profiles (KYC, documents, vehicle info)
- Fleet members (riders, drivers)
- Delivery tasks
- Shifts and availability
- Telemetry and location data
- Proof of delivery
- Rider earnings and payouts

**Other Services Reference**:
- `rider_id` (UUID) - Reference to logistics-service fleet member
- `logistics_task_id` (UUID) - Reference to delivery task
- All rider queries go to logistics-service APIs: `GET /v1/{tenant}/fleet-members`

### Inventory-Service
**Owns**:
- Inventory items (SKUs, stock levels, locations)
- Recipes and BOMs
- Stock adjustments and movements
- Low-stock alerts

**Other Services Reference**:
- `inventory_sku` (String) - Reference to inventory item
- `inventory_item_id` (UUID) - Reference to inventory item
- All inventory queries go to inventory-service APIs

### POS-Service
**Owns**:
- POS connections and credentials
- POS outlets and locations
- POS orders and tickets
- Settlement data

**Other Services Reference**:
- `pos_connection_id` (UUID) - Reference to POS connection
- `pos_outlet_id` (UUID) - Reference to POS outlet
- `pos_order_id` (String) - Reference to POS order

### Treasury-Service
**Owns**:
- Payment intents and transactions
- Payment methods
- Refunds
- Payouts and settlements
- Invoices

**Other Services Reference**:
- `payment_intent_id` (UUID) - Reference to payment intent
- `payment_id` (UUID) - Reference to payment
- `payout_id` (UUID) - Reference to payout

### Notifications-Service
**Owns**:
- Notification templates
- Message delivery status
- Channel preferences (per user, per tenant)

**Other Services Reference**:
- `notification_template_id` (UUID) - Reference to template
- `notification_message_id` (UUID) - Reference to sent message

### Cafe-Service
**Owns**:
- Cafe-specific user data (preferences, cafe roles, loyalty points)
- Menu items and categories (references inventory SKUs)
- Orders and carts
- Promo codes and redemptions
- Loyalty accounts and transactions

**Other Services Reference**:
- `order_id` (UUID) - Reference to cafe order
- `cafe_id` (UUID) - Reference to cafe outlet

---

## User Management Patterns

### Pattern 1: Service-Specific User Data

**Example: Rider User Management**

1. **User Identity** (auth-service):
   - User created in auth-service with email, password, tenant membership
   - Role: `rider` assigned in auth-service
   - User authenticates via auth-service (SSO)

2. **Rider Profile** (logistics-service):
   - Rider-specific data stored in logistics-service:
     - KYC documents (national ID, license)
     - Vehicle information
     - Shift availability
     - Earnings and payouts
   - Linked to auth-service user via `auth_service_user_id`

3. **Rider Creation Flow**:

   **From Cafe Service**:
   ```
   1. User initiates rider onboarding in cafe UI
   2. Check tenant has logistics service enabled:
      GET /api/v1/tenants/{tenant_id}/services
      → Verify "logistics" in enabled_services
   3. If not enabled: Show error "Logistics service not available. Upgrade plan."
   4. If enabled, choose one:
      Option A - API Push:
        - POST /api/v1/cafe/riders/onboard (cafe-backend)
        - Cafe-backend pushes to logistics-service:
          POST /v1/{tenant}/fleet-members
        - Logistics-service creates rider in auth-service (if needed)
        - Returns rider_id
        - Cafe stores rider_id reference
      
      Option B - UI Redirect:
        - Redirect to: https://logistics.codevertexitsolutions.com/{tenant_slug}/riders/onboard?return_url={cafe_url}
        - User authenticates with auth-service (SSO)
        - User completes onboarding in logistics-service UI
        - Logistics-service redirects back with rider_id
   ```

   **Standalone Logistics Service**:
   ```
   1. User goes directly to logistics-service UI
   2. User authenticates via auth-service (SSO)
   3. User completes rider onboarding
   4. All rider data stored in logistics-service
   5. No cafe-service involvement
   ```

### Pattern 2: Tenant Service Availability Check

**Before creating/referencing data in another service:**

```go
// Pseudo-code example
func createRider(ctx context.Context, tenantID uuid.UUID, riderData RiderData) error {
    // 1. Check tenant has logistics service enabled
    tenant, err := subscriptionService.GetTenantServices(ctx, tenantID)
    if err != nil {
        return err
    }
    
    if !contains(tenant.EnabledServices, "logistics") {
        return ErrServiceNotAvailable("Logistics service not enabled for this tenant")
    }
    
    // 2. Verify tenant exists in logistics-service
    exists, err := logisticsService.TenantExists(ctx, tenantID)
    if err != nil {
        return err
    }
    if !exists {
        return ErrTenantNotFound("Tenant not found in logistics-service")
    }
    
    // 3. Create rider in logistics-service
    riderID, err := logisticsService.CreateFleetMember(ctx, tenantID, riderData)
    if err != nil {
        return err
    }
    
    // 4. Store only reference ID locally
    return cafeRepo.StoreRiderReference(ctx, tenantID, riderID)
}
```

**Note**: This pattern applies to services that require billing plan verification (inventory, POS, logistics, treasury). Auth-service is **always available** regardless of billing plan and supports tenant auto-discovery (see Pattern 3).

### Pattern 3: Tenant Auto-Discovery to Auth-Service

**Auth-service is special**: Unlike other services that require billing plan verification, auth-service is accessible in all plans (free or paid) and supports automatic tenant discovery.

**When creating a user from any service:**

```go
// Pseudo-code example
func registerUser(ctx context.Context, email, password, tenantSlug string) error {
    // 1. Ensure tenant exists in auth-service (auto-discovery)
    // This is a public operation - no authentication required
    exists, err := authService.CheckTenantExists(ctx, tenantSlug)
    if err != nil {
        // Log warning but continue - auth-service might create tenant on registration
        log.Warn("Failed to check tenant existence", err)
    }
    
    if !exists {
        // Pull full tenant details from local database
        localTenant, err := repo.FindTenantBySlug(ctx, tenantSlug)
        if err != nil {
            // Tenant doesn't exist locally, use defaults
            localTenant = &Tenant{
                ID: uuid.New(), // Generate new UUID
                Slug: tenantSlug,
                Name: deriveNameFromSlug(tenantSlug),
                // ... defaults
            }
        }
        
        // Auto-create tenant in auth-service with SAME UUID and slug
        tenantData := TenantData{
            ID: localTenant.ID.String(), // CRITICAL: Use same UUID across all services
            Slug: localTenant.Slug,      // CRITICAL: Use same slug across all services
            Name: localTenant.Name,
            ContactEmail: localTenant.ContactEmail,
            ContactPhone: localTenant.ContactPhone,
            Metadata: localTenant.Metadata,
        }
        tenantData.Metadata["source"] = "cafe-service"
        tenantData.Metadata["auto_created"] = true
        tenantData.Metadata["synced_at"] = time.Now().Format(time.RFC3339)
        
        _, err := authService.CreateTenant(ctx, tenantData)
        if err != nil {
            // Log warning but continue - auth-service might create tenant on registration
            log.Warn("Failed to create tenant in auth-service", err)
        }
    }
    
    // 2. Proceed with user registration
    // Auth-service will handle tenant creation if it doesn't exist
    return authService.Register(ctx, RegisterRequest{
        Email: email,
        Password: password,
        TenantSlug: tenantSlug,
    })
}
```

**Key Points**:
- **Tenant ID Matching**: Tenant IDs (UUIDs) and slugs **must match** across all services. When syncing to auth-service, use the same UUID from the local database.
- **No Authentication Required**: Tenant check (`GET /api/v1/tenants/by-slug/{slug}`) and creation (`POST /api/v1/tenants`) endpoints in auth-service are public (no auth headers needed)
- **Billing Plan Independent**: Works for all plans (free or paid), unlike other services
- **Best Effort**: If tenant sync fails (network issues, etc.), the operation continues - auth-service may create the tenant automatically during registration
- **Idempotent**: Safe to call multiple times - if tenant already exists, creation is skipped
- **Service Origin Tracking**: Metadata includes `source` field to track which service created the tenant
- **Full Tenant Details**: Pull complete tenant information (ID, slug, name, contact info, metadata) from local database when available

### Pattern 3: Service-to-Service Data Queries

**Never duplicate data, always query the owning service:**

```go
// ❌ WRONG: Storing rider data locally
type OrderAssignment struct {
    RiderID      uuid.UUID
    RiderName    string  // ❌ Don't store
    RiderPhone   string  // ❌ Don't store
    VehicleType  string  // ❌ Don't store
}

// ✅ CORRECT: Store only reference, query when needed
type OrderAssignment struct {
    RiderID      uuid.UUID  // ✅ Only reference
}

// Query rider data from logistics-service when needed
func getRiderDetails(ctx context.Context, riderID uuid.UUID) (*Rider, error) {
    return logisticsService.GetFleetMember(ctx, tenantID, riderID)
}
```

---

## Subscription Plan Integration

### Service Availability Check

**Subscription Plans** define which services are available:
- Starter Plan: cafe-service only
- Growth Plan: cafe-service + logistics-service
- Professional Plan: All services (cafe, logistics, inventory, POS, treasury, notifications)

**Before creating/referencing data in another service:**
1. Check tenant subscription plan: `GET /api/v1/tenants/{tenant_id}/subscription`
2. Verify service in plan features: `plan.features.includes("logistics")`
3. If not available: Show error or redirect to upgrade

---

## Authentication & SSO

### Single Sign-On (SSO)

- All users authenticate via **auth-service** (`https://sso.codevertexitsolutions.com/`)
- JWT tokens contain: `user_id`, `tenant_id`, `roles`, `permissions`
- All services validate tokens via JWKS from auth-service
- Users can access multiple services with same credentials

### Service-Specific Roles

- **Auth-Service**: Global roles (`superuser`, `admin`, `user`)
- **Cafe-Service**: Cafe-specific roles (`customer`, `staff`, `admin`)
- **Logistics-Service**: Logistics-specific roles (`rider`, `fleet_manager`)
- **Combined**: User can have multiple roles across services

---

## Examples

### Example 1: Creating a Rider from Cafe Service

**Scenario**: Tenant has cafe-service and logistics-service enabled

1. User clicks "Become a Rider" in cafe UI
2. Cafe-frontend checks tenant services: `GET /api/v1/tenants/{tenant_id}/services`
3. If logistics enabled:
   - Option A: Submit form to cafe-backend → pushes to logistics-service API
   - Option B: Redirect to logistics-service UI for self-onboarding
4. Logistics-service creates rider user in auth-service (if not exists)
5. Logistics-service stores rider profile locally
6. Returns `rider_id` to cafe service
7. Cafe service stores `rider_id` reference

### Example 2: Standalone Logistics Service

**Scenario**: Tenant only has logistics-service (no cafe-service)

1. User goes to logistics-service UI
2. User authenticates via auth-service (SSO)
3. User completes rider onboarding
4. All rider data stored in logistics-service
5. No cafe-service involvement needed

### Example 3: Order Assignment with Rider

**Scenario**: Assign rider to order

1. Cafe service queries available riders: `GET /v1/{tenant}/fleet-members?status=available`
2. Logistics-service returns rider list (all data from logistics-service)
3. Cafe service selects rider and stores `rider_id` in `order_assignments` table
4. Cafe service creates delivery task: `POST /v1/{tenant}/tasks` with `order_id` and `rider_id`
5. Logistics-service manages task lifecycle (assignment, acceptance, completion)
6. Cafe service consumes events: `logistics.task.assigned`, `logistics.task.completed`

---

## Best Practices

1. **Always Check Service Availability**: Before creating/referencing data, verify tenant has service enabled
2. **Store Only References**: Never duplicate data, always store reference IDs
3. **Query When Needed**: Query owning service for data when needed, don't cache long-term
4. **Use Events for Sync**: Subscribe to events from owning service for real-time updates
5. **Handle Service Unavailability**: Gracefully handle cases where service is not available
6. **Support Standalone Mode**: Services should work independently if tenant only has that service

---

## Migration Notes

- Legacy `riderprofile` and `riderdocument` schemas in cafe-backend are **deprecated** and **unused**
- All rider data migration should go to logistics-service
- Cafe-backend should only store `rider_id` references going forward

---

## References

- [Auth-Service Integration](integrations.md#auth-service)
- [Logistics-Service Integration](integrations.md#logistics-service)
- [Entity Relationship Diagram](erd.md)

