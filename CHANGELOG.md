# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed
- Standardized Swagger documentation path to `/v1/docs` (previously `/swagger/*`)
- Updated Swagger specifications to support both HTTP and HTTPS schemes

### Added
- Initial Go service scaffolding with configuration, logging, health endpoints, and documentation
- Delivery plan updates covering rider KYC verification service, look & feel configuration APIs, and task gating based on verification status.
- Identity module with RBAC roles/permissions, JWT access/refresh tokens, Google OAuth2 initiation/callback handlers, profile/preferences/security endpoints, and customer order summaries for dashboards.
- Auth middleware stack (Bearer JWT verification, role/permission guards) and `/v1/auth` + `/v1/users` routes wired to the in-memory identity repository for Sprint 0 demo flows.
- PostgreSQL schema migrations covering identity, catalog, orders, payments, logistics, operations, notifications, analytics, and support domains.
- `docs/erd.md` ERD reference plus seed script for core roles, permissions, and bootstrap super admin.
- Ent ORM scaffolding (code generation + migrations) wired to Postgres using the `pgx` driver via `ent/dialect/sql` wrappers, including updated migrate/seed commands.
- Delivery plan expansion for subscription/licensing, POS integrations, and system configuration surfaces aligning with inception/proposal documents.
- ERD updates outlining subscription plans, tenant subscriptions, POS integration tables, backup jobs, and integration settings to guide upcoming schema work.
- Tenant sync event pipeline (`tenant_sync_events` Ent schema) to broadcast webhook discovery payloads to logistics, inventory, POS, notifications, and treasury services.
- **Auth-Service SSO Integration:** Integrated `shared/auth-client` v0.1.0 library for production-ready JWT validation using JWKS from auth-service. All protected `/api/v1` routes require valid Bearer tokens. Swagger documentation updated with BearerAuth security definition. Uses monorepo `replace` directives with versioned dependency. See `shared/auth-client/DEPLOYMENT.md` and `shared/auth-client/TAGGING.md` for details.

### Changed
- Migration workflow supports full schema resets via `FOOD_DELIVERY_RESET_DB=true`; seed command now enqueues tenant discovery events after inserting bootstrap data.
- Updated all module references from `github.com/bengobox/food-delivery-backend` to `github.com/bengobox/cafe-backend` for consistency.
- Replaced local `replace` directive with Go workspace (`go.work`) for local development; production deployments use private Go module approach.
