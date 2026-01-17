package treasury

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
)

// WebhookEvent represents an incoming webhook event from treasury.
type WebhookEvent struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
}

// WebhookPaymentData represents payment data in a webhook event.
type WebhookPaymentData struct {
	PaymentIntentID    uuid.UUID `json:"payment_intent_id"`
	OrderID            uuid.UUID `json:"order_id"`
	TenantID           uuid.UUID `json:"tenant_id"`
	Amount             float64   `json:"amount"`
	Currency           string    `json:"currency"`
	Provider           string    `json:"provider"`
	ProviderReference  string    `json:"provider_reference,omitempty"`
	ProviderReceipt    string    `json:"provider_receipt,omitempty"`
	MpesaTransactionID string    `json:"mpesa_transaction_id,omitempty"`
	MpesaPhoneNumber   string    `json:"mpesa_phone_number,omitempty"`
	Status             string    `json:"status"`
	ErrorCode          string    `json:"error_code,omitempty"`
	ErrorMessage       string    `json:"error_message,omitempty"`
	ProcessedAt        *time.Time `json:"processed_at,omitempty"`
}

// WebhookRefundData represents refund data in a webhook event.
type WebhookRefundData struct {
	RefundID          uuid.UUID  `json:"refund_id"`
	PaymentID         uuid.UUID  `json:"payment_id"`
	OrderID           uuid.UUID  `json:"order_id"`
	TenantID          uuid.UUID  `json:"tenant_id"`
	Amount            float64    `json:"amount"`
	Currency          string     `json:"currency"`
	Status            string     `json:"status"`
	ProviderRefundID  string     `json:"provider_refund_id,omitempty"`
	ProviderReference string     `json:"provider_reference,omitempty"`
	ErrorCode         string     `json:"error_code,omitempty"`
	ErrorMessage      string     `json:"error_message,omitempty"`
	ProcessedAt       *time.Time `json:"processed_at,omitempty"`
}

// WebhookVerifier verifies webhook signatures.
type WebhookVerifier struct {
	secret string
}

// NewWebhookVerifier creates a new webhook verifier.
func NewWebhookVerifier(secret string) *WebhookVerifier {
	return &WebhookVerifier{secret: secret}
}

// Verify verifies the webhook signature.
// The signature should be in the format: t=timestamp,v1=signature
// The signature is computed as: HMAC-SHA256(timestamp.payload, secret)
func (v *WebhookVerifier) Verify(payload []byte, signature string, tolerance time.Duration) error {
	// Parse signature header
	timestamp, sig, err := parseSignature(signature)
	if err != nil {
		return fmt.Errorf("invalid signature format: %w", err)
	}

	// Check timestamp tolerance (prevents replay attacks)
	if tolerance > 0 {
		signedAt := time.Unix(timestamp, 0)
		if time.Since(signedAt) > tolerance {
			return fmt.Errorf("signature timestamp too old: signed at %v", signedAt)
		}
	}

	// Compute expected signature
	expectedSig := v.computeSignature(timestamp, payload)

	// Compare signatures using constant-time comparison
	if !hmac.Equal([]byte(sig), []byte(expectedSig)) {
		return fmt.Errorf("signature mismatch")
	}

	return nil
}

// computeSignature computes the expected signature for a payload.
func (v *WebhookVerifier) computeSignature(timestamp int64, payload []byte) string {
	signedPayload := fmt.Sprintf("%d.%s", timestamp, payload)
	mac := hmac.New(sha256.New, []byte(v.secret))
	mac.Write([]byte(signedPayload))
	return hex.EncodeToString(mac.Sum(nil))
}

// SignPayload signs a payload for testing purposes.
func (v *WebhookVerifier) SignPayload(payload []byte) string {
	timestamp := time.Now().Unix()
	sig := v.computeSignature(timestamp, payload)
	return fmt.Sprintf("t=%d,v1=%s", timestamp, sig)
}

// parseSignature parses the signature header.
func parseSignature(header string) (int64, string, error) {
	var timestamp int64
	var sig string

	// Parse key=value pairs
	for i := 0; i < len(header); {
		// Find key
		keyStart := i
		for i < len(header) && header[i] != '=' {
			i++
		}
		if i >= len(header) {
			break
		}
		key := header[keyStart:i]
		i++ // skip '='

		// Find value
		valueStart := i
		for i < len(header) && header[i] != ',' {
			i++
		}
		value := header[valueStart:i]
		if i < len(header) {
			i++ // skip ','
		}

		switch key {
		case "t":
			var err error
			timestamp, err = strconv.ParseInt(value, 10, 64)
			if err != nil {
				return 0, "", fmt.Errorf("invalid timestamp: %w", err)
			}
		case "v1":
			sig = value
		}
	}

	if timestamp == 0 {
		return 0, "", fmt.Errorf("missing timestamp")
	}
	if sig == "" {
		return 0, "", fmt.Errorf("missing signature")
	}

	return timestamp, sig, nil
}

// ParsePaymentEvent parses a payment event from webhook data.
func ParsePaymentEvent(event *WebhookEvent) (*WebhookPaymentData, error) {
	data, err := json.Marshal(event.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal event data: %w", err)
	}

	var payment WebhookPaymentData
	if err := json.Unmarshal(data, &payment); err != nil {
		return nil, fmt.Errorf("unmarshal payment data: %w", err)
	}

	return &payment, nil
}

// ParseRefundEvent parses a refund event from webhook data.
func ParseRefundEvent(event *WebhookEvent) (*WebhookRefundData, error) {
	data, err := json.Marshal(event.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal event data: %w", err)
	}

	var refund WebhookRefundData
	if err := json.Unmarshal(data, &refund); err != nil {
		return nil, fmt.Errorf("unmarshal refund data: %w", err)
	}

	return &refund, nil
}

// IsPaymentEvent checks if the event is a payment-related event.
func IsPaymentEvent(eventType string) bool {
	switch eventType {
	case "payment_initiated", "payment_processing", "payment_succeeded", "payment_failed", "payment_cancelled", "payment_expired":
		return true
	case "mpesa_stk_push_initiated", "mpesa_stk_push_success", "mpesa_stk_push_failed", "mpesa_stk_push_timeout":
		return true
	case "mpesa_c2b_received":
		return true
	}
	return false
}

// IsRefundEvent checks if the event is a refund-related event.
func IsRefundEvent(eventType string) bool {
	switch eventType {
	case "refund_initiated", "refund_processing", "refund_succeeded", "refund_failed":
		return true
	}
	return false
}

// IsSuccessEvent checks if the event indicates a successful outcome.
func IsSuccessEvent(eventType string) bool {
	switch eventType {
	case "payment_succeeded", "mpesa_stk_push_success", "mpesa_c2b_received", "refund_succeeded":
		return true
	}
	return false
}

// IsFailureEvent checks if the event indicates a failed outcome.
func IsFailureEvent(eventType string) bool {
	switch eventType {
	case "payment_failed", "payment_cancelled", "payment_expired", "mpesa_stk_push_failed", "mpesa_stk_push_timeout", "refund_failed":
		return true
	}
	return false
}
