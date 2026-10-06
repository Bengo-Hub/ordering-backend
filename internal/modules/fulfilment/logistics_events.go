package fulfilment

import (
	"context"
	"fmt"
	"time"

	sharedevents "github.com/Bengo-Hub/shared-events"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/modules/ordering"
	"github.com/bengobox/ordering-backend/internal/platform/events"
	"github.com/bengobox/ordering-backend/internal/platform/treasury"
)

// LogisticsEventHandler handles logistics task events to update order/assignment state.
// The per-event handler implementations live in logistics_event_handlers.go.
type LogisticsEventHandler struct {
	repo           Repository
	orderingSvc    *ordering.OrderService
	orderingRepo   ordering.Repository
	eventPublisher *events.Publisher
	treasuryClient *treasury.Client
	logger         *zap.Logger
}

// NewLogisticsEventHandler creates a new logistics event handler.
func NewLogisticsEventHandler(
	repo Repository,
	orderingSvc *ordering.OrderService,
	orderingRepo ordering.Repository,
	eventPublisher *events.Publisher,
	logger *zap.Logger,
) *LogisticsEventHandler {
	return &LogisticsEventHandler{
		repo:           repo,
		orderingSvc:    orderingSvc,
		orderingRepo:   orderingRepo,
		eventPublisher: eventPublisher,
		logger:         logger.Named("fulfilment.logistics_events"),
	}
}

// SetTreasuryClient sets the treasury client for COD settlement on delivery.
func (h *LogisticsEventHandler) SetTreasuryClient(client *treasury.Client) {
	h.treasuryClient = client
}

const (
	logisticsStreamName     = "logistics"
	logisticsStreamSubjects = "logistics.>"
	logisticsStreamMaxAge   = 72 * time.Hour
)

// ensureLogisticsStream ensures the "logistics" JetStream stream exists.
func ensureLogisticsStream(js nats.JetStreamContext) error {
	_, err := js.StreamInfo(logisticsStreamName)
	if err == nil {
		return nil
	}
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     logisticsStreamName,
		Subjects: []string{logisticsStreamSubjects},
		MaxAge:   logisticsStreamMaxAge,
		Storage:  nats.FileStorage,
		Replicas: 1,
	})
	if err != nil {
		return fmt.Errorf("create logistics stream: %w", err)
	}
	return nil
}

// subscribeLogisticsDurable sets up a single JetStream durable push subscription for a logistics subject.
func (h *LogisticsEventHandler) subscribeLogisticsDurable(
	js nats.JetStreamContext,
	subject string,
	durable string,
	handler func(context.Context, *sharedevents.Event) error,
) error {
	sharedevents.SubscribeQueueWithRebind(h.logger, js, logisticsStreamName, subject, durable, func(msg *nats.Msg) {
		evt, parseErr := sharedevents.FromJSON(msg.Data)
		if parseErr != nil {
			h.logger.Error("failed to parse logistics event envelope",
				zap.String("subject", subject),
				zap.Error(parseErr))
			_ = msg.Ack()
			return
		}
		// Tasks raised by POS or other services share the logistics stream. Their references are
		// not ordering orders, so they are acknowledged and skipped instead of failing and
		// redelivering, or leaving stray assignment rows behind.
		if !isOrderingTaskEvent(evt.Payload) {
			_ = msg.Ack()
			return
		}
		ctx := context.Background()
		if err := handler(ctx, evt); err != nil {
			h.logger.Error("logistics event handler error, will redeliver",
				zap.String("subject", subject),
				zap.Error(err))
			_ = msg.Nak()
			return
		}
		_ = msg.Ack()
	},
		nats.Durable(durable),
		nats.DeliverAll(),
		nats.AckExplicit(),
		nats.AckWait(30*time.Second),
		nats.MaxDeliver(5),
		nats.BindStream(logisticsStreamName),
	)
	return nil
}

// SubscribeToLogisticsEvents subscribes to logistics task events via JetStream durable consumers.
// Subjects/stream/envelope match logistics-api's publisher (shared-events aggregate "logistics" →
// subjects "logistics.task.*", wire envelope event_type + payload, on the "logistics" stream).
func (h *LogisticsEventHandler) SubscribeToLogisticsEvents(js nats.JetStreamContext) error {
	if err := ensureLogisticsStream(js); err != nil {
		return fmt.Errorf("fulfilment: ensure logistics stream: %w", err)
	}

	subs := []struct {
		subject string
		durable string
		handler func(context.Context, *sharedevents.Event) error
	}{
		{"logistics.task.created", "ord-logistics-task-created", h.handleTaskCreated},
		{"logistics.task.completed", "ord-logistics-task-completed", h.handleTaskCompleted},
		{"logistics.task.assigned", "ord-logistics-task-assigned", h.handleTaskAssigned},
		{"logistics.task.accepted", "ord-logistics-task-accepted", h.handleTaskAccepted},
		{"logistics.task.unassigned", "ord-logistics-task-unassigned", h.handleTaskUnassigned},
		{"logistics.task.en_route_pickup", "ord-logistics-task-en-route-pickup", func(ctx context.Context, evt *sharedevents.Event) error {
			return h.handleTaskLeg(ctx, evt, AssignmentStatusEnRoutePickup)
		}},
		{"logistics.task.arrived_pickup", "ord-logistics-task-arrived-pickup", func(ctx context.Context, evt *sharedevents.Event) error {
			return h.handleTaskLeg(ctx, evt, AssignmentStatusArrivedPickup)
		}},
		{"logistics.task.picked_up", "ord-logistics-task-picked-up", h.handleTaskPickedUp},
		// Legacy single-leg rider flow (accepted -> en_route -> delivered): en_route means the rider
		// left with the order, the same moment as picked_up.
		{"logistics.task.en_route", "ord-logistics-task-en-route", h.handleTaskPickedUp},
		{"logistics.task.en_route_dropoff", "ord-logistics-task-en-route-dropoff", func(ctx context.Context, evt *sharedevents.Event) error {
			return h.handleTaskLeg(ctx, evt, AssignmentStatusEnRouteDropoff)
		}},
		{"logistics.task.arrived_dropoff", "ord-logistics-task-arrived-dropoff", func(ctx context.Context, evt *sharedevents.Event) error {
			return h.handleTaskLeg(ctx, evt, AssignmentStatusArrivedDropoff)
		}},
		{"logistics.task.delivered", "ord-logistics-task-delivered", h.handleTaskDelivered},
		{"logistics.task.cancelled", "ord-logistics-task-cancelled", h.handleTaskCancelled},
		{"logistics.task.failed", "ord-logistics-task-failed", h.handleTaskFailed},
	}

	for _, s := range subs {
		if err := h.subscribeLogisticsDurable(js, s.subject, s.durable, s.handler); err != nil {
			return err
		}
	}

	subjects := make([]string, 0, len(subs))
	for _, s := range subs {
		subjects = append(subjects, s.subject)
	}
	h.logger.Info("logistics event subscriptions active (JetStream)", zap.Strings("subjects", subjects))
	return nil
}
