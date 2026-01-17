package payments

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/platform/treasury"
)

// PaymentService provides payment business logic.
type PaymentService struct {
	repo           Repository
	treasuryClient *treasury.Client
	logger         *zap.Logger
}

// NewPaymentService creates a new payment service.
func NewPaymentService(
	repo Repository,
	treasuryClient *treasury.Client,
	logger *zap.Logger,
) *PaymentService {
	return &PaymentService{
		repo:           repo,
		treasuryClient: treasuryClient,
		logger:         logger,
	}
}

// CreatePaymentIntent creates a new payment intent.
func (s *PaymentService) CreatePaymentIntent(ctx context.Context, req CreatePaymentIntentRequest) (*PaymentIntent, error) {
	// Check for existing intent with same idempotency key
	if req.IdempotencyKey != "" {
		existing, err := s.repo.GetPaymentIntentByIdempotencyKey(ctx, req.TenantID, req.IdempotencyKey)
		if err == nil && existing != nil {
			return existing, nil
		}
	}

	// Validate amount
	if req.Amount <= 0 {
		return nil, ErrInvalidAmount
	}

	// Create intent in treasury service
	treasuryReq := treasury.PaymentIntentRequest{
		TenantID:       req.TenantID,
		OrderID:        req.OrderID,
		Amount:         req.Amount,
		Currency:       req.Currency,
		Provider:       treasury.PaymentProvider(req.Provider),
		Description:    req.Description,
		IdempotencyKey: req.IdempotencyKey,
		CustomerEmail:  req.CustomerEmail,
		CustomerPhone:  req.CustomerPhone,
	}

	treasuryResp, err := s.treasuryClient.CreatePaymentIntent(ctx, treasuryReq)
	if err != nil {
		s.logger.Error("failed to create payment intent in treasury",
			zap.Error(err),
			zap.String("order_id", req.OrderID.String()))
		return nil, fmt.Errorf("create payment intent: %w", err)
	}

	// Create local intent record
	expiresAt := time.Now().Add(30 * time.Minute)
	intent := &PaymentIntent{
		TenantID:         req.TenantID,
		OrderID:          req.OrderID,
		PaymentMethodID:  req.PaymentMethodID,
		Provider:         req.Provider,
		ProviderIntentID: treasuryResp.ProviderIntentID,
		ClientSecret:     treasuryResp.ClientSecret,
		Status:           PaymentIntentStatus(treasuryResp.Status),
		Amount:           req.Amount,
		Currency:         req.Currency,
		Description:      req.Description,
		IdempotencyKey:   req.IdempotencyKey,
		ExpiresAt:        &expiresAt,
	}

	if err := s.repo.CreatePaymentIntent(ctx, intent); err != nil {
		return nil, err
	}

	s.logger.Info("payment intent created",
		zap.String("id", intent.ID.String()),
		zap.String("order_id", req.OrderID.String()),
		zap.Float64("amount", req.Amount))

	return intent, nil
}

// InitiateMpesaPayment initiates an M-Pesa STK Push payment.
func (s *PaymentService) InitiateMpesaPayment(ctx context.Context, req InitiateMpesaPaymentRequest) (*PaymentIntent, error) {
	// Check for existing intent with same idempotency key
	if req.IdempotencyKey != "" {
		existing, err := s.repo.GetPaymentIntentByIdempotencyKey(ctx, req.TenantID, req.IdempotencyKey)
		if err == nil && existing != nil {
			return existing, nil
		}
	}

	// Validate amount
	if req.Amount <= 0 {
		return nil, ErrInvalidAmount
	}

	// Validate phone number
	if req.PhoneNumber == "" {
		return nil, ErrInvalidPhoneNumber
	}

	// Initiate STK Push via treasury
	treasuryReq := treasury.MpesaSTKPushRequest{
		TenantID:       req.TenantID,
		OrderID:        req.OrderID,
		Amount:         req.Amount,
		PhoneNumber:    req.PhoneNumber,
		Description:    req.Description,
		IdempotencyKey: req.IdempotencyKey,
	}

	treasuryResp, err := s.treasuryClient.InitiateMpesaSTKPush(ctx, treasuryReq)
	if err != nil {
		s.logger.Error("failed to initiate M-Pesa STK Push",
			zap.Error(err),
			zap.String("order_id", req.OrderID.String()),
			zap.String("phone", req.PhoneNumber))
		return nil, fmt.Errorf("initiate M-Pesa payment: %w", err)
	}

	// Create local intent record
	expiresAt := time.Now().Add(5 * time.Minute) // M-Pesa STK Push expires in ~2 minutes
	intent := &PaymentIntent{
		TenantID:               req.TenantID,
		OrderID:                req.OrderID,
		Provider:               ProviderMpesa,
		Status:                 IntentStatusRequiresAction,
		Amount:                 req.Amount,
		Currency:               "KES",
		Description:            req.Description,
		IdempotencyKey:         req.IdempotencyKey,
		MpesaCheckoutRequestID: treasuryResp.CheckoutRequestID,
		MpesaPhoneNumber:       req.PhoneNumber,
		ExpiresAt:              &expiresAt,
	}

	if err := s.repo.CreatePaymentIntent(ctx, intent); err != nil {
		return nil, err
	}

	s.logger.Info("M-Pesa STK Push initiated",
		zap.String("id", intent.ID.String()),
		zap.String("order_id", req.OrderID.String()),
		zap.String("checkout_request_id", treasuryResp.CheckoutRequestID))

	return intent, nil
}

// GetPaymentIntent retrieves a payment intent.
func (s *PaymentService) GetPaymentIntent(ctx context.Context, tenantID, id uuid.UUID) (*PaymentIntent, error) {
	return s.repo.GetPaymentIntent(ctx, tenantID, id)
}

// ListPaymentIntents lists payment intents with filters.
func (s *PaymentService) ListPaymentIntents(ctx context.Context, filter PaymentIntentFilter) ([]PaymentIntent, int, error) {
	return s.repo.ListPaymentIntents(ctx, filter)
}

// CancelPaymentIntent cancels a pending payment intent.
func (s *PaymentService) CancelPaymentIntent(ctx context.Context, tenantID, id uuid.UUID) error {
	intent, err := s.repo.GetPaymentIntent(ctx, tenantID, id)
	if err != nil {
		return err
	}

	if intent.Status != IntentStatusPending && intent.Status != IntentStatusRequiresAction {
		return ErrPaymentIntentNotPending
	}

	// Cancel in treasury
	if err := s.treasuryClient.CancelPaymentIntent(ctx, tenantID, id); err != nil {
		s.logger.Warn("failed to cancel payment intent in treasury",
			zap.Error(err),
			zap.String("id", id.String()))
	}

	// Update local status
	intent.Status = IntentStatusCancelled
	return s.repo.UpdatePaymentIntent(ctx, intent)
}

// GetPayment retrieves a payment.
func (s *PaymentService) GetPayment(ctx context.Context, tenantID, id uuid.UUID) (*Payment, error) {
	return s.repo.GetPayment(ctx, tenantID, id)
}

// GetPaymentByOrderID retrieves a successful payment for an order.
func (s *PaymentService) GetPaymentByOrderID(ctx context.Context, tenantID, orderID uuid.UUID) (*Payment, error) {
	return s.repo.GetPaymentByOrderID(ctx, tenantID, orderID)
}

// ListPayments lists payments with filters.
func (s *PaymentService) ListPayments(ctx context.Context, filter PaymentFilter) ([]Payment, int, error) {
	return s.repo.ListPayments(ctx, filter)
}

// CreateRefund creates a new refund request.
func (s *PaymentService) CreateRefund(ctx context.Context, req CreateRefundRequest) (*Refund, error) {
	// Get the payment
	payment, err := s.repo.GetPayment(ctx, req.TenantID, req.PaymentID)
	if err != nil {
		return nil, err
	}

	// Validate payment status
	if payment.Status != PaymentStatusSucceeded && payment.Status != PaymentStatusPartiallyRefunded {
		return nil, ErrPaymentNotSucceeded
	}

	// Validate refund amount
	availableAmount := payment.Amount - payment.RefundedAmount
	if req.Amount <= 0 || req.Amount > availableAmount {
		return nil, ErrRefundAmountExceedsPayment
	}

	// Create refund in treasury
	treasuryReq := treasury.RefundRequest{
		TenantID:       req.TenantID,
		PaymentID:      req.PaymentID,
		Amount:         req.Amount,
		Reason:         string(req.Reason),
		IdempotencyKey: req.IdempotencyKey,
	}

	treasuryResp, err := s.treasuryClient.CreateRefund(ctx, treasuryReq)
	if err != nil {
		s.logger.Error("failed to create refund in treasury",
			zap.Error(err),
			zap.String("payment_id", req.PaymentID.String()))
		return nil, fmt.Errorf("create refund: %w", err)
	}

	// Create local refund record
	refund := &Refund{
		TenantID:          req.TenantID,
		PaymentID:         req.PaymentID,
		OrderID:           payment.OrderID,
		Amount:            req.Amount,
		Currency:          payment.Currency,
		Status:            RefundStatus(treasuryResp.Status),
		Reason:            req.Reason,
		ReasonNotes:       req.ReasonNotes,
		Provider:          payment.Provider,
		ProviderRefundID:  treasuryResp.ProviderRefundID,
		ProviderReference: treasuryResp.ProviderReference,
		RequestedBy:       &req.RequestedBy,
		RequestedAt:       time.Now(),
	}

	if err := s.repo.CreateRefund(ctx, refund); err != nil {
		return nil, err
	}

	s.logger.Info("refund created",
		zap.String("id", refund.ID.String()),
		zap.String("payment_id", req.PaymentID.String()),
		zap.Float64("amount", req.Amount))

	return refund, nil
}

// GetRefund retrieves a refund.
func (s *PaymentService) GetRefund(ctx context.Context, tenantID, id uuid.UUID) (*Refund, error) {
	return s.repo.GetRefund(ctx, tenantID, id)
}

// ListRefunds lists refunds with filters.
func (s *PaymentService) ListRefunds(ctx context.Context, filter RefundFilter) ([]Refund, int, error) {
	return s.repo.ListRefunds(ctx, filter)
}

// CheckPaymentStatus checks the current status of a payment with treasury.
func (s *PaymentService) CheckPaymentStatus(ctx context.Context, tenantID, paymentIntentID uuid.UUID) (*PaymentIntent, error) {
	intent, err := s.repo.GetPaymentIntent(ctx, tenantID, paymentIntentID)
	if err != nil {
		return nil, err
	}

	// Query treasury for current status
	status, err := s.treasuryClient.GetPaymentStatus(ctx, tenantID, paymentIntentID)
	if err != nil {
		s.logger.Warn("failed to check payment status in treasury",
			zap.Error(err),
			zap.String("id", paymentIntentID.String()))
		return intent, nil
	}

	// Update local status if changed
	newStatus := PaymentIntentStatus(status.Status)
	if intent.Status != newStatus {
		intent.Status = newStatus
		if status.ErrorMessage != "" {
			intent.ErrorMessage = status.ErrorMessage
		}
		if status.ErrorCode != "" {
			intent.ErrorCode = status.ErrorCode
		}
		if err := s.repo.UpdatePaymentIntent(ctx, intent); err != nil {
			s.logger.Error("failed to update payment intent status",
				zap.Error(err),
				zap.String("id", paymentIntentID.String()))
		}
	}

	return intent, nil
}
