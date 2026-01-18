package notifications

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	serviceclient "github.com/Bengo-Hub/shared-service-client"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/config"
)

// Client provides methods to interact with the notifications service.
type Client struct {
	baseURL       string
	apiKey        string
	serviceClient *serviceclient.Client
	logger        *zap.Logger
}

// NewClient creates a new notifications service client.
func NewClient(cfg config.NotificationsConfig, logger *zap.Logger) *Client {
	scCfg := serviceclient.DefaultConfig(
		cfg.ServiceURL,
		"ordering-service",
		logger.Named("notifications.client"),
	)
	scCfg.Timeout = cfg.RequestTimeout

	return &Client{
		baseURL:       cfg.ServiceURL,
		apiKey:        cfg.APIKey,
		serviceClient: serviceclient.New(scCfg),
		logger:        logger.Named("notifications.client"),
	}
}

// Channel represents notification channels.
type Channel string

const (
	ChannelEmail Channel = "email"
	ChannelSMS   Channel = "sms"
	ChannelPush  Channel = "push"
)

// SendMessageRequest represents a request to send a notification.
type SendMessageRequest struct {
	Channel  Channel                `json:"channel"`
	Tenant   string                 `json:"tenant"`
	Template string                 `json:"template"`
	To       []string               `json:"to"`
	Data     map[string]interface{} `json:"data"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// SendMessageResponse represents a notification send response.
type SendMessageResponse struct {
	Status    string `json:"status"` // queued, sent, failed
	RequestID string `json:"requestId"`
}

// TemplateInfo represents notification template information.
type TemplateInfo struct {
	ID       string  `json:"id"`
	Channel  Channel `json:"channel"`
	Locale   string  `json:"locale"`
	Name     string  `json:"name"`
	EventKey string  `json:"event_key"`
}

// TemplateContent represents notification template content.
type TemplateContent struct {
	ID      string  `json:"id"`
	Channel Channel `json:"channel"`
	Locale  string  `json:"locale"`
	Subject string  `json:"subject,omitempty"`
	Body    string  `json:"body"`
}

// APIError represents an error response from the notifications API.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("notifications API error: %s - %s", e.Code, e.Message)
}

// headers returns common headers for requests.
func (c *Client) headers() map[string]string {
	h := map[string]string{
		"Content-Type": "application/json",
		"Accept":       "application/json",
	}

	if c.apiKey != "" {
		h["X-API-Key"] = c.apiKey
	}

	return h
}

// parseError parses an error response from the API.
func (c *Client) parseError(resp *serviceclient.Response) error {
	var apiErr APIError
	if err := resp.DecodeJSON(&apiErr); err != nil {
		return fmt.Errorf("notifications API error (status %d)", resp.StatusCode)
	}
	return &apiErr
}

// SendMessage sends a notification message.
func (c *Client) SendMessage(ctx context.Context, tenantID string, req SendMessageRequest) (*SendMessageResponse, error) {
	path := fmt.Sprintf("/v1/%s/notifications/messages", tenantID)

	resp, err := c.serviceClient.Post(ctx, path, req, c.headers())
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}

	// 202 Accepted is success for async operations
	if resp.StatusCode != 202 && !resp.IsSuccess() {
		return nil, c.parseError(resp)
	}

	var result SendMessageResponse
	if err := resp.DecodeJSON(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

// ListTemplates lists available templates for a tenant.
func (c *Client) ListTemplates(ctx context.Context, tenantID string) ([]TemplateInfo, error) {
	path := fmt.Sprintf("/v1/%s/templates", tenantID)

	resp, err := c.serviceClient.Get(ctx, path, c.headers())
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}

	if !resp.IsSuccess() {
		return nil, c.parseError(resp)
	}

	var result []TemplateInfo
	if err := resp.DecodeJSON(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return result, nil
}

// GetTemplate retrieves a template by ID.
func (c *Client) GetTemplate(ctx context.Context, tenantID string, templateID string, channel Channel) (*TemplateContent, error) {
	path := fmt.Sprintf("/v1/%s/templates/%s?channel=%s", tenantID, templateID, channel)

	resp, err := c.serviceClient.Get(ctx, path, c.headers())
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}

	if !resp.IsSuccess() {
		return nil, c.parseError(resp)
	}

	var result TemplateContent
	if err := resp.DecodeJSON(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

// HealthCheck checks if the notifications service is healthy.
func (c *Client) HealthCheck(ctx context.Context) error {
	resp, err := c.serviceClient.Get(ctx, "/healthz", nil)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}

	if !resp.IsSuccess() {
		return fmt.Errorf("notifications service unhealthy: status %d", resp.StatusCode)
	}

	return nil
}

// --- Order Notification Helpers ---

// SendOrderConfirmation sends an order confirmation notification.
func (c *Client) SendOrderConfirmation(ctx context.Context, tenantSlug string, orderID uuid.UUID, customerEmail string, customerPhone string, data map[string]interface{}) error {
	// Send email notification
	if customerEmail != "" {
		req := SendMessageRequest{
			Channel:  ChannelEmail,
			Tenant:   tenantSlug,
			Template: "order_confirmation",
			To:       []string{customerEmail},
			Data:     data,
			Metadata: map[string]interface{}{
				"order_id": orderID.String(),
			},
		}

		if _, err := c.SendMessage(ctx, tenantSlug, req); err != nil {
			c.logger.Warn("failed to send order confirmation email",
				zap.Error(err),
				zap.String("order_id", orderID.String()))
		}
	}

	// Send SMS notification
	if customerPhone != "" {
		req := SendMessageRequest{
			Channel:  ChannelSMS,
			Tenant:   tenantSlug,
			Template: "order_confirmation_sms",
			To:       []string{customerPhone},
			Data:     data,
			Metadata: map[string]interface{}{
				"order_id": orderID.String(),
			},
		}

		if _, err := c.SendMessage(ctx, tenantSlug, req); err != nil {
			c.logger.Warn("failed to send order confirmation SMS",
				zap.Error(err),
				zap.String("order_id", orderID.String()))
		}
	}

	return nil
}

// SendOrderReady sends an order ready notification.
func (c *Client) SendOrderReady(ctx context.Context, tenantSlug string, orderID uuid.UUID, customerEmail string, customerPhone string, data map[string]interface{}) error {
	if customerPhone != "" {
		req := SendMessageRequest{
			Channel:  ChannelSMS,
			Tenant:   tenantSlug,
			Template: "order_ready",
			To:       []string{customerPhone},
			Data:     data,
			Metadata: map[string]interface{}{
				"order_id": orderID.String(),
			},
		}

		if _, err := c.SendMessage(ctx, tenantSlug, req); err != nil {
			c.logger.Warn("failed to send order ready SMS",
				zap.Error(err),
				zap.String("order_id", orderID.String()))
		}
	}

	return nil
}

// SendDriverAssigned sends a driver assigned notification.
func (c *Client) SendDriverAssigned(ctx context.Context, tenantSlug string, orderID uuid.UUID, customerPhone string, data map[string]interface{}) error {
	if customerPhone != "" {
		req := SendMessageRequest{
			Channel:  ChannelSMS,
			Tenant:   tenantSlug,
			Template: "driver_assigned",
			To:       []string{customerPhone},
			Data:     data,
			Metadata: map[string]interface{}{
				"order_id": orderID.String(),
			},
		}

		if _, err := c.SendMessage(ctx, tenantSlug, req); err != nil {
			c.logger.Warn("failed to send driver assigned SMS",
				zap.Error(err),
				zap.String("order_id", orderID.String()))
		}
	}

	return nil
}

// SendDeliveryComplete sends a delivery complete notification.
func (c *Client) SendDeliveryComplete(ctx context.Context, tenantSlug string, orderID uuid.UUID, customerEmail string, customerPhone string, data map[string]interface{}) error {
	if customerEmail != "" {
		req := SendMessageRequest{
			Channel:  ChannelEmail,
			Tenant:   tenantSlug,
			Template: "delivery_complete",
			To:       []string{customerEmail},
			Data:     data,
			Metadata: map[string]interface{}{
				"order_id": orderID.String(),
			},
		}

		if _, err := c.SendMessage(ctx, tenantSlug, req); err != nil {
			c.logger.Warn("failed to send delivery complete email",
				zap.Error(err),
				zap.String("order_id", orderID.String()))
		}
	}

	return nil
}

// SendPaymentReceipt sends a payment receipt notification.
func (c *Client) SendPaymentReceipt(ctx context.Context, tenantSlug string, orderID uuid.UUID, paymentID uuid.UUID, customerEmail string, data map[string]interface{}) error {
	if customerEmail != "" {
		req := SendMessageRequest{
			Channel:  ChannelEmail,
			Tenant:   tenantSlug,
			Template: "payment_receipt",
			To:       []string{customerEmail},
			Data:     data,
			Metadata: map[string]interface{}{
				"order_id":   orderID.String(),
				"payment_id": paymentID.String(),
			},
		}

		if _, err := c.SendMessage(ctx, tenantSlug, req); err != nil {
			c.logger.Warn("failed to send payment receipt email",
				zap.Error(err),
				zap.String("order_id", orderID.String()))
		}
	}

	return nil
}

// SendLoyaltyPointsEarned sends a loyalty points notification.
func (c *Client) SendLoyaltyPointsEarned(ctx context.Context, tenantSlug string, userID uuid.UUID, customerEmail string, data map[string]interface{}) error {
	if customerEmail != "" {
		req := SendMessageRequest{
			Channel:  ChannelEmail,
			Tenant:   tenantSlug,
			Template: "loyalty_points_earned",
			To:       []string{customerEmail},
			Data:     data,
			Metadata: map[string]interface{}{
				"user_id": userID.String(),
			},
		}

		if _, err := c.SendMessage(ctx, tenantSlug, req); err != nil {
			c.logger.Warn("failed to send loyalty points email",
				zap.Error(err),
				zap.String("user_id", userID.String()))
		}
	}

	return nil
}
