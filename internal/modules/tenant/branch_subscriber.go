package tenant

import (
	"context"
	"fmt"
	"strings"
	"time"

	sharedevents "github.com/Bengo-Hub/shared-events"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/ent"
	entoutlet "github.com/bengobox/ordering-backend/internal/ent/outlet"
)

const authStream = "auth"

// orderingAcceptedUseCases is the set of outlet use_cases that ordering supports.
// Logistics hubs, warehouses, and weighbridge stations do not place orders.
var orderingAcceptedUseCases = map[string]bool{
	"hospitality":  true,
	"retail":       true,
	"quick_service": true,
}

// BranchSubscriber syncs auth.outlet.* JetStream events from auth-api into the
// local ordering-backend outlets table. Replaces the legacy plain-NATS
// auth.tenant.branch.created subscription.
type BranchSubscriber struct {
	orm    *ent.Client
	logger *zap.Logger
}

// NewBranchSubscriber creates a new BranchSubscriber.
func NewBranchSubscriber(orm *ent.Client, logger *zap.Logger) *BranchSubscriber {
	return &BranchSubscriber{
		orm:    orm,
		logger: logger.Named("tenant.branch_subscriber"),
	}
}

// Start subscribes to auth.outlet.* via JetStream durable consumers.
// This aligns with the POS and inventory canonical pattern.
func (s *BranchSubscriber) Start(nc *nats.Conn) error {
	if nc == nil {
		s.logger.Warn("NATS not available, skipping outlet event subscriptions")
		return nil
	}

	js, err := nc.JetStream()
	if err != nil {
		return fmt.Errorf("outlet events: jetstream init: %w", err)
	}

	// Ensure the auth stream exists (guard against startup race with auth-api).
	if _, err := js.StreamInfo(authStream); err != nil {
		if _, addErr := js.AddStream(&nats.StreamConfig{
			Name:      authStream,
			Subjects:  []string{"auth.>"},
			Retention: nats.LimitsPolicy,
			MaxAge:    72 * time.Hour,
			Storage:   nats.FileStorage,
		}); addErr != nil && addErr != nats.ErrStreamNameAlreadyInUse {
			s.logger.Warn("outlet events: ensure auth stream failed", zap.Error(addErr))
		}
	}

	type sub struct {
		subject string
		durable string
		handler func(context.Context, *sharedevents.Event) error
	}
	subs := []sub{
		{"auth.outlet.created", "ordering-auth-outlet-created", s.handleUpsert},
		{"auth.outlet.updated", "ordering-auth-outlet-updated", s.handleUpsert},
		{"auth.outlet.archived", "ordering-auth-outlet-archived", s.handleArchive},
	}

	for _, cfg := range subs {
		cfg := cfg
		if _, subErr := js.Subscribe(cfg.subject, func(msg *nats.Msg) {
			evt, err := sharedevents.FromJSON(msg.Data)
			if err != nil {
				s.logger.Error("failed to unmarshal outlet event",
					zap.String("subject", cfg.subject), zap.Error(err))
				_ = msg.Nak()
				return
			}
			if err := cfg.handler(context.Background(), evt); err != nil {
				s.logger.Error("failed to handle outlet event",
					zap.String("subject", cfg.subject), zap.Error(err))
				_ = msg.Nak()
				return
			}
			_ = msg.Ack()
		},
			nats.Durable(cfg.durable),
			nats.AckExplicit(),
			nats.AckWait(30*time.Second),
			nats.MaxDeliver(5),
			nats.DeliverAll(),
		); subErr != nil {
			s.logger.Warn("outlet events: subscribe failed",
				zap.String("subject", cfg.subject), zap.Error(subErr))
		}
	}

	s.logger.Info("outlet event subscriptions active",
		zap.String("subjects", "auth.outlet.created, auth.outlet.updated, auth.outlet.archived"))
	return nil
}

// handleUpsert creates or updates a local outlet projection from auth.outlet.created/updated.
func (s *BranchSubscriber) handleUpsert(ctx context.Context, evt *sharedevents.Event) error {
	outletIDStr, _ := evt.Payload["outlet_id"].(string)
	code, _ := evt.Payload["code"].(string)
	name, _ := evt.Payload["name"].(string)
	useCase, _ := evt.Payload["use_case"].(string)
	isHQ, _ := evt.Payload["is_hq"].(bool)
	status, _ := evt.Payload["status"].(string)
	address, _ := evt.Payload["address"].(string)
	if status == "" {
		status = "active"
	}

	// Only sync outlet types that ordering supports.
	if useCase != "" && !orderingAcceptedUseCases[useCase] {
		s.logger.Info("skipping outlet: use_case not applicable to ordering",
			zap.String("outlet_id", outletIDStr),
			zap.String("use_case", useCase))
		return nil
	}

	outletID, err := uuid.Parse(outletIDStr)
	if err != nil {
		return fmt.Errorf("invalid outlet_id %q: %w", outletIDStr, err)
	}
	if evt.TenantID == uuid.Nil {
		return fmt.Errorf("missing tenant_id in outlet event")
	}

	// slug is derived from code so it satisfies the (tenant_id, slug) unique index.
	slug := strings.ToLower(strings.ReplaceAll(code, " ", "-"))
	if slug == "" {
		slug = outletID.String()[:8]
	}

	existing, err := s.orm.Outlet.Get(ctx, outletID)
	if err != nil {
		// Create new outlet projection using the deterministic UUID from auth-api.
		create := s.orm.Outlet.Create().
			SetID(outletID).
			SetTenantID(evt.TenantID).
			SetName(name).
			SetSlug(slug).
			SetStatus(status).
			SetSupportsPickup(!isHQ) // non-HQ outlets typically support pickup
		if useCase != "" {
			create = create.SetUseCase(useCase)
		}
		if address != "" {
			create = create.SetAddress(address)
		}
		if _, createErr := create.Save(ctx); createErr != nil {
			return fmt.Errorf("create outlet projection: %w", createErr)
		}
		s.logger.Info("outlet created from auth event",
			zap.String("outlet_id", outletIDStr), zap.String("code", code))
		return nil
	}

	upd := s.orm.Outlet.UpdateOne(existing).
		SetName(name).
		SetStatus(status)
	if useCase != "" {
		upd = upd.SetUseCase(useCase)
	}
	if address != "" {
		upd = upd.SetAddress(address)
	}
	if _, updErr := upd.Save(ctx); updErr != nil {
		return fmt.Errorf("update outlet projection: %w", updErr)
	}
	s.logger.Info("outlet updated from auth event",
		zap.String("outlet_id", outletIDStr), zap.String("code", code))
	return nil
}

// handleArchive sets status = "archived" for the outlet.
// If the outlet was never synced (filtered by use_case), this is a no-op.
func (s *BranchSubscriber) handleArchive(ctx context.Context, evt *sharedevents.Event) error {
	outletIDStr, _ := evt.Payload["outlet_id"].(string)
	outletID, err := uuid.Parse(outletIDStr)
	if err != nil {
		return fmt.Errorf("invalid outlet_id %q: %w", outletIDStr, err)
	}

	n, err := s.orm.Outlet.Update().
		Where(entoutlet.ID(outletID), entoutlet.TenantID(evt.TenantID)).
		SetStatus("archived").
		Save(ctx)
	if err != nil {
		return fmt.Errorf("archive outlet projection: %w", err)
	}
	if n > 0 {
		s.logger.Info("outlet archived from auth event", zap.String("outlet_id", outletIDStr))
	}
	return nil // n==0 means outlet was never synced — safe to ignore
}
