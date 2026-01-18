# Sprint 2 - Catalog & Localization

**Duration**: Weeks 4-5
**Status**: ✅ Complete (January 2026)

---

## Sprint Progress (Updated January 2026)

| Task | Status | Notes |
|------|--------|-------|
| Ent schema: MenuCategory | ✅ Complete | `internal/ent/schema/menucategory.go` |
| Ent schema: MenuItem | ✅ Complete | `internal/ent/schema/menuitem.go` |
| Ent schema: MenuItemVariant | ✅ Complete | `internal/ent/schema/menuitemvariant.go` |
| Ent schema: MenuItemTranslation | ✅ Complete | `internal/ent/schema/menuitemtranslation.go` |
| Ent schema: DietaryTag | ✅ Complete | `internal/ent/schema/dietarytag.go` |
| Ent schema: MenuItemAsset | ✅ Complete | `internal/ent/schema/menuitemasset.go` |
| Ent schema: MenuItemSchedule | ✅ Complete | `internal/ent/schema/menuitemschedule.go` |
| Run Ent code generation | ✅ Complete | Generated code in `internal/ent/` |
| Run database migrations | ✅ Complete | Ent auto-migration on startup |
| Category CRUD endpoints | ✅ Complete | `internal/http/handlers/catalog/category_handler.go` |
| Menu item CRUD endpoints | ✅ Complete | `internal/http/handlers/catalog/item_handler.go` |
| Variant management | ✅ Complete | Create/List variants per item |
| Translation management | ✅ Complete | Multi-locale support (EN/SW) |
| Dietary tag system | ✅ Complete | Tag management and item assignment |
| Public menu API | ✅ Complete | `/api/v1/menu/*` endpoints |
| Admin catalog API | ✅ Complete | `/api/v1/catalog/*` with RBAC |
| Localization support | ✅ Complete | Locale-aware menu API |
| Image upload | ⏳ Deferred | S3 integration (Sprint 11) |
| CDN integration | ⏳ Deferred | CloudFront/CDN setup (Sprint 11) |

**Implementation Summary**:
- Full catalog module: `internal/modules/catalog/`
- Service layer: `service.go` (679 lines of business logic)
- Ent repository: `repository_ent.go` (967 lines)
- Domain models: `domain.go`, errors: `errors.go`
- HTTP handlers with RBAC: `internal/http/handlers/catalog/`
- Routes wired into main router with permission checks

**API Endpoints Implemented**:
- `GET /api/v1/menu/categories` - Public categories
- `GET /api/v1/menu/items` - Public menu items
- `GET /api/v1/menu/items/{id}` - Public item details
- `POST/GET/PUT/DELETE /api/v1/catalog/categories` - Admin category CRUD
- `POST/GET/PUT/DELETE /api/v1/catalog/items` - Admin item CRUD
- `POST/GET /api/v1/catalog/items/{id}/variants` - Variant management
- `POST/GET /api/v1/catalog/items/{id}/translations` - Translations
- `GET /api/v1/catalog/dietary-tags` - List dietary tags
- `POST/DELETE /api/v1/catalog/items/{id}/dietary-tags` - Tag assignment

---

## Overview

Sprint 2 focuses on building the menu catalog system with full localization support (EN/SW), category hierarchy, image handling, and availability scheduling.

---

## Objectives

1. Menu CRUD operations
2. Category hierarchy management
3. Image handling and CDN integration
4. Localization fields (EN/SW)
5. Public menu API
6. Dietary tags and variants

---

## Technology Stack

### Image Storage
- **Storage**: S3-compatible storage
- **CDN**: CloudFront or similar
- **Image Processing**: Server-side resizing/optimization

### Localization
- **i18n**: Database-level localization fields
- **Languages**: English (en), Swahili (sw)
- **Translation Management**: Database tables for translations

### File Upload
- **Upload Handler**: Multipart form handling
- **Validation**: File type, size limits
- **Processing**: Image optimization, thumbnail generation

---

## User Stories

### US-2.1: Menu Categories
**As a** cafe administrator
**I want** to manage menu categories
**So that** I can organize menu items

**Acceptance Criteria**:
- [x] Create, read, update, delete categories
- [x] Category hierarchy support
- [x] Display order management
- [x] Category activation/deactivation

### US-2.2: Menu Items
**As a** cafe administrator
**I want** to manage menu items
**So that** I can offer products to customers

**Acceptance Criteria**:
- [x] Create, read, update, delete menu items
- [x] Item variants (size, flavor)
- [x] Pricing management
- [x] Availability scheduling
- [ ] Image upload and management (Deferred to Sprint 11)

### US-2.3: Localization
**As a** cafe administrator
**I want** to provide menu content in multiple languages
**So that** customers can understand items in their preferred language

**Acceptance Criteria**:
- [x] English and Swahili translations
- [x] Translation CRUD operations
- [x] Fallback to default language
- [x] Locale-aware menu API

### US-2.4: Dietary Tags
**As a** customer
**I want** to see dietary information for menu items
**So that** I can make informed choices

**Acceptance Criteria**:
- [x] Dietary tag management
- [x] Tag assignment to items
- [x] Filter by dietary tags
- [x] Tag display in menu

### US-2.5: Public Menu API
**As a** customer
**I want** to browse the menu without authentication
**So that** I can see available items

**Acceptance Criteria**:
- [x] Public menu endpoint
- [x] Filter by category, dietary tags
- [x] Search functionality
- [x] Availability filtering

---

## API Endpoints

### Categories

**GET /api/v1/{tenant}/catalog/categories**
- List all categories for tenant
- Query params: `cafe_id`, `is_active`
- Response: Array of category objects

**POST /api/v1/{tenant}/catalog/categories**
- Create new category
- Request: `{ "name": "...", "description": "...", "display_order": 1, "cafe_id": "..." }`

**PUT /api/v1/{tenant}/catalog/categories/{id}**
- Update category
- Request: Same as POST

**DELETE /api/v1/{tenant}/catalog/categories/{id}**
- Delete category (soft delete)

### Menu Items

**GET /api/v1/{tenant}/catalog/items**
- List menu items
- Query params: `cafe_id`, `category_id`, `is_available`, `dietary_tag`, `search`
- Pagination support
- Response: Array of menu item objects with variants

**GET /api/v1/{tenant}/catalog/items/{id}**
- Get single menu item details
- Includes variants, translations, dietary tags

**POST /api/v1/{tenant}/catalog/items**
- Create new menu item
- Request: `{ "name": "...", "description": "...", "base_price": 100, "category_id": "...", "cafe_id": "..." }`

**PUT /api/v1/{tenant}/catalog/items/{id}**
- Update menu item

**DELETE /api/v1/{tenant}/catalog/items/{id}**
- Delete menu item (soft delete)

### Variants

**POST /api/v1/{tenant}/catalog/items/{id}/variants**
- Add variant to menu item
- Request: `{ "name": "...", "price_delta": 50, "sku": "..." }`

**PUT /api/v1/{tenant}/catalog/variants/{id}**
- Update variant

**DELETE /api/v1/{tenant}/catalog/variants/{id}**
- Delete variant

### Translations

**POST /api/v1/{tenant}/catalog/items/{id}/translations**
- Add translation for menu item
- Request: `{ "locale": "sw", "name": "...", "description": "..." }`

**PUT /api/v1/{tenant}/catalog/translations/{id}**
- Update translation

### Images

**POST /api/v1/{tenant}/catalog/items/{id}/images**
- Upload image for menu item
- Multipart form data
- Response: Image URL

**DELETE /api/v1/{tenant}/catalog/images/{id}**
- Delete image

### Public Menu

**GET /api/v1/public/{tenant_slug}/menu**
- Public menu endpoint (no authentication)
- Query params: `cafe_id`, `category_id`, `locale`, `dietary_tag`
- Response: Menu structure with items

---

## Database Schema

### Catalog Module

**menu_categories**
- `id` (UUID, PK)
- `tenant_id` (UUID, FK → tenants)
- `cafe_id` (UUID, FK → cafes)
- `name` (VARCHAR)
- `description` (TEXT)
- `display_order` (INTEGER)
- `is_active` (BOOLEAN, default: true)
- `created_at`, `updated_at` (TIMESTAMPTZ)

**menu_items**
- `id` (UUID, PK)
- `tenant_id` (UUID, FK → tenants)
- `cafe_id` (UUID, FK → cafes)
- `category_id` (UUID, FK → menu_categories)
- `name` (VARCHAR)
- `description` (TEXT)
- `base_price` (DECIMAL)
- `currency` (VARCHAR, default: 'KES')
- `is_available` (BOOLEAN, default: true)
- `lead_time_minutes` (INTEGER)
- `image_url` (VARCHAR)
- `nutrition_json` (JSONB)
- `created_at`, `updated_at` (TIMESTAMPTZ)

**menu_item_variants**
- `id` (UUID, PK)
- `menu_item_id` (UUID, FK → menu_items)
- `name` (VARCHAR)
- `price_delta` (DECIMAL)
- `is_available` (BOOLEAN, default: true)
- `sku` (VARCHAR)
- `created_at`, `updated_at` (TIMESTAMPTZ)

**menu_item_translations**
- `menu_item_id` (UUID, FK → menu_items)
- `locale` (VARCHAR, PK)
- `name` (VARCHAR)
- `description` (TEXT)
- `created_at`, `updated_at` (TIMESTAMPTZ)
- Composite PK: (menu_item_id, locale)

**dietary_tags**
- `code` (VARCHAR, PK)
- `label` (VARCHAR)
- `description` (TEXT)
- `icon_url` (VARCHAR)

**menu_item_dietary_tags**
- `menu_item_id` (UUID, FK → menu_items)
- `dietary_code` (VARCHAR, FK → dietary_tags)
- Composite PK: (menu_item_id, dietary_code)

**menu_item_assets**
- `id` (UUID, PK)
- `menu_item_id` (UUID, FK → menu_items)
- `asset_type` (VARCHAR)
- `url` (VARCHAR)
- `metadata` (JSONB)
- `created_at` (TIMESTAMPTZ)

**menu_item_schedules**
- `id` (UUID, PK)
- `menu_item_id` (UUID, FK → menu_items)
- `day_of_week` (INTEGER, 0-6)
- `time_start` (TIME)
- `time_end` (TIME)
- `created_at` (TIMESTAMPTZ)

---

## Code Structure

### Module Organization

**Catalog Module** (`internal/modules/catalog/`):
- `category.go` - Category domain models and service
- `item.go` - Menu item domain models and service
- `variant.go` - Variant domain models and service
- `translation.go` - Translation domain models and service
- `image.go` - Image upload and management service
- `dietary.go` - Dietary tag domain models and service

**Handlers** (`internal/http/handlers/catalog/`):
- `category_handler.go` - Category HTTP handlers
- `item_handler.go` - Menu item HTTP handlers
- `public_handler.go` - Public menu HTTP handlers

---

## Integration Points

### Inventory Service
- **Query**: Stock availability for menu items (by SKU)
- **Event**: `inventory.stock.updated` - Update item availability
- **Event**: `inventory.stock.low` - Handle low stock alerts

### S3 Storage
- **Upload**: Menu item images
- **CDN**: Image delivery via CDN URLs
- **Processing**: Image resizing and optimization

---

## Testing Strategy

### Unit Tests
- Service layer tests for CRUD operations
- Translation fallback logic tests
- Image upload validation tests

### Integration Tests
- End-to-end menu creation flow
- Image upload and CDN integration
- Public menu API tests
- Localization switching tests

---

## Deliverables

- [x] Menu category CRUD endpoints (`internal/http/handlers/catalog/category_handler.go`)
- [x] Menu item CRUD endpoints (`internal/http/handlers/catalog/item_handler.go`)
- [x] Variant management endpoints (Create/List variants per item)
- [x] Translation management endpoints (Multi-locale support EN/SW)
- [ ] Image upload and management (Deferred - Sprint 11, S3 integration)
- [x] Dietary tag system (Tag management and item assignment)
- [x] Public menu API (`/api/v1/menu/*` endpoints)
- [x] Availability scheduling (`MenuItemSchedule` entity)
- [x] Database migrations (Ent auto-migration)
- [ ] Integration tests (Ongoing)

---

## Dependencies

- S3-compatible storage for images
- Inventory service for stock availability
- CDN for image delivery

---

## Next Steps

- Sprint 3: Orders & Cart
  - Cart service implementation
  - Checkout workflow
  - Promo code validation
  - Order state machine

