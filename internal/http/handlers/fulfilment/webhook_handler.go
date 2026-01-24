package fulfilment

import (
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	handlers "github.com/bengobox/ordering-backend/internal/http/handlers"
	"github.com/bengobox/ordering-backend/internal/modules/fulfilment"
)

// WebhookHandler handles logistics webhook HTTP requests.
type WebhookHandler struct {
	logger     *zap.Logger
	webhookSvc *fulfilment.WebhookService
}

// NewWebhookHandler creates a new webhook handler.
func NewWebhookHandler(logger *zap.Logger, webhookSvc *fulfilment.WebhookService) *WebhookHandler {
	return &WebhookHandler{
		logger:     logger,
		webhookSvc: webhookSvc,
	}
}

// Register registers the webhook routes.
// Routes are registered directly to avoid conflicts with other webhook handlers.
func (h *WebhookHandler) Register(r chi.Router) {
	r.Post("/webhooks/logistics", h.HandleLogisticsWebhook)
}

// HandleLogisticsWebhook handles incoming logistics webhooks.
// @Summary Handle logistics webhook
// @Tags Webhooks
// @Accept json
// @Produce json
// @Success 200 {object} map[string]string
// @Router /api/v1/webhooks/logistics [post]
func (h *WebhookHandler) HandleLogisticsWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Read raw body for signature verification
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Error("failed to read webhook body", zap.Error(err))
		handlers.RespondError(w, http.StatusBadRequest, "failed to read request body")
		return
	}
	defer r.Body.Close()

	// Get signature from header
	signature := r.Header.Get("X-Logistics-Signature")
	if signature == "" {
		signature = r.Header.Get("X-Webhook-Signature")
	}

	// Get client IP
	ipAddress := r.Header.Get("X-Forwarded-For")
	if ipAddress == "" {
		ipAddress = r.RemoteAddr
	}

	// Collect headers
	headers := make(map[string]string)
	for key := range r.Header {
		headers[key] = r.Header.Get(key)
	}

	// Process webhook
	if err := h.webhookSvc.ProcessWebhook(ctx, body, signature, ipAddress, headers); err != nil {
		h.logger.Error("failed to process logistics webhook",
			zap.Error(err),
			zap.String("ip", ipAddress))

		// Still return 200 to prevent webhook retries for permanent errors
		// The webhook service will handle retries internally
		if err == fulfilment.ErrInvalidSignature {
			handlers.RespondError(w, http.StatusUnauthorized, "invalid signature")
			return
		}

		// Return 200 for processed (even if failed) to prevent external retries
		handlers.RespondJSON(w, http.StatusOK, map[string]string{
			"status":  "received",
			"message": "webhook received but processing failed",
		})
		return
	}

	handlers.RespondJSON(w, http.StatusOK, map[string]string{
		"status":  "processed",
		"message": "webhook processed successfully",
	})
}
