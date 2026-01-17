package payments

import (
	"context"

	"github.com/google/uuid"
)

// Repository defines the interface for payments data access.
type Repository interface {
	// Payment Methods
	CreatePaymentMethod(ctx context.Context, method *PaymentMethod) error
	GetPaymentMethod(ctx context.Context, tenantID, id uuid.UUID) (*PaymentMethod, error)
	GetPaymentMethodByFingerprint(ctx context.Context, tenantID uuid.UUID, fingerprint string) (*PaymentMethod, error)
	ListPaymentMethods(ctx context.Context, filter PaymentMethodFilter) ([]PaymentMethod, int, error)
	UpdatePaymentMethod(ctx context.Context, method *PaymentMethod) error
	DeletePaymentMethod(ctx context.Context, tenantID, id uuid.UUID) error
	SetDefaultPaymentMethod(ctx context.Context, tenantID, userID, id uuid.UUID) error

	// Payment Intents
	CreatePaymentIntent(ctx context.Context, intent *PaymentIntent) error
	GetPaymentIntent(ctx context.Context, tenantID, id uuid.UUID) (*PaymentIntent, error)
	GetPaymentIntentByIdempotencyKey(ctx context.Context, tenantID uuid.UUID, key string) (*PaymentIntent, error)
	GetPaymentIntentByMpesaCheckoutRequestID(ctx context.Context, checkoutRequestID string) (*PaymentIntent, error)
	ListPaymentIntents(ctx context.Context, filter PaymentIntentFilter) ([]PaymentIntent, int, error)
	UpdatePaymentIntent(ctx context.Context, intent *PaymentIntent) error

	// Payments
	CreatePayment(ctx context.Context, payment *Payment) error
	GetPayment(ctx context.Context, tenantID, id uuid.UUID) (*Payment, error)
	GetPaymentByProviderReference(ctx context.Context, provider PaymentProvider, reference string) (*Payment, error)
	GetPaymentByOrderID(ctx context.Context, tenantID, orderID uuid.UUID) (*Payment, error)
	ListPayments(ctx context.Context, filter PaymentFilter) ([]Payment, int, error)
	UpdatePayment(ctx context.Context, payment *Payment) error

	// Refunds
	CreateRefund(ctx context.Context, refund *Refund) error
	GetRefund(ctx context.Context, tenantID, id uuid.UUID) (*Refund, error)
	ListRefunds(ctx context.Context, filter RefundFilter) ([]Refund, int, error)
	UpdateRefund(ctx context.Context, refund *Refund) error

	// Treasury Events
	CreateTreasuryEvent(ctx context.Context, event *TreasuryEvent) error
	GetTreasuryEvent(ctx context.Context, id uuid.UUID) (*TreasuryEvent, error)
	GetTreasuryEventByExternalID(ctx context.Context, externalID string) (*TreasuryEvent, error)
	UpdateTreasuryEvent(ctx context.Context, event *TreasuryEvent) error
	ListPendingTreasuryEvents(ctx context.Context, limit int) ([]TreasuryEvent, error)
}
