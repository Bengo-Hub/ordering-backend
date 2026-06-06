package fulfilment

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/platform/logistics"
)

// OrderInfo contains order information needed for task creation.
type OrderInfo struct {
	ID              uuid.UUID
	TenantID        uuid.UUID
	TenantSlug      string
	OrderNumber     string
	CustomerName    string
	CustomerPhone   string
	PickupAddress   string
	PickupLat       float64
	PickupLng       float64
	PickupNotes     string
	DeliveryAddress string
	DeliveryLat     float64
	DeliveryLng     float64
	DeliveryNotes   string
	Instructions    string
	ItemCount       int
	ItemsDescription string
	// PODCode is the 6-digit proof-of-delivery confirmation code; surfaced to logistics via task metadata.
	PODCode         string
}

// CreateDeliveryTaskRequest represents a request to create a delivery task.
type CreateDeliveryTaskRequest struct {
	OrderInfo      OrderInfo
	Priority       TaskPriority
	IdempotencyKey string
}

// TaskService provides delivery task business logic.
type TaskService struct {
	repo            Repository
	logisticsClient *logistics.Client
	logger          *zap.Logger
}

// NewTaskService creates a new task service.
func NewTaskService(
	repo Repository,
	logisticsClient *logistics.Client,
	logger *zap.Logger,
) *TaskService {
	return &TaskService{
		repo:            repo,
		logisticsClient: logisticsClient,
		logger:          logger,
	}
}

// CreateDeliveryTask creates a delivery task for an order.
func (s *TaskService) CreateDeliveryTask(ctx context.Context, req CreateDeliveryTaskRequest) (*OrderAssignment, error) {
	// Check if assignment already exists
	existing, err := s.repo.GetAssignmentByOrderID(ctx, req.OrderInfo.TenantID, req.OrderInfo.ID)
	if err == nil && existing != nil {
		// Already has an active assignment
		return existing, nil
	}

	// Create task in logistics service
	logisticsReq := logistics.CreateTaskRequest{
		ExternalReference: req.OrderInfo.ID.String(),
		SourceService:     "ordering",
		TaskType:          "delivery",
		Priority:          logistics.TaskPriorityToInt(logistics.TaskPriority(req.Priority)),
		PickupAddress:     req.OrderInfo.PickupAddress,
		PickupLat:         req.OrderInfo.PickupLat,
		PickupLng:         req.OrderInfo.PickupLng,
		PickupContact:     "Kitchen",
		DropoffAddress:    req.OrderInfo.DeliveryAddress,
		DropoffLat:        req.OrderInfo.DeliveryLat,
		DropoffLng:        req.OrderInfo.DeliveryLng,
		DropoffContact:    req.OrderInfo.CustomerName,
		Instructions:      req.OrderInfo.Instructions,
		CustomerName:      req.OrderInfo.CustomerName,
		CustomerPhone:     req.OrderInfo.CustomerPhone,
		IdempotencyKey:    req.IdempotencyKey,
		Metadata: map[string]interface{}{
			"order_number":      req.OrderInfo.OrderNumber,
			"items_description": req.OrderInfo.ItemsDescription,
			"item_count":        req.OrderInfo.ItemCount,
			"pod_code":          req.OrderInfo.PODCode,
		},
	}

	taskResp, err := s.logisticsClient.CreateTask(ctx, req.OrderInfo.TenantSlug, logisticsReq)
	if err != nil {
		s.logger.Error("failed to create delivery task in logistics",
			zap.Error(err),
			zap.String("order_id", req.OrderInfo.ID.String()))
		return nil, fmt.Errorf("%w: %v", ErrTaskCreationFailed, err)
	}

	// Create local assignment record
	assignment := &OrderAssignment{
		TenantID:            req.OrderInfo.TenantID,
		OrderID:             req.OrderInfo.ID,
		LogisticsTaskID:     taskResp.ID.String(),
		Status:              AssignmentStatus(taskResp.Status),
		Priority:            req.Priority,
		SpecialInstructions: req.OrderInfo.Instructions,
		AttemptCount:        1,
	}

	if err := s.repo.CreateAssignment(ctx, assignment); err != nil {
		return nil, err
	}

	// Create initial delivery window if ETA is available
	if taskResp.ETAAt != nil {
		etaMinutes := taskResp.ETAMinutes
		window := &DeliveryWindow{
			TenantID:     req.OrderInfo.TenantID,
			OrderID:      req.OrderInfo.ID,
			AssignmentID: assignment.ID,
			ETAStart:     time.Now(),
			ETAEnd:       *taskResp.ETAAt,
			ETAMinutes:   &etaMinutes,
			DistanceKm:   &taskResp.DistanceKm,
			Source:       "logistics",
			IsCurrent:    true,
		}

		if err := s.repo.CreateDeliveryWindow(ctx, window); err != nil {
			s.logger.Warn("failed to create delivery window",
				zap.Error(err),
				zap.String("assignment_id", assignment.ID.String()))
		}
	}

	s.logger.Info("delivery task created",
		zap.String("assignment_id", assignment.ID.String()),
		zap.String("order_id", req.OrderInfo.ID.String()),
		zap.String("logistics_task_id", taskResp.ID.String()))

	return assignment, nil
}

// GetAssignment retrieves an assignment by ID.
func (s *TaskService) GetAssignment(ctx context.Context, tenantID, id uuid.UUID) (*OrderAssignment, error) {
	return s.repo.GetAssignment(ctx, tenantID, id)
}

// GetAssignmentByOrderID retrieves the active assignment for an order.
func (s *TaskService) GetAssignmentByOrderID(ctx context.Context, tenantID, orderID uuid.UUID) (*OrderAssignment, error) {
	return s.repo.GetAssignmentByOrderID(ctx, tenantID, orderID)
}

// ListAssignments lists assignments with filters.
func (s *TaskService) ListAssignments(ctx context.Context, filter AssignmentFilter) ([]OrderAssignment, int, error) {
	return s.repo.ListAssignments(ctx, filter)
}

// CancelDeliveryTask cancels a delivery task.
func (s *TaskService) CancelDeliveryTask(ctx context.Context, tenantSlug string, tenantID, orderID uuid.UUID, reason string) error {
	assignment, err := s.repo.GetAssignmentByOrderID(ctx, tenantID, orderID)
	if err != nil {
		return err
	}

	// Check if cancellable
	if !s.isCancellableStatus(assignment.Status) {
		return ErrAssignmentNotCancellable
	}

	// Cancel in logistics service
	taskID, err := uuid.Parse(assignment.LogisticsTaskID)
	if err == nil {
		if err := s.logisticsClient.CancelTask(ctx, tenantSlug, taskID, reason); err != nil {
			s.logger.Warn("failed to cancel task in logistics",
				zap.Error(err),
				zap.String("task_id", assignment.LogisticsTaskID))
		}
	}

	// Update local status
	now := time.Now()
	assignment.Status = AssignmentStatusCancelled
	assignment.CancellationReason = reason
	assignment.CancelledAt = &now

	return s.repo.UpdateAssignment(ctx, assignment)
}

// GetTracking retrieves real-time tracking information.
func (s *TaskService) GetTracking(ctx context.Context, tenantSlug string, tenantID, orderID uuid.UUID) (*TrackingInfo, error) {
	assignment, err := s.repo.GetAssignmentByOrderID(ctx, tenantID, orderID)
	if err != nil {
		return nil, err
	}

	// Get tracking from logistics service
	taskID, err := uuid.Parse(assignment.LogisticsTaskID)
	if err != nil {
		return nil, ErrTrackingNotAvailable
	}

	logisticsTracking, err := s.logisticsClient.GetTracking(ctx, tenantSlug, taskID)
	if err != nil {
		s.logger.Warn("failed to get tracking from logistics",
			zap.Error(err),
			zap.String("task_id", assignment.LogisticsTaskID))
		// Return cached data if logistics unavailable
		return s.buildCachedTracking(assignment)
	}

	tracking := &TrackingInfo{
		AssignmentID:  assignment.ID,
		OrderID:       orderID,
		Status:        AssignmentStatus(logisticsTracking.Status),
		ETAMinutes:    &logisticsTracking.ETAMinutes,
		ETAAt:         logisticsTracking.ETAAt,
		DistanceKm:    &logisticsTracking.DistanceKm,
		LastUpdatedAt: logisticsTracking.LastUpdatedAt,
	}

	if logisticsTracking.RiderLocation != nil {
		tracking.RiderID = logisticsTracking.RiderLocation.RiderID
		tracking.RiderLatitude = &logisticsTracking.RiderLocation.Latitude
		tracking.RiderLongitude = &logisticsTracking.RiderLocation.Longitude
	}

	return tracking, nil
}

// AssignRider assigns a fleet member (rider) to the delivery task for an order.
// If no task exists yet, ErrAssignmentNotFound is returned — caller should create the task first.
func (s *TaskService) AssignRider(ctx context.Context, tenantSlug string, tenantID, orderID uuid.UUID, fleetMemberID string) (*OrderAssignment, error) {
	assignment, err := s.repo.GetAssignmentByOrderID(ctx, tenantID, orderID)
	if err != nil {
		return nil, err
	}

	taskID, err := uuid.Parse(assignment.LogisticsTaskID)
	if err != nil {
		return nil, fmt.Errorf("invalid logistics task ID: %w", err)
	}

	taskResp, err := s.logisticsClient.AssignTask(ctx, tenantSlug, taskID, fleetMemberID, orderID.String())
	if err != nil {
		s.logger.Error("failed to assign rider via logistics",
			zap.Error(err),
			zap.String("task_id", assignment.LogisticsTaskID),
			zap.String("fleet_member_id", fleetMemberID))
		return nil, fmt.Errorf("assign rider failed: %w", err)
	}

	now := time.Now()
	assignment.RiderID = fleetMemberID
	assignment.Status = AssignmentStatus(taskResp.Status)
	assignment.AssignedAt = &now

	if err := s.repo.UpdateAssignment(ctx, assignment); err != nil {
		s.logger.Warn("failed to update local assignment after rider assign",
			zap.Error(err),
			zap.String("assignment_id", assignment.ID.String()))
	}

	return assignment, nil
}

// GetDeliveryWindow retrieves the current delivery window.
func (s *TaskService) GetDeliveryWindow(ctx context.Context, assignmentID uuid.UUID) (*DeliveryWindow, error) {
	return s.repo.GetCurrentDeliveryWindow(ctx, assignmentID)
}

func (s *TaskService) isCancellableStatus(status AssignmentStatus) bool {
	switch status {
	case AssignmentStatusPending,
		AssignmentStatusAssigned,
		AssignmentStatusAccepted:
		return true
	default:
		return false
	}
}

func (s *TaskService) buildCachedTracking(assignment *OrderAssignment) (*TrackingInfo, error) {
	tracking := &TrackingInfo{
		AssignmentID:  assignment.ID,
		OrderID:       assignment.OrderID,
		Status:        assignment.Status,
		RiderID:       assignment.RiderID,
		LastUpdatedAt: assignment.UpdatedAt,
	}

	return tracking, nil
}
