package treasury

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	serviceclient "github.com/Bengo-Hub/shared-service-client"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/config"
)

// Client provides methods to interact with the treasury service.
type Client struct {
	baseURL       string
	apiKey        string
	serviceClient *serviceclient.Client
	logger        *zap.Logger
}

// NewClient creates a new treasury service client.
func NewClient(cfg config.TreasuryConfig, logger *zap.Logger) *Client {
	scCfg := serviceclient.DefaultConfig(
		cfg.ServiceURL,
		"ordering-service",
		logger.Named("treasury.client"),
	)
	scCfg.Timeout = cfg.RequestTimeout

	return &Client{
		baseURL:       cfg.ServiceURL,
		apiKey:        cfg.APIKey,
		serviceClient: serviceclient.New(scCfg),
		logger:        logger.Named("treasury.client"),
	}
}

// PaymentProvider represents supported payment providers.
type PaymentProvider string

const (
	ProviderMpesa       PaymentProvider = "mpesa"
	ProviderStripe      PaymentProvider = "stripe"
	ProviderPaystack    PaymentProvider = "paystack"
	ProviderFlutterwave PaymentProvider = "flutterwave"
	ProviderManual      PaymentProvider = "manual"
)

// PaymentIntentRequest represents a request to create a payment intent.
// Aligned with treasury-api's CreateIntentRequest contract.
type PaymentIntentRequest struct {
	TenantID       uuid.UUID              `json:"tenant_id"`
	ReferenceID    string                 `json:"reference_id"`    // order ID or external reference
	ReferenceType  string                 `json:"reference_type"`  // "order", "subscription", "invoice"
	OrderID        uuid.UUID              `json:"order_id"`        // backward compat
	Amount         float64                `json:"amount"`
	Currency       string                 `json:"currency"`
	Provider       PaymentProvider        `json:"provider"`
	PaymentMethod  string                 `json:"payment_method"`  // "pending", "paystack", "mpesa", "cash"
	SourceService  string                 `json:"source_service"`  // "ordering" — identifies origin for equity tracking
	Description    string                 `json:"description,omitempty"`
	IdempotencyKey string                 `json:"idempotency_key,omitempty"`
	CustomerEmail  string                 `json:"customer_email,omitempty"`
	CustomerPhone  string                 `json:"customer_phone,omitempty"`
	CallbackURL    string                 `json:"callback_url,omitempty"`
	// Service charge fields — from subscriptions-api service charge plan
	ServiceChargePercentage *float64 `json:"service_charge_percentage,omitempty"`
	ServiceChargeAmount     *string  `json:"service_charge_amount,omitempty"`
	ServiceChargePlanCode   string   `json:"service_charge_plan_code,omitempty"`
	Metadata                map[string]interface{} `json:"metadata,omitempty"`
}

// PaymentIntentResponse represents a payment intent from treasury.
type PaymentIntentResponse struct {
	// IntentID is the primary field from the s2s create endpoint (json:"intent_id").
	IntentID               uuid.UUID       `json:"intent_id"`
	// ID is kept for other treasury endpoints that return json:"id".
	ID                     uuid.UUID       `json:"id"`
	TenantID               uuid.UUID       `json:"tenant_id"`
	OrderID                uuid.UUID       `json:"order_id"`
	Provider               PaymentProvider `json:"provider"`
	ProviderIntentID       string          `json:"provider_intent_id,omitempty"`
	ClientSecret           string          `json:"client_secret,omitempty"`
	Status                 string          `json:"status"`
	Amount                 float64         `json:"amount"`
	Currency               string          `json:"currency"`
	MpesaCheckoutRequestID string          `json:"mpesa_checkout_request_id,omitempty"`
	CreatedAt              time.Time       `json:"created_at"`
	ExpiresAt              *time.Time      `json:"expires_at,omitempty"`
}

// ResolvedID returns IntentID if non-nil, falling back to ID.
// The s2s create endpoint returns "intent_id"; other endpoints return "id".
func (r *PaymentIntentResponse) ResolvedID() uuid.UUID {
	if r.IntentID != uuid.Nil {
		return r.IntentID
	}
	return r.ID
}

// MpesaSTKPushRequest represents an M-Pesa STK Push request.
type MpesaSTKPushRequest struct {
	TenantID       uuid.UUID `json:"tenant_id"`
	OrderID        uuid.UUID `json:"order_id"`
	Amount         float64   `json:"amount"`
	PhoneNumber    string    `json:"phone_number"`
	Description    string    `json:"description,omitempty"`
	IdempotencyKey string    `json:"idempotency_key,omitempty"`
	CallbackURL    string    `json:"callback_url,omitempty"`
}

// MpesaSTKPushResponse represents the response from an STK Push initiation.
type MpesaSTKPushResponse struct {
	PaymentIntentID   uuid.UUID `json:"payment_intent_id"`
	CheckoutRequestID string    `json:"checkout_request_id"`
	MerchantRequestID string    `json:"merchant_request_id"`
	ResponseCode      string    `json:"response_code"`
	ResponseMessage   string    `json:"response_message"`
	CustomerMessage   string    `json:"customer_message"`
	Status            string    `json:"status"`
}

// RefundRequest represents a refund request.
type RefundRequest struct {
	TenantID       uuid.UUID `json:"tenant_id"`
	PaymentID      uuid.UUID `json:"payment_id"`
	Amount         float64   `json:"amount"`
	Reason         string    `json:"reason"`
	IdempotencyKey string    `json:"idempotency_key,omitempty"`
}

// RefundResponse represents a refund response.
type RefundResponse struct {
	ID                uuid.UUID  `json:"id"`
	PaymentID         uuid.UUID  `json:"payment_id"`
	Amount            float64    `json:"amount"`
	Currency          string     `json:"currency"`
	Status            string     `json:"status"`
	Reason            string     `json:"reason"`
	ProviderRefundID  string     `json:"provider_refund_id,omitempty"`
	ProviderReference string     `json:"provider_reference,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	ProcessedAt       *time.Time `json:"processed_at,omitempty"`
}

// PaymentStatusResponse represents a payment status response.
type PaymentStatusResponse struct {
	PaymentIntentID   uuid.UUID  `json:"payment_intent_id"`
	Status            string     `json:"status"`
	ProviderReference string     `json:"provider_reference,omitempty"`
	ProviderReceipt   string     `json:"provider_receipt,omitempty"`
	Amount            float64    `json:"amount"`
	Currency          string     `json:"currency"`
	ProcessedAt       *time.Time `json:"processed_at,omitempty"`
	ErrorMessage      string     `json:"error_message,omitempty"`
	ErrorCode         string     `json:"error_code,omitempty"`
}

// APIError represents an error response from the treasury API.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("treasury API error: %s - %s", e.Code, e.Message)
}

// headers returns common headers for requests.
func (c *Client) headers(idempotencyKey string) map[string]string {
	h := map[string]string{
		"Content-Type": "application/json",
		"Accept":       "application/json",
	}

	if c.apiKey != "" {
		h["X-API-Key"] = c.apiKey
	}

	if idempotencyKey != "" {
		h["Idempotency-Key"] = idempotencyKey
	}

	return h
}

// parseError parses an error response from the API.
func (c *Client) parseError(resp *serviceclient.Response) error {
	var apiErr APIError
	if err := resp.DecodeJSON(&apiErr); err != nil {
		return fmt.Errorf("treasury API error (status %d)", resp.StatusCode)
	}
	return &apiErr
}

// BaseURL returns the treasury service base URL (for building initiate URLs).
func (c *Client) BaseURL() string {
	return c.baseURL
}

// CreatePaymentIntent creates a payment intent with the treasury service.
// Uses the /api/v1/s2s/ route which accepts a pre-shared INTERNAL_SERVICE_KEY
// (X-API-Key header) without requiring an auth-api-registered JWT or API key.
func (c *Client) CreatePaymentIntent(ctx context.Context, req PaymentIntentRequest) (*PaymentIntentResponse, error) {
	path := fmt.Sprintf("/api/v1/s2s/%s/payments/intents", req.TenantID.String())
	resp, err := c.serviceClient.Post(ctx, path, req, c.headers(req.IdempotencyKey))
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}

	if !resp.IsSuccess() {
		return nil, c.parseError(resp)
	}

	var result PaymentIntentResponse
	if err := resp.DecodeJSON(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

// InitiateMpesaSTKPush initiates an M-Pesa STK Push payment.
func (c *Client) InitiateMpesaSTKPush(ctx context.Context, req MpesaSTKPushRequest) (*MpesaSTKPushResponse, error) {
	resp, err := c.serviceClient.Post(ctx, "/api/v1/mpesa/stk-push", req, c.headers(req.IdempotencyKey))
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}

	if !resp.IsSuccess() {
		return nil, c.parseError(resp)
	}

	var result MpesaSTKPushResponse
	if err := resp.DecodeJSON(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

// GetPaymentIntent retrieves a payment intent by ID.
func (c *Client) GetPaymentIntent(ctx context.Context, tenantID, intentID uuid.UUID) (*PaymentIntentResponse, error) {
	path := fmt.Sprintf("/api/v1/payments/intents/%s?tenant_id=%s", intentID.String(), tenantID.String())

	resp, err := c.serviceClient.Get(ctx, path, c.headers(""))
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}

	if !resp.IsSuccess() {
		return nil, c.parseError(resp)
	}

	var result PaymentIntentResponse
	if err := resp.DecodeJSON(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

// GetPaymentStatus retrieves the current status of a payment.
func (c *Client) GetPaymentStatus(ctx context.Context, tenantID, paymentIntentID uuid.UUID) (*PaymentStatusResponse, error) {
	path := fmt.Sprintf("/api/v1/payments/intents/%s/status?tenant_id=%s", paymentIntentID.String(), tenantID.String())

	resp, err := c.serviceClient.Get(ctx, path, c.headers(""))
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}

	if !resp.IsSuccess() {
		return nil, c.parseError(resp)
	}

	var result PaymentStatusResponse
	if err := resp.DecodeJSON(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

// CreateRefund initiates a refund for a payment.
func (c *Client) CreateRefund(ctx context.Context, req RefundRequest) (*RefundResponse, error) {
	resp, err := c.serviceClient.Post(ctx, "/api/v1/refunds", req, c.headers(req.IdempotencyKey))
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}

	if !resp.IsSuccess() {
		return nil, c.parseError(resp)
	}

	var result RefundResponse
	if err := resp.DecodeJSON(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

// GetRefund retrieves a refund by ID.
func (c *Client) GetRefund(ctx context.Context, tenantID, refundID uuid.UUID) (*RefundResponse, error) {
	path := fmt.Sprintf("/api/v1/refunds/%s?tenant_id=%s", refundID.String(), tenantID.String())

	resp, err := c.serviceClient.Get(ctx, path, c.headers(""))
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}

	if !resp.IsSuccess() {
		return nil, c.parseError(resp)
	}

	var result RefundResponse
	if err := resp.DecodeJSON(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

// CancelPaymentIntent cancels a pending payment intent.
func (c *Client) CancelPaymentIntent(ctx context.Context, tenantID, intentID uuid.UUID) error {
	path := fmt.Sprintf("/api/v1/payments/intents/%s/cancel", intentID.String())
	reqBody := map[string]interface{}{"tenant_id": tenantID.String()}

	resp, err := c.serviceClient.Post(ctx, path, reqBody, c.headers(""))
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}

	if !resp.IsSuccess() {
		return c.parseError(resp)
	}

	return nil
}

// WalletBalanceResponse holds the balance result from treasury S2S.
type WalletBalanceResponse struct {
	Balance  float64 `json:"balance"`
	Currency string  `json:"currency"`
}

// WalletDebitResponse holds the result of a wallet debit.
type WalletDebitResponse struct {
	TransactionID string  `json:"transaction_id"`
	BalanceBefore float64 `json:"balance_before"`
	BalanceAfter  float64 `json:"balance_after"`
	AmountDebited float64 `json:"amount_debited"`
	Reference     string  `json:"reference"`
}

// GetUserWalletBalance fetches a user's wallet balance via the S2S route.
func (c *Client) GetUserWalletBalance(ctx context.Context, tenantID, userID uuid.UUID) (*WalletBalanceResponse, error) {
	path := fmt.Sprintf("/api/v1/s2s/%s/wallets/%s/balance", tenantID, userID)
	resp, err := c.serviceClient.Get(ctx, path, c.headers(""))
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, c.parseError(resp)
	}
	var result WalletBalanceResponse
	if err := resp.DecodeJSON(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &result, nil
}

// DebitUserWallet debits a user's wallet via the S2S route.
func (c *Client) DebitUserWallet(ctx context.Context, tenantID, userID uuid.UUID, amount float64, reference, description string) (*WalletDebitResponse, error) {
	path := fmt.Sprintf("/api/v1/s2s/%s/wallets/%s/debit", tenantID, userID)
	reqBody := map[string]any{
		"amount":      amount,
		"reference":   reference,
		"description": description,
	}
	resp, err := c.serviceClient.Post(ctx, path, reqBody, c.headers(""))
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, c.parseError(resp)
	}
	var result WalletDebitResponse
	if err := resp.DecodeJSON(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &result, nil
}

// HealthCheck checks if the treasury service is healthy.
func (c *Client) HealthCheck(ctx context.Context) error {
	resp, err := c.serviceClient.Get(ctx, "/health", nil)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}

	if !resp.IsSuccess() {
		return fmt.Errorf("treasury service unhealthy: status %d", resp.StatusCode)
	}

	return nil
}
