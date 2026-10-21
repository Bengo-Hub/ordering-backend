package treasury

import (
	"testing"
	"time"
)

func TestWebhookVerifierFailsClosed(t *testing.T) {
	payload := []byte(`{"id":"evt_1","type":"payment.succeeded"}`)

	// No secret: even a signature computed with the empty key must be refused.
	open := NewWebhookVerifier("")
	if open.Configured() {
		t.Fatal("an empty secret is not a configured verifier")
	}
	if err := open.Verify(payload, open.SignPayload(payload), time.Minute); err == nil {
		t.Fatal("a signature made with an empty secret must not verify")
	}
	if err := open.Verify(payload, "", time.Minute); err == nil {
		t.Fatal("an unsigned request must not verify")
	}

	v := NewWebhookVerifier("test-secret-not-real")
	if err := v.Verify(payload, "", time.Minute); err == nil {
		t.Fatal("a missing signature must not verify")
	}
	if err := v.Verify(payload, v.SignPayload(payload), time.Minute); err != nil {
		t.Fatalf("a valid signature must verify: %v", err)
	}
	if err := v.Verify([]byte(`{"id":"evt_2"}`), v.SignPayload(payload), time.Minute); err == nil {
		t.Fatal("a signature for another payload must not verify")
	}
	other := NewWebhookVerifier("another-secret")
	if err := v.Verify(payload, other.SignPayload(payload), time.Minute); err == nil {
		t.Fatal("a signature from another secret must not verify")
	}
}
