package logistics

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/config"
)

// Client provides methods to interact with the logistics service.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	logger     *zap.Logger
}

// NewClient creates a new logistics service client.
func NewClient(cfg config.LogisticsConfig, logger *zap.Logger) *Client {
	return &Client{
		baseURL: cfg.ServiceURL,
		apiKey:  cfg.APIKey,
		httpClient: &http.Client{
			Timeout: cfg.RequestTimeout,
		},
		logger: logger,
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

// CreateTaskRequest represents a request to create a delivery task.
type CreateTaskRequest struct {
	TenantID           uuid.UUID              `json:"tenant_id"`
	ExternalReference  string                 `json:"external_reference"` // order_id
	ExternalType       string                 `json:"external_type"`      // "order"
	Priority           TaskPriority           `json:"priority"`
	PickupLocation     Location               `json:"pickup_location"`
	DropoffLocation    Location               `json:"dropoff_location"`
	ScheduledPickupAt  *time.Time             `json:"scheduled_pickup_at,omitempty"`
	ScheduledDropoffAt *time.Time             `json:"scheduled_dropoff_at,omitempty"`
	Instructions       string                 `json:"instructions,omitempty"`
	CustomerName       string                 `json:"customer_name,omitempty"`
	CustomerPhone      string                 `json:"customer_phone,omitempty"`
	ItemsDescription   string                 `json:"items_description,omitempty"`
	ItemCount          int                    `json:"item_count,omitempty"`
	CashOnDelivery     float64                `json:"cash_on_delivery,omitempty"`
	IdempotencyKey     string                 `json:"idempotency_key,omitempty"`
	Metadata           map[string]interface{} `json:"metadata,omitempty"`
}

// Location represents a geographical location.
type Location struct {
	Address    string  `json:"address"`
	Latitude   float64 `json:"latitude"`
	Longitude  float64 `json:"longitude"`
	PlaceID    string  `json:"place_id,omitempty"`
	Notes      string  `json:"notes,omitempty"`
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

// CreateTask creates a delivery task with the logistics service.
func (c *Client) CreateTask(ctx context.Context, tenantSlug string, req CreateTaskRequest) (*TaskResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/%s/tasks", c.baseURL, tenantSlug)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	c.setHeaders(httpReq, req.IdempotencyKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, c.parseError(resp)
	}

	var result TaskResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

// GetTask retrieves a task by ID.
func (c *Client) GetTask(ctx context.Context, tenantSlug string, taskID uuid.UUID) (*TaskResponse, error) {
	url := fmt.Sprintf("%s/api/v1/%s/tasks/%s", c.baseURL, tenantSlug, taskID.String())

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	c.setHeaders(httpReq, "")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, c.parseError(resp)
	}

	var result TaskResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

// GetTaskByExternalRef retrieves a task by external reference (order_id).
func (c *Client) GetTaskByExternalRef(ctx context.Context, tenantSlug string, externalRef string) (*TaskResponse, error) {
	url := fmt.Sprintf("%s/api/v1/%s/tasks?external_reference=%s", c.baseURL, tenantSlug, url.QueryEscape(externalRef))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	c.setHeaders(httpReq, "")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, c.parseError(resp)
	}

	var tasks []TaskResponse
	if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(tasks) == 0 {
		return nil, &APIError{Code: "NOT_FOUND", Message: "task not found"}
	}

	return &tasks[0], nil
}

// CancelTask cancels a pending or assigned task.
func (c *Client) CancelTask(ctx context.Context, tenantSlug string, taskID uuid.UUID, reason string) error {
	reqBody := CancelTaskRequest{Reason: reason}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/%s/tasks/%s/cancel", c.baseURL, tenantSlug, taskID.String())
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	c.setHeaders(httpReq, "")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return c.parseError(resp)
	}

	return nil
}

// GetTracking retrieves real-time tracking information for a task.
func (c *Client) GetTracking(ctx context.Context, tenantSlug string, taskID uuid.UUID) (*TrackingInfo, error) {
	url := fmt.Sprintf("%s/api/v1/%s/tasks/%s/tracking", c.baseURL, tenantSlug, taskID.String())

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	c.setHeaders(httpReq, "")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, c.parseError(resp)
	}

	var result TrackingInfo
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

// GetFleetMember retrieves fleet member/rider details.
func (c *Client) GetFleetMember(ctx context.Context, tenantSlug string, memberID string) (*FleetMemberResponse, error) {
	// Note: The API structure uses nested routes, but we need to query without fleet context
	// This may need to be adjusted based on actual logistics API implementation
	url := fmt.Sprintf("%s/api/v1/%s/fleet-members/%s", c.baseURL, tenantSlug, memberID)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	c.setHeaders(httpReq, "")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, c.parseError(resp)
	}

	var result FleetMemberResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

// GetProofOfDelivery retrieves proof of delivery for a completed task.
func (c *Client) GetProofOfDelivery(ctx context.Context, tenantSlug string, taskID uuid.UUID) (*ProofOfDeliveryResponse, error) {
	url := fmt.Sprintf("%s/api/v1/%s/tasks/%s/proof-of-delivery", c.baseURL, tenantSlug, taskID.String())

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	c.setHeaders(httpReq, "")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, c.parseError(resp)
	}

	var result ProofOfDeliveryResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

// setHeaders sets common headers for requests.
func (c *Client) setHeaders(req *http.Request, idempotencyKey string) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}

	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
}

// parseError parses an error response from the API.
func (c *Client) parseError(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read error body: %w", err)
	}

	var apiErr APIError
	if err := json.Unmarshal(body, &apiErr); err != nil {
		// If we can't parse the error, return a generic error with the body
		return fmt.Errorf("logistics API error (status %d): %s", resp.StatusCode, string(body))
	}

	return &apiErr
}

// HealthCheck checks if the logistics service is healthy.
func (c *Client) HealthCheck(ctx context.Context) error {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/healthz", nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("logistics service unhealthy: status %d", resp.StatusCode)
	}

	return nil
}
