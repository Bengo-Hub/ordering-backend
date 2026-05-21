package payments

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/modules/ordering"
	"github.com/bengobox/ordering-backend/internal/platform/treasury"
)

// OrderingRepository is the minimal interface PaymentService needs for polling stale orders.
type OrderingRepository interface {
	GetStalePaymentOrders(ctx context.Context, olderThan time.Time, limit int) ([]ordering.StalePaymentOrder, error)
}

// PaymentService provides payment business logic.
type PaymentService struct {
	repo             Repository
	treasuryClient   *treasury.Client
	logger           *zap.Logger
	orderingRepo     OrderingRepository
	onPaymentSuccess func(ctx context.Context, tenantID, orderID uuid.UUID) error
	onPaymentFailed  func(ctx context.Context, tenantID, orderID uuid.UUID, errMsg string) error
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

// SetOrderingRepo wires the ordering repository for cross-tenant payment polling.
func (s *PaymentService) SetOrderingRepo(repo OrderingRepository) {
	s.orderingRepo = repo
}

// SetPaymentSuccessCallback wires the callback invoked when polling finds a succeeded payment.
func (s *PaymentService) SetPaymentSuccessCallback(fn func(ctx context.Context, tenantID, orderID uuid.UUID) error) {
	s.onPaymentSuccess = fn
}

// SetPaymentFailedCallback wires the callback invoked when polling detects a failed/timed-out payment.
func (s *PaymentService) SetPaymentFailedCallback(fn func(ctx context.Context, tenantID, orderID uuid.UUID, errMsg string) error) {
	s.onPaymentFailed = fn
}

// StartPaymentPolling starts a background goroutine that polls for orders stuck in payment_status=pending.
// Every 2 minutes it checks orders older than 5 minutes. Orders older than 15 minutes are timed out.
func (s *PaymentService) StartPaymentPolling(ctx context.Context) {
	if s.orderingRepo == nil {
		s.logger.Warn("payment poller: ordering repo not set, skipping payment polling")
		return
	}

	s.logger.Info("payment poller: started (2-minute interval)")
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("payment poller: stopping")
			return
		case <-ticker.C:
			s.pollPendingPayments(ctx)
		}
	}
}

func (s *PaymentService) pollPendingPayments(ctx context.Context) {
	cutoff := time.Now().Add(-5 * time.Minute)
	timeoutCutoff := time.Now().Add(-15 * time.Minute)

	orders, err := s.orderingRepo.GetStalePaymentOrders(ctx, cutoff, 50)
	if err != nil {
		s.logger.Error("payment poller: failed to list stale orders", zap.Error(err))
		return
	}

	for _, o := range orders {
		// Orders older than 15 minutes are timed out regardless of treasury state
		if o.PlacedAt != nil && o.PlacedAt.Before(timeoutCutoff) {
			s.logger.Info("payment poller: order payment timed out",
				zap.String("order_id", o.ID.String()),
				zap.String("tenant_id", o.TenantID.String()))
					if s.onPaymentFailed != nil {
				_ = s.onPaymentFailed(ctx, o.TenantID, o.ID, "payment_timeout")
			}
			continue
		}

		if o.PaymentIntentID == nil {
			continue
		}

		status, err := s.treasuryClient.GetPaymentStatus(ctx, o.TenantID, *o.PaymentIntentID)
		if err != nil {
			s.logger.Debug("payment poller: failed to get status from treasury",
				zap.Error(err), zap.String("order_id", o.ID.String()))
			continue
		}

		switch status.Status {
		case "succeeded":
			s.logger.Info("payment poller: payment succeeded, confirming order",
				zap.String("order_id", o.ID.String()))
			if s.onPaymentSuccess != nil {
				if err := s.onPaymentSuccess(ctx, o.TenantID, o.ID); err != nil {
					s.logger.Error("payment poller: onPaymentSuccess callback failed",
						zap.Error(err), zap.String("order_id", o.ID.String()))
				}
			}
		case "failed", "cancelled", "expired":
			s.logger.Info("payment poller: payment failed, notifying",
				zap.String("order_id", o.ID.String()), zap.String("status", status.Status))
			if s.onPaymentFailed != nil {
				errMsg := status.ErrorMessage
				if errMsg == "" {
					errMsg = status.Status
				}
				if err := s.onPaymentFailed(ctx, o.TenantID, o.ID, errMsg); err != nil {
					s.logger.Error("payment poller: onPaymentFailed callback failed",
						zap.Error(err), zap.String("order_id", o.ID.String()))
				}
			}
		}
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

	// Build intent from treasury response (ordering-backend does not store intents; treasury-api is source of truth)
	expiresAt := time.Now().Add(30 * time.Minute)
	if treasuryResp.ExpiresAt != nil {
		expiresAt = *treasuryResp.ExpiresAt
	}
	intent := &PaymentIntent{
		ID:               treasuryResp.ID,
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
		CreatedAt:        treasuryResp.CreatedAt,
		UpdatedAt:        treasuryResp.CreatedAt,
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

	// Build intent from treasury response (ordering-backend does not store intents; treasury-api is source of truth)
	expiresAt := time.Now().Add(5 * time.Minute) // M-Pesa STK Push expires in ~2 minutes
	intent := &PaymentIntent{
		ID:                     treasuryResp.PaymentIntentID,
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
		CreatedAt:              time.Now(),
		UpdatedAt:              time.Now(),
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

	// Build refund from treasury response (ordering-backend does not store refunds; treasury-api is source of truth)
	refund := &Refund{
		ID:                treasuryResp.ID,
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
		CreatedAt:         treasuryResp.CreatedAt,
		UpdatedAt:         treasuryResp.CreatedAt,
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
