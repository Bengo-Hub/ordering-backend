package ordering

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/payref"
	"github.com/bengobox/ordering-backend/internal/platform/treasury"
)

// Payment channels an online order can be paid through (order.metadata.payment_channel). The
// stored payment_method enum stays mpesa/paystack/cod/...; the channel tells integrated M-Pesa
// (STK push, confirmed by the treasury callback) apart from manual M-Pesa, where the customer paid
// the business's own Till/Paybill from their phone and keyed the confirmation code in at checkout.
const (
	PaymentChannelManualMpesa = "mpesa_manual"

	metaPaymentChannel      = "payment_channel"
	metaMpesaCode           = "mpesa_code"
	metaPaymentVerification = "payment_verification"
	metaCODMethod           = "cod_collection_method"
	metaCODReference        = "cod_collection_reference"
	// metaPaidAtPOS marks a pay-on-collection order the counter rang through the POS terminal
	// checkout; its money is in treasury as a POS intent, not ordering's.
	metaPaidAtPOS = "paid_at_pos"
)

var (
	ErrMpesaCodeRequired  = errors.New("enter the M-Pesa confirmation code from your payment message")
	ErrMpesaCodeInvalid   = errors.New("that does not look like an M-Pesa code (10 letters and digits, e.g. SGH7K2L9QP)")
	ErrMpesaCodeUsed      = errors.New("this M-Pesa code has already been used for another order")
	ErrNotManualPayment   = errors.New("order was not paid by manual M-Pesa")
	ErrPaymentAlreadyPaid = errors.New("order is already paid")
)

var mpesaCodePattern = regexp.MustCompile(`^[A-Z0-9]{10}$`)

// NormalizeMpesaCode upper-cases and strips spaces from a code as customers type it.
func NormalizeMpesaCode(code string) string {
	return strings.ToUpper(strings.Join(strings.Fields(code), ""))
}

// resolvePaymentMethod maps the checkout's payment choice onto the stored method, the initial
// payment status and any metadata the channel needs. "mpesa_manual" is stored as method mpesa with
// payment_channel mpesa_manual and the customer's code, awaiting the outlet's verification.
func resolvePaymentMethod(raw, mpesaCode string) (PaymentMethod, PaymentStatus, map[string]interface{}, error) {
	method := strings.ToLower(strings.TrimSpace(raw))
	switch method {
	case "", "mpesa":
		return PaymentMethodMpesa, PaymentStatusPending, nil, nil
	case "cod", "cash":
		return PaymentMethodCOD, "cod_pending", nil, nil
	case PaymentChannelManualMpesa, "manual_mpesa":
		code := NormalizeMpesaCode(mpesaCode)
		if code == "" {
			return "", "", nil, ErrMpesaCodeRequired
		}
		if !mpesaCodePattern.MatchString(code) {
			return "", "", nil, ErrMpesaCodeInvalid
		}
		return PaymentMethodMpesa, PaymentStatusPending, map[string]interface{}{
			metaPaymentChannel:      PaymentChannelManualMpesa,
			metaMpesaCode:           code,
			metaPaymentVerification: "pending",
		}, nil
	default:
		return PaymentMethod(method), PaymentStatusPending, nil, nil
	}
}

// PaymentChannelOf returns the order's payment channel ("mpesa_manual" or "").
func PaymentChannelOf(order *Order) string {
	if order == nil || order.Metadata == nil {
		return ""
	}
	v, _ := order.Metadata[metaPaymentChannel].(string)
	return v
}

// isOfflinePayment reports whether nothing will confirm the order automatically: cash on
// delivery/collection, or manual M-Pesa awaiting the outlet's check. Such orders are accepted as
// soon as they are placed so the kitchen can start (see autoAcceptCOD).
func isOfflinePayment(order *Order) bool {
	return order != nil && (order.PaymentMethod == PaymentMethodCOD || PaymentChannelOf(order) == PaymentChannelManualMpesa)
}

// mergeMetadata copies extra keys into an order's metadata map, creating it when needed.
func mergeMetadata(metadata, extra map[string]interface{}) map[string]interface{} {
	if len(extra) == 0 {
		return metadata
	}
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	for k, v := range extra {
		metadata[k] = v
	}
	return metadata
}

// ensureMpesaCodeUnused rejects a manual M-Pesa code another order of the tenant already used, the
// cheapest guard against a customer re-using one real payment for several orders.
func (s *OrderService) ensureMpesaCodeUnused(ctx context.Context, tenantID uuid.UUID, extra map[string]interface{}) error {
	code, _ := extra[metaMpesaCode].(string)
	if code == "" {
		return nil
	}
	used, err := s.repo.MpesaCodeUsed(ctx, tenantID, code)
	if err != nil {
		s.logger.Warn("mpesa code duplicate check failed; allowing (outlet still verifies)", zap.Error(err))
		return nil
	}
	if used {
		return ErrMpesaCodeUsed
	}
	return nil
}

// VerifyManualPayment records that the outlet matched a manual M-Pesa payment against its M-Pesa
// statement. The order is marked paid (payment confirmed events fire as for any online payment) and
// the treasury intent is settled as mpesa_manual under the verified code.
func (s *OrderService) VerifyManualPayment(ctx context.Context, tenantID, orderID uuid.UUID, reference string, actorID *uuid.UUID) (*Order, error) {
	order, err := s.repo.GetOrder(ctx, tenantID, orderID)
	if err != nil {
		return nil, err
	}
	if order.PaymentStatus == PaymentStatusPaid {
		return order, nil
	}
	if PaymentChannelOf(order) != PaymentChannelManualMpesa {
		return nil, ErrNotManualPayment
	}
	code := NormalizeMpesaCode(reference)
	if code == "" {
		code, _ = order.Metadata[metaMpesaCode].(string)
	}
	if !mpesaCodePattern.MatchString(code) {
		return nil, ErrMpesaCodeInvalid
	}
	patch := map[string]interface{}{
		metaMpesaCode:           code,
		metaPaymentVerification: "verified",
		"payment_verified_at":   time.Now().UTC().Format(time.RFC3339),
	}
	if actorID != nil {
		patch["payment_verified_by"] = actorID.String()
	}
	if err := s.repo.MergeOrderMetadata(ctx, tenantID, orderID, patch); err != nil {
		return nil, err
	}
	updated, err := s.UpdatePaymentStatus(ctx, tenantID, orderID, PaymentStatusPaid, map[string]interface{}{
		"payment_method": PaymentChannelManualMpesa,
		"reference":      code,
	})
	if err != nil {
		return nil, err
	}
	s.settleOfflinePayment(ctx, tenantID, updated, PaymentChannelManualMpesa, code)
	return updated, nil
}

// codSettlement returns the tender recorded when a cash-on-delivery order was collected: cash
// (the default) or M-Pesa paid to the business at the door/counter, with its code.
func codSettlement(order *Order) (method, reference string) {
	method = "cod"
	if order == nil || order.Metadata == nil {
		return method, ""
	}
	if m, _ := order.Metadata[metaCODMethod].(string); strings.Contains(strings.ToLower(m), "mpesa") {
		method = PaymentChannelManualMpesa
	}
	reference, _ = order.Metadata[metaCODReference].(string)
	return method, reference
}

// settleOfflinePayment settles the order's treasury intent for a tenant-collected tender (cash,
// or M-Pesa to the business's own Till/Paybill). Best-effort and asynchronous: treasury has its
// own reconciler for intents left pending.
func (s *OrderService) settleOfflinePayment(ctx context.Context, tenantID uuid.UUID, order *Order, method, reference string) {
	if s.treasuryClient == nil || order == nil {
		return
	}
	go func(oid uuid.UUID, amount float64, currency string) {
		// Treasury finds the intent by the reference it was created with: payref's ORD-... form
		// (what checkout sends), or the bare order id for intents created before payref.
		var err error
		for _, ref := range []string{payref.Build("ORD", "", tenantID, oid), oid.String()} {
			if _, err = s.treasuryClient.SettleCODPayment(context.WithoutCancel(ctx), treasury.SettleCODPaymentRequest{
				TenantID:      tenantID,
				OrderID:       ref,
				AmountPaid:    amount,
				Currency:      currency,
				PaymentMethod: method,
				Reference:     reference,
			}); err == nil {
				return
			}
		}
		s.logger.Error("failed to settle tenant-collected payment in treasury",
			zap.String("order_id", oid.String()), zap.String("method", method), zap.Error(err))
	}(order.ID, order.GrandTotal, order.Currency)
}

// RecordCODCollection stamps how a cash-on-delivery order was actually paid at the door or counter
// (cash, or M-Pesa with its code) so the settlement books the right tender.
func (s *OrderService) RecordCODCollection(ctx context.Context, tenantID, orderID uuid.UUID, method, reference string) {
	method = strings.ToLower(strings.TrimSpace(method))
	if method == "" {
		return
	}
	patch := map[string]interface{}{metaCODMethod: method}
	if ref := NormalizeMpesaCode(reference); ref != "" {
		patch[metaCODReference] = ref
	}
	if err := s.repo.MergeOrderMetadata(ctx, tenantID, orderID, patch); err != nil {
		s.logger.Warn("failed to record COD collection method", zap.String("order_id", orderID.String()), zap.Error(err))
	}
}

// stringMeta reads a string metadata value ("" when absent).
func stringMeta(metadata map[string]interface{}, key string) string {
	if metadata == nil {
		return ""
	}
	v, _ := metadata[key].(string)
	return v
}

// releaseReservationOnError frees a stock hold taken for a checkout that then failed validation.
func (s *OrderService) releaseReservationOnError(ctx context.Context, tenantID uuid.UUID, reservationID *uuid.UUID) {
	if reservationID == nil {
		return
	}
	go s.releaseOrderReservation(context.WithoutCancel(ctx), &Order{TenantID: tenantID, ReservationID: reservationID}, "order_creation_failed")
}
