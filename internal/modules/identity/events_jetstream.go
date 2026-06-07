package identity

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	sharedevents "github.com/Bengo-Hub/shared-events"
	"go.uber.org/zap"
)

const (
	authStreamName     = "auth"
	authStreamSubjects = "auth.>"
	authStreamMaxAge   = 72 * time.Hour
)

// ensureAuthStream ensures the "auth" JetStream stream exists.
func ensureAuthStream(js nats.JetStreamContext) error {
	_, err := js.StreamInfo(authStreamName)
	if err == nil {
		return nil // already exists
	}

	_, err = js.AddStream(&nats.StreamConfig{
		Name:     authStreamName,
		Subjects: []string{authStreamSubjects},
		MaxAge:   authStreamMaxAge,
		Storage:  nats.FileStorage,
		Replicas: 1,
	})
	if err != nil {
		return fmt.Errorf("create auth stream: %w", err)
	}
	return nil
}

// durableConsumerConfig returns a standard push-subscriber consumer config for auth events.
func durableConsumerConfig(subject, durable string) *nats.ConsumerConfig {
	return &nats.ConsumerConfig{
		Durable:        durable,
		FilterSubject:  subject,
		DeliverPolicy:  nats.DeliverAllPolicy,
		AckPolicy:      nats.AckExplicitPolicy,
		AckWait:        30 * time.Second,
		MaxDeliver:     5,
		DeliverSubject: nats.NewInbox(),
	}
}

// subscribeAuthDurable subscribes to a single auth subject with a durable consumer.
// handler receives the parsed *sharedevents.Event and returns an error; on error the message
// is nak'd so JetStream will redeliver (up to MaxDeliver).
func (h *EventHandler) subscribeAuthDurable(
	js nats.JetStreamContext,
	subject string,
	durable string,
	handler func(context.Context, *sharedevents.Event) error,
) error {
	cfg := durableConsumerConfig(subject, durable)

	sharedevents.SubscribeWithRebind(h.logger, js, subject, func(msg *nats.Msg) {
		evt, parseErr := sharedevents.FromJSON(msg.Data)
		if parseErr != nil {
			h.logger.Error("failed to parse shared-events envelope",
				zap.String("subject", subject),
				zap.String("durable", durable),
				zap.Error(parseErr))
			_ = msg.Ack() // bad payload — don't retry forever
			return
		}

		ctx := context.Background()
		if err := handler(ctx, evt); err != nil {
			h.logger.Error("event handler error, will redeliver",
				zap.String("subject", subject),
				zap.String("durable", durable),
				zap.Error(err))
			_ = msg.Nak()
			return
		}
		_ = msg.Ack()
	},
		nats.Durable(cfg.Durable),
		nats.DeliverAll(),
		nats.AckExplicit(),
		nats.AckWait(cfg.AckWait),
		nats.MaxDeliver(cfg.MaxDeliver),
		nats.BindStream(authStreamName),
	)
	return nil
}

// SubscribeToAuthEvents subscribes to auth-service events via JetStream durable consumers.
func (h *EventHandler) SubscribeToAuthEvents(js nats.JetStreamContext) error {
	if err := ensureAuthStream(js); err != nil {
		return fmt.Errorf("identity: ensure auth stream: %w", err)
	}

	subs := []struct {
		subject string
		durable string
		handler func(context.Context, *sharedevents.Event) error
	}{
		{"auth.user.created", "ord-auth-user-created", h.HandleAuthUserCreated},
		{"auth.user.updated", "ord-auth-user-updated", h.HandleAuthUserUpdated},
		{"auth.user.deactivated", "ord-auth-user-deactivated", h.HandleAuthUserDeactivated},
		{"auth.tenant.created", "ord-auth-tenant-created", h.HandleAuthTenantCreated},
		{"auth.tenant.updated", "ord-auth-tenant-updated", h.HandleAuthTenantUpdated},
	}

	for _, s := range subs {
		if err := h.subscribeAuthDurable(js, s.subject, s.durable, s.handler); err != nil {
			return err
		}
	}

	h.logger.Info("Subscribed to auth-service events via JetStream durable consumers",
		zap.Strings("subjects", []string{
			"auth.user.created", "auth.user.updated", "auth.user.deactivated",
			"auth.tenant.created", "auth.tenant.updated",
		}))
	return nil
}
