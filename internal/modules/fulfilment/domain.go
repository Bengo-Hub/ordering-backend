package fulfilment

import (
	"time"

	"github.com/google/uuid"
)

// AssignmentStatus represents the status of an order assignment.
type AssignmentStatus string

const (
	AssignmentStatusPending         AssignmentStatus = "pending"
	AssignmentStatusAssigned        AssignmentStatus = "assigned"
	AssignmentStatusAccepted        AssignmentStatus = "accepted"
	AssignmentStatusEnRoutePickup   AssignmentStatus = "en_route_pickup"
	AssignmentStatusArrivedPickup   AssignmentStatus = "arrived_pickup"
	AssignmentStatusPickedUp        AssignmentStatus = "picked_up"
	AssignmentStatusEnRouteDropoff  AssignmentStatus = "en_route_dropoff"
	AssignmentStatusArrivedDropoff  AssignmentStatus = "arrived_dropoff"
	AssignmentStatusCompleted       AssignmentStatus = "completed"
	AssignmentStatusCancelled       AssignmentStatus = "cancelled"
	AssignmentStatusFailed          AssignmentStatus = "failed"
)

// TaskPriority represents delivery priority levels.
type TaskPriority string

const (
	PriorityLow    TaskPriority = "low"
	PriorityNormal TaskPriority = "normal"
	PriorityHigh   TaskPriority = "high"
	PriorityUrgent TaskPriority = "urgent"
)

// PODType represents types of proof of delivery.
type PODType string

const (
	PODTypeSignature     PODType = "signature"
	PODTypePhoto         PODType = "photo"
	PODTypeOTP           PODType = "otp"
	PODTypePIN           PODType = "pin"
	PODTypeContactless   PODType = "contactless"
	PODTypeRecipientName PODType = "recipient_name"
)

// OrderAssignment represents a delivery assignment for an order.
type OrderAssignment struct {
	ID                  uuid.UUID              `json:"id"`
	TenantID            uuid.UUID              `json:"tenant_id"`
	OrderID             uuid.UUID              `json:"order_id"`
	LogisticsTaskID     string                 `json:"logistics_task_id"`
	RiderID             string                 `json:"rider_id,omitempty"`
	Status              AssignmentStatus       `json:"status"`
	Priority            TaskPriority           `json:"priority"`
	SpecialInstructions string                 `json:"special_instructions,omitempty"`
	RejectionReason     string                 `json:"rejection_reason,omitempty"`
	CancellationReason  string                 `json:"cancellation_reason,omitempty"`
	FailureReason       string                 `json:"failure_reason,omitempty"`
	AttemptCount        int                    `json:"attempt_count"`
	Metadata            map[string]interface{} `json:"metadata,omitempty"`
	AssignedAt          *time.Time             `json:"assigned_at,omitempty"`
	AcceptedAt          *time.Time             `json:"accepted_at,omitempty"`
	PickedUpAt          *time.Time             `json:"picked_up_at,omitempty"`
	CompletedAt         *time.Time             `json:"completed_at,omitempty"`
	CancelledAt         *time.Time             `json:"cancelled_at,omitempty"`
	CreatedAt           time.Time              `json:"created_at"`
	UpdatedAt           time.Time              `json:"updated_at"`
}

// DeliveryWindow represents an ETA window for delivery.
type DeliveryWindow struct {
	ID            uuid.UUID              `json:"id"`
	TenantID      uuid.UUID              `json:"tenant_id"`
	OrderID       uuid.UUID              `json:"order_id"`
	AssignmentID  uuid.UUID              `json:"assignment_id"`
	ETAStart      time.Time              `json:"eta_start"`
	ETAEnd        time.Time              `json:"eta_end"`
	ETAMinutes    *int                   `json:"eta_minutes,omitempty"`
	DistanceKm    *float64               `json:"distance_km,omitempty"`
	ActualArrival *time.Time             `json:"actual_arrival,omitempty"`
	ActualDropoff *time.Time             `json:"actual_dropoff,omitempty"`
	Source        string                 `json:"source"`
	IsCurrent     bool                   `json:"is_current"`
	RouteInfo     map[string]interface{} `json:"route_info,omitempty"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
}

// ProofOfDelivery represents proof of delivery artifacts.
type ProofOfDelivery struct {
	ID                uuid.UUID              `json:"id"`
	TenantID          uuid.UUID              `json:"tenant_id"`
	OrderID           uuid.UUID              `json:"order_id"`
	AssignmentID      uuid.UUID              `json:"assignment_id"`
	LogisticsTaskID   string                 `json:"logistics_task_id"`
	Type              PODType                `json:"type"`
	SignatureURL      string                 `json:"signature_url,omitempty"`
	PhotoURLs         []string               `json:"photo_urls,omitempty"`
	OTPVerified       bool                   `json:"otp_verified"`
	OTPCode           string                 `json:"otp_code,omitempty"`
	RecipientName     string                 `json:"recipient_name,omitempty"`
	RecipientRelation string                 `json:"recipient_relation,omitempty"`
	DeliveryLatitude  *float64               `json:"delivery_latitude,omitempty"`
	DeliveryLongitude *float64               `json:"delivery_longitude,omitempty"`
	RiderNotes        string                 `json:"rider_notes,omitempty"`
	CustomerRating    string                 `json:"customer_rating,omitempty"`
	CustomerFeedback  string                 `json:"customer_feedback,omitempty"`
	IsVerified        bool                   `json:"is_verified"`
	VerifiedBy        string                 `json:"verified_by,omitempty"`
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
	DeliveredAt       time.Time              `json:"delivered_at"`
	VerifiedAt        *time.Time             `json:"verified_at,omitempty"`
	CreatedAt         time.Time              `json:"created_at"`
	UpdatedAt         time.Time              `json:"updated_at"`
}

// LogisticsEvent represents an event received from logistics service.
type LogisticsEvent struct {
	ID              uuid.UUID              `json:"id"`
	TenantID        *uuid.UUID             `json:"tenant_id,omitempty"`
	ExternalID      string                 `json:"external_id"`
	EventType       string                 `json:"event_type"`
	OrderID         *uuid.UUID             `json:"order_id,omitempty"`
	AssignmentID    *uuid.UUID             `json:"assignment_id,omitempty"`
	LogisticsTaskID string                 `json:"logistics_task_id,omitempty"`
	RiderID         string                 `json:"rider_id,omitempty"`
	Payload         map[string]interface{} `json:"payload"`
	Headers         map[string]string      `json:"headers,omitempty"`
	Signature       string                 `json:"signature,omitempty"`
	SignatureValid  *bool                  `json:"signature_valid,omitempty"`
	Status          string                 `json:"status"`
	RetryCount      int                    `json:"retry_count"`
	LastRetryAt     *time.Time             `json:"last_retry_at,omitempty"`
	ErrorMessage    string                 `json:"error_message,omitempty"`
	ErrorCode       string                 `json:"error_code,omitempty"`
	IPAddress       string                 `json:"ip_address,omitempty"`
	ReceivedAt      time.Time              `json:"received_at"`
	ProcessedAt     *time.Time             `json:"processed_at,omitempty"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
}

// TrackingInfo represents real-time tracking information.
type TrackingInfo struct {
	AssignmentID   uuid.UUID    `json:"assignment_id"`
	OrderID        uuid.UUID    `json:"order_id"`
	Status         AssignmentStatus `json:"status"`
	RiderID        string       `json:"rider_id,omitempty"`
	RiderName      string       `json:"rider_name,omitempty"`
	RiderPhone     string       `json:"rider_phone,omitempty"`
	RiderLatitude  *float64     `json:"rider_latitude,omitempty"`
	RiderLongitude *float64     `json:"rider_longitude,omitempty"`
	ETAMinutes     *int         `json:"eta_minutes,omitempty"`
	ETAAt          *time.Time   `json:"eta_at,omitempty"`
	DistanceKm     *float64     `json:"distance_km,omitempty"`
	LastUpdatedAt  time.Time    `json:"last_updated_at"`
}

// RiderInfo represents rider information from logistics service.
type RiderInfo struct {
	RiderID     string  `json:"rider_id"`
	Name        string  `json:"name"`
	Phone       string  `json:"phone"`
	PhotoURL    string  `json:"photo_url,omitempty"`
	Rating      float64 `json:"rating,omitempty"`
	VehicleType string  `json:"vehicle_type,omitempty"`
}
