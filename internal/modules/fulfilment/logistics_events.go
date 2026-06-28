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
		{"logistics.task.completed", "ord-logistics-task-completed", h.handleTaskCompleted},
		{"logistics.task.assigned", "ord-logistics-task-assigned", h.handleTaskAssigned},
		{"logistics.task.accepted", "ord-logistics-task-accepted", h.handleTaskAccepted},
		{"logistics.task.en_route", "ord-logistics-task-en-route", func(ctx context.Context, evt *sharedevents.Event) error {
			return h.handleTaskStatusUpdate(ctx, evt, "en_route")
		}},
		{"logistics.task.delivered", "ord-logistics-task-delivered", h.handleTaskDelivered},
		{"logistics.task.failed", "ord-logistics-task-failed", h.handleTaskFailed},
	}

	for _, s := range subs {
		if err := h.subscribeLogisticsDurable(js, s.subject, s.durable, s.handler); err != nil {
			return err
		}
	}

	h.logger.Info("logistics event subscriptions active (JetStream)",
		zap.Strings("subjects", []string{
			"logistics.task.completed", "logistics.task.assigned",
			"logistics.task.accepted", "logistics.task.en_route",
			"logistics.task.delivered", "logistics.task.failed",
		}))
	return nil
}
