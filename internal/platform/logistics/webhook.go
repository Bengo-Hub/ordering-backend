package logistics

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// WebhookVerifier verifies webhook signatures from logistics service.
type WebhookVerifier struct {
	secret string
}

// NewWebhookVerifier creates a new webhook signature verifier.
func NewWebhookVerifier(secret string) *WebhookVerifier {
	return &WebhookVerifier{secret: secret}
}

// VerifySignature verifies the HMAC-SHA256 signature of a webhook payload.
func (v *WebhookVerifier) VerifySignature(payload []byte, signature string) bool {
	if v.secret == "" {
		return true // No secret configured, skip verification (development mode)
	}

	mac := hmac.New(sha256.New, []byte(v.secret))
	mac.Write(payload)
	expectedMAC := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(signature), []byte(expectedMAC))
}

// WebhookEventType represents types of logistics webhook events.
type WebhookEventType string

const (
	// Task lifecycle events
	EventTaskCreated        WebhookEventType = "logistics.task.created"
	EventTaskAssigned       WebhookEventType = "logistics.task.assigned"
	EventTaskAccepted       WebhookEventType = "logistics.task.accepted"
	EventTaskRejected       WebhookEventType = "logistics.task.rejected"
	EventTaskEnRoutePickup  WebhookEventType = "logistics.task.en_route_pickup"
	EventTaskArrivedPickup  WebhookEventType = "logistics.task.arrived_pickup"
	EventTaskPickedUp       WebhookEventType = "logistics.task.picked_up"
	EventTaskEnRouteDropoff WebhookEventType = "logistics.task.en_route_dropoff"
	EventTaskArrivedDropoff WebhookEventType = "logistics.task.arrived_dropoff"
	EventTaskCompleted      WebhookEventType = "logistics.task.completed"
	EventTaskCancelled      WebhookEventType = "logistics.task.cancelled"
	EventTaskFailed         WebhookEventType = "logistics.task.failed"

	// Route/ETA events
	EventRouteUpdated    WebhookEventType = "logistics.route.updated"
	EventETAUpdated      WebhookEventType = "logistics.eta.updated"
	EventLocationUpdated WebhookEventType = "logistics.location.updated"

	// PoD events
	EventPODSubmitted WebhookEventType = "logistics.pod.submitted"
	EventPODVerified  WebhookEventType = "logistics.pod.verified"

	// Other events
	EventRiderUnavailable   WebhookEventType = "logistics.rider.unavailable"
	EventReassignmentNeeded WebhookEventType = "logistics.reassignment.needed"
)

// WebhookEvent represents a logistics webhook event.
type WebhookEvent struct {
	ID        string                 `json:"event_id"`
	Type      WebhookEventType       `json:"event_type"`
	TenantID  uuid.UUID              `json:"tenant_id"`
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
}

// TaskEventData represents the data payload for task events.
type TaskEventData struct {
	TaskID            uuid.UUID  `json:"task_id"`
	ExternalReference string     `json:"external_reference"` // order_id
	Status            TaskStatus `json:"status"`
	RiderID           string     `json:"rider_id,omitempty"`
	RiderName         string     `json:"rider_name,omitempty"`
	RiderPhone        string     `json:"rider_phone,omitempty"`
	FailureReason     string     `json:"failure_reason,omitempty"`
	CancellationReason string    `json:"cancellation_reason,omitempty"`
	Timestamp         time.Time  `json:"timestamp"`
}

// RouteEventData represents the data payload for route/ETA update events.
type RouteEventData struct {
	TaskID            uuid.UUID `json:"task_id"`
	ExternalReference string    `json:"external_reference"`
	ETAMinutes        int       `json:"eta_minutes"`
	ETAAt             time.Time `json:"eta_at"`
	DistanceKm        float64   `json:"distance_km"`
	DistanceRemaining float64   `json:"distance_remaining"`
}

// LocationEventData represents the data payload for location update events.
type LocationEventData struct {
	TaskID            uuid.UUID `json:"task_id"`
	ExternalReference string    `json:"external_reference"`
	RiderID           string    `json:"rider_id"`
	Latitude          float64   `json:"latitude"`
	Longitude         float64   `json:"longitude"`
	Heading           float64   `json:"heading,omitempty"`
	Speed             float64   `json:"speed,omitempty"`
	Accuracy          float64   `json:"accuracy,omitempty"`
	Timestamp         time.Time `json:"timestamp"`
}

// PODEventData represents the data payload for proof of delivery events.
type PODEventData struct {
	TaskID            uuid.UUID `json:"task_id"`
	ExternalReference string    `json:"external_reference"`
	Type              string    `json:"type"` // signature, photo, otp
	SignatureURL      string    `json:"signature_url,omitempty"`
	PhotoURLs         []string  `json:"photo_urls,omitempty"`
	OTPVerified       bool      `json:"otp_verified,omitempty"`
	RecipientName     string    `json:"recipient_name,omitempty"`
	RecipientRelation string    `json:"recipient_relation,omitempty"`
	RiderNotes        string    `json:"rider_notes,omitempty"`
	DeliveryLatitude  float64   `json:"delivery_latitude,omitempty"`
	DeliveryLongitude float64   `json:"delivery_longitude,omitempty"`
	DeliveredAt       time.Time `json:"delivered_at"`
}

// ParseTaskEventData parses task event data from webhook event.
func ParseTaskEventData(data map[string]interface{}) (*TaskEventData, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	var eventData TaskEventData
	if err := json.Unmarshal(jsonData, &eventData); err != nil {
		return nil, err
	}

	return &eventData, nil
}

// ParseRouteEventData parses route event data from webhook event.
func ParseRouteEventData(data map[string]interface{}) (*RouteEventData, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	var eventData RouteEventData
	if err := json.Unmarshal(jsonData, &eventData); err != nil {
		return nil, err
	}

	return &eventData, nil
}

// ParseLocationEventData parses location event data from webhook event.
func ParseLocationEventData(data map[string]interface{}) (*LocationEventData, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	var eventData LocationEventData
	if err := json.Unmarshal(jsonData, &eventData); err != nil {
		return nil, err
	}

	return &eventData, nil
}

// ParsePODEventData parses proof of delivery event data from webhook event.
func ParsePODEventData(data map[string]interface{}) (*PODEventData, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	var eventData PODEventData
	if err := json.Unmarshal(jsonData, &eventData); err != nil {
		return nil, err
	}

	return &eventData, nil
}
