package fulfilment

import "errors"

var (
	// Assignment errors
	ErrAssignmentNotFound       = errors.New("order assignment not found")
	ErrAssignmentAlreadyExists  = errors.New("order assignment already exists")
	ErrAssignmentNotCancellable = errors.New("order assignment cannot be cancelled in current status")
	ErrInvalidAssignmentStatus  = errors.New("invalid assignment status")

	// Order errors
	ErrOrderNotFound    = errors.New("order not found")
	ErrOrderNotReady    = errors.New("order is not ready for delivery")
	ErrInvalidOrderType = errors.New("order type does not support delivery")

	// Delivery window errors
	ErrDeliveryWindowNotFound = errors.New("delivery window not found")

	// Proof of delivery errors
	ErrPODNotFound     = errors.New("proof of delivery not found")
	ErrPODAlreadyExists = errors.New("proof of delivery already exists")

	// Logistics event errors
	ErrEventNotFound       = errors.New("logistics event not found")
	ErrEventAlreadyProcessed = errors.New("logistics event already processed")
	ErrInvalidEventType    = errors.New("invalid event type")
	ErrInvalidSignature    = errors.New("invalid webhook signature")

	// Tracking errors
	ErrTrackingNotAvailable = errors.New("tracking not available")

	// Logistics service errors
	ErrLogisticsServiceUnavailable = errors.New("logistics service unavailable")
	ErrTaskCreationFailed          = errors.New("failed to create delivery task")
)
