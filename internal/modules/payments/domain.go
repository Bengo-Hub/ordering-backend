package payments

import (
	"time"

	"github.com/google/uuid"
)

// PaymentProvider represents supported payment providers.
type PaymentProvider string

const (
	ProviderMpesa       PaymentProvider = "mpesa"
	ProviderStripe      PaymentProvider = "stripe"
	ProviderPaystack    PaymentProvider = "paystack"
	ProviderFlutterwave PaymentProvider = "flutterwave"
	ProviderManual      PaymentProvider = "manual"
)

// PaymentMethodType represents the type of payment method.
type PaymentMethodType string

const (
	MethodTypeMobileMoney PaymentMethodType = "mobile_money"
	MethodTypeCard        PaymentMethodType = "card"
	MethodTypeBankAccount PaymentMethodType = "bank_account"
	MethodTypeWallet      PaymentMethodType = "wallet"
	MethodTypeCash        PaymentMethodType = "cash"
)

// PaymentIntentStatus represents the status of a payment intent.
type PaymentIntentStatus string

const (
	IntentStatusPending        PaymentIntentStatus = "pending"
	IntentStatusRequiresAction PaymentIntentStatus = "requires_action"
	IntentStatusProcessing     PaymentIntentStatus = "processing"
	IntentStatusSucceeded      PaymentIntentStatus = "succeeded"
	IntentStatusFailed         PaymentIntentStatus = "failed"
	IntentStatusCancelled      PaymentIntentStatus = "cancelled"
	IntentStatusExpired        PaymentIntentStatus = "expired"
)

// PaymentStatus represents the status of a payment.
type PaymentStatus string

const (
	PaymentStatusPending           PaymentStatus = "pending"
	PaymentStatusProcessing        PaymentStatus = "processing"
	PaymentStatusSucceeded         PaymentStatus = "succeeded"
	PaymentStatusFailed            PaymentStatus = "failed"
	PaymentStatusRefunded          PaymentStatus = "refunded"
	PaymentStatusPartiallyRefunded PaymentStatus = "partially_refunded"
)

// RefundStatus represents the status of a refund.
type RefundStatus string

const (
	RefundStatusPending    RefundStatus = "pending"
	RefundStatusProcessing RefundStatus = "processing"
	RefundStatusSucceeded  RefundStatus = "succeeded"
	RefundStatusFailed     RefundStatus = "failed"
	RefundStatusCancelled  RefundStatus = "cancelled"
)

// RefundReason represents the reason for a refund.
type RefundReason string

const (
	RefundReasonCustomerRequest    RefundReason = "customer_request"
	RefundReasonOrderCancelled     RefundReason = "order_cancelled"
	RefundReasonDuplicate          RefundReason = "duplicate"
	RefundReasonFraudulent         RefundReason = "fraudulent"
	RefundReasonProductUnavailable RefundReason = "product_unavailable"
	RefundReasonOther              RefundReason = "other"
)

// PaymentMethod represents a saved payment method.
type PaymentMethod struct {
	ID            uuid.UUID              `json:"id"`
	TenantID      uuid.UUID              `json:"tenant_id"`
	UserID        uuid.UUID              `json:"user_id"`
	Provider      PaymentProvider        `json:"provider"`
	Type          PaymentMethodType      `json:"type"`
	Mask          string                 `json:"mask,omitempty"`
	Label         string                 `json:"label,omitempty"`
	ExpMonth      *int                   `json:"exp_month,omitempty"`
	ExpYear       *int                   `json:"exp_year,omitempty"`
	IsDefault     bool                   `json:"is_default"`
	Fingerprint   string                 `json:"-"`
	ProviderToken string                 `json:"-"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
}

// PaymentIntent represents an intent to collect payment.
type PaymentIntent struct {
	ID                     uuid.UUID              `json:"id"`
	TenantID               uuid.UUID              `json:"tenant_id"`
	OrderID                uuid.UUID              `json:"order_id"`
	PaymentMethodID        *uuid.UUID             `json:"payment_method_id,omitempty"`
	Provider               PaymentProvider        `json:"provider"`
	ProviderIntentID       string                 `json:"provider_intent_id,omitempty"`
	ClientSecret           string                 `json:"client_secret,omitempty"`
	Status                 PaymentIntentStatus    `json:"status"`
	Amount                 float64                `json:"amount"`
	Currency               string                 `json:"currency"`
	Description            string                 `json:"description,omitempty"`
	IdempotencyKey         string                 `json:"idempotency_key,omitempty"`
	MpesaCheckoutRequestID string                 `json:"mpesa_checkout_request_id,omitempty"`
	MpesaPhoneNumber       string                 `json:"mpesa_phone_number,omitempty"`
	RetryCount             int                    `json:"retry_count"`
	LastRetryAt            *time.Time             `json:"last_retry_at,omitempty"`
	ErrorMessage           string                 `json:"error_message,omitempty"`
	ErrorCode              string                 `json:"error_code,omitempty"`
	Metadata               map[string]interface{} `json:"metadata,omitempty"`
	ExpiresAt              *time.Time             `json:"expires_at,omitempty"`
	CreatedAt              time.Time              `json:"created_at"`
	UpdatedAt              time.Time              `json:"updated_at"`
}

// Payment represents a successful payment.
type Payment struct {
	ID                 uuid.UUID              `json:"id"`
	TenantID           uuid.UUID              `json:"tenant_id"`
	PaymentIntentID    uuid.UUID              `json:"payment_intent_id"`
	OrderID            uuid.UUID              `json:"order_id"`
	Amount             float64                `json:"amount"`
	Currency           string                 `json:"currency"`
	Status             PaymentStatus          `json:"status"`
	Provider           PaymentProvider        `json:"provider"`
	ProviderReference  string                 `json:"provider_reference,omitempty"`
	ProviderReceipt    string                 `json:"provider_receipt,omitempty"`
	MpesaTransactionID string                 `json:"mpesa_transaction_id,omitempty"`
	MpesaPhoneNumber   string                 `json:"mpesa_phone_number,omitempty"`
	RefundedAmount     float64                `json:"refunded_amount"`
	ProviderResponse   map[string]interface{} `json:"provider_response,omitempty"`
	Metadata           map[string]interface{} `json:"metadata,omitempty"`
	ProcessedAt        *time.Time             `json:"processed_at,omitempty"`
	CapturedAt         *time.Time             `json:"captured_at,omitempty"`
	CreatedAt          time.Time              `json:"created_at"`
	UpdatedAt          time.Time              `json:"updated_at"`
}

// Refund represents a refund on a payment.
type Refund struct {
	ID                uuid.UUID              `json:"id"`
	TenantID          uuid.UUID              `json:"tenant_id"`
	PaymentID         uuid.UUID              `json:"payment_id"`
	OrderID           uuid.UUID              `json:"order_id"`
	Amount            float64                `json:"amount"`
	Currency          string                 `json:"currency"`
	Status            RefundStatus           `json:"status"`
	Reason            RefundReason           `json:"reason"`
	ReasonNotes       string                 `json:"reason_notes,omitempty"`
	Provider          PaymentProvider        `json:"provider"`
	ProviderRefundID  string                 `json:"provider_refund_id,omitempty"`
	ProviderReference string                 `json:"provider_reference,omitempty"`
	RequestedBy       *uuid.UUID             `json:"requested_by,omitempty"`
	ApprovedBy        *uuid.UUID             `json:"approved_by,omitempty"`
	ErrorMessage      string                 `json:"error_message,omitempty"`
	ErrorCode         string                 `json:"error_code,omitempty"`
	ProviderResponse  map[string]interface{} `json:"provider_response,omitempty"`
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
	RequestedAt       time.Time              `json:"requested_at"`
	ApprovedAt        *time.Time             `json:"approved_at,omitempty"`
	ProcessedAt       *time.Time             `json:"processed_at,omitempty"`
	CreatedAt         time.Time              `json:"created_at"`
	UpdatedAt         time.Time              `json:"updated_at"`
}

// TreasuryEventStatus represents the processing status of a treasury event.
type TreasuryEventStatus string

const (
	EventStatusPending    TreasuryEventStatus = "pending"
	EventStatusProcessing TreasuryEventStatus = "processing"
	EventStatusProcessed  TreasuryEventStatus = "processed"
	EventStatusFailed     TreasuryEventStatus = "failed"
	EventStatusSkipped    TreasuryEventStatus = "skipped"
)

// TreasuryEvent represents a webhook event from treasury.
type TreasuryEvent struct {
	ID              uuid.UUID              `json:"id"`
	TenantID        *uuid.UUID             `json:"tenant_id,omitempty"`
	ExternalID      string                 `json:"external_id"`
	EventType       string                 `json:"event_type"`
	Provider        *PaymentProvider       `json:"provider,omitempty"`
	OrderID         *uuid.UUID             `json:"order_id,omitempty"`
	PaymentID       *uuid.UUID             `json:"payment_id,omitempty"`
	PaymentIntentID *uuid.UUID             `json:"payment_intent_id,omitempty"`
	RefundID        *uuid.UUID             `json:"refund_id,omitempty"`
	Payload         map[string]interface{} `json:"payload"`
	Headers         map[string]string      `json:"headers,omitempty"`
	Signature       string                 `json:"signature,omitempty"`
	SignatureValid  *bool                  `json:"signature_valid,omitempty"`
	Status          TreasuryEventStatus    `json:"status"`
	RetryCount      int                    `json:"retry_count"`
	LastRetryAt     *time.Time             `json:"last_retry_at,omitempty"`
	ErrorMessage    string                 `json:"error_message,omitempty"`
	ErrorCode       string                 `json:"error_code,omitempty"`
	IPAddress       string                 `json:"ip_address,omitempty"`
	ReceivedAt      time.Time              `json:"received_at"`
	ProcessedAt     *time.Time             `json:"processed_at,omitempty"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
}

// CreatePaymentIntentRequest represents a request to create a payment intent.
type CreatePaymentIntentRequest struct {
	TenantID        uuid.UUID       `json:"tenant_id"`
	OrderID         uuid.UUID       `json:"order_id"`
	Amount          float64         `json:"amount"`
	Currency        string          `json:"currency"`
	Provider        PaymentProvider `json:"provider"`
	PaymentMethodID *uuid.UUID      `json:"payment_method_id,omitempty"`
	Description     string          `json:"description,omitempty"`
	IdempotencyKey  string          `json:"idempotency_key,omitempty"`
	CustomerEmail   string          `json:"customer_email,omitempty"`
	CustomerPhone   string          `json:"customer_phone,omitempty"`
}

// InitiateMpesaPaymentRequest represents a request to initiate M-Pesa payment.
type InitiateMpesaPaymentRequest struct {
	TenantID       uuid.UUID `json:"tenant_id"`
	OrderID        uuid.UUID `json:"order_id"`
	Amount         float64   `json:"amount"`
	PhoneNumber    string    `json:"phone_number"`
	Description    string    `json:"description,omitempty"`
	IdempotencyKey string    `json:"idempotency_key,omitempty"`
}

// CreateRefundRequest represents a request to create a refund.
type CreateRefundRequest struct {
	TenantID       uuid.UUID    `json:"tenant_id"`
	PaymentID      uuid.UUID    `json:"payment_id"`
	Amount         float64      `json:"amount"`
	Reason         RefundReason `json:"reason"`
	ReasonNotes    string       `json:"reason_notes,omitempty"`
	RequestedBy    uuid.UUID    `json:"requested_by"`
	IdempotencyKey string       `json:"idempotency_key,omitempty"`
}

// PaymentMethodFilter defines filters for listing payment methods.
type PaymentMethodFilter struct {
	TenantID uuid.UUID
	UserID   uuid.UUID
	Provider *PaymentProvider
	Type     *PaymentMethodType
	Limit    int
	Offset   int
}

// PaymentIntentFilter defines filters for listing payment intents.
type PaymentIntentFilter struct {
	TenantID uuid.UUID
	OrderID  *uuid.UUID
	Status   *PaymentIntentStatus
	Provider *PaymentProvider
	Limit    int
	Offset   int
}

// PaymentFilter defines filters for listing payments.
type PaymentFilter struct {
	TenantID uuid.UUID
	OrderID  *uuid.UUID
	Status   *PaymentStatus
	Provider *PaymentProvider
	DateFrom *time.Time
	DateTo   *time.Time
	Limit    int
	Offset   int
}

// RefundFilter defines filters for listing refunds.
type RefundFilter struct {
	TenantID  uuid.UUID
	PaymentID *uuid.UUID
	OrderID   *uuid.UUID
	Status    *RefundStatus
	Limit     int
	Offset    int
}
