package logistics

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/google/uuid"
	serviceclient "github.com/Bengo-Hub/shared-service-client"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/config"
)

// Client provides methods to interact with the logistics service.
type Client struct {
	baseURL       string
	apiKey        string
	serviceClient *serviceclient.Client
	logger        *zap.Logger
}

// NewClient creates a new logistics service client.
func NewClient(cfg config.LogisticsConfig, logger *zap.Logger) *Client {
	scCfg := serviceclient.DefaultConfig(
		cfg.ServiceURL,
		"ordering-service",
		logger.Named("logistics.client"),
	)
	scCfg.Timeout = cfg.RequestTimeout

	return &Client{
		baseURL:       cfg.ServiceURL,
		apiKey:        cfg.APIKey,
		serviceClient: serviceclient.New(scCfg),
		logger:        logger.Named("logistics.client"),
	}
}

// TaskPriority represents task priority levels.
type TaskPriority string

const (
	PriorityLow    TaskPriority = "low"
	PriorityNormal TaskPriority = "normal"
	PriorityHigh   TaskPriority = "high"
	PriorityUrgent TaskPriority = "urgent"
)

// TaskStatus represents the status of a delivery task.
type TaskStatus string

const (
	TaskStatusPending         TaskStatus = "pending"
	TaskStatusAssigned        TaskStatus = "assigned"
	TaskStatusAccepted        TaskStatus = "accepted"
	TaskStatusEnRoutePickup   TaskStatus = "en_route_pickup"
	TaskStatusArrivedPickup   TaskStatus = "arrived_pickup"
	TaskStatusPickedUp        TaskStatus = "picked_up"
	TaskStatusEnRouteDropoff  TaskStatus = "en_route_dropoff"
	TaskStatusArrivedDropoff  TaskStatus = "arrived_dropoff"
	TaskStatusCompleted       TaskStatus = "completed"
	TaskStatusCancelled       TaskStatus = "cancelled"
	TaskStatusFailed          TaskStatus = "failed"
)

// TaskPriorityToInt maps string priority levels to int values expected by logistics-api.
func TaskPriorityToInt(p TaskPriority) int {
	switch p {
	case PriorityLow:
		return 0
	case PriorityHigh:
		return 2
	case PriorityUrgent:
		return 3
	default: // PriorityNormal or empty
		return 1
	}
}

// CreateTaskRequest represents a request to create a delivery task.
// Field names match the logistics-api CreateTaskRequest JSON contract.
type CreateTaskRequest struct {
	ExternalReference string                 `json:"external_reference"`
	SourceService     string                 `json:"source_service"`
	TaskType          string                 `json:"task_type"`
	Priority          int                    `json:"priority"`
	SLADueAt          *time.Time             `json:"sla_due_at,omitempty"`
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
	IdempotencyKey    string                 `json:"idempotency_key,omitempty"`

	// Delivery-specific fields — used to auto-create pickup/dropoff steps.
	PickupAddress  string  `json:"pickup_address,omitempty"`
	PickupLat      float64 `json:"pickup_lat,omitempty"`
	PickupLng      float64 `json:"pickup_lng,omitempty"`
	PickupContact  string  `json:"pickup_contact,omitempty"`
	DropoffAddress string  `json:"dropoff_address,omitempty"`
	DropoffLat     float64 `json:"dropoff_lat,omitempty"`
	DropoffLng     float64 `json:"dropoff_lng,omitempty"`
	DropoffContact string  `json:"dropoff_contact,omitempty"`
	CustomerName   string  `json:"customer_name,omitempty"`
	CustomerPhone  string  `json:"customer_phone,omitempty"`
	Instructions   string  `json:"instructions,omitempty"`
}

// Location represents a geographical location.
type Location struct {
	Address      string  `json:"address"`
	Latitude     float64 `json:"latitude"`
	Longitude    float64 `json:"longitude"`
	PlaceID      string  `json:"place_id,omitempty"`
	Notes        string  `json:"notes,omitempty"`
	ContactName  string  `json:"contact_name,omitempty"`
	ContactPhone string  `json:"contact_phone,omitempty"`
}

// TaskResponse represents a delivery task from logistics service.
type TaskResponse struct {
	ID                 uuid.UUID              `json:"id"`
	TenantID           uuid.UUID              `json:"tenant_id"`
	ExternalReference  string                 `json:"external_reference"`
	ExternalType       string                 `json:"external_type"`
	Status             TaskStatus             `json:"status"`
	Priority           TaskPriority           `json:"priority"`
	RiderID            string                 `json:"rider_id,omitempty"`
	RiderName          string                 `json:"rider_name,omitempty"`
	RiderPhone         string                 `json:"rider_phone,omitempty"`
	VehicleType        string                 `json:"vehicle_type,omitempty"`
	PickupLocation     Location               `json:"pickup_location"`
	DropoffLocation    Location               `json:"dropoff_location"`
	ScheduledPickupAt  *time.Time             `json:"scheduled_pickup_at,omitempty"`
	ScheduledDropoffAt *time.Time             `json:"scheduled_dropoff_at,omitempty"`
	ETAMinutes         int                    `json:"eta_minutes,omitempty"`
	ETAAt              *time.Time             `json:"eta_at,omitempty"`
	DistanceKm         float64                `json:"distance_km,omitempty"`
	Instructions       string                 `json:"instructions,omitempty"`
	AssignedAt         *time.Time             `json:"assigned_at,omitempty"`
	AcceptedAt         *time.Time             `json:"accepted_at,omitempty"`
	PickedUpAt         *time.Time             `json:"picked_up_at,omitempty"`
	CompletedAt        *time.Time             `json:"completed_at,omitempty"`
	CancelledAt        *time.Time             `json:"cancelled_at,omitempty"`
	FailureReason      string                 `json:"failure_reason,omitempty"`
	Metadata           map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt          time.Time              `json:"created_at"`
	UpdatedAt          time.Time              `json:"updated_at"`
}

// CancelTaskRequest represents a request to cancel a task.
type CancelTaskRequest struct {
	TenantID uuid.UUID `json:"tenant_id"`
	Reason   string    `json:"reason"`
}

// RiderLocation represents real-time rider location.
type RiderLocation struct {
	RiderID   string    `json:"rider_id"`
	Latitude  float64   `json:"latitude"`
	Longitude float64   `json:"longitude"`
	Heading   float64   `json:"heading,omitempty"`
	Speed     float64   `json:"speed,omitempty"`
	Accuracy  float64   `json:"accuracy,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TrackingInfo represents tracking information for a task.
type TrackingInfo struct {
	TaskID         uuid.UUID      `json:"task_id"`
	Status         TaskStatus     `json:"status"`
	RiderLocation  *RiderLocation `json:"rider_location,omitempty"`
	ETAMinutes     int            `json:"eta_minutes,omitempty"`
	ETAAt          *time.Time     `json:"eta_at,omitempty"`
	DistanceKm     float64        `json:"distance_km,omitempty"`
	CurrentAddress string         `json:"current_address,omitempty"`
	LastUpdatedAt  time.Time      `json:"last_updated_at"`
}

// FleetMemberResponse represents a fleet member/rider from logistics service.
type FleetMemberResponse struct {
	ID          uuid.UUID              `json:"id"`
	TenantID    uuid.UUID              `json:"tenant_id"`
	FleetID     uuid.UUID              `json:"fleet_id"`
	UserID      uuid.UUID              `json:"user_id"`
	DriverCode  string                 `json:"driver_code,omitempty"`
	Status      string                 `json:"status"`
	VehicleID   *uuid.UUID             `json:"vehicle_id,omitempty"`
	JoinedAt    *time.Time             `json:"joined_at,omitempty"`
	SuspendedAt *time.Time             `json:"suspended_at,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// ProofOfDeliveryResponse represents proof of delivery from logistics.
type ProofOfDeliveryResponse struct {
	TaskID            uuid.UUID              `json:"task_id"`
	Type              string                 `json:"type"` // signature, photo, otp, pin
	SignatureURL      string                 `json:"signature_url,omitempty"`
	PhotoURLs         []string               `json:"photo_urls,omitempty"`
	OTPVerified       bool                   `json:"otp_verified,omitempty"`
	RecipientName     string                 `json:"recipient_name,omitempty"`
	RecipientRelation string                 `json:"recipient_relation,omitempty"`
	RiderNotes        string                 `json:"rider_notes,omitempty"`
	DeliveryLatitude  float64                `json:"delivery_latitude,omitempty"`
	DeliveryLongitude float64                `json:"delivery_longitude,omitempty"`
	DeliveredAt       time.Time              `json:"delivered_at"`
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
}

// APIError represents an error response from the logistics API.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("logistics API error: %s - %s", e.Code, e.Message)
}

// headers returns common headers for requests.
func (c *Client) headers(idempotencyKey string) map[string]string {
	h := map[string]string{
		"Content-Type": "application/json",
		"Accept":       "application/json",
	}

	if c.apiKey != "" {
		h["X-API-Key"] = c.apiKey
	}

	if idempotencyKey != "" {
		h["Idempotency-Key"] = idempotencyKey
	}

	return h
}

// parseError parses an error response from the API.
func (c *Client) parseError(resp *serviceclient.Response) error {
	var apiErr APIError
	if err := resp.DecodeJSON(&apiErr); err != nil {
		return fmt.Errorf("logistics API error (status %d)", resp.StatusCode)
	}
	return &apiErr
}

// CreateTask creates a delivery task with the logistics service.
func (c *Client) CreateTask(ctx context.Context, tenantSlug string, req CreateTaskRequest) (*TaskResponse, error) {
	path := fmt.Sprintf("/api/v1/%s/tasks", tenantSlug)

	resp, err := c.serviceClient.Post(ctx, path, req, c.headers(req.IdempotencyKey))
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}

	if !resp.IsSuccess() {
		return nil, c.parseError(resp)
	}

	var result TaskResponse
	if err := resp.DecodeJSON(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

// GetTask retrieves a task by ID.
func (c *Client) GetTask(ctx context.Context, tenantSlug string, taskID uuid.UUID) (*TaskResponse, error) {
	path := fmt.Sprintf("/api/v1/%s/tasks/%s", tenantSlug, taskID.String())

	resp, err := c.serviceClient.Get(ctx, path, c.headers(""))
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}

	if !resp.IsSuccess() {
		return nil, c.parseError(resp)
	}

	var result TaskResponse
	if err := resp.DecodeJSON(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

// GetTaskByExternalRef retrieves a task by external reference (order_id).
func (c *Client) GetTaskByExternalRef(ctx context.Context, tenantSlug string, externalRef string) (*TaskResponse, error) {
	path := fmt.Sprintf("/api/v1/%s/tasks?external_reference=%s", tenantSlug, url.QueryEscape(externalRef))

	resp, err := c.serviceClient.Get(ctx, path, c.headers(""))
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}

	if !resp.IsSuccess() {
		return nil, c.parseError(resp)
	}

	var tasks []TaskResponse
	if err := resp.DecodeJSON(&tasks); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(tasks) == 0 {
		return nil, &APIError{Code: "NOT_FOUND", Message: "task not found"}
	}

	return &tasks[0], nil
}

// CancelTask cancels a pending or assigned task.
func (c *Client) CancelTask(ctx context.Context, tenantSlug string, taskID uuid.UUID, reason string) error {
	path := fmt.Sprintf("/api/v1/%s/tasks/%s/cancel", tenantSlug, taskID.String())
	reqBody := CancelTaskRequest{Reason: reason}

	resp, err := c.serviceClient.Post(ctx, path, reqBody, c.headers(""))
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}

	if !resp.IsSuccess() {
		return c.parseError(resp)
	}

	return nil
}

// GetTracking retrieves real-time tracking information for a task.
func (c *Client) GetTracking(ctx context.Context, tenantSlug string, taskID uuid.UUID) (*TrackingInfo, error) {
	path := fmt.Sprintf("/api/v1/%s/tasks/%s/tracking", tenantSlug, taskID.String())

	resp, err := c.serviceClient.Get(ctx, path, c.headers(""))
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}

	if !resp.IsSuccess() {
		return nil, c.parseError(resp)
	}

	var result TrackingInfo
	if err := resp.DecodeJSON(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

// GetFleetMember retrieves fleet member/rider details.
func (c *Client) GetFleetMember(ctx context.Context, tenantSlug string, memberID string) (*FleetMemberResponse, error) {
	path := fmt.Sprintf("/api/v1/%s/fleet-members/%s", tenantSlug, memberID)

	resp, err := c.serviceClient.Get(ctx, path, c.headers(""))
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}

	if !resp.IsSuccess() {
		return nil, c.parseError(resp)
	}

	var result FleetMemberResponse
	if err := resp.DecodeJSON(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

// GetProofOfDelivery retrieves proof of delivery for a completed task.
func (c *Client) GetProofOfDelivery(ctx context.Context, tenantSlug string, taskID uuid.UUID) (*ProofOfDeliveryResponse, error) {
	path := fmt.Sprintf("/api/v1/%s/tasks/%s/proof-of-delivery", tenantSlug, taskID.String())

	resp, err := c.serviceClient.Get(ctx, path, c.headers(""))
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}

	if !resp.IsSuccess() {
		return nil, c.parseError(resp)
	}

	var result ProofOfDeliveryResponse
	if err := resp.DecodeJSON(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

// AssignTaskRequest represents a request to assign a fleet member to a task.
type AssignTaskRequest struct {
	FleetMemberID     string `json:"fleet_member_id"`
	ExternalReference string `json:"external_reference,omitempty"` // order_id for logistics-side lookup-or-create
}

// AssignTask assigns a fleet member (rider) to a pending task.
// orderID is passed as external_reference so the logistics service can look up or create the task by order ID.
func (c *Client) AssignTask(ctx context.Context, tenantSlug string, taskID uuid.UUID, fleetMemberID string, orderID string) (*TaskResponse, error) {
	path := fmt.Sprintf("/api/v1/%s/tasks/%s/assign", tenantSlug, taskID.String())
	reqBody := AssignTaskRequest{FleetMemberID: fleetMemberID, ExternalReference: orderID}

	resp, err := c.serviceClient.Post(ctx, path, reqBody, c.headers(""))
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}

	if !resp.IsSuccess() {
		return nil, c.parseError(resp)
	}

	var result TaskResponse
	if err := resp.DecodeJSON(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

// RateRiderRequest is the request to rate a rider on a completed task.
type RateRiderRequest struct {
	Rating  int    `json:"rating"`
	Comment string `json:"comment,omitempty"`
}

// RateRider submits a customer rating for the rider who delivered a task.
func (c *Client) RateRider(ctx context.Context, tenantSlug, taskID string, req RateRiderRequest) error {
	path := fmt.Sprintf("/api/v1/%s/tasks/%s/rate", tenantSlug, taskID)

	resp, err := c.serviceClient.Post(ctx, path, req, c.headers(""))
	if err != nil {
		return fmt.Errorf("rate rider request: %w", err)
	}

	if !resp.IsSuccess() {
		return fmt.Errorf("rate rider failed: status %d", resp.StatusCode)
	}

	return nil
}

// HealthCheck checks if the logistics service is healthy.
func (c *Client) HealthCheck(ctx context.Context) error {
	resp, err := c.serviceClient.Get(ctx, "/healthz", nil)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}

	if !resp.IsSuccess() {
		return fmt.Errorf("logistics service unhealthy: status %d", resp.StatusCode)
	}

	return nil
}
