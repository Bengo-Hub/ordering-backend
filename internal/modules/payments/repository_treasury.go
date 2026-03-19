package payments

import (
	"context"

	"github.com/google/uuid"

	"github.com/bengobox/ordering-backend/internal/platform/treasury"
)

// TreasuryRepository implements Repository using treasury-api as source of truth.
// Ordering-backend does not store payment intents, payments, payment methods, or refunds;
// it only keeps payment_intent_id on Order. All reads/writes for payment data go through treasury client or are no-op.
type TreasuryRepository struct {
	treasury *treasury.Client
}

// NewTreasuryRepository returns a repository that uses treasury-api for intents and stubs the rest.
func NewTreasuryRepository(treasuryClient *treasury.Client) *TreasuryRepository {
	return &TreasuryRepository{treasury: treasuryClient}
}

// ---- Payment Methods (owned by treasury-api; ordering does not store) ----

func (r *TreasuryRepository) CreatePaymentMethod(ctx context.Context, method *PaymentMethod) error {
	return ErrPaymentMethodsNotStored
}
func (r *TreasuryRepository) GetPaymentMethod(ctx context.Context, tenantID, id uuid.UUID) (*PaymentMethod, error) {
	return nil, ErrPaymentMethodNotFound
}
func (r *TreasuryRepository) GetPaymentMethodByFingerprint(ctx context.Context, tenantID uuid.UUID, fingerprint string) (*PaymentMethod, error) {
	return nil, ErrPaymentMethodNotFound
}
func (r *TreasuryRepository) ListPaymentMethods(ctx context.Context, filter PaymentMethodFilter) ([]PaymentMethod, int, error) {
	return nil, 0, nil
}
func (r *TreasuryRepository) UpdatePaymentMethod(ctx context.Context, method *PaymentMethod) error {
	return ErrPaymentMethodsNotStored
}
func (r *TreasuryRepository) DeletePaymentMethod(ctx context.Context, tenantID, id uuid.UUID) error {
	return ErrPaymentMethodsNotStored
}
func (r *TreasuryRepository) SetDefaultPaymentMethod(ctx context.Context, tenantID, userID, id uuid.UUID) error {
	return ErrPaymentMethodsNotStored
}

// ---- Payment Intents (read from treasury; writes are no-op after service assigns ID from treasury response) ----

func (r *TreasuryRepository) CreatePaymentIntent(ctx context.Context, intent *PaymentIntent) error {
	return nil
}
func (r *TreasuryRepository) GetPaymentIntent(ctx context.Context, tenantID, id uuid.UUID) (*PaymentIntent, error) {
	resp, err := r.treasury.GetPaymentIntent(ctx, tenantID, id)
	if err != nil {
		if treasuryErr, ok := err.(*treasury.APIError); ok && treasuryErr.Code == "not_found" {
			return nil, ErrPaymentIntentNotFound
		}
		return nil, err
	}
	return treasuryIntentToPaymentIntent(resp), nil
}
func (r *TreasuryRepository) GetPaymentIntentByIdempotencyKey(ctx context.Context, tenantID uuid.UUID, key string) (*PaymentIntent, error) {
	return nil, nil
}
func (r *TreasuryRepository) GetPaymentIntentByMpesaCheckoutRequestID(ctx context.Context, checkoutRequestID string) (*PaymentIntent, error) {
	return nil, ErrPaymentIntentNotFound
}
func (r *TreasuryRepository) ListPaymentIntents(ctx context.Context, filter PaymentIntentFilter) ([]PaymentIntent, int, error) {
	return nil, 0, nil
}
func (r *TreasuryRepository) UpdatePaymentIntent(ctx context.Context, intent *PaymentIntent) error {
	return nil
}

// ---- Payments (owned by treasury-api; ordering does not store) ----

func (r *TreasuryRepository) CreatePayment(ctx context.Context, payment *Payment) error {
	return nil
}
func (r *TreasuryRepository) GetPayment(ctx context.Context, tenantID, id uuid.UUID) (*Payment, error) {
	return nil, ErrPaymentNotFound
}
func (r *TreasuryRepository) GetPaymentByProviderReference(ctx context.Context, provider PaymentProvider, reference string) (*Payment, error) {
	return nil, ErrPaymentNotFound
}
func (r *TreasuryRepository) GetPaymentByOrderID(ctx context.Context, tenantID, orderID uuid.UUID) (*Payment, error) {
	return nil, nil
}
func (r *TreasuryRepository) ListPayments(ctx context.Context, filter PaymentFilter) ([]Payment, int, error) {
	return nil, 0, nil
}
func (r *TreasuryRepository) UpdatePayment(ctx context.Context, payment *Payment) error {
	return nil
}

// ---- Refunds (owned by treasury-api; ordering does not store) ----

func (r *TreasuryRepository) CreateRefund(ctx context.Context, refund *Refund) error {
	return nil
}
func (r *TreasuryRepository) GetRefund(ctx context.Context, tenantID, id uuid.UUID) (*Refund, error) {
	return nil, ErrRefundNotFound
}
func (r *TreasuryRepository) ListRefunds(ctx context.Context, filter RefundFilter) ([]Refund, int, error) {
	return nil, 0, nil
}
func (r *TreasuryRepository) UpdateRefund(ctx context.Context, refund *Refund) error {
	return nil
}

// ---- Treasury events (webhook idempotency: we do not store; rely on order status update idempotency) ----

func (r *TreasuryRepository) CreateTreasuryEvent(ctx context.Context, event *TreasuryEvent) error {
	return nil
}
func (r *TreasuryRepository) GetTreasuryEvent(ctx context.Context, id uuid.UUID) (*TreasuryEvent, error) {
	return nil, ErrTreasuryEventNotFound
}
func (r *TreasuryRepository) GetTreasuryEventByExternalID(ctx context.Context, externalID string) (*TreasuryEvent, error) {
	return nil, nil
}
func (r *TreasuryRepository) UpdateTreasuryEvent(ctx context.Context, event *TreasuryEvent) error {
	return nil
}
func (r *TreasuryRepository) ListPendingTreasuryEvents(ctx context.Context, limit int) ([]TreasuryEvent, error) {
	return nil, nil
}

func treasuryIntentToPaymentIntent(resp *treasury.PaymentIntentResponse) *PaymentIntent {
	intent := &PaymentIntent{
		ID:               resp.ID,
		TenantID:         resp.TenantID,
		OrderID:          resp.OrderID,
		Provider:         PaymentProvider(resp.Provider),
		ProviderIntentID: resp.ProviderIntentID,
		ClientSecret:     resp.ClientSecret,
		Status:           PaymentIntentStatus(resp.Status),
		Amount:           resp.Amount,
		Currency:         resp.Currency,
		CreatedAt:        resp.CreatedAt,
		UpdatedAt:        resp.CreatedAt,
	}
	if resp.ExpiresAt != nil {
		intent.ExpiresAt = resp.ExpiresAt
	}
	return intent
}
