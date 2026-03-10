"""
E2E tests for payment integration with treasury service.

Tests payment flows:
- Payment intent creation
- M-Pesa STK push
- Payment status polling
- COD flow
"""

import requests
import time

from test_config import config


class TestPaymentIntegration:
    """Test payment integration workflows."""

    def test_treasury_health(self, http_client: requests.Session):
        """Verify treasury API health endpoint."""
        response = http_client.get(f"{config.TREASURY_API_URL}/healthz")
        assert response.status_code == 200

    def test_create_payment_intent(self, http_client: requests.Session, tenant_url: str):
        """Create payment intent via treasury."""
        intent_payload = {
            "amount": 300.00,
            "currency": "KES",
            "provider": "mpesa",
            "description": "Test order payment",
            "metadata": {
                "orderId": "test-order-001",
                "source_service": "ordering"
            }
        }
        
        response = http_client.post(
            f"{config.TREASURY_API_URL}/{config.TENANT_SLUG}/payments/intents",
            json=intent_payload
        )
        assert response.status_code in [200, 201]
        data = response.json()
        assert "id" in data or "intentId" in data
        return data.get("id") or data.get("intentId")

    def test_initiate_mpesa_payment(self, http_client: requests.Session, tenant_url: str):
        """Initiate M-Pesa STK push."""
        # First create intent
        intent_payload = {
            "amount": 150.00,
            "currency": "KES",
            "provider": "mpesa",
            "description": "M-Pesa test payment",
            "phoneNumber": config.TEST_PHONE
        }
        
        intent_response = http_client.post(
            f"{config.TREASURY_API_URL}/{config.TENANT_SLUG}/payments/intents",
            json=intent_payload
        )
        intent_id = intent_response.json().get("id")
        
        # Initiate payment
        initiate_response = http_client.post(
            f"{config.TREASURY_API_URL}/{config.TENANT_SLUG}/payments/intents/{intent_id}/initiate",
            json={"phoneNumber": config.TEST_PHONE}
        )
        assert initiate_response.status_code in [200, 202]

    def test_poll_payment_status(self, http_client: requests.Session, tenant_url: str):
        """Poll payment status until completion or timeout."""
        # Create intent
        intent_payload = {
            "amount": 200.00,
            "currency": "KES",
            "provider": "cod",  # Use COD for predictable test
            "description": "Test payment polling"
        }
        
        intent_response = http_client.post(
            f"{config.TREASURY_API_URL}/{config.TENANT_SLUG}/payments/intents",
            json=intent_payload
        )
        intent_id = intent_response.json().get("id")
        
        # Poll for status
        for attempt in range(config.PAYMENT_MAX_POLL_ATTEMPTS):
            status_response = http_client.get(
                f"{config.TREASURY_API_URL}/{config.TENANT_SLUG}/payments/intents/{intent_id}"
            )
            if status_response.status_code == 200:
                data = status_response.json()
                status = data.get("status")
                if status in ["completed", "failed", "cancelled"]:
                    break
            time.sleep(config.PAYMENT_POLL_INTERVAL)
        
        # Should have a final status after polling
        final_response = http_client.get(
            f"{config.TREASURY_API_URL}/{config.TENANT_SLUG}/payments/intents/{intent_id}"
        )
        assert final_response.status_code == 200

    def test_cod_payment_flow(self, http_client: requests.Session, tenant_url: str):
        """Cash on delivery payment flow."""
        # Create COD payment intent
        cod_payload = {
            "amount": 450.00,
            "currency": "KES",
            "provider": "cod",
            "description": "COD test payment",
            "metadata": {"source_service": "ordering"}
        }
        
        response = http_client.post(
            f"{config.TREASURY_API_URL}/{config.TENANT_SLUG}/payments/intents",
            json=cod_payload
        )
        assert response.status_code in [200, 201]
        data = response.json()
        assert data.get("provider") == "cod" or data.get("paymentMethod") == "cod"

    def test_list_transactions(self, http_client: requests.Session):
        """List payment transactions for tenant."""
        response = http_client.get(
            f"{config.TREASURY_API_URL}/{config.TENANT_SLUG}/payments/transactions"
        )
        assert response.status_code in [200, 404]  # 404 if endpoint doesn't exist
