package ordering

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Payment retry window for online-payment orders.
//
// When a customer's STK prompt, PayHero or Paystack checkout is declined, cancelled or times out,
// the order stays pending (stock still held) until the retry window closes. During the window the
// customer can retry from the order page, choosing a gateway again. Every failed attempt is
// recorded in the order's metadata. Only when the window closes without a successful payment does
// the payment poller cancel the order, which releases the stock hold and tells the customer.
//
// The deadline is stamped on the order at checkout from the tenant's service config, so the poller
// (all tenants) never reads config per order. Orders placed before the stamp existed fall back to
// placed_at plus DefaultPaymentRetryWindow.
const (
	// ConfigKeyPaymentRetryWindow is the tenant service-config key, in minutes.
	ConfigKeyPaymentRetryWindow = "orders.payment_retry_window_minutes"
	// DefaultPaymentRetryWindow applies when the tenant has not set the key.
	DefaultPaymentRetryWindow = 30 * time.Minute
	minPaymentRetryWindow     = 5 * time.Minute
	maxPaymentRetryWindow     = 24 * time.Hour

	// MaxPaymentRetries caps the fresh payment intents a customer can start for one order.
	MaxPaymentRetries = 5
	// PaymentRetryMinInterval is the shortest gap between two retries of one order.
	PaymentRetryMinInterval = 20 * time.Second

	metaPaymentRetryUntil       = "payment_retry_until"
	metaPaymentAttempts         = "payment_attempts"
	metaPaymentLastFailure      = "payment_last_failure_reason"
	metaPaymentLastAttemptAt    = "payment_last_attempt_at"
	metaPaymentLastFailedIntent = "payment_last_failed_intent"
	metaPaymentIntentIDs        = "payment_intent_ids"
	metaPaymentRetries          = "payment_retries"
	metaPaymentRetryLastAt      = "payment_retry_last_at"
	// metaPaymentIntentID is the intent a refund is drawn on (read by refundCancelledPrepaidOrder).
	metaPaymentIntentID = "payment_intent_id"
	// metaPaymentPaidIntentID is the intent whose success paid the order.
	metaPaymentPaidIntentID = "payment_paid_intent_id"
	// metaPaymentExtraPaidIntents lists intents that succeeded after the order was already paid:
	// a customer who completed two attempts was charged twice and needs one refunded.
	metaPaymentExtraPaidIntents = "payment_extra_paid_intents"
)

// Errors returned by RetryOrderPayment.
var (
	ErrPaymentRetryClosed      = errors.New("the time to retry this payment has passed")
	ErrPaymentNotRetryable     = errors.New("this order is not waiting for an online payment")
	ErrPaymentAlreadyCompleted = errors.New("this order is already paid")
	ErrPaymentInFlight         = errors.New("a payment for this order is still being processed; wait a minute and check again")
	ErrPaymentRetryTooSoon     = errors.New("please wait a few seconds before trying again")
	ErrPaymentRetryLimit       = errors.New("too many payment attempts for this order")
)

// PaymentRetryInfo is what the order page and the staff dashboard show for an online-payment order
// that is not paid yet.
type PaymentRetryInfo struct {
	Open              bool       `json:"open"`
	Until             time.Time  `json:"until"`
	Attempts          int        `json:"attempts"`
	Retries           int        `json:"retries"`
	LastFailureReason string     `json:"lastFailureReason,omitempty"`
	LastAttemptAt     *time.Time `json:"lastAttemptAt,omitempty"`
}

// ParsePaymentRetryWindow reads the config value (minutes). Missing, unparsable or non-positive
// values give the default; others are clamped to 5 minutes .. 24 hours.
func ParsePaymentRetryWindow(raw string, ok bool) time.Duration {
	if !ok {
		return DefaultPaymentRetryWindow
	}
	n, err := strconv.Atoi(strings.Trim(strings.TrimSpace(raw), `"`))
	if err != nil || n <= 0 {
		return DefaultPaymentRetryWindow
	}
	d := time.Duration(n) * time.Minute
	if d < minPaymentRetryWindow {
		return minPaymentRetryWindow
	}
	if d > maxPaymentRetryWindow {
		return maxPaymentRetryWindow
	}
	return d
}

// paymentRetryWindow is the tenant's configured window.
func (s *OrderService) paymentRetryWindow(ctx context.Context, tenantID uuid.UUID) time.Duration {
	v, ok := s.repo.GetConfigValue(ctx, tenantID, ConfigKeyPaymentRetryWindow)
	return ParsePaymentRetryWindow(v, ok)
}

// PaymentRetryDeadline is when an unpaid online-payment order stops accepting a retry: the stamped
// payment_retry_until, else placed_at (or created_at) plus the default window.
func PaymentRetryDeadline(placedAt *time.Time, createdAt time.Time, meta map[string]interface{}) time.Time {
	if v := stringMeta(meta, metaPaymentRetryUntil); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			return t
		}
	}
	base := createdAt
	if placedAt != nil && !placedAt.IsZero() {
		base = *placedAt
	}
	return base.Add(DefaultPaymentRetryWindow)
}

// awaitsOnlinePayment reports whether the order is an online-payment order still waiting for its
// money: pending, payment pending or failed, not cash/manual M-Pesa, and something to pay.
func awaitsOnlinePayment(o *Order) bool {
	if o == nil || o.Status != OrderStatusPending || isOfflinePayment(o) || o.GrandTotal <= 0 {
		return false
	}
	return o.PaymentStatus == PaymentStatusPending || o.PaymentStatus == PaymentStatusFailed
}

// PaymentRetryInfoFor returns the retry state of an order, or nil when it is not waiting for an
// online payment.
func PaymentRetryInfoFor(o *Order, now time.Time) *PaymentRetryInfo {
	if !awaitsOnlinePayment(o) {
		return nil
	}
	until := PaymentRetryDeadline(o.PlacedAt, o.CreatedAt, o.Metadata)
	info := &PaymentRetryInfo{
		Open:              now.Before(until),
		Until:             until,
		Attempts:          intMeta(o.Metadata, metaPaymentAttempts),
		Retries:           intMeta(o.Metadata, metaPaymentRetries),
		LastFailureReason: stringMeta(o.Metadata, metaPaymentLastFailure),
	}
	if v := stringMeta(o.Metadata, metaPaymentLastAttemptAt); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			info.LastAttemptAt = &t
		}
	}
	return info
}

// PaymentFailurePatch records one failed payment attempt. A failed intent stays failed and the
// poller sees it on every pass, so an intent already recorded as the last failure is not counted
// again (nil patch).
func PaymentFailurePatch(meta map[string]interface{}, intentID, reason string, at time.Time) map[string]interface{} {
	if intentID != "" && stringMeta(meta, metaPaymentLastFailedIntent) == intentID {
		return nil
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "payment not completed"
	}
	patch := map[string]interface{}{
		metaPaymentAttempts:      intMeta(meta, metaPaymentAttempts) + 1,
		metaPaymentLastFailure:   reason,
		metaPaymentLastAttemptAt: at.UTC().Format(time.RFC3339),
	}
	if intentID != "" {
		patch[metaPaymentLastFailedIntent] = intentID
	}
	return patch
}

// paymentRetryClaim decides whether a retry may start now and returns the metadata that claims
// it (retry counter and time). It runs inside a compare-and-set write, so two concurrent retry
// requests for one order cannot both pass.
func paymentRetryClaim(cur OrderSnapshot, deadline, now time.Time) (map[string]interface{}, error) {
	if cur.PaymentStatus == PaymentStatusPaid {
		return nil, ErrPaymentAlreadyCompleted
	}
	if cur.Status != OrderStatusPending || (cur.PaymentStatus != PaymentStatusPending && cur.PaymentStatus != PaymentStatusFailed) {
		return nil, ErrPaymentNotRetryable
	}
	if !now.Before(deadline) {
		return nil, ErrPaymentRetryClosed
	}
	retries := intMeta(cur.Metadata, metaPaymentRetries)
	if retries >= MaxPaymentRetries {
		return nil, ErrPaymentRetryLimit
	}
	if v := stringMeta(cur.Metadata, metaPaymentRetryLastAt); v != "" {
		if last, err := time.Parse(time.RFC3339Nano, v); err == nil && now.Sub(last) < PaymentRetryMinInterval {
			return nil, ErrPaymentRetryTooSoon
		}
	}
	return map[string]interface{}{
		metaPaymentRetries:     retries + 1,
		metaPaymentRetryLastAt: now.UTC().Format(time.RFC3339Nano),
	}, nil
}

// paidIntentPatch records which intent paid the order. A success for another intent after the
// order is already paid means the customer was charged twice; it is listed for a refund and the
// paying intent is left as it is.
func paidIntentPatch(cur OrderSnapshot, intentID string) map[string]interface{} {
	if intentID == "" {
		return nil
	}
	paidBy := stringMeta(cur.Metadata, metaPaymentPaidIntentID)
	if paidBy == intentID {
		return nil
	}
	if cur.PaymentStatus == PaymentStatusPaid && paidBy != "" {
		extra := stringSliceMeta(cur.Metadata, metaPaymentExtraPaidIntents)
		for _, id := range extra {
			if id == intentID {
				return nil
			}
		}
		return map[string]interface{}{metaPaymentExtraPaidIntents: append(extra, intentID)}
	}
	return map[string]interface{}{
		metaPaymentPaidIntentID: intentID,
		metaPaymentIntentID:     intentID,
	}
}

// PaymentIntentIDs lists every intent created for the order, current one first, without
// duplicates, so the poller can find a success on an earlier attempt too.
func PaymentIntentIDs(current *uuid.UUID, meta map[string]interface{}) []uuid.UUID {
	seen := map[uuid.UUID]bool{}
	var out []uuid.UUID
	add := func(id uuid.UUID) {
		if id != uuid.Nil && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	if current != nil {
		add(*current)
	}
	ids := stringSliceMeta(meta, metaPaymentIntentIDs)
	for i := len(ids) - 1; i >= 0; i-- {
		if id, err := uuid.Parse(ids[i]); err == nil {
			add(id)
		}
	}
	return out
}

// intentRecordPatch adds a new intent to the order's intent list and makes it the refund intent.
func intentRecordPatch(meta map[string]interface{}, intentID uuid.UUID) map[string]interface{} {
	ids := stringSliceMeta(meta, metaPaymentIntentIDs)
	for _, id := range ids {
		if id == intentID.String() {
			return map[string]interface{}{metaPaymentIntentID: intentID.String()}
		}
	}
	return map[string]interface{}{
		metaPaymentIntentIDs: append(ids, intentID.String()),
		metaPaymentIntentID:  intentID.String(),
	}
}

// LastPaymentFailure is the reason recorded for the order's last failed payment attempt.
func LastPaymentFailure(meta map[string]interface{}) string {
	return stringMeta(meta, metaPaymentLastFailure)
}

func intMeta(meta map[string]interface{}, key string) int {
	if meta == nil {
		return 0
	}
	switch v := meta[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		n, _ := strconv.Atoi(v)
		return n
	}
	return 0
}

func stringSliceMeta(meta map[string]interface{}, key string) []string {
	if meta == nil {
		return nil
	}
	switch v := meta[key].(type) {
	case []string:
		return append([]string(nil), v...)
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, x := range v {
			if s, ok := x.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// RecordPaymentAttemptFailure records a failed, cancelled or expired payment attempt on an order
// that is still waiting for its payment. The order is not cancelled and its stock stays held.
func (s *OrderService) RecordPaymentAttemptFailure(ctx context.Context, tenantID, orderID uuid.UUID, intentID uuid.UUID, reason string) error {
	now := time.Now()
	id := ""
	if intentID != uuid.Nil {
		id = intentID.String()
	}
	_, err := s.repo.MergeOrderMetadataIf(ctx, tenantID, orderID, func(cur OrderSnapshot) map[string]interface{} {
		if cur.Status != OrderStatusPending || cur.PaymentStatus == PaymentStatusPaid {
			return nil
		}
		return PaymentFailurePatch(cur.Metadata, id, reason, now)
	})
	return err
}

// RecordPaidIntent notes which intent paid the order, before the paid transition runs, so a later
// refund is drawn on the right intent. See paidIntentPatch for a second success.
func (s *OrderService) RecordPaidIntent(ctx context.Context, tenantID, orderID, intentID uuid.UUID) {
	if intentID == uuid.Nil {
		return
	}
	var extra bool
	_, err := s.repo.MergeOrderMetadataIf(ctx, tenantID, orderID, func(cur OrderSnapshot) map[string]interface{} {
		patch := paidIntentPatch(cur, intentID.String())
		_, extra = patch[metaPaymentExtraPaidIntents]
		return patch
	})
	if err != nil {
		s.logger.Warn("could not record the paying intent",
			zap.String("order_id", orderID.String()), zap.String("intent_id", intentID.String()), zap.Error(err))
		return
	}
	if extra {
		s.logger.Warn("second successful payment on an already paid order; refund the extra payment",
			zap.String("order_id", orderID.String()), zap.String("intent_id", intentID.String()))
	}
}

// PaymentRetryResult is what the retry endpoint returns: the intent to pay and the treasury
// initiate URL, the same shape checkout returns.
type PaymentRetryResult struct {
	OrderID         uuid.UUID `json:"orderId"`
	OrderNumber     string    `json:"orderNumber"`
	PaymentIntentID string    `json:"paymentIntentId"`
	InitiateURL     string    `json:"initiateUrl,omitempty"`
	Amount          float64   `json:"amount"`
	Currency        string    `json:"currency"`
	RetryUntil      time.Time `json:"retryUntil"`
	Reused          bool      `json:"reused"`
}

// RetryOrderPayment lets the customer pay an unpaid online-payment order again while its retry
// window is open. The caller has already checked the customer may act on the order.
//
// It looks at the current intent first. Succeeded: the order is confirmed through the normal paid
// path and ErrPaymentAlreadyCompleted is returned. Processing: refused, a prompt is still open.
// Pending (never initiated, or the customer closed the payment dialog): the same intent is
// returned. Failed, cancelled or expired: the failure is recorded and a fresh intent is created
// with its own reference (payref attempt suffix), because treasury refuses to initiate a closed
// intent and returns the old one for a reused reference.
func (s *OrderService) RetryOrderPayment(ctx context.Context, tenantID, orderID uuid.UUID) (*PaymentRetryResult, error) {
	if s.treasuryClient == nil {
		return nil, ErrTreasuryNotConfigured
	}
	order, err := s.repo.GetOrder(ctx, tenantID, orderID)
	if err != nil {
		return nil, err
	}
	if order.PaymentStatus == PaymentStatusPaid {
		return nil, ErrPaymentAlreadyCompleted
	}
	if !awaitsOnlinePayment(order) {
		return nil, ErrPaymentNotRetryable
	}
	deadline := PaymentRetryDeadline(order.PlacedAt, order.CreatedAt, order.Metadata)
	if !time.Now().Before(deadline) {
		return nil, ErrPaymentRetryClosed
	}
	amount := payableAmountOf(order)

	if order.PaymentIntentID != nil && *order.PaymentIntentID != uuid.Nil {
		status, sErr := s.treasuryClient.GetPaymentStatus(ctx, tenantID, *order.PaymentIntentID)
		if sErr != nil {
			return nil, fmt.Errorf("%w: %v", ErrPaymentInitiateFailed, sErr)
		}
		switch status.Status {
		case "succeeded":
			s.RecordPaidIntent(ctx, tenantID, orderID, *order.PaymentIntentID)
			if _, uErr := s.UpdatePaymentStatus(ctx, tenantID, orderID, PaymentStatusPaid, nil); uErr != nil {
				return nil, uErr
			}
			return nil, ErrPaymentAlreadyCompleted
		case "processing":
			return nil, ErrPaymentInFlight
		case "pending":
			return &PaymentRetryResult{
				OrderID: order.ID, OrderNumber: order.OrderNumber,
				PaymentIntentID: order.PaymentIntentID.String(),
				InitiateURL:     s.initiateURL(order.TenantID, *order.PaymentIntentID),
				Amount:          amount, Currency: order.Currency, RetryUntil: deadline, Reused: true,
			}, nil
		default:
			reason := status.ErrorMessage
			if reason == "" {
				reason = status.Status
			}
			if rErr := s.RecordPaymentAttemptFailure(ctx, tenantID, orderID, *order.PaymentIntentID, reason); rErr != nil {
				s.logger.Warn("could not record failed payment attempt", zap.String("order_id", orderID.String()), zap.Error(rErr))
			}
		}
	}

	// Claim a retry slot (limit, spacing, window) in one compare-and-set write.
	var claimErr error
	var attempt int
	wrote, err := s.repo.MergeOrderMetadataIf(ctx, tenantID, orderID, func(cur OrderSnapshot) map[string]interface{} {
		patch, cErr := paymentRetryClaim(cur, deadline, time.Now())
		claimErr = cErr
		if patch != nil {
			attempt = patch[metaPaymentRetries].(int) + 1 // the checkout intent was attempt 1
		}
		return patch
	})
	if err != nil {
		return nil, err
	}
	if claimErr != nil {
		return nil, claimErr
	}
	if !wrote {
		return nil, ErrPaymentNotRetryable
	}

	ci := s.orderContactInfo(ctx, order)
	intentID, iErr := s.createOrderIntent(ctx, order, amount, ci.Email, ci.Phone, attempt)
	if iErr != nil {
		return nil, fmt.Errorf("%w: %v", ErrPaymentInitiateFailed, iErr)
	}
	return &PaymentRetryResult{
		OrderID: order.ID, OrderNumber: order.OrderNumber,
		PaymentIntentID: intentID.String(),
		InitiateURL:     s.initiateURL(order.TenantID, intentID),
		Amount:          amount, Currency: order.Currency, RetryUntil: deadline,
	}, nil
}

// payableAmountOf is what the customer pays online: the booking deposit when one was set at
// checkout, else the grand total.
func payableAmountOf(order *Order) float64 {
	if order.Metadata != nil {
		if v, ok := order.Metadata["deposit_amount"].(float64); ok && v > 0 && v < order.GrandTotal {
			return v
		}
	}
	return order.GrandTotal
}
