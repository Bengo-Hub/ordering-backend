"""
E2E Test Configuration for Codevertex MVP Production Testing
==========================================================

This module contains configuration and base classes for E2E testing
the Codevertex microservices ecosystem.

Production Domains (from devops-k8s values.yaml audit):
- Auth API: sso.codevertexafrica.com
- Auth UI: accounts.codevertexafrica.com
- Ordering API: orderingapi.codevertexafrica.com
- Ordering UI: ordering.codevertexafrica.com (also ordering.codevertexafrica.com)
- Treasury API: booksapi.codevertexafrica.com
- Treasury UI: books.codevertexafrica.com
- Subscription API: pricingapi.codevertexafrica.com
- Subscription UI: subscriptions.codevertexafrica.com
- Logistics API: logisticsapi.codevertexafrica.com
- Logistics UI: logistics.codevertexafrica.com
- Rider App: riderapp.codevertexafrica.com
- Inventory API: inventoryapi.codevertexafrica.com
- Inventory UI: inventory.codevertexafrica.com
- POS API: posapi.codevertexafrica.com
- POS UI: pos.codevertexafrica.com
- Notifications API: notificationsapi.codevertexafrica.com
- Notifications UI: notifications.codevertexafrica.com
- Cafe Website: theurbanloftcafe.com

Usage:
    from config import TestConfig, ServiceEndpoints
    
    config = TestConfig(
        tenant_slug="urban-loft",
        timeout=30
    )
    endpoints = ServiceEndpoints(config)
"""

import os
from dataclasses import dataclass, field
from typing import Optional, Dict, Any
from urllib.parse import urljoin


@dataclass
class TestConfig:
    """Configuration for E2E tests."""
    
    # Tenant configuration
    tenant_slug: str = "urban-loft"
    tenant_id: Optional[str] = None
    
    # Test timeouts
    timeout: int = 30
    poll_interval: float = 3.0  # For payment polling
    max_poll_attempts: int = 40  # ~2 minutes for payment
    
    # Authentication
    test_email: str = field(default_factory=lambda: os.getenv("TEST_EMAIL", "test@example.com"))
    test_password: str = field(default_factory=lambda: os.getenv("TEST_PASSWORD", "TestPass123!"))
    admin_email: str = field(default_factory=lambda: os.getenv("ADMIN_EMAIL", "admin@ur.com"))
    admin_password: str = field(default_factory=lambda: os.getenv("ADMIN_PASSWORD", ""))
    
    # API Keys (loaded from env)
    treasury_api_key: str = field(default_factory=lambda: os.getenv("TREASURY_API_KEY", ""))
    ordering_api_key: str = field(default_factory=lambda: os.getenv("ORDERING_API_KEY", ""))
    
    # Feature flags
    headless: bool = True
    screenshot_on_failure: bool = True
    
    def __post_init__(self):
        """Validate configuration."""
        if not self.tenant_slug:
            raise ValueError("tenant_slug is required")


@dataclass
class ServiceEndpoints:
    """Service endpoint URLs for E2E testing."""
    
    def __init__(self, config: TestConfig = None):
        self.config = config or TestConfig()
        self._base_urls = self._get_base_urls()
    
    def _get_base_urls(self) -> Dict[str, str]:
        """Get base URLs for all services."""
        return {
            # Auth Service
            "auth_api": "https://sso.codevertexafrica.com",
            "auth_ui": "https://accounts.codevertexafrica.com",
            
            # Ordering Service
            "ordering_api": "https://orderingapi.codevertexafrica.com",
            "ordering_ui": "https://ordering.codevertexafrica.com",
            
            # Treasury Service
            "treasury_api": "https://booksapi.codevertexafrica.com",
            "treasury_ui": "https://books.codevertexafrica.com",
            
            # Subscription Service
            "subscription_api": "https://pricingapi.codevertexafrica.com",
            "subscription_ui": "https://subscriptions.codevertexafrica.com",
            
            # Logistics Service
            "logistics_api": "https://logisticsapi.codevertexafrica.com",
            "logistics_ui": "https://logistics.codevertexafrica.com",
            # Rider App
            "rider_app": "https://riderapp.codevertexafrica.com",
            
            # Inventory Service
            "inventory_api": "https://inventoryapi.codevertexafrica.com",
            "inventory_ui": "https://inventory.codevertexafrica.com",
            
            # POS Service
            "pos_api": "https://posapi.codevertexafrica.com",
            "pos_ui": "https://pos.codevertexafrica.com",
            
            # Notifications Service
            "notifications_api": "https://notificationsapi.codevertexafrica.com",
            "notifications_ui": "https://notifications.codevertexafrica.com",
            
            # Cafe Website
            "cafe_website": "https://theurbanloftcafe.com",
        }
    
    def get_url(self, service: str, path: str = "") -> str:
        """Get full URL for a service endpoint."""
        base = self._base_urls.get(service)
        if not base:
            raise ValueError(f"Unknown service: {service}")
        if path:
            return urljoin(base, path)
        return base
    
    def get_tenant_url(self, service: str, path: str = "") -> str:
        """Get tenant-scoped URL for a service."""
        base = self._base_urls.get(service)
        if not base:
            raise ValueError(f"Unknown service: {service}")
        
        tenant_path = f"/{self.config.tenant_slug}"
        if path:
            tenant_path = f"{tenant_path}{path}"
        
        return urljoin(base, tenant_path)
    
    # Auth Service Endpoints
    @property
    def auth_health(self) -> str:
        return self.get_url("auth_api", "/healthz")
    
    @property
    def auth_oidc_discovery(self) -> str:
        return self.get_url("auth_api", "/.well-known/openid-configuration")
    
    @property
    def auth_token(self) -> str:
        return self.get_url("auth_api", "/api/v1/token")
    
    @property
    def auth_login(self) -> str:
        return self.get_url("auth_ui", "/auth")
    
    # Ordering Service Endpoints
    @property
    def ordering_health(self) -> str:
        return self.get_url("ordering_api", "/healthz")
    
    def ordering_menu_categories(self) -> str:
        return self.get_tenant_url("ordering_api", "/menu/categories")
    
    def ordering_menu_items(self) -> str:
        return self.get_tenant_url("ordering_api", "/menu/items")
    
    def ordering_create_order(self) -> str:
        return self.get_tenant_url("ordering_api", "/orders")
    
    def ordering_get_order(self, order_id: str) -> str:
        return self.get_tenant_url("ordering_api", f"/orders/{order_id}")
    
    # Treasury Service Endpoints
    @property
    def treasury_health(self) -> str:
        return self.get_url("treasury_api", "/healthz")
    
    def treasury_create_payment_intent(self) -> str:
        return self.get_tenant_url("treasury_api", "/payments/intents")
    
    def treasury_initiate_payment(self, intent_id: str) -> str:
        return self.get_tenant_url("treasury_api", f"/payments/intents/{intent_id}/initiate")
    
    @property
    def treasury_paystack_webhook(self) -> str:
        return self.get_url("treasury_api", "/api/v1/webhooks/paystack")
    
    # Subscription Service Endpoints
    @property
    def subscription_health(self) -> str:
        return self.get_url("subscription_api", "/healthz")
    
    def subscription_plans(self) -> str:
        return self.get_tenant_url("subscription_api", "/plans")
    
    def subscription_subscribe(self) -> str:
        return self.get_tenant_url("subscription_api", "/subscriptions")
    
    # Logistics Service Endpoints
    @property
    def logistics_health(self) -> str:
        return self.get_url("logistics_api", "/healthz")
    
    def logistics_fleet(self) -> str:
        return self.get_tenant_url("logistics_api", "/fleet")
    
    def logistics_tasks(self) -> str:
        return self.get_tenant_url("logistics_api", "/tasks")
    
    # Rider App Endpoints
    @property
    def rider_health(self) -> str:
        return self.get_url("rider_app", "/healthz")
    
    # Inventory Service Endpoints
    @property
    def inventory_health(self) -> str:
        return self.get_url("inventory_api", "/healthz")
    
    def inventory_items(self) -> str:
        return self.get_tenant_url("inventory_api", "/items")
    
    # Notifications Service Endpoints
    @property
    def notifications_health(self) -> str:
        return self.get_url("notifications_api", "/healthz")


# Default instances for convenience
config = TestConfig()
endpoints = ServiceEndpoints(config)
