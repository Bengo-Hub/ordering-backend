package logistics

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func sign(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestVerifySignatureFailsClosed(t *testing.T) {
	payload := []byte(`{"event":"task.completed"}`)

	// No secret configured: nothing verifies, not even a signature made with the empty key.
	open := NewWebhookVerifier("")
	if open.VerifySignature(payload, sign("", payload)) || open.VerifySignature(payload, "") {
		t.Fatal("a verifier without a secret must reject everything")
	}

	v := NewWebhookVerifier("test-secret-not-real")
	if v.VerifySignature(payload, "") {
		t.Fatal("an unsigned request must be rejected")
	}
	if !v.VerifySignature(payload, sign("test-secret-not-real", payload)) {
		t.Fatal("a valid signature must verify")
	}
	if v.VerifySignature(payload, sign("another-secret", payload)) {
		t.Fatal("a signature from another secret must be rejected")
	}
	if v.VerifySignature([]byte(`{"event":"task.failed"}`), sign("test-secret-not-real", payload)) {
		t.Fatal("a signature for another payload must be rejected")
	}
}
