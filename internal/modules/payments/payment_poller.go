package payments

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/modules/ordering"
)

const (
	// pollStaleAfter: orders younger than this are left to the payment dialog and the
	// treasury.payment.succeeded consumer.
	pollStaleAfter = 5 * time.Minute
	pollPageSize   = 50
	// pollMaxPages bounds one sweep (500 orders); the rest wait for the next tick.
	pollMaxPages = 10
	// processingGrace keeps an order whose last prompt is still processing open a little past its
	// window, so a customer entering their PIN at the last minute is not cancelled under them.
	processingGrace = 10 * time.Minute
	// maxIntentsChecked bounds treasury calls per order (the checkout intent plus retries).
	maxIntentsChecked = ordering.MaxPaymentRetries + 1
)

// intentCheck is one payment intent's state as treasury reported it.
type intentCheck struct {
	ID     uuid.UUID
	Status string
	Reason string
	Err    error
}

// pollDecision is what the poller does with one unpaid order.
type pollDecision struct {
	// Confirm is the intent that succeeded: the order is confirmed through the normal paid path.
	Confirm *intentCheck
	// RecordFailure is the current intent when it failed, was cancelled or expired. The attempt is
	// recorded and the order stays open while the window lasts.
	RecordFailure *intentCheck
	// Expire cancels the order: the window closed without a payment.
	Expire bool
	Reason string
}

func isClosedFailure(status string) bool {
	switch status {
	case "failed", "cancelled", "expired":
		return true
	}
	return false
}

// decidePaymentPoll decides from the intents' states (current intent first) and the retry
// deadline. A success on any intent, including an earlier attempt, always wins. A status check
// error never cancels: the payment may have gone through and treasury may just be unreachable.
// A prompt still processing at the deadline gets processingGrace before the order is cancelled.
func decidePaymentPoll(now, deadline time.Time, checks []intentCheck, lastFailure string) pollDecision {
	var d pollDecision
	anyErr := false
	for i := range checks {
		if checks[i].Err != nil {
			anyErr = true
			continue
		}
		if checks[i].Status == "succeeded" {
			d.Confirm = &checks[i]
			return d
		}
	}
	if len(checks) > 0 && checks[0].Err == nil && isClosedFailure(checks[0].Status) {
		d.RecordFailure = &checks[0]
		if checks[0].Reason != "" {
			lastFailure = checks[0].Reason
		}
	}
	if anyErr || now.Before(deadline) {
		return d
	}
	if len(checks) > 0 && checks[0].Status == "processing" && now.Before(deadline.Add(processingGrace)) {
		return d
	}
	d.Expire = true
	d.Reason = "payment not completed in time"
	if lastFailure = strings.TrimSpace(lastFailure); lastFailure != "" {
		d.Reason += " (" + lastFailure + ")"
	}
	return d
}

// sweepPendingPayments walks every order still waiting for an online payment, page by page, and
// confirms, records or cancels each one per decidePaymentPoll.
func (s *PaymentService) sweepPendingPayments(ctx context.Context, now time.Time) {
	cutoff := now.Add(-pollStaleAfter)
	var cursor *ordering.StalePaymentCursor
	for page := 0; page < pollMaxPages; page++ {
		orders, err := s.orderingRepo.GetStalePaymentOrders(ctx, cutoff, cursor, pollPageSize)
		if err != nil {
			s.logger.Error("payment poller: failed to list stale orders", zap.Error(err))
			return
		}
		for _, o := range orders {
			if ctx.Err() != nil {
				return
			}
			s.pollOrder(ctx, o, now)
		}
		if len(orders) < pollPageSize {
			return
		}
		last := orders[len(orders)-1]
		if last.PlacedAt == nil {
			return
		}
		cursor = &ordering.StalePaymentCursor{PlacedAt: *last.PlacedAt, ID: last.ID}
	}
}

func (s *PaymentService) pollOrder(ctx context.Context, o ordering.StalePaymentOrder, now time.Time) {
	ids := ordering.PaymentIntentIDs(o.PaymentIntentID, o.Metadata)
	if len(ids) > maxIntentsChecked {
		ids = ids[:maxIntentsChecked]
	}
	checks := make([]intentCheck, 0, len(ids))
	for _, id := range ids {
		c := intentCheck{ID: id}
		status, err := s.treasuryClient.GetPaymentStatus(ctx, o.TenantID, id)
		if err != nil {
			// Warn, not Debug: a persistent failure here stops the poller confirming anything.
			s.logger.Warn("payment poller: failed to get status from treasury (will retry)",
				zap.Error(err), zap.String("order_id", o.ID.String()), zap.String("intent_id", id.String()))
			c.Err = err
		} else {
			c.Status = status.Status
			c.Reason = status.ErrorMessage
			if c.Reason == "" && isClosedFailure(status.Status) {
				c.Reason = status.Status
			}
		}
		checks = append(checks, c)
		if c.Status == "succeeded" {
			break
		}
	}

	deadline := ordering.PaymentRetryDeadline(o.PlacedAt, o.CreatedAt, o.Metadata)
	d := decidePaymentPoll(now, deadline, checks, ordering.LastPaymentFailure(o.Metadata))

	if d.Confirm != nil {
		s.logger.Info("payment poller: payment succeeded, confirming order",
			zap.String("order_id", o.ID.String()), zap.String("intent_id", d.Confirm.ID.String()))
		if s.onPaymentSuccess != nil {
			if err := s.onPaymentSuccess(ctx, o.TenantID, o.ID, d.Confirm.ID); err != nil {
				s.logger.Error("payment poller: onPaymentSuccess callback failed",
					zap.Error(err), zap.String("order_id", o.ID.String()))
			}
		}
		return
	}
	if d.RecordFailure != nil && s.onAttemptFailed != nil {
		if err := s.onAttemptFailed(ctx, o.TenantID, o.ID, d.RecordFailure.ID, d.RecordFailure.Reason); err != nil {
			s.logger.Warn("payment poller: could not record failed attempt",
				zap.Error(err), zap.String("order_id", o.ID.String()))
		}
	}
	if d.Expire {
		s.logger.Info("payment poller: retry window closed, cancelling order",
			zap.String("order_id", o.ID.String()),
			zap.String("tenant_id", o.TenantID.String()),
			zap.String("reason", d.Reason))
		if s.onPaymentFailed != nil {
			if err := s.onPaymentFailed(ctx, o.TenantID, o.ID, d.Reason); err != nil {
				s.logger.Error("payment poller: onPaymentFailed callback failed",
					zap.Error(err), zap.String("order_id", o.ID.String()))
			}
		}
	}
}
