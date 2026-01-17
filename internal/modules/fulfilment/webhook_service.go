package fulfilment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
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
func (s *WebhookService) ProcessWebhook(ctx context.Context, payload []byte, signature, ipAddress string, headers map[string]string) error {
	// Verify signature
	signatureValid := s.verifier.VerifySignature(payload, signature)
	if !signatureValid && s.webhookSecret != "" {
		s.logger.Warn("invalid webhook signature",
			zap.String("ip", ipAddress))
		return ErrInvalidSignature
	}

	// Parse event
	var event logistics.WebhookEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("parse webhook payload: %w", err)
	}

	// Check for duplicate (idempotency)
	existing, err := s.repo.GetLogisticsEventByExternalID(ctx, event.ID)
	if err == nil && existing != nil {
		if existing.Status == "processed" {
			s.logger.Debug("event already processed",
				zap.String("event_id", event.ID))
			return nil
		}
	}

	// Store event
	logEvent := &LogisticsEvent{
		ExternalID: event.ID,
		EventType:  string(event.Type),
		TenantID:   &event.TenantID,
		Payload:    event.Data,
		Headers:    headers,
		Signature:  signature,
		SignatureValid: &signatureValid,
		Status:     "pending",
		IPAddress:  ipAddress,
		ReceivedAt: time.Now(),
	}

	// Extract task ID and order ID from event data
	if taskIDStr, ok := event.Data["task_id"].(string); ok {
		logEvent.LogisticsTaskID = taskIDStr

		// Find assignment by task ID
		assignment, err := s.repo.GetAssignmentByLogisticsTaskID(ctx, taskIDStr)
		if err == nil && assignment != nil {
			logEvent.OrderID = &assignment.OrderID
			logEvent.AssignmentID = &assignment.ID
		}
	}

	if riderID, ok := event.Data["rider_id"].(string); ok {
		logEvent.RiderID = riderID
	}

	if err := s.repo.CreateLogisticsEvent(ctx, logEvent); err != nil {
		if !errors.Is(err, ErrEventAlreadyProcessed) {
			return err
		}
	}

	// Process event
	if err := s.handleEvent(ctx, event, logEvent); err != nil {
		// Update event with error
		logEvent.Status = "failed"
		logEvent.ErrorMessage = err.Error()
		logEvent.RetryCount++
		now := time.Now()
		logEvent.LastRetryAt = &now
		s.repo.UpdateLogisticsEvent(ctx, logEvent)
		return err
	}

	// Mark as processed
	logEvent.Status = "processed"
	now := time.Now()
	logEvent.ProcessedAt = &now
	s.repo.UpdateLogisticsEvent(ctx, logEvent)

	return nil
}

func (s *WebhookService) handleEvent(ctx context.Context, event logistics.WebhookEvent, logEvent *LogisticsEvent) error {
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

func (s *WebhookService) handlePODSubmitted(ctx context.Context, event logistics.WebhookEvent) error {
	podData, err := logistics.ParsePODEventData(event.Data)
	if err != nil {
		return err
	}

	assignment, err := s.repo.GetAssignmentByLogisticsTaskID(ctx, podData.TaskID.String())
	if err != nil {
		return err
	}

	// Check if POD already exists
	existing, _ := s.repo.GetProofOfDeliveryByAssignment(ctx, assignment.ID)
	if existing != nil {
		// Update existing POD
		existing.SignatureURL = podData.SignatureURL
		existing.PhotoURLs = podData.PhotoURLs
		existing.OTPVerified = podData.OTPVerified
		existing.RecipientName = podData.RecipientName
		existing.RecipientRelation = podData.RecipientRelation
		existing.RiderNotes = podData.RiderNotes
		existing.DeliveryLatitude = &podData.DeliveryLatitude
		existing.DeliveryLongitude = &podData.DeliveryLongitude
		existing.DeliveredAt = podData.DeliveredAt

		return s.repo.UpdateProofOfDelivery(ctx, existing)
	}

	// Create new POD
	pod := &ProofOfDelivery{
		TenantID:          assignment.TenantID,
		OrderID:           assignment.OrderID,
		AssignmentID:      assignment.ID,
		LogisticsTaskID:   assignment.LogisticsTaskID,
		Type:              PODType(podData.Type),
		SignatureURL:      podData.SignatureURL,
		PhotoURLs:         podData.PhotoURLs,
		OTPVerified:       podData.OTPVerified,
		RecipientName:     podData.RecipientName,
		RecipientRelation: podData.RecipientRelation,
		DeliveryLatitude:  &podData.DeliveryLatitude,
		DeliveryLongitude: &podData.DeliveryLongitude,
		RiderNotes:        podData.RiderNotes,
		DeliveredAt:       podData.DeliveredAt,
	}

	return s.repo.CreateProofOfDelivery(ctx, pod)
}

// GetProofOfDelivery retrieves proof of delivery for an order.
func (s *WebhookService) GetProofOfDelivery(ctx context.Context, tenantID, orderID uuid.UUID) (*ProofOfDelivery, error) {
	return s.repo.GetProofOfDelivery(ctx, tenantID, orderID)
}
