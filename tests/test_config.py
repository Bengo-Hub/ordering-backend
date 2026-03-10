"""
Test configuration for ordering-backend E2E tests.

Production domains from devops-k8s/apps/ordering-backend/values.yaml
"""

import os
from dataclasses import dataclass


@dataclass
class TestConfig:
    """Configuration for ordering service E2E tests."""
    
    # Production API URLs (with /api/v1 path prefix for ordering-backend)
    API_BASE_URL: str = "https://orderingapi.codevertexitsolutions.com/api/v1"
    AUTH_API_URL: str = "https://sso.codevertexitsolutions.com"
    TREASURY_API_URL: str = "https://booksapi.codevertexitsolutions.com"
    LOGISTICS_API_URL: str = "https://logisticsapi.codevertexitsolutions.com"
    INVENTORY_API_URL: str = "https://inventoryapi.codevertexitsolutions.com"
    
    # Frontend URL
    FRONTEND_URL: str = "https://ordersapp.codevertexitsolutions.com"
    
    # Test tenant
    TENANT_SLUG: str = "urban-loft"
    
    # Test credentials (from auth-api seed script)
    # Demo user - safe to share, has member role in all tenants
    TEST_EMAIL: str = os.getenv("TEST_EMAIL", "demo@bengobox.dev")
    TEST_PASSWORD: str = os.getenv("TEST_PASSWORD", "DemoUser2024!")
    
    # Staff/Admin credentials for urban-loft tenant
    STAFF_EMAIL: str = os.getenv("STAFF_EMAIL", "staff@urban-loft.com")
    STAFF_PASSWORD: str = os.getenv("STAFF_PASSWORD", "Staffurban-loft2024!")
    
    # Admin credentials for urban-loft tenant  
    ADMIN_EMAIL: str = os.getenv("ADMIN_EMAIL", "admin@theurbanloftcafe.com")
    ADMIN_PASSWORD: str = os.getenv("ADMIN_PASSWORD", "TenantAdmin2024!")
    
    TEST_PHONE: str = os.getenv("TEST_PHONE", "+254700000001")
    
    # Timeouts
    DEFAULT_TIMEOUT: int = 30
    PAYMENT_POLL_INTERVAL: float = 3.0
    PAYMENT_MAX_POLL_ATTEMPTS: int = 40
    
    # Auth endpoints
    AUTH_TOKEN_URL: str = "https://sso.codevertexitsolutions.com/api/v1/token"
    AUTH_ME_URL: str = "https://sso.codevertexitsolutions.com/api/v1/auth/me"
    AUTH_JWKS_URL: str = "https://sso.codevertexitsolutions.com/api/v1/.well-known/jwks.json"
    AUTH_LOGIN_URL: str = "https://sso.codevertexitsolutions.com/api/v1/auth/login"
    
    # Ordering API endpoints (from internal/http/handlers/catalog/handler.go)
    # Public menu endpoints (no auth required)
    MENU_CATEGORIES_URL: str = "/menu/categories"  # GET - ListPublicCategories
    MENU_ITEMS_URL: str = "/menu/items"          # GET - ListPublicMenuItems
    MENU_ITEM_DETAIL_URL: str = "/menu/items/{id}"  # GET - GetPublicMenuItem
    
    # Public cafes/outlets endpoints (no auth required)
    CAFES_URL: str = "/cafes"                    # GET - ListCafes
    CAFE_DETAIL_URL: str = "/cafes/{id}"         # GET - GetCafe
    
    # Admin catalog endpoints (auth required with catalog:view/manage permissions)
    CATALOG_CATEGORIES_URL: str = "/catalog/categories"  # GET/POST
    CATALOG_ITEMS_URL: str = "/catalog/items"            # GET/POST
    CATALOG_ITEM_VARIANTS_URL: str = "/catalog/items/{id}/variants"
    CATALOG_DIETARY_TAGS_URL: str = "/catalog/dietary-tags"
    
    # Ordering endpoints
    ORDERS_URL: str = "/orders"                  # GET/POST
    CART_URL: str = "/cart"                      # GET/POST
    ADDRESSES_URL: str = "/addresses"            # GET/POST
    PAYMENTS_URL: str = "/payments"              # GET/POST
    
    # Fulfilment endpoints
    DELIVERY_TASKS_URL: str = "/fulfilment/tasks"  # GET - delivery tasks
    TRACKING_URL: str = "/fulfilment/tracking"     # GET - order tracking


# Default config instance
config = TestConfig()
