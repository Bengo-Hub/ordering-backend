package fulfilment

import (
	"context"

	"github.com/google/uuid"
)

// Repository defines the data access interface for fulfilment entities.
type Repository interface {
	// Order Assignment operations
	CreateAssignment(ctx context.Context, assignment *OrderAssignment) error
	GetAssignment(ctx context.Context, tenantID, id uuid.UUID) (*OrderAssignment, error)
	GetAssignmentByOrderID(ctx context.Context, tenantID, orderID uuid.UUID) (*OrderAssignment, error)
	GetAssignmentByLogisticsTaskID(ctx context.Context, taskID string) (*OrderAssignment, error)
	UpdateAssignment(ctx context.Context, assignment *OrderAssignment) error
	ListAssignments(ctx context.Context, filter AssignmentFilter) ([]OrderAssignment, int, error)

	// Delivery Window operations
	CreateDeliveryWindow(ctx context.Context, window *DeliveryWindow) error
	GetCurrentDeliveryWindow(ctx context.Context, assignmentID uuid.UUID) (*DeliveryWindow, error)
	UpdateDeliveryWindow(ctx context.Context, window *DeliveryWindow) error
	MarkPreviousWindowsNotCurrent(ctx context.Context, assignmentID uuid.UUID) error

	// Proof of Delivery operations
	CreateProofOfDelivery(ctx context.Context, pod *ProofOfDelivery) error
	GetProofOfDelivery(ctx context.Context, tenantID, orderID uuid.UUID) (*ProofOfDelivery, error)
	GetProofOfDeliveryByAssignment(ctx context.Context, assignmentID uuid.UUID) (*ProofOfDelivery, error)
	UpdateProofOfDelivery(ctx context.Context, pod *ProofOfDelivery) error

	// Logistics Event operations
	CreateLogisticsEvent(ctx context.Context, event *LogisticsEvent) error
	GetLogisticsEventByExternalID(ctx context.Context, externalID string) (*LogisticsEvent, error)
	UpdateLogisticsEvent(ctx context.Context, event *LogisticsEvent) error
	ListPendingLogisticsEvents(ctx context.Context, limit int) ([]LogisticsEvent, error)
}

// AssignmentFilter defines filtering options for listing assignments.
type AssignmentFilter struct {
	TenantID uuid.UUID
	OrderID  *uuid.UUID
	RiderID  string
	Status   []AssignmentStatus
	Limit    int
	Offset   int
}
