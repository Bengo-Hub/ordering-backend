package treasury

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/config"
)

// Client provides methods to interact with the treasury service.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	logger     *zap.Logger
}

// NewClient creates a new treasury service client.
func NewClient(cfg config.TreasuryConfig, logger *zap.Logger) *Client {
	return &Client{
		baseURL: cfg.ServiceURL,
		apiKey:  cfg.APIKey,
		httpClient: &http.Client{
			Timeout: cfg.RequestTimeout,
		},
		logger: logger,
	}
}

// PaymentProvider represents supported payment providers.
type PaymentProvider string

const (
	ProviderMpesa      PaymentProvider = "mpesa"
	ProviderStripe     PaymentProvider = "stripe"
	ProviderPaystack   PaymentProvider = "paystack"
	ProviderFlutterwave PaymentProvider = "flutterwave"
	ProviderManual     PaymentProvider = "manual"
)

// PaymentIntentRequest represents a request to create a payment intent.
type PaymentIntentRequest struct {
	TenantID       uuid.UUID              `json:"tenant_id"`
	OrderID        uuid.UUID              `json:"order_id"`
	Amount         float64                `json:"amount"`
	Currency       string                 `json:"currency"`
	Provider       PaymentProvider        `json:"provider"`
	Description    string                 `json:"description,omitempty"`
	IdempotencyKey string                 `json:"idempotency_key,omitempty"`
	CustomerEmail  string                 `json:"customer_email,omitempty"`
	CustomerPhone  string                 `json:"customer_phone,omitempty"`
	CallbackURL    string                 `json:"callback_url,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// PaymentIntentResponse represents a payment intent from treasury.
type PaymentIntentResponse struct {
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
	PaymentIntentID    uuid.UUID `json:"payment_intent_id"`
	CheckoutRequestID  string    `json:"checkout_request_id"`
	MerchantRequestID  string    `json:"merchant_request_id"`
	ResponseCode       string    `json:"response_code"`
	ResponseMessage    string    `json:"response_message"`
	CustomerMessage    string    `json:"customer_message"`
	Status             string    `json:"status"`
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
	ID                uuid.UUID `json:"id"`
	PaymentID         uuid.UUID `json:"payment_id"`
	Amount            float64   `json:"amount"`
	Currency          string    `json:"currency"`
	Status            string    `json:"status"`
	Reason            string    `json:"reason"`
	ProviderRefundID  string    `json:"provider_refund_id,omitempty"`
	ProviderReference string    `json:"provider_reference,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
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

// CreatePaymentIntent creates a payment intent with the treasury service.
func (c *Client) CreatePaymentIntent(ctx context.Context, req PaymentIntentRequest) (*PaymentIntentResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/payments/intents", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	c.setHeaders(httpReq, req.IdempotencyKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, c.parseError(resp)
	}

	var result PaymentIntentResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

// InitiateMpesaSTKPush initiates an M-Pesa STK Push payment.
func (c *Client) InitiateMpesaSTKPush(ctx context.Context, req MpesaSTKPushRequest) (*MpesaSTKPushResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/mpesa/stk-push", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	c.setHeaders(httpReq, req.IdempotencyKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, c.parseError(resp)
	}

	var result MpesaSTKPushResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

// GetPaymentIntent retrieves a payment intent by ID.
func (c *Client) GetPaymentIntent(ctx context.Context, tenantID, intentID uuid.UUID) (*PaymentIntentResponse, error) {
	url := fmt.Sprintf("%s/api/v1/payments/intents/%s?tenant_id=%s", c.baseURL, intentID.String(), tenantID.String())

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	c.setHeaders(httpReq, "")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, c.parseError(resp)
	}

	var result PaymentIntentResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

// GetPaymentStatus retrieves the current status of a payment.
func (c *Client) GetPaymentStatus(ctx context.Context, tenantID, paymentIntentID uuid.UUID) (*PaymentStatusResponse, error) {
	url := fmt.Sprintf("%s/api/v1/payments/intents/%s/status?tenant_id=%s", c.baseURL, paymentIntentID.String(), tenantID.String())

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	c.setHeaders(httpReq, "")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, c.parseError(resp)
	}

	var result PaymentStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

// CreateRefund initiates a refund for a payment.
func (c *Client) CreateRefund(ctx context.Context, req RefundRequest) (*RefundResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/refunds", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	c.setHeaders(httpReq, req.IdempotencyKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, c.parseError(resp)
	}

	var result RefundResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

// GetRefund retrieves a refund by ID.
func (c *Client) GetRefund(ctx context.Context, tenantID, refundID uuid.UUID) (*RefundResponse, error) {
	url := fmt.Sprintf("%s/api/v1/refunds/%s?tenant_id=%s", c.baseURL, refundID.String(), tenantID.String())

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	c.setHeaders(httpReq, "")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, c.parseError(resp)
	}

	var result RefundResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

// CancelPaymentIntent cancels a pending payment intent.
func (c *Client) CancelPaymentIntent(ctx context.Context, tenantID, intentID uuid.UUID) error {
	url := fmt.Sprintf("%s/api/v1/payments/intents/%s/cancel", c.baseURL, intentID.String())

	reqBody := map[string]interface{}{"tenant_id": tenantID.String()}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	c.setHeaders(httpReq, "")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return c.parseError(resp)
	}

	return nil
}

// setHeaders sets common headers for requests.
func (c *Client) setHeaders(req *http.Request, idempotencyKey string) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}

	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
}

// parseError parses an error response from the API.
func (c *Client) parseError(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read error body: %w", err)
	}

	var apiErr APIError
	if err := json.Unmarshal(body, &apiErr); err != nil {
		// If we can't parse the error, return a generic error with the body
		return fmt.Errorf("treasury API error (status %d): %s", resp.StatusCode, string(body))
	}

	return &apiErr
}

// HealthCheck checks if the treasury service is healthy.
func (c *Client) HealthCheck(ctx context.Context) error {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("treasury service unhealthy: status %d", resp.StatusCode)
	}

	return nil
}
