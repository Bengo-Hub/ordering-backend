package tenant

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/ent"
	"github.com/bengobox/ordering-backend/internal/platform/events"
)

// BranchSubscriber handles branch-related event subscriptions.
type BranchSubscriber struct {
	orm    *ent.Client
	logger *zap.Logger
}

// NewBranchSubscriber creates a new branch subscriber.
func NewBranchSubscriber(orm *ent.Client, logger *zap.Logger) *BranchSubscriber {
	return &BranchSubscriber{
		orm:    orm,
		logger: logger.Named("branch.subscriber"),
	}
}

// RegisterSubscribers registers all branch-related event handlers.
func (s *BranchSubscriber) RegisterSubscribers(sub *events.Subscriber) error {
	if err := sub.Subscribe("auth.tenant.branch.created", s.handleBranchCreated); err != nil {
		return fmt.Errorf("subscribe to branch.created: %w", err)
	}
	return nil
}

func (s *BranchSubscriber) handleBranchCreated(ctx context.Context, event events.Event) error {
	s.logger.Info("received branch created event",
		zap.String("tenant_id", event.TenantID),
		zap.Any("data", event.Data))

	tenantID, err := uuid.Parse(event.TenantID)
	if err != nil {
		return fmt.Errorf("invalid tenant_id %q: %w", event.TenantID, err)
	}

	name, _ := event.Data["name"].(string)
	useCase, _ := event.Data["use_case"].(string)

	if name == "" {
		return fmt.Errorf("branch name is missing in event data")
	}

	// Create Outlet for this branch
	_, err = s.orm.Outlet.Create().
		SetTenantID(tenantID).
		SetName(name).
		SetSlug(fmt.Sprintf("%s-%s", tenantID.String()[:8], name)). // basic slug
		SetStatus("active").
		SetUseCase(useCase).
		// We could add more fields if we had a mapping or if they were in the event
		Save(ctx)

	if err != nil {
		return fmt.Errorf("failed to create outlet: %w", err)
	}

	s.logger.Info("successfully created outlet for branch", zap.String("name", name))
	return nil
}
