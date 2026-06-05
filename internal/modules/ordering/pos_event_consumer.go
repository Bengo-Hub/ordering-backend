package ordering

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	sharedevents "github.com/Bengo-Hub/shared-events"
)

// POS JetStream consumer settings. PULL durables are used (mirroring the treasury payment consumer)
// so BOTH ordering replicas share one durable (work-queue semantics) and it survives pod restarts —
// unlike push durables, which fail every replica after the first with "consumer is already bound".
// Each subject gets its own durable because a JetStream durable's filter subject is immutable.
const (
	posStreamName     = "pos"
	posStreamSubjects = "pos.>"
	posStreamMaxAge   = 72 * time.Hour

	posKDSReadyDurable    = "ordering-pos-kds-ready"
	posCollectedDurable   = "ordering-pos-online-collected"
	posConsumerAckWait    = 30 * time.Second
	posConsumerMaxDeliver = 8
	posConsumerFetchBatch = 10
	posConsumerFetchWait  = 5 * time.Second
)

// POSEventConsumer reacts to pos-api lifecycle events (published via shared-events on the "pos"
// JetStream stream, wire envelope = event_type + payload):
//   - pos.kds.order.ready        -> advance the matching online order to "ready"
//   - pos.online_order.collected -> mark the matching online order "completed"
//
// It reuses OrderService.UpdateOrderStatus (the same method the admin status-update path uses) so the
// existing order.ready / order.for_pickup / order.completed customer notifications fire automatically.
type POSEventConsumer struct {
	log          *zap.Logger
	orderSvc     *OrderService
	orderingRepo Repository
}

// NewPOSEventConsumer constructs the consumer.
func NewPOSEventConsumer(log *zap.Logger, orderSvc *OrderService, orderingRepo Repository) *POSEventConsumer {
	return &POSEventConsumer{
		log:          log.Named("consumers.pos_events"),
		orderSvc:     orderSvc,
		orderingRepo: orderingRepo,
	}
}

// Start ensures the pos stream exists then pulls each subscribed subject in its own goroutine until
// ctx is done.
func (c *POSEventConsumer) Start(ctx context.Context, js nats.JetStreamContext) error {
	if err := c.ensurePOSStream(js); err != nil {
		return err
	}

	subs := []struct {
		subject string
		durable string
		handler func(context.Context, *sharedevents.Event) error
	}{
		{"pos.kds.order.ready", posKDSReadyDurable, c.handleKDSOrderReady},
		{"pos.online_order.collected", posCollectedDurable, c.handleOnlineOrderCollected},
	}

	for _, s := range subs {
		s := s
		sub, err := js.PullSubscribe(
			s.subject,
			s.durable,
			nats.AckExplicit(),
			nats.AckWait(posConsumerAckWait),
			nats.MaxDeliver(posConsumerMaxDeliver),
			nats.BindStream(posStreamName),
		)
		if err != nil {
			return fmt.Errorf("pos consumer: pull subscribe %s: %w", s.subject, err)
		}
		c.log.Info("pos event consumer started (pull)",
			zap.String("subject", s.subject), zap.String("durable", s.durable))
		go c.consume(ctx, sub, s.subject, s.handler)
	}
	return nil
}

// ensurePOSStream ensures the "pos" JetStream stream exists (idempotent; pos-api ensures it too).
func (c *POSEventConsumer) ensurePOSStream(js nats.JetStreamContext) error {
	if _, err := js.StreamInfo(posStreamName); err == nil {
		return nil
	}
	if _, err := js.AddStream(&nats.StreamConfig{
		Name:      posStreamName,
		Subjects:  []string{posStreamSubjects},
		Retention: nats.LimitsPolicy,
		MaxAge:    posStreamMaxAge,
		Storage:   nats.FileStorage,
	}); err != nil && err != nats.ErrStreamNameAlreadyInUse {
		return fmt.Errorf("pos consumer: ensure stream: %w", err)
	}
	return nil
}

// consume pulls messages for one subject in a loop and dispatches them to handler.
func (c *POSEventConsumer) consume(ctx context.Context, sub *nats.Subscription, subject string, handler func(context.Context, *sharedevents.Event) error) {
	for {
		if ctx.Err() != nil {
			_ = sub.Unsubscribe()
			return
		}
		msgs, ferr := sub.Fetch(posConsumerFetchBatch, nats.MaxWait(posConsumerFetchWait))
		if ferr != nil {
			if errors.Is(ferr, nats.ErrTimeout) {
				continue
			}
			if ctx.Err() != nil {
				_ = sub.Unsubscribe()
				return
			}
			c.log.Warn("pos consumer: fetch error", zap.String("subject", subject), zap.Error(ferr))
			continue
		}
		for _, m := range msgs {
			c.handleMessage(ctx, m, subject, handler)
		}
	}
}

// handleMessage decodes the shared-events envelope and dispatches to the typed handler, acking on
// success / malformed input and naking on a retriable error.
func (c *POSEventConsumer) handleMessage(ctx context.Context, msg *nats.Msg, subject string, handler func(context.Context, *sharedevents.Event) error) {
	evt, err := sharedevents.FromJSON(msg.Data)
	if err != nil {
		c.log.Warn("pos consumer: malformed envelope", zap.String("subject", subject), zap.Error(err))
		_ = msg.Ack() // malformed — don't redeliver
		return
	}
	if herr := handler(ctx, evt); herr != nil {
		c.log.Error("pos consumer: handler error (will retry)",
			zap.String("subject", subject), zap.Error(herr))
		_ = msg.Nak()
		return
	}
	_ = msg.Ack()
}

// handleKDSOrderReady advances the online order referenced by external_order_id to "ready".
func (c *POSEventConsumer) handleKDSOrderReady(ctx context.Context, evt *sharedevents.Event) error {
	tenantID, orderID, ok := posOrderRef(evt)
	if !ok {
		// No external online-order reference (e.g. a POS-native ticket) — nothing for ordering to do.
		return nil
	}

	order, err := c.orderingRepo.GetOrder(ctx, tenantID, orderID)
	if err != nil {
		return fmt.Errorf("pos.kds.order.ready: get order %s: %w", orderID, err)
	}

	// Idempotent: once the order is ready or further along, ignore duplicate/late KDS ready events.
	switch order.Status {
	case OrderStatusReady, OrderStatusOutForDelivery, OrderStatusDelivered,
		OrderStatusCompleted, OrderStatusCancelled, OrderStatusRefunded:
		c.log.Info("pos.kds.order.ready: order already at/past ready, skipping",
			zap.String("order_id", orderID.String()), zap.String("status", string(order.Status)))
		return nil
	}

	// Walk the state machine up to "ready" using the same admin method, so order.ready /
	// order.for_pickup side-effects (customer email) fire on the final hop.
	if err := c.advanceOrderTo(ctx, tenantID, orderID, OrderStatusReady); err != nil {
		return fmt.Errorf("pos.kds.order.ready: advance order %s to ready: %w", orderID, err)
	}
	c.log.Info("order marked ready from pos.kds.order.ready", zap.String("order_id", orderID.String()))
	return nil
}

// handleOnlineOrderCollected marks the online order referenced by external_order_id as completed.
func (c *POSEventConsumer) handleOnlineOrderCollected(ctx context.Context, evt *sharedevents.Event) error {
	tenantID, orderID, ok := posOrderRef(evt)
	if !ok {
		return nil
	}

	order, err := c.orderingRepo.GetOrder(ctx, tenantID, orderID)
	if err != nil {
		return fmt.Errorf("pos.online_order.collected: get order %s: %w", orderID, err)
	}

	// Idempotent: skip if already completed or terminal.
	switch order.Status {
	case OrderStatusCompleted, OrderStatusCancelled, OrderStatusRefunded:
		c.log.Info("pos.online_order.collected: order already terminal, skipping",
			zap.String("order_id", orderID.String()), zap.String("status", string(order.Status)))
		return nil
	}

	if err := c.advanceOrderTo(ctx, tenantID, orderID, OrderStatusCompleted); err != nil {
		return fmt.Errorf("pos.online_order.collected: complete order %s: %w", orderID, err)
	}
	c.log.Info("order completed from pos.online_order.collected", zap.String("order_id", orderID.String()))
	return nil
}

// advanceOrderTo walks the order state machine from its current status to target, calling the admin
// UpdateOrderStatus for each valid hop so all per-transition side-effects fire. It re-reads the order
// between hops to use the authoritative current status.
func (c *POSEventConsumer) advanceOrderTo(ctx context.Context, tenantID, orderID uuid.UUID, target OrderStatus) error {
	for i := 0; i < len(posReadyPath); i++ {
		order, err := c.orderingRepo.GetOrder(ctx, tenantID, orderID)
		if err != nil {
			return err
		}
		if order.Status == target {
			return nil
		}
		next, ok := nextStatusToward(order.Status, target)
		if !ok {
			return fmt.Errorf("no path from %s to %s", order.Status, target)
		}
		if _, err := c.orderSvc.UpdateOrderStatus(ctx, tenantID, orderID, next, nil, "system", ""); err != nil {
			return err
		}
	}
	return nil
}

// posReadyPath bounds the advanceOrderTo loop (longest path pending->preparing->ready->...->completed).
var posReadyPath = []OrderStatus{
	OrderStatusConfirmed, OrderStatusPreparing, OrderStatusReady,
	OrderStatusOutForDelivery, OrderStatusDelivered, OrderStatusCompleted,
}

// nextStatusToward returns the next status to step to when walking the happy-path order lifecycle
// from `from` toward `target`. Only the linear fulfilment path is walked (no cancel/refund).
func nextStatusToward(from, target OrderStatus) (OrderStatus, bool) {
	linear := []OrderStatus{
		OrderStatusPending,
		OrderStatusConfirmed,
		OrderStatusPreparing,
		OrderStatusReady,
		OrderStatusOutForDelivery,
		OrderStatusDelivered,
		OrderStatusCompleted,
	}
	fromIdx, targetIdx := -1, -1
	for i, s := range linear {
		if s == from {
			fromIdx = i
		}
		if s == target {
			targetIdx = i
		}
	}
	if fromIdx < 0 || targetIdx < 0 || targetIdx <= fromIdx {
		return "", false
	}
	// For pickup orders ready -> completed is a direct edge in the state machine; skip the
	// delivery-only hops when heading straight to completed from ready.
	if from == OrderStatusReady && target == OrderStatusCompleted {
		return OrderStatusCompleted, true
	}
	return linear[fromIdx+1], true
}

// posOrderRef extracts the (tenantID, online orderID) the event refers to. Per the cross-service
// contract pos-api carries the ordering order id in payload.external_order_id and the tenant in the
// envelope tenant_id. Returns ok=false when no external reference is present.
func posOrderRef(evt *sharedevents.Event) (uuid.UUID, uuid.UUID, bool) {
	if evt == nil {
		return uuid.Nil, uuid.Nil, false
	}
	refStr, _ := evt.Payload["external_order_id"].(string)
	if refStr == "" {
		return uuid.Nil, uuid.Nil, false
	}
	orderID, err := uuid.Parse(refStr)
	if err != nil {
		return uuid.Nil, uuid.Nil, false
	}
	tenantID := evt.TenantID
	if tenantID == uuid.Nil {
		// Fall back to a tenant_id inside the payload if the envelope did not carry one.
		if t, terr := uuid.Parse(fmt.Sprint(evt.Payload["tenant_id"])); terr == nil {
			tenantID = t
		}
	}
	if tenantID == uuid.Nil {
		return uuid.Nil, uuid.Nil, false
	}
	return tenantID, orderID, true
}
