package payments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

const (
	// Fresh durable name (the earlier push durable is intentionally abandoned). A PULL consumer is
	// used so BOTH ordering replicas can share one durable (work-queue semantics) and it survives pod
	// restarts — unlike push durables, which fail every replica after the first with "consumer is
	// already bound to a subscription".
	treasuryPaymentDurable    = "ordering-treasury-pay"
	treasuryPaymentAckWait    = 30 * time.Second
	treasuryPaymentMaxDeliver = 8
	treasuryPaymentFetchBatch = 10
	treasuryPaymentFetchWait  = 5 * time.Second
)

// TreasuryPaymentConsumer confirms an order the instant treasury publishes treasury.payment.succeeded
// (shared-events subject on the "treasury" JetStream stream), instead of waiting for the ~5-minute
// HTTP poller. It uses a durable PULL subscription so it is multi-replica and restart safe. The
// poller remains as a durable fallback for any event missed while the consumer is down.
type TreasuryPaymentConsumer struct {
	log              *zap.Logger
	onPaymentSuccess func(ctx context.Context, tenantID, orderID uuid.UUID) error
	// hasFeature gates treasury→ordering payment sync by subscription entitlement. Nil → fail open.
	hasFeature func(ctx context.Context, tenantID, feature string) bool
}

// NewTreasuryPaymentConsumer constructs the consumer. onPaymentSuccess is the same idempotent
// handler the poller uses (UpdatePaymentStatus -> paid -> publish payment_confirmed).
func NewTreasuryPaymentConsumer(log *zap.Logger, onPaymentSuccess func(ctx context.Context, tenantID, orderID uuid.UUID) error) *TreasuryPaymentConsumer {
	return &TreasuryPaymentConsumer{
		log:              log.Named("consumers.treasury_payment"),
		onPaymentSuccess: onPaymentSuccess,
	}
}

// SetFeatureGate wires the subscription entitlement check used to gate payment sync.
func (c *TreasuryPaymentConsumer) SetFeatureGate(fn func(ctx context.Context, tenantID, feature string) bool) {
	c.hasFeature = fn
}

// Start ensures the treasury stream exists then pulls treasury.payment.succeeded in a loop until ctx
// is done.
func (c *TreasuryPaymentConsumer) Start(ctx context.Context, js nats.JetStreamContext) error {
	// Ensure the stream that retains treasury events exists (idempotent; treasury ensures it too).
	if _, err := js.StreamInfo("treasury"); err != nil {
		if _, aerr := js.AddStream(&nats.StreamConfig{
			Name:      "treasury",
			Subjects:  []string{"treasury.>"},
			Retention: nats.LimitsPolicy,
			MaxAge:    72 * time.Hour,
			Storage:   nats.FileStorage,
		}); aerr != nil && aerr != nats.ErrStreamNameAlreadyInUse {
			return fmt.Errorf("treasury payment consumer: ensure stream: %w", aerr)
		}
	}

	sub, err := js.PullSubscribe(
		"treasury.payment.succeeded",
		treasuryPaymentDurable,
		nats.AckExplicit(),
		nats.AckWait(treasuryPaymentAckWait),
		nats.MaxDeliver(treasuryPaymentMaxDeliver),
		nats.BindStream("treasury"),
	)
	if err != nil {
		return fmt.Errorf("treasury payment consumer: pull subscribe: %w", err)
	}
	c.log.Info("treasury payment consumer started (pull)", zap.String("durable", treasuryPaymentDurable))

	for {
		if ctx.Err() != nil {
			_ = sub.Unsubscribe()
			return nil
		}
		msgs, ferr := sub.Fetch(treasuryPaymentFetchBatch, nats.MaxWait(treasuryPaymentFetchWait))
		if ferr != nil {
			// Timeout simply means no messages were ready in this window — keep polling.
			if errors.Is(ferr, nats.ErrTimeout) {
				continue
			}
			if ctx.Err() != nil {
				_ = sub.Unsubscribe()
				return nil
			}
			c.log.Warn("treasury payment: fetch error", zap.Error(ferr))
			continue
		}
		for _, m := range msgs {
			c.handleMessage(ctx, m)
		}
	}
}

// sharedEventEnvelope matches the JSON of github.com/Bengo-Hub/shared-events Event (the format
// treasury publishes): a flat envelope with tenant_id and a payload map.
type sharedEventEnvelope struct {
	EventType string                 `json:"event_type"`
	TenantID  string                 `json:"tenant_id"`
	Payload   map[string]interface{} `json:"payload"`
}

func (c *TreasuryPaymentConsumer) handleMessage(ctx context.Context, msg *nats.Msg) {
	var env sharedEventEnvelope
	if err := json.Unmarshal(msg.Data, &env); err != nil {
		c.log.Warn("treasury payment: unmarshal failed", zap.Error(err))
		_ = msg.Ack() // malformed — don't redeliver
		return
	}

	// Only order payments concern ordering; ignore subscription/invoice/etc. references.
	refType, _ := env.Payload["reference_type"].(string)
	status, _ := env.Payload["status"].(string)
	refID, _ := env.Payload["reference_id"].(string)
	if refType != "order" || status != "succeeded" {
		_ = msg.Ack()
		return
	}

	tenantID, terr := uuid.Parse(env.TenantID)
	orderID, oerr := uuid.Parse(refID)
	if terr != nil || oerr != nil {
		c.log.Warn("treasury payment: bad tenant/order id",
			zap.String("tenant_id", env.TenantID), zap.String("reference_id", refID))
		_ = msg.Ack()
		return
	}

	// Gate treasury→ordering payment sync by entitlement. basic_treasury_access is auto-injected
	// into every ordering plan, so this only blocks tenants with no ordering/treasury entitlement.
	// Fails open on lookup failure so a real payment is never stranded by subscriptions-api downtime.
	if c.hasFeature != nil && !c.hasFeature(ctx, tenantID.String(), "basic_treasury_access") {
		c.log.Debug("treasury payment: tenant lacks basic_treasury_access — skipping order sync",
			zap.String("tenant_id", tenantID.String()))
		_ = msg.Ack()
		return
	}

	if c.onPaymentSuccess != nil {
		// UpdatePaymentStatus is idempotent: re-confirming an already-paid order is a no-op and does
		// not re-publish payment_confirmed, so duplicate delivery / overlap with the poller is safe.
		if err := c.onPaymentSuccess(ctx, tenantID, orderID); err != nil {
			c.log.Error("treasury payment: confirm order failed (will retry)",
				zap.String("order_id", orderID.String()), zap.Error(err))
			_ = msg.Nak()
			return
		}
	}
	c.log.Info("order confirmed from treasury.payment.succeeded",
		zap.String("order_id", orderID.String()), zap.String("tenant_id", tenantID.String()))
	_ = msg.Ack()
}
