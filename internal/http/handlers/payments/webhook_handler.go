package paymentshandler

import (
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/http/handlers"
	"github.com/bengobox/ordering-backend/internal/modules/payments"
)

// WebhookHandler exposes treasury webhook endpoints.
type WebhookHandler struct {
	log            *zap.Logger
	webhookService *payments.WebhookService
}

// NewWebhookHandler constructs a WebhookHandler instance.
func NewWebhookHandler(log *zap.Logger, webhookService *payments.WebhookService) *WebhookHandler {
	return &WebhookHandler{
		log:            log.Named("payments.WebhookHandler"),
		webhookService: webhookService,
	}
}

// Register mounts webhook routes on the supplied router.
// Note: Webhooks don't require authentication - they use signature verification.
// Routes are registered directly to avoid conflicts with other webhook handlers.
func (h *WebhookHandler) Register(r chi.Router) {
	// Treasury payment webhooks - register directly without creating a route group
	r.Post("/webhooks/treasury", h.HandleTreasuryWebhook)

	// M-Pesa specific webhooks (may come directly from Safaricom via treasury)
	r.Post("/webhooks/mpesa/callback", h.HandleMpesaCallback)
	r.Post("/webhooks/mpesa/confirmation", h.HandleMpesaConfirmation)
	r.Post("/webhooks/mpesa/validation", h.HandleMpesaValidation)
}

// HandleTreasuryWebhook processes incoming treasury webhooks.
// @Summary Handle treasury webhook
// @Tags Webhooks
// @Accept json
// @Produce json
// @Success 200 {object} map[string]string
// @Router /webhooks/treasury [post]
func (h *WebhookHandler) HandleTreasuryWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Read the raw body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.log.Error("failed to read webhook body", zap.Error(err))
		handlers.RespondError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	defer r.Body.Close()

	// Get signature from header
	signature := r.Header.Get("X-Treasury-Signature")
	if signature == "" {
		signature = r.Header.Get("X-Webhook-Signature")
	}

	// Build headers map
	headers := make(map[string]string)
	for key := range r.Header {
		headers[key] = r.Header.Get(key)
	}

	// Get client IP
	ipAddress := r.Header.Get("X-Forwarded-For")
	if ipAddress == "" {
		ipAddress = r.Header.Get("X-Real-IP")
	}
	if ipAddress == "" {
		ipAddress = r.RemoteAddr
	}

	// Process the webhook
	err = h.webhookService.ProcessWebhook(ctx, payments.ProcessWebhookRequest{
		Payload:   body,
		Signature: signature,
		Headers:   headers,
		IPAddress: ipAddress,
	})
	if err != nil {
		h.log.Error("failed to process webhook", zap.Error(err))
		// Return 200 anyway to prevent retries for validation errors
		// We log the error for debugging
		handlers.RespondJSON(w, http.StatusOK, map[string]string{
			"status":  "error",
			"message": err.Error(),
		})
		return
	}

	handlers.RespondJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

// HandleMpesaCallback handles M-Pesa STK Push callback.
// This is typically forwarded through treasury service.
// @Summary Handle M-Pesa callback
// @Tags Webhooks
// @Accept json
// @Produce json
// @Success 200 {object} map[string]string
// @Router /webhooks/mpesa/callback [post]
func (h *WebhookHandler) HandleMpesaCallback(w http.ResponseWriter, r *http.Request) {
	// M-Pesa callbacks are typically forwarded through treasury service
	// as treasury webhook events. This endpoint serves as a fallback
	// or direct callback endpoint if needed.
	h.HandleTreasuryWebhook(w, r)
}

// HandleMpesaConfirmation handles M-Pesa C2B confirmation requests.
// @Summary Handle M-Pesa C2B confirmation
// @Tags Webhooks
// @Accept json
// @Produce json
// @Success 200 {object} map[string]string
// @Router /webhooks/mpesa/confirmation [post]
func (h *WebhookHandler) HandleMpesaConfirmation(w http.ResponseWriter, r *http.Request) {
	// C2B confirmations are typically handled by treasury service
	// Forward to standard webhook processing
	h.HandleTreasuryWebhook(w, r)
}

// HandleMpesaValidation handles M-Pesa C2B validation requests.
// @Summary Handle M-Pesa C2B validation
// @Tags Webhooks
// @Accept json
// @Produce json
// @Success 200 {object} map[string]string
// @Router /webhooks/mpesa/validation [post]
func (h *WebhookHandler) HandleMpesaValidation(w http.ResponseWriter, r *http.Request) {
	// M-Pesa validation requests should always accept (return 0)
	// The actual validation happens at treasury service level
	handlers.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"ResultCode":        0,
		"ResultDesc":        "Accepted",
		"ThirdPartyTransID": "",
	})
}
