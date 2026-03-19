package fulfilment

import (
	"context"

	"github.com/google/uuid"

	"github.com/bengobox/ordering-backend/internal/ent"
	"github.com/bengobox/ordering-backend/internal/ent/deliverywindow"
	"github.com/bengobox/ordering-backend/internal/ent/orderassignment"
)

// EntRepository implements Repository using Ent ORM.
type EntRepository struct {
	client *ent.Client
}

// NewEntRepository creates a new Ent-based repository.
func NewEntRepository(client *ent.Client) *EntRepository {
	return &EntRepository{client: client}
}

// CreateAssignment creates a new order assignment.
func (r *EntRepository) CreateAssignment(ctx context.Context, assignment *OrderAssignment) error {
	builder := r.client.OrderAssignment.Create().
		SetTenantID(assignment.TenantID).
		SetOrderID(assignment.OrderID).
		SetLogisticsTaskID(assignment.LogisticsTaskID).
		SetStatus(orderassignment.Status(assignment.Status)).
		SetPriority(orderassignment.Priority(assignment.Priority)).
		SetAttemptCount(assignment.AttemptCount)

	if assignment.RiderID != "" {
		builder = builder.SetRiderID(assignment.RiderID)
	}
	if assignment.SpecialInstructions != "" {
		builder = builder.SetSpecialInstructions(assignment.SpecialInstructions)
	}
	if assignment.Metadata != nil {
		builder = builder.SetMetadata(assignment.Metadata)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return err
	}

	assignment.ID = created.ID
	assignment.CreatedAt = created.CreatedAt
	assignment.UpdatedAt = created.UpdatedAt
	return nil
}

// GetAssignment retrieves an assignment by ID.
func (r *EntRepository) GetAssignment(ctx context.Context, tenantID, id uuid.UUID) (*OrderAssignment, error) {
	a, err := r.client.OrderAssignment.Query().
		Where(
			orderassignment.ID(id),
			orderassignment.TenantID(tenantID),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrAssignmentNotFound
		}
		return nil, err
	}

	return entAssignmentToDomain(a), nil
}

// GetAssignmentByOrderID retrieves the active assignment for an order.
func (r *EntRepository) GetAssignmentByOrderID(ctx context.Context, tenantID, orderID uuid.UUID) (*OrderAssignment, error) {
	a, err := r.client.OrderAssignment.Query().
		Where(
			orderassignment.TenantID(tenantID),
			orderassignment.OrderID(orderID),
			orderassignment.StatusNotIn(
				orderassignment.StatusCancelled,
				orderassignment.StatusFailed,
			),
		).
		Order(ent.Desc(orderassignment.FieldCreatedAt)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrAssignmentNotFound
		}
		return nil, err
	}

	return entAssignmentToDomain(a), nil
}

// GetAssignmentByLogisticsTaskID retrieves an assignment by logistics task ID.
func (r *EntRepository) GetAssignmentByLogisticsTaskID(ctx context.Context, taskID string) (*OrderAssignment, error) {
	a, err := r.client.OrderAssignment.Query().
		Where(orderassignment.LogisticsTaskID(taskID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrAssignmentNotFound
		}
		return nil, err
	}

	return entAssignmentToDomain(a), nil
}

// UpdateAssignment updates an existing assignment.
func (r *EntRepository) UpdateAssignment(ctx context.Context, assignment *OrderAssignment) error {
	update := r.client.OrderAssignment.UpdateOneID(assignment.ID).
		SetStatus(orderassignment.Status(assignment.Status)).
		SetAttemptCount(assignment.AttemptCount)

	if assignment.RiderID != "" {
		update = update.SetRiderID(assignment.RiderID)
	}
	if assignment.RejectionReason != "" {
		update = update.SetRejectionReason(assignment.RejectionReason)
	}
	if assignment.CancellationReason != "" {
		update = update.SetCancellationReason(assignment.CancellationReason)
	}
	if assignment.FailureReason != "" {
		update = update.SetFailureReason(assignment.FailureReason)
	}
	if assignment.Metadata != nil {
		update = update.SetMetadata(assignment.Metadata)
	}
	if assignment.AssignedAt != nil {
		update = update.SetAssignedAt(*assignment.AssignedAt)
	}
	if assignment.AcceptedAt != nil {
		update = update.SetAcceptedAt(*assignment.AcceptedAt)
	}
	if assignment.PickedUpAt != nil {
		update = update.SetPickedUpAt(*assignment.PickedUpAt)
	}
	if assignment.CompletedAt != nil {
		update = update.SetCompletedAt(*assignment.CompletedAt)
	}
	if assignment.CancelledAt != nil {
		update = update.SetCancelledAt(*assignment.CancelledAt)
	}

	_, err := update.Save(ctx)
	return err
}

// ListAssignments lists assignments with filters.
func (r *EntRepository) ListAssignments(ctx context.Context, filter AssignmentFilter) ([]OrderAssignment, int, error) {
	query := r.client.OrderAssignment.Query().
		Where(orderassignment.TenantID(filter.TenantID))

	if filter.OrderID != nil {
		query = query.Where(orderassignment.OrderID(*filter.OrderID))
	}
	if filter.RiderID != "" {
		query = query.Where(orderassignment.RiderID(filter.RiderID))
	}
	if len(filter.Status) > 0 {
		statuses := make([]orderassignment.Status, len(filter.Status))
		for i, s := range filter.Status {
			statuses[i] = orderassignment.Status(s)
		}
		query = query.Where(orderassignment.StatusIn(statuses...))
	}

	total, err := query.Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}

	assignments, err := query.Order(ent.Desc(orderassignment.FieldCreatedAt)).All(ctx)
	if err != nil {
		return nil, 0, err
	}

	result := make([]OrderAssignment, len(assignments))
	for i, a := range assignments {
		result[i] = *entAssignmentToDomain(a)
	}

	return result, total, nil
}

// CreateDeliveryWindow creates a new delivery window.
func (r *EntRepository) CreateDeliveryWindow(ctx context.Context, window *DeliveryWindow) error {
	builder := r.client.DeliveryWindow.Create().
		SetTenantID(window.TenantID).
		SetOrderID(window.OrderID).
		SetAssignmentID(window.AssignmentID).
		SetEtaStart(window.ETAStart).
		SetEtaEnd(window.ETAEnd).
		SetSource(window.Source).
		SetIsCurrent(window.IsCurrent)

	if window.ETAMinutes != nil {
		builder = builder.SetEtaMinutes(*window.ETAMinutes)
	}
	if window.DistanceKm != nil {
		builder = builder.SetDistanceKm(*window.DistanceKm)
	}
	if window.RouteInfo != nil {
		builder = builder.SetRouteInfo(window.RouteInfo)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return err
	}

	window.ID = created.ID
	window.CreatedAt = created.CreatedAt
	window.UpdatedAt = created.UpdatedAt
	return nil
}

// GetCurrentDeliveryWindow retrieves the current active delivery window.
func (r *EntRepository) GetCurrentDeliveryWindow(ctx context.Context, assignmentID uuid.UUID) (*DeliveryWindow, error) {
	w, err := r.client.DeliveryWindow.Query().
		Where(
			deliverywindow.AssignmentID(assignmentID),
			deliverywindow.IsCurrent(true),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrDeliveryWindowNotFound
		}
		return nil, err
	}

	return entDeliveryWindowToDomain(w), nil
}

// UpdateDeliveryWindow updates an existing delivery window.
func (r *EntRepository) UpdateDeliveryWindow(ctx context.Context, window *DeliveryWindow) error {
	update := r.client.DeliveryWindow.UpdateOneID(window.ID).
		SetEtaStart(window.ETAStart).
		SetEtaEnd(window.ETAEnd).
		SetIsCurrent(window.IsCurrent)

	if window.ETAMinutes != nil {
		update = update.SetEtaMinutes(*window.ETAMinutes)
	}
	if window.DistanceKm != nil {
		update = update.SetDistanceKm(*window.DistanceKm)
	}
	if window.ActualArrival != nil {
		update = update.SetActualArrival(*window.ActualArrival)
	}
	if window.ActualDropoff != nil {
		update = update.SetActualDropoff(*window.ActualDropoff)
	}
	if window.RouteInfo != nil {
		update = update.SetRouteInfo(window.RouteInfo)
	}

	_, err := update.Save(ctx)
	return err
}

// MarkPreviousWindowsNotCurrent marks all previous windows as not current.
func (r *EntRepository) MarkPreviousWindowsNotCurrent(ctx context.Context, assignmentID uuid.UUID) error {
	_, err := r.client.DeliveryWindow.Update().
		Where(
			deliverywindow.AssignmentID(assignmentID),
			deliverywindow.IsCurrent(true),
		).
		SetIsCurrent(false).
		Save(ctx)
	return err
}

// Helper functions for entity conversion

func entAssignmentToDomain(a *ent.OrderAssignment) *OrderAssignment {
	return &OrderAssignment{
		ID:                  a.ID,
		TenantID:            a.TenantID,
		OrderID:             a.OrderID,
		LogisticsTaskID:     a.LogisticsTaskID,
		RiderID:             a.RiderID,
		Status:              AssignmentStatus(a.Status),
		Priority:            TaskPriority(a.Priority),
		SpecialInstructions: a.SpecialInstructions,
		RejectionReason:     a.RejectionReason,
		CancellationReason:  a.CancellationReason,
		FailureReason:       a.FailureReason,
		AttemptCount:        a.AttemptCount,
		Metadata:            a.Metadata,
		AssignedAt:          a.AssignedAt,
		AcceptedAt:          a.AcceptedAt,
		PickedUpAt:          a.PickedUpAt,
		CompletedAt:         a.CompletedAt,
		CancelledAt:         a.CancelledAt,
		CreatedAt:           a.CreatedAt,
		UpdatedAt:           a.UpdatedAt,
	}
}

func entDeliveryWindowToDomain(w *ent.DeliveryWindow) *DeliveryWindow {
	return &DeliveryWindow{
		ID:            w.ID,
		TenantID:      w.TenantID,
		OrderID:       w.OrderID,
		AssignmentID:  w.AssignmentID,
		ETAStart:      w.EtaStart,
		ETAEnd:        w.EtaEnd,
		ETAMinutes:    w.EtaMinutes,
		DistanceKm:    w.DistanceKm,
		ActualArrival: w.ActualArrival,
		ActualDropoff: w.ActualDropoff,
		Source:        w.Source,
		IsCurrent:     w.IsCurrent,
		RouteInfo:     w.RouteInfo,
		CreatedAt:     w.CreatedAt,
		UpdatedAt:     w.UpdatedAt,
	}
}

// Compile-time interface check
var _ Repository = (*EntRepository)(nil)
