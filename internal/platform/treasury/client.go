package treasury

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	serviceclient "github.com/Bengo-Hub/shared-service-client"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/config"
)

// money decodes a monetary amount from treasury-api, which serializes decimals as a
// quoted JSON string (e.g. "638.00") — and tolerates a plain JSON number. Underlying float64.
type money float64

func (m *money) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	if s == "" || s == "null" {
		*m = 0
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("treasury: parse amount %q: %w", s, err)
	}
	*m = money(v)
	return nil
}

// Client provides methods to interact with the treasury service.
type Client struct {
	baseURL       string
	publicBaseURL string // browser-accessible URL for initiate_url construction
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

	publicURL := cfg.PublicURL
	if publicURL == "" {
		publicURL = cfg.ServiceURL
	}

	return &Client{
		baseURL:       cfg.ServiceURL,
		publicBaseURL: publicURL,
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
	// CrmContactId references the MarketFlow CRM contact (single source of truth). Set by ordering
	// after upserting the buyer at checkout; treasury stores only this reference, never PII.
	CrmContactId   *uuid.UUID             `json:"crm_contact_id,omitempty"`
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
	Amount                 money           `json:"amount"`
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
	Amount            money      `json:"amount"`
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
	Amount            money      `json:"amount"`
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

// BaseURL returns the internal treasury service base URL (for S2S API calls).
func (c *Client) BaseURL() string {
	return c.baseURL
}

// PublicBaseURL returns the browser-accessible treasury API URL used when
// building initiate_url values that are returned to the frontend.
func (c *Client) PublicBaseURL() string {
	return c.publicBaseURL
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

// GetPaymentIntent retrieves a payment intent by ID via the S2S route (X-API-Key auth).
func (c *Client) GetPaymentIntent(ctx context.Context, tenantID, intentID uuid.UUID) (*PaymentIntentResponse, error) {
	path := fmt.Sprintf("/api/v1/s2s/%s/payments/intents/%s", tenantID.String(), intentID.String())

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

// GetPaymentStatus retrieves the current status of a payment intent via the S2S route. Treasury has
// no dedicated /status sub-route; it returns the full intent (whose Status field is what the poller
// reads), served by GET /api/v1/s2s/{tenant}/payments/intents/{id} with X-API-Key auth. The old
// /api/v1/payments/intents/{id}/status path 404'd, which (logged only at Debug) silently broke the
// payment poller — orders paid outside the webhook path were never confirmed.
func (c *Client) GetPaymentStatus(ctx context.Context, tenantID, paymentIntentID uuid.UUID) (*PaymentStatusResponse, error) {
	path := fmt.Sprintf("/api/v1/s2s/%s/payments/intents/%s", tenantID.String(), paymentIntentID.String())

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

// EnabledGateway is a payment gateway type key the tenant has active, as returned by
// treasury-api's PublicListActiveGateways endpoint. Treasury returns a flat list of type
// keys (e.g. "paystack", "mpesa", "wallet", "cod") under {"gateways":[...]} — there are no
// per-gateway display fields, so the caller maps each key to its presentation metadata.
type EnabledGateway string

// enabledGatewaysResponse mirrors treasury's PublicListActiveGateways body: {"gateways":[...]}.
type enabledGatewaysResponse struct {
	Gateways []EnabledGateway `json:"gateways"`
}

// ListEnabledGateways returns the payment gateway type keys active for a tenant via the
// PUBLIC treasury endpoint GET {baseURL}/api/v1/pay/{tenantID}/gateways (no auth required).
// The endpoint is served by the same treasury-api service, so it is reachable over the
// internal serviceClient just like the S2S routes.
func (c *Client) ListEnabledGateways(ctx context.Context, tenantID uuid.UUID) ([]EnabledGateway, error) {
	path := fmt.Sprintf("/api/v1/pay/%s/gateways", tenantID.String())
	resp, err := c.serviceClient.Get(ctx, path, c.headers(""))
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, c.parseError(resp)
	}
	var result enabledGatewaysResponse
	if err := resp.DecodeJSON(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return result.Gateways, nil
}

// InitiateIntentPaymentResponse mirrors the body returned by treasury-api's public
// PublicInitiateIntent endpoint. The endpoint is provider-polymorphic, so this struct is
// permissive: M-Pesa returns checkout_request_id + customer_message + status ("processing"),
// Paystack returns authorization_url, and any provider may return a redirect_url. Unknown
// fields are ignored; absent fields decode to their zero value.
type InitiateIntentPaymentResponse struct {
	Status            string `json:"status,omitempty"`
	Success           bool   `json:"success,omitempty"`
	Message           string `json:"message,omitempty"`
	CustomerMessage   string `json:"customer_message,omitempty"`
	CheckoutRequestID string `json:"checkout_request_id,omitempty"`
	AuthorizationURL  string `json:"authorization_url,omitempty"`
	RedirectURL       string `json:"redirect_url,omitempty"`
	IntentID          string `json:"intent_id,omitempty"`
}

// InitiateIntentPayment triggers payment on an existing intent via treasury-api's PUBLIC endpoint
// POST /api/v1/pay/{tenant}/intents/{intentID}/initiate, posting {payment_method, phone_number}.
// This is the same primitive the treasury-ui pay page POSTs to; it fires the provider action
// (e.g. an M-Pesa STK push) for the intent. {tenant} accepts the tenant UUID, matching the other
// /api/v1/pay/{tenant}/... call (ListEnabledGateways). The route is public, but we still send the
// same headers as the other treasury calls (X-API-Key when configured) — treasury ignores it on
// public routes, so this is safe whether or not the route requires auth.
func (c *Client) InitiateIntentPayment(ctx context.Context, tenant uuid.UUID, intentID, paymentMethod, phoneNumber string) (*InitiateIntentPaymentResponse, error) {
	path := fmt.Sprintf("/api/v1/pay/%s/intents/%s/initiate", tenant.String(), intentID)
	reqBody := map[string]any{
		"payment_method": paymentMethod,
		"phone_number":   phoneNumber,
	}
	resp, err := c.serviceClient.Post(ctx, path, reqBody, c.headers(""))
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, c.parseError(resp)
	}
	var result InitiateIntentPaymentResponse
	if err := resp.DecodeJSON(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &result, nil
}

// AvailableGateway describes a payment gateway type the tenant could enable, as returned by
// treasury-api's S2S GET /api/v1/s2s/{tenant}/gateways/available endpoint.
type AvailableGateway struct {
	GatewayType        string `json:"gateway_type"`
	Name               string `json:"name"`
	TransactionFeeType string `json:"transaction_fee_type"`
	SupportsStkPush    bool   `json:"supports_stk_push"`
}

// SelectedGateway describes a payment gateway the tenant has selected/configured, as returned
// by treasury-api's S2S GET /api/v1/s2s/{tenant}/gateways/selected endpoint. URL fields are kept
// as a free-form map so this client need not track every per-provider url field treasury adds.
type SelectedGateway struct {
	ID                 string    `json:"id"`
	GatewayType        string    `json:"gateway_type"`
	Name               string    `json:"name"`
	IsActive           bool      `json:"is_active"`
	IsPrimary          bool      `json:"is_primary"`
	Status             string    `json:"status"`
	TransactionFeeType string    `json:"transaction_fee_type"`
	TotalTransactions  int64     `json:"total_transactions"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	// URLs captures any provider url fields treasury returns (e.g. callback/webhook/return urls)
	// that are not modelled explicitly above, so the proxy can pass them through unchanged.
	URLs map[string]any `json:"-"`
}

// knownSelectedGatewayFields are the JSON keys modelled by SelectedGateway's explicit struct
// fields; any other key returned by treasury is preserved in SelectedGateway.URLs.
var knownSelectedGatewayFields = map[string]struct{}{
	"id": {}, "gateway_type": {}, "name": {}, "is_active": {}, "is_primary": {},
	"status": {}, "transaction_fee_type": {}, "total_transactions": {},
	"created_at": {}, "updated_at": {},
}

// UnmarshalJSON decodes the explicit fields and stashes any remaining (url) fields into URLs so
// the proxy can forward provider-specific url fields without this client tracking each of them.
func (g *SelectedGateway) UnmarshalJSON(b []byte) error {
	type alias SelectedGateway
	if err := json.Unmarshal(b, (*alias)(g)); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	for k, v := range raw {
		if _, known := knownSelectedGatewayFields[k]; known {
			continue
		}
		var val any
		if err := json.Unmarshal(v, &val); err != nil {
			continue
		}
		if g.URLs == nil {
			g.URLs = map[string]any{}
		}
		g.URLs[k] = val
	}
	return nil
}

// MarshalJSON re-emits the explicit fields together with any preserved url fields so the proxy
// response to ordering-frontend mirrors treasury's selected-gateway shape.
func (g SelectedGateway) MarshalJSON() ([]byte, error) {
	type alias SelectedGateway
	base, err := json.Marshal(alias(g))
	if err != nil {
		return nil, err
	}
	if len(g.URLs) == 0 {
		return base, nil
	}
	var merged map[string]json.RawMessage
	if err := json.Unmarshal(base, &merged); err != nil {
		return nil, err
	}
	for k, v := range g.URLs {
		vb, err := json.Marshal(v)
		if err != nil {
			continue
		}
		merged[k] = vb
	}
	return json.Marshal(merged)
}

// availableGatewaysResponse mirrors treasury's body: {"gateways":[...]}.
type availableGatewaysResponse struct {
	Gateways []AvailableGateway `json:"gateways"`
}

// selectedGatewaysResponse mirrors treasury's body: {"selected":[...]}.
type selectedGatewaysResponse struct {
	Selected []SelectedGateway `json:"selected"`
}

// ListAvailableGateways returns the gateway types a tenant could enable, via the S2S route
// GET /api/v1/s2s/{tenant}/gateways/available (X-API-Key auth). {tenant} accepts UUID or slug;
// this passes the tenant UUID, matching ListEnabledGateways.
func (c *Client) ListAvailableGateways(ctx context.Context, tenant uuid.UUID) ([]AvailableGateway, error) {
	path := fmt.Sprintf("/api/v1/s2s/%s/gateways/available", tenant.String())
	resp, err := c.serviceClient.Get(ctx, path, c.headers(""))
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, c.parseError(resp)
	}
	var result availableGatewaysResponse
	if err := resp.DecodeJSON(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return result.Gateways, nil
}

// ListSelectedGateways returns the gateways the tenant has selected/configured, via the S2S route
// GET /api/v1/s2s/{tenant}/gateways/selected (X-API-Key auth).
func (c *Client) ListSelectedGateways(ctx context.Context, tenant uuid.UUID) ([]SelectedGateway, error) {
	path := fmt.Sprintf("/api/v1/s2s/%s/gateways/selected", tenant.String())
	resp, err := c.serviceClient.Get(ctx, path, c.headers(""))
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	if !resp.IsSuccess() {
		return nil, c.parseError(resp)
	}
	var result selectedGatewaysResponse
	if err := resp.DecodeJSON(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return result.Selected, nil
}

// SelectGateway selects (enables) a gateway type for the tenant via the S2S route
// POST /api/v1/s2s/{tenant}/gateways/select/{gatewayType} with body {"is_primary": bool}.
func (c *Client) SelectGateway(ctx context.Context, tenant uuid.UUID, gatewayType string, isPrimary bool) error {
	path := fmt.Sprintf("/api/v1/s2s/%s/gateways/select/%s", tenant.String(), gatewayType)
	reqBody := map[string]any{"is_primary": isPrimary}
	resp, err := c.serviceClient.Post(ctx, path, reqBody, c.headers(""))
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	if !resp.IsSuccess() {
		return c.parseError(resp)
	}
	return nil
}

// DeactivateGateway deselects (disables) a gateway type for the tenant via the S2S route
// DELETE /api/v1/s2s/{tenant}/gateways/select/{gatewayType}.
func (c *Client) DeactivateGateway(ctx context.Context, tenant uuid.UUID, gatewayType string) error {
	path := fmt.Sprintf("/api/v1/s2s/%s/gateways/select/%s", tenant.String(), gatewayType)
	resp, err := c.serviceClient.Delete(ctx, path, c.headers(""))
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	if !resp.IsSuccess() {
		return c.parseError(resp)
	}
	return nil
}

// WalletBalanceResponse holds the balance result from treasury S2S.
// Balance uses the money type because treasury serializes decimal amounts as a
// quoted JSON string (e.g. "638.00"); money also tolerates a plain JSON number.
type WalletBalanceResponse struct {
	Balance  money  `json:"balance"`
	Currency string `json:"currency"`
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

// SettleCODPaymentRequest is sent to treasury when COD is collected at delivery.
type SettleCODPaymentRequest struct {
	TenantID    uuid.UUID `json:"tenant_id"`
	OrderID     string    `json:"order_id"`
	AmountPaid  float64   `json:"amount_paid"`
	Currency    string    `json:"currency"`
}

// SettleCODPaymentResponse is returned after COD settlement.
type SettleCODPaymentResponse struct {
	IntentID string `json:"intent_id"`
	Status   string `json:"status"`
}

// SettleCODPayment marks the COD payment intent for an order as succeeded
// via the treasury S2S endpoint. This is called when delivery is confirmed
// (rider marks order delivered with cash collected).
func (c *Client) SettleCODPayment(ctx context.Context, req SettleCODPaymentRequest) (*SettleCODPaymentResponse, error) {
	path := fmt.Sprintf("/api/v1/s2s/%s/payments/cod/settle", req.TenantID.String())
	resp, err := c.serviceClient.Post(ctx, path, req, c.headers(fmt.Sprintf("cod-settle-%s", req.OrderID)))
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}

	if !resp.IsSuccess() {
		return nil, c.parseError(resp)
	}

	var result SettleCODPaymentResponse
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
