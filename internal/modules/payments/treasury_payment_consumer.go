package payments

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

const (
	treasuryPaymentDurable    = "ordering-treasury-payment-succeeded"
	treasuryPaymentAckWait    = 30 * time.Second
	treasuryPaymentMaxDeliver = 8
)

// TreasuryPaymentConsumer confirms an order the instant treasury reports its payment succeeded,
// instead of waiting for the ~5-minute HTTP poller. It subscribes to the shared-events subject
// treasury.payment.succeeded (published by treasury's outbox) via a durable JetStream consumer, so
// it is event-driven, low-latency, and survives restarts. The poller remains as a durable fallback
// for any event that is missed (e.g. published while this consumer was briefly down).
type TreasuryPaymentConsumer struct {
	log              *zap.Logger
	onPaymentSuccess func(ctx context.Context, tenantID, orderID uuid.UUID) error
}

// NewTreasuryPaymentConsumer constructs the consumer. onPaymentSuccess is the same callback the
// poller uses (UpdatePaymentStatus -> paid -> publish payment_confirmed), which is idempotent.
func NewTreasuryPaymentConsumer(log *zap.Logger, onPaymentSuccess func(ctx context.Context, tenantID, orderID uuid.UUID) error) *TreasuryPaymentConsumer {
	return &TreasuryPaymentConsumer{
		log:              log.Named("consumers.treasury_payment"),
		onPaymentSuccess: onPaymentSuccess,
	}
}

// Start ensures the treasury stream exists then subscribes to treasury.payment.succeeded. Blocks
// until ctx is done.
func (c *TreasuryPaymentConsumer) Start(ctx context.Context, js nats.JetStreamContext) error {
	// Ensure the stream that retains treasury events exists, so a durable consumer can bind even if
	// ordering starts before treasury. Mirrors treasury's own stream-ensure (subjects treasury.>).
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

	sub, err := js.Subscribe(
		"treasury.payment.succeeded",
		c.handleMessage,
		nats.Durable(treasuryPaymentDurable),
		nats.AckExplicit(),
		nats.AckWait(treasuryPaymentAckWait),
		nats.MaxDeliver(treasuryPaymentMaxDeliver),
		nats.DeliverAll(),
	)
	if err != nil {
		return fmt.Errorf("treasury payment consumer: subscribe: %w", err)
	}
	c.log.Info("treasury payment consumer started", zap.String("durable", treasuryPaymentDurable))
	<-ctx.Done()
	return sub.Unsubscribe()
}

// sharedEventEnvelope matches the JSON of github.com/Bengo-Hub/shared-events Event (the format
// treasury publishes): a flat envelope with tenant_id and a payload map.
type sharedEventEnvelope struct {
	EventType string                 `json:"event_type"`
	TenantID  string                 `json:"tenant_id"`
	Payload   map[string]interface{} `json:"payload"`
}

func (c *TreasuryPaymentConsumer) handleMessage(msg *nats.Msg) {
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

	if c.onPaymentSuccess != nil {
		// UpdatePaymentStatus is idempotent: re-confirming an already-paid order is a no-op and does
		// not re-publish payment_confirmed, so duplicate delivery / overlap with the poller is safe.
		if err := c.onPaymentSuccess(context.Background(), tenantID, orderID); err != nil {
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
