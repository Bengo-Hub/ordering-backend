package fulfilment

import (
	"context"

	"github.com/google/uuid"

	"github.com/bengobox/ordering-backend/internal/ent"
	"github.com/bengobox/ordering-backend/internal/ent/deliverywindow"
	"github.com/bengobox/ordering-backend/internal/ent/logisticsevent"
	"github.com/bengobox/ordering-backend/internal/ent/orderassignment"
	"github.com/bengobox/ordering-backend/internal/ent/proofofdelivery"
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

// CreateProofOfDelivery creates a new proof of delivery.
func (r *EntRepository) CreateProofOfDelivery(ctx context.Context, pod *ProofOfDelivery) error {
	builder := r.client.ProofOfDelivery.Create().
		SetTenantID(pod.TenantID).
		SetOrderID(pod.OrderID).
		SetAssignmentID(pod.AssignmentID).
		SetLogisticsTaskID(pod.LogisticsTaskID).
		SetType(proofofdelivery.Type(pod.Type)).
		SetOtpVerified(pod.OTPVerified).
		SetIsVerified(pod.IsVerified).
		SetDeliveredAt(pod.DeliveredAt)

	if pod.SignatureURL != "" {
		builder = builder.SetSignatureURL(pod.SignatureURL)
	}
	if len(pod.PhotoURLs) > 0 {
		builder = builder.SetPhotoUrls(pod.PhotoURLs)
	}
	if pod.OTPCode != "" {
		builder = builder.SetOtpCode(pod.OTPCode)
	}
	if pod.RecipientName != "" {
		builder = builder.SetRecipientName(pod.RecipientName)
	}
	if pod.RecipientRelation != "" {
		builder = builder.SetRecipientRelation(pod.RecipientRelation)
	}
	if pod.DeliveryLatitude != nil {
		builder = builder.SetDeliveryLatitude(*pod.DeliveryLatitude)
	}
	if pod.DeliveryLongitude != nil {
		builder = builder.SetDeliveryLongitude(*pod.DeliveryLongitude)
	}
	if pod.RiderNotes != "" {
		builder = builder.SetRiderNotes(pod.RiderNotes)
	}
	if pod.Metadata != nil {
		builder = builder.SetMetadata(pod.Metadata)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return err
	}

	pod.ID = created.ID
	pod.CreatedAt = created.CreatedAt
	pod.UpdatedAt = created.UpdatedAt
	return nil
}

// GetProofOfDelivery retrieves proof of delivery by order ID.
func (r *EntRepository) GetProofOfDelivery(ctx context.Context, tenantID, orderID uuid.UUID) (*ProofOfDelivery, error) {
	p, err := r.client.ProofOfDelivery.Query().
		Where(
			proofofdelivery.TenantID(tenantID),
			proofofdelivery.OrderID(orderID),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrPODNotFound
		}
		return nil, err
	}

	return entPODToDomain(p), nil
}

// GetProofOfDeliveryByAssignment retrieves proof of delivery by assignment ID.
func (r *EntRepository) GetProofOfDeliveryByAssignment(ctx context.Context, assignmentID uuid.UUID) (*ProofOfDelivery, error) {
	p, err := r.client.ProofOfDelivery.Query().
		Where(proofofdelivery.AssignmentID(assignmentID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrPODNotFound
		}
		return nil, err
	}

	return entPODToDomain(p), nil
}

// UpdateProofOfDelivery updates an existing proof of delivery.
func (r *EntRepository) UpdateProofOfDelivery(ctx context.Context, pod *ProofOfDelivery) error {
	update := r.client.ProofOfDelivery.UpdateOneID(pod.ID).
		SetIsVerified(pod.IsVerified)

	if pod.VerifiedBy != "" {
		update = update.SetVerifiedBy(pod.VerifiedBy)
	}
	if pod.VerifiedAt != nil {
		update = update.SetVerifiedAt(*pod.VerifiedAt)
	}
	if pod.CustomerRating != "" {
		update = update.SetCustomerRating(pod.CustomerRating)
	}
	if pod.CustomerFeedback != "" {
		update = update.SetCustomerFeedback(pod.CustomerFeedback)
	}

	_, err := update.Save(ctx)
	return err
}

// CreateLogisticsEvent creates a new logistics event.
func (r *EntRepository) CreateLogisticsEvent(ctx context.Context, event *LogisticsEvent) error {
	builder := r.client.LogisticsEvent.Create().
		SetExternalID(event.ExternalID).
		SetEventType(logisticsevent.EventType(event.EventType)).
		SetPayload(event.Payload).
		SetStatus(logisticsevent.Status(event.Status)).
		SetReceivedAt(event.ReceivedAt)

	if event.TenantID != nil {
		builder = builder.SetTenantID(*event.TenantID)
	}
	if event.OrderID != nil {
		builder = builder.SetOrderID(*event.OrderID)
	}
	if event.AssignmentID != nil {
		builder = builder.SetAssignmentID(*event.AssignmentID)
	}
	if event.LogisticsTaskID != "" {
		builder = builder.SetLogisticsTaskID(event.LogisticsTaskID)
	}
	if event.RiderID != "" {
		builder = builder.SetRiderID(event.RiderID)
	}
	if event.Headers != nil {
		builder = builder.SetHeaders(event.Headers)
	}
	if event.Signature != "" {
		builder = builder.SetSignature(event.Signature)
	}
	if event.SignatureValid != nil {
		builder = builder.SetSignatureValid(*event.SignatureValid)
	}
	if event.IPAddress != "" {
		builder = builder.SetIPAddress(event.IPAddress)
	}

	created, err := builder.Save(ctx)
	if err != nil {
		return err
	}

	event.ID = created.ID
	event.CreatedAt = created.CreatedAt
	event.UpdatedAt = created.UpdatedAt
	return nil
}

// GetLogisticsEventByExternalID retrieves an event by external ID.
func (r *EntRepository) GetLogisticsEventByExternalID(ctx context.Context, externalID string) (*LogisticsEvent, error) {
	e, err := r.client.LogisticsEvent.Query().
		Where(logisticsevent.ExternalID(externalID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrEventNotFound
		}
		return nil, err
	}

	return entEventToDomain(e), nil
}

// UpdateLogisticsEvent updates an existing logistics event.
func (r *EntRepository) UpdateLogisticsEvent(ctx context.Context, event *LogisticsEvent) error {
	update := r.client.LogisticsEvent.UpdateOneID(event.ID).
		SetStatus(logisticsevent.Status(event.Status)).
		SetRetryCount(event.RetryCount)

	if event.ProcessedAt != nil {
		update = update.SetProcessedAt(*event.ProcessedAt)
	}
	if event.LastRetryAt != nil {
		update = update.SetLastRetryAt(*event.LastRetryAt)
	}
	if event.ErrorMessage != "" {
		update = update.SetErrorMessage(event.ErrorMessage)
	}
	if event.ErrorCode != "" {
		update = update.SetErrorCode(event.ErrorCode)
	}

	_, err := update.Save(ctx)
	return err
}

// ListPendingLogisticsEvents lists events pending processing.
func (r *EntRepository) ListPendingLogisticsEvents(ctx context.Context, limit int) ([]LogisticsEvent, error) {
	events, err := r.client.LogisticsEvent.Query().
		Where(logisticsevent.StatusEQ(logisticsevent.StatusPending)).
		Order(ent.Asc(logisticsevent.FieldReceivedAt)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]LogisticsEvent, len(events))
	for i, e := range events {
		result[i] = *entEventToDomain(e)
	}

	return result, nil
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

func entPODToDomain(p *ent.ProofOfDelivery) *ProofOfDelivery {
	return &ProofOfDelivery{
		ID:                p.ID,
		TenantID:          p.TenantID,
		OrderID:           p.OrderID,
		AssignmentID:      p.AssignmentID,
		LogisticsTaskID:   p.LogisticsTaskID,
		Type:              PODType(p.Type),
		SignatureURL:      p.SignatureURL,
		PhotoURLs:         p.PhotoUrls,
		OTPVerified:       p.OtpVerified,
		OTPCode:           p.OtpCode,
		RecipientName:     p.RecipientName,
		RecipientRelation: p.RecipientRelation,
		DeliveryLatitude:  p.DeliveryLatitude,
		DeliveryLongitude: p.DeliveryLongitude,
		RiderNotes:        p.RiderNotes,
		CustomerRating:    p.CustomerRating,
		CustomerFeedback:  p.CustomerFeedback,
		IsVerified:        p.IsVerified,
		VerifiedBy:        p.VerifiedBy,
		Metadata:          p.Metadata,
		DeliveredAt:       p.DeliveredAt,
		VerifiedAt:        p.VerifiedAt,
		CreatedAt:         p.CreatedAt,
		UpdatedAt:         p.UpdatedAt,
	}
}

func entEventToDomain(e *ent.LogisticsEvent) *LogisticsEvent {
	return &LogisticsEvent{
		ID:              e.ID,
		TenantID:        e.TenantID,
		ExternalID:      e.ExternalID,
		EventType:       string(e.EventType),
		OrderID:         e.OrderID,
		AssignmentID:    e.AssignmentID,
		LogisticsTaskID: e.LogisticsTaskID,
		RiderID:         e.RiderID,
		Payload:         e.Payload,
		Headers:         e.Headers,
		Signature:       e.Signature,
		SignatureValid:  e.SignatureValid,
		Status:          string(e.Status),
		RetryCount:      e.RetryCount,
		LastRetryAt:     e.LastRetryAt,
		ErrorMessage:    e.ErrorMessage,
		ErrorCode:       e.ErrorCode,
		IPAddress:       e.IPAddress,
		ReceivedAt:      e.ReceivedAt,
		ProcessedAt:     e.ProcessedAt,
		CreatedAt:       e.CreatedAt,
		UpdatedAt:       e.UpdatedAt,
	}
}

// Compile-time interface check
var _ Repository = (*EntRepository)(nil)
