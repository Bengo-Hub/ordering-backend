package fulfilment

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/platform/logistics"
)

// WebhookService handles logistics webhook events.
type WebhookService struct {
	repo          Repository
	verifier      *logistics.WebhookVerifier
	logger        *zap.Logger
	webhookSecret string
}

// NewWebhookService creates a new webhook service.
func NewWebhookService(
	repo Repository,
	webhookSecret string,
	logger *zap.Logger,
) *WebhookService {
	return &WebhookService{
		repo:          repo,
		verifier:      logistics.NewWebhookVerifier(webhookSecret),
		logger:        logger,
		webhookSecret: webhookSecret,
	}
}

// ProcessWebhook processes an incoming logistics webhook.
// Events are not stored locally; assignment state is updated only. PoD and event history live in logistics-api.
func (s *WebhookService) ProcessWebhook(ctx context.Context, payload []byte, signature, ipAddress string, headers map[string]string) error {
	// Verify signature
	if !s.verifier.VerifySignature(payload, signature) && s.webhookSecret != "" {
		s.logger.Warn("invalid webhook signature",
			zap.String("ip", ipAddress))
		return ErrInvalidSignature
	}

	// Parse event
	var event logistics.WebhookEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("parse webhook payload: %w", err)
	}

	// Process event (idempotent: duplicate events re-apply same assignment updates).
	return s.handleEvent(ctx, event)
}

func (s *WebhookService) handleEvent(ctx context.Context, event logistics.WebhookEvent) error {
	s.logger.Info("processing logistics event",
		zap.String("event_id", event.ID),
		zap.String("event_type", string(event.Type)))

	switch event.Type {
	case logistics.EventTaskAssigned:
		return s.handleTaskAssigned(ctx, event)
	case logistics.EventTaskAccepted:
		return s.handleTaskAccepted(ctx, event)
	case logistics.EventTaskRejected:
		return s.handleTaskRejected(ctx, event)
	case logistics.EventTaskEnRoutePickup:
		return s.handleTaskStatusUpdate(ctx, event, AssignmentStatusEnRoutePickup)
	case logistics.EventTaskArrivedPickup:
		return s.handleTaskStatusUpdate(ctx, event, AssignmentStatusArrivedPickup)
	case logistics.EventTaskPickedUp:
		return s.handleTaskPickedUp(ctx, event)
	case logistics.EventTaskEnRouteDropoff:
		return s.handleTaskStatusUpdate(ctx, event, AssignmentStatusEnRouteDropoff)
	case logistics.EventTaskArrivedDropoff:
		return s.handleTaskStatusUpdate(ctx, event, AssignmentStatusArrivedDropoff)
	case logistics.EventTaskCompleted:
		return s.handleTaskCompleted(ctx, event)
	case logistics.EventTaskCancelled:
		return s.handleTaskCancelled(ctx, event)
	case logistics.EventTaskFailed:
		return s.handleTaskFailed(ctx, event)
	case logistics.EventRouteUpdated, logistics.EventETAUpdated:
		return s.handleRouteUpdated(ctx, event)
	case logistics.EventPODSubmitted:
		return s.handlePODSubmitted(ctx, event)
	default:
		s.logger.Debug("unhandled event type",
			zap.String("event_type", string(event.Type)))
		return nil
	}
}

func (s *WebhookService) handleTaskAssigned(ctx context.Context, event logistics.WebhookEvent) error {
	taskData, err := logistics.ParseTaskEventData(event.Data)
	if err != nil {
		return err
	}

	assignment, err := s.repo.GetAssignmentByLogisticsTaskID(ctx, taskData.TaskID.String())
	if err != nil {
		return err
	}

	now := time.Now()
	assignment.Status = AssignmentStatusAssigned
	assignment.RiderID = taskData.RiderID
	assignment.AssignedAt = &now

	return s.repo.UpdateAssignment(ctx, assignment)
}

func (s *WebhookService) handleTaskAccepted(ctx context.Context, event logistics.WebhookEvent) error {
	taskData, err := logistics.ParseTaskEventData(event.Data)
	if err != nil {
		return err
	}

	assignment, err := s.repo.GetAssignmentByLogisticsTaskID(ctx, taskData.TaskID.String())
	if err != nil {
		return err
	}

	now := time.Now()
	assignment.Status = AssignmentStatusAccepted
	assignment.AcceptedAt = &now

	return s.repo.UpdateAssignment(ctx, assignment)
}

func (s *WebhookService) handleTaskRejected(ctx context.Context, event logistics.WebhookEvent) error {
	taskData, err := logistics.ParseTaskEventData(event.Data)
	if err != nil {
		return err
	}

	assignment, err := s.repo.GetAssignmentByLogisticsTaskID(ctx, taskData.TaskID.String())
	if err != nil {
		return err
	}

	// Reset to pending for reassignment
	assignment.Status = AssignmentStatusPending
	assignment.RejectionReason = taskData.FailureReason
	assignment.RiderID = ""
	assignment.AssignedAt = nil
	assignment.AttemptCount++

	return s.repo.UpdateAssignment(ctx, assignment)
}

func (s *WebhookService) handleTaskStatusUpdate(ctx context.Context, event logistics.WebhookEvent, status AssignmentStatus) error {
	taskData, err := logistics.ParseTaskEventData(event.Data)
	if err != nil {
		return err
	}

	assignment, err := s.repo.GetAssignmentByLogisticsTaskID(ctx, taskData.TaskID.String())
	if err != nil {
		return err
	}

	assignment.Status = status

	return s.repo.UpdateAssignment(ctx, assignment)
}

func (s *WebhookService) handleTaskPickedUp(ctx context.Context, event logistics.WebhookEvent) error {
	taskData, err := logistics.ParseTaskEventData(event.Data)
	if err != nil {
		return err
	}

	assignment, err := s.repo.GetAssignmentByLogisticsTaskID(ctx, taskData.TaskID.String())
	if err != nil {
		return err
	}

	now := time.Now()
	assignment.Status = AssignmentStatusPickedUp
	assignment.PickedUpAt = &now

	return s.repo.UpdateAssignment(ctx, assignment)
}

func (s *WebhookService) handleTaskCompleted(ctx context.Context, event logistics.WebhookEvent) error {
	taskData, err := logistics.ParseTaskEventData(event.Data)
	if err != nil {
		return err
	}

	assignment, err := s.repo.GetAssignmentByLogisticsTaskID(ctx, taskData.TaskID.String())
	if err != nil {
		return err
	}

	now := time.Now()
	assignment.Status = AssignmentStatusCompleted
	assignment.CompletedAt = &now

	return s.repo.UpdateAssignment(ctx, assignment)
}

func (s *WebhookService) handleTaskCancelled(ctx context.Context, event logistics.WebhookEvent) error {
	taskData, err := logistics.ParseTaskEventData(event.Data)
	if err != nil {
		return err
	}

	assignment, err := s.repo.GetAssignmentByLogisticsTaskID(ctx, taskData.TaskID.String())
	if err != nil {
		return err
	}

	now := time.Now()
	assignment.Status = AssignmentStatusCancelled
	assignment.CancellationReason = taskData.CancellationReason
	assignment.CancelledAt = &now

	return s.repo.UpdateAssignment(ctx, assignment)
}

func (s *WebhookService) handleTaskFailed(ctx context.Context, event logistics.WebhookEvent) error {
	taskData, err := logistics.ParseTaskEventData(event.Data)
	if err != nil {
		return err
	}

	assignment, err := s.repo.GetAssignmentByLogisticsTaskID(ctx, taskData.TaskID.String())
	if err != nil {
		return err
	}

	assignment.Status = AssignmentStatusFailed
	assignment.FailureReason = taskData.FailureReason

	return s.repo.UpdateAssignment(ctx, assignment)
}

func (s *WebhookService) handleRouteUpdated(ctx context.Context, event logistics.WebhookEvent) error {
	routeData, err := logistics.ParseRouteEventData(event.Data)
	if err != nil {
		return err
	}

	assignment, err := s.repo.GetAssignmentByLogisticsTaskID(ctx, routeData.TaskID.String())
	if err != nil {
		return err
	}

	// Mark previous windows as not current
	if err := s.repo.MarkPreviousWindowsNotCurrent(ctx, assignment.ID); err != nil {
		s.logger.Warn("failed to mark previous windows",
			zap.Error(err))
	}

	// Create new delivery window
	now := time.Now()
	window := &DeliveryWindow{
		TenantID:     assignment.TenantID,
		OrderID:      assignment.OrderID,
		AssignmentID: assignment.ID,
		ETAStart:     now,
		ETAEnd:       routeData.ETAAt,
		ETAMinutes:   &routeData.ETAMinutes,
		DistanceKm:   &routeData.DistanceRemaining,
		Source:       "logistics",
		IsCurrent:    true,
		RouteInfo: map[string]interface{}{
			"distance_remaining": routeData.DistanceRemaining,
		},
	}

	return s.repo.CreateDeliveryWindow(ctx, window)
}

// handlePODSubmitted: PoD is owned by logistics-api; we do not store it. Optionally ensure assignment is completed.
func (s *WebhookService) handlePODSubmitted(ctx context.Context, event logistics.WebhookEvent) error {
	podData, err := logistics.ParsePODEventData(event.Data)
	if err != nil {
		return err
	}

	assignment, err := s.repo.GetAssignmentByLogisticsTaskID(ctx, podData.TaskID.String())
	if err != nil {
		return err
	}

	// Ensure assignment marked completed if not already (PoD stored in logistics-api only).
	if assignment.Status != AssignmentStatusCompleted {
		now := time.Now()
		assignment.Status = AssignmentStatusCompleted
		assignment.CompletedAt = &now
		return s.repo.UpdateAssignment(ctx, assignment)
	}
	return nil
}
