package payments

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/platform/treasury"
)

// WebhookService processes treasury webhook events.
type WebhookService struct {
	repo     Repository
	verifier *treasury.WebhookVerifier
	logger   *zap.Logger

	// Callback for order status updates
	onPaymentSuccess func(ctx context.Context, tenantID, orderID uuid.UUID) error
	onPaymentFailed  func(ctx context.Context, tenantID, orderID uuid.UUID, errorMsg string) error
}

// NewWebhookService creates a new webhook service.
func NewWebhookService(
	repo Repository,
	webhookSecret string,
	logger *zap.Logger,
) *WebhookService {
	return &WebhookService{
		repo:     repo,
		verifier: treasury.NewWebhookVerifier(webhookSecret),
		logger:   logger,
	}
}

// SetPaymentSuccessCallback sets the callback for successful payments.
func (s *WebhookService) SetPaymentSuccessCallback(fn func(ctx context.Context, tenantID, orderID uuid.UUID) error) {
	s.onPaymentSuccess = fn
}

// SetPaymentFailedCallback sets the callback for failed payments.
func (s *WebhookService) SetPaymentFailedCallback(fn func(ctx context.Context, tenantID, orderID uuid.UUID, errorMsg string) error) {
	s.onPaymentFailed = fn
}

// ProcessWebhookRequest represents a webhook request to process.
type ProcessWebhookRequest struct {
	Payload   []byte
	Signature string
	Headers   map[string]string
	IPAddress string
}

// ProcessWebhook processes an incoming treasury webhook.
func (s *WebhookService) ProcessWebhook(ctx context.Context, req ProcessWebhookRequest) error {
	// Parse the event
	var webhookEvent treasury.WebhookEvent
	if err := json.Unmarshal(req.Payload, &webhookEvent); err != nil {
		return fmt.Errorf("parse webhook payload: %w", err)
	}

	// Check for duplicate event
	existing, err := s.repo.GetTreasuryEventByExternalID(ctx, webhookEvent.ID)
	if err == nil && existing != nil {
		if existing.Status == EventStatusProcessed {
			s.logger.Debug("skipping duplicate webhook event",
				zap.String("external_id", webhookEvent.ID))
			return nil
		}
	}

	// Verify signature
	signatureValid := true
	if req.Signature != "" {
		if err := s.verifier.Verify(req.Payload, req.Signature, 5*time.Minute); err != nil {
			s.logger.Warn("invalid webhook signature",
				zap.Error(err),
				zap.String("external_id", webhookEvent.ID))
			signatureValid = false
		}
	}

	// Create event record
	event := &TreasuryEvent{
		ExternalID:     webhookEvent.ID,
		EventType:      webhookEvent.Type,
		Payload:        webhookEvent.Data,
		Headers:        req.Headers,
		Signature:      req.Signature,
		SignatureValid: &signatureValid,
		Status:         EventStatusPending,
		IPAddress:      req.IPAddress,
		ReceivedAt:     time.Now(),
	}

	if err := s.repo.CreateTreasuryEvent(ctx, event); err != nil {
		if err == ErrDuplicateTreasuryEvent {
			// Already processing this event
			return nil
		}
		return err
	}

	// If signature is invalid, don't process
	if !signatureValid {
		event.Status = EventStatusFailed
		event.ErrorMessage = "invalid signature"
		_ = s.repo.UpdateTreasuryEvent(ctx, event)
		return ErrInvalidWebhookSignature
	}

	// Process the event
	if err := s.processEvent(ctx, event, &webhookEvent); err != nil {
		event.Status = EventStatusFailed
		event.ErrorMessage = err.Error()
		event.RetryCount++
		now := time.Now()
		event.LastRetryAt = &now
		_ = s.repo.UpdateTreasuryEvent(ctx, event)
		return err
	}

	// Mark as processed
	now := time.Now()
	event.Status = EventStatusProcessed
	event.ProcessedAt = &now
	return s.repo.UpdateTreasuryEvent(ctx, event)
}

// processEvent handles the event based on its type.
func (s *WebhookService) processEvent(ctx context.Context, event *TreasuryEvent, webhookEvent *treasury.WebhookEvent) error {
	switch {
	case treasury.IsPaymentEvent(webhookEvent.Type):
		return s.processPaymentEvent(ctx, event, webhookEvent)
	case treasury.IsRefundEvent(webhookEvent.Type):
		return s.processRefundEvent(ctx, event, webhookEvent)
	default:
		s.logger.Debug("skipping unhandled event type",
			zap.String("type", webhookEvent.Type))
		event.Status = EventStatusSkipped
		return nil
	}
}

// processPaymentEvent handles payment-related webhook events.
func (s *WebhookService) processPaymentEvent(ctx context.Context, event *TreasuryEvent, webhookEvent *treasury.WebhookEvent) error {
	paymentData, err := treasury.ParsePaymentEvent(webhookEvent)
	if err != nil {
		return fmt.Errorf("parse payment event: %w", err)
	}

	// Update event with references
	event.TenantID = &paymentData.TenantID
	event.OrderID = &paymentData.OrderID
	event.PaymentIntentID = &paymentData.PaymentIntentID
	provider := PaymentProvider(paymentData.Provider)
	event.Provider = &provider

	// Get or create payment intent
	intent, err := s.repo.GetPaymentIntent(ctx, paymentData.TenantID, paymentData.PaymentIntentID)
	if err != nil && err != ErrPaymentIntentNotFound {
		return err
	}

	if intent != nil {
		// Update intent status
		intent.Status = PaymentIntentStatus(paymentData.Status)
		if paymentData.ErrorMessage != "" {
			intent.ErrorMessage = paymentData.ErrorMessage
		}
		if paymentData.ErrorCode != "" {
			intent.ErrorCode = paymentData.ErrorCode
		}
		if err := s.repo.UpdatePaymentIntent(ctx, intent); err != nil {
			s.logger.Error("failed to update payment intent",
				zap.Error(err),
				zap.String("intent_id", intent.ID.String()))
		}
	}

	// Handle successful payments
	if treasury.IsSuccessEvent(webhookEvent.Type) {
		return s.handlePaymentSuccess(ctx, event, paymentData, intent)
	}

	// Handle failed payments
	if treasury.IsFailureEvent(webhookEvent.Type) {
		return s.handlePaymentFailure(ctx, event, paymentData, intent)
	}

	return nil
}

// handlePaymentSuccess handles successful payment events.
func (s *WebhookService) handlePaymentSuccess(
	ctx context.Context,
	event *TreasuryEvent,
	paymentData *treasury.WebhookPaymentData,
	intent *PaymentIntent,
) error {
	// Check if payment already exists
	existingPayment, err := s.repo.GetPaymentByOrderID(ctx, paymentData.TenantID, paymentData.OrderID)
	if err == nil && existingPayment != nil {
		s.logger.Debug("payment already exists for order",
			zap.String("order_id", paymentData.OrderID.String()))
		return nil
	}

	// Create payment record
	now := time.Now()
	payment := &Payment{
		TenantID:           paymentData.TenantID,
		PaymentIntentID:    paymentData.PaymentIntentID,
		OrderID:            paymentData.OrderID,
		Amount:             paymentData.Amount,
		Currency:           paymentData.Currency,
		Status:             PaymentStatusSucceeded,
		Provider:           PaymentProvider(paymentData.Provider),
		ProviderReference:  paymentData.ProviderReference,
		ProviderReceipt:    paymentData.ProviderReceipt,
		MpesaTransactionID: paymentData.MpesaTransactionID,
		MpesaPhoneNumber:   paymentData.MpesaPhoneNumber,
		ProcessedAt:        &now,
		CapturedAt:         &now,
	}

	if err := s.repo.CreatePayment(ctx, payment); err != nil {
		return fmt.Errorf("create payment: %w", err)
	}

	event.PaymentID = &payment.ID

	s.logger.Info("payment succeeded",
		zap.String("payment_id", payment.ID.String()),
		zap.String("order_id", payment.OrderID.String()),
		zap.Float64("amount", payment.Amount))

	// Notify order service
	if s.onPaymentSuccess != nil {
		if err := s.onPaymentSuccess(ctx, paymentData.TenantID, paymentData.OrderID); err != nil {
			s.logger.Error("failed to notify order service of payment success",
				zap.Error(err),
				zap.String("order_id", paymentData.OrderID.String()))
		}
	}

	return nil
}

// handlePaymentFailure handles failed payment events.
func (s *WebhookService) handlePaymentFailure(
	ctx context.Context,
	event *TreasuryEvent,
	paymentData *treasury.WebhookPaymentData,
	intent *PaymentIntent,
) error {
	s.logger.Warn("payment failed",
		zap.String("order_id", paymentData.OrderID.String()),
		zap.String("error_code", paymentData.ErrorCode),
		zap.String("error_message", paymentData.ErrorMessage))

	// Notify order service
	if s.onPaymentFailed != nil {
		if err := s.onPaymentFailed(ctx, paymentData.TenantID, paymentData.OrderID, paymentData.ErrorMessage); err != nil {
			s.logger.Error("failed to notify order service of payment failure",
				zap.Error(err),
				zap.String("order_id", paymentData.OrderID.String()))
		}
	}

	return nil
}

// processRefundEvent handles refund-related webhook events.
func (s *WebhookService) processRefundEvent(ctx context.Context, event *TreasuryEvent, webhookEvent *treasury.WebhookEvent) error {
	refundData, err := treasury.ParseRefundEvent(webhookEvent)
	if err != nil {
		return fmt.Errorf("parse refund event: %w", err)
	}

	// Update event with references
	event.TenantID = &refundData.TenantID
	event.OrderID = &refundData.OrderID
	event.RefundID = &refundData.RefundID
	event.PaymentID = &refundData.PaymentID

	// Get refund
	refund, err := s.repo.GetRefund(ctx, refundData.TenantID, refundData.RefundID)
	if err != nil {
		return fmt.Errorf("get refund: %w", err)
	}

	// Update refund status
	refund.Status = RefundStatus(refundData.Status)
	if refundData.ProviderRefundID != "" {
		refund.ProviderRefundID = refundData.ProviderRefundID
	}
	if refundData.ProviderReference != "" {
		refund.ProviderReference = refundData.ProviderReference
	}
	if refundData.ErrorCode != "" {
		refund.ErrorCode = refundData.ErrorCode
	}
	if refundData.ErrorMessage != "" {
		refund.ErrorMessage = refundData.ErrorMessage
	}
	if refundData.ProcessedAt != nil {
		refund.ProcessedAt = refundData.ProcessedAt
	}

	if err := s.repo.UpdateRefund(ctx, refund); err != nil {
		return fmt.Errorf("update refund: %w", err)
	}

	// If refund succeeded, update payment
	if treasury.IsSuccessEvent(webhookEvent.Type) {
		payment, err := s.repo.GetPayment(ctx, refundData.TenantID, refundData.PaymentID)
		if err == nil {
			payment.RefundedAmount += refundData.Amount
			if payment.RefundedAmount >= payment.Amount {
				payment.Status = PaymentStatusRefunded
			} else {
				payment.Status = PaymentStatusPartiallyRefunded
			}
			if err := s.repo.UpdatePayment(ctx, payment); err != nil {
				s.logger.Error("failed to update payment refunded amount",
					zap.Error(err),
					zap.String("payment_id", payment.ID.String()))
			}
		}
	}

	s.logger.Info("refund status updated",
		zap.String("refund_id", refund.ID.String()),
		zap.String("status", string(refund.Status)))

	return nil
}

// RetryPendingEvents retries processing of pending/failed events.
func (s *WebhookService) RetryPendingEvents(ctx context.Context, limit int) error {
	events, err := s.repo.ListPendingTreasuryEvents(ctx, limit)
	if err != nil {
		return err
	}

	for _, event := range events {
		var webhookEvent treasury.WebhookEvent
		webhookEvent.ID = event.ExternalID
		webhookEvent.Type = event.EventType
		webhookEvent.Data = event.Payload

		if err := s.processEvent(ctx, &event, &webhookEvent); err != nil {
			s.logger.Warn("failed to retry event",
				zap.Error(err),
				zap.String("external_id", event.ExternalID))
			continue
		}

		now := time.Now()
		event.Status = EventStatusProcessed
		event.ProcessedAt = &now
		if err := s.repo.UpdateTreasuryEvent(ctx, &event); err != nil {
			s.logger.Error("failed to update event status",
				zap.Error(err),
				zap.String("external_id", event.ExternalID))
		}
	}

	return nil
}
