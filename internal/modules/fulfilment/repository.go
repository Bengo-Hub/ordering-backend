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
