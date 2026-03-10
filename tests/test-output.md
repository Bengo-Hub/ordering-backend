# Ordering Service E2E Test Results

**Test Date:** 2026-03-09 22:17:18
**Tenant:** urban-loft
**Environment:** Production APIs

## Summary

**Total: 5/7 passed (71% success rate)**

- Passed: 5
- Failed: 1
- Skipped: 1

## Test Details

| Phase | Test | Status | Details |
|-------|------|--------|---------|
| AUTH | sso_login | PASS PASS | Login successful |
| AUTH | sso_me | FAIL FAIL | Missing tenant data in /me response |
| AUTH | tenant_sync | SKIP SKIP | No tenant_id from /me endpoint |
| DATA | fetch_categories | PASS PASS | Fetched 8 categories |
| DATA | fetch_items | PASS PASS | Fetched 20 menu items |
| DATA | fetch_outlets | PASS PASS | Fetched 1 outlets |
| DATA | auth_endpoint | PASS PASS | Endpoint status: 400 |

## Failed Test Details

### AUTH - sso_me

- **Status:** FAIL
- **Details:** Missing tenant data in /me response
- **Response:** ```json
{
  "created_at": "2026-01-19T09:23:15.099047Z",
  "email": "demo@bengobox.dev",
  "id": "46898d72-650b-4f2f-8ccc-10a18aae4df6",
  "last_login_at": "2026-03-09T19:17:04.505229Z",
  "permissions": [
    "orders:change_own",
    "orders:add",
    "menu:read",
    "catalog:view",
    "orders:read_own"
  ],
  "primary_tenant": "f2cd3bbc-e54b-4016-8dd1-4510d0664313",
  "profile": {
    "created_by": "seed",
    "is_demo": true,
    "name": "Demo User"
  },
  "roles": [
    "member"
  ],
  "status": "active",
  "updated_at": "2026-03-09T19:17:04.505239Z"
}
```


## API Endpoints Tested

- **Auth API:** https://sso.codevertexitsolutions.com
- **Ordering API:** https://orderingapi.codevertexitsolutions.com/api/v1
- **Menu Categories:** /menu/categories
- **Menu Items:** /menu/items
- **Cafes/Outlets:** /cafes
- **Orders:** /orders
- **Cart:** /cart
