package paymentshandler

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/Bengo-Hub/httpware"
	"github.com/bengobox/ordering-backend/internal/http/handlers"
	identityhandler "github.com/bengobox/ordering-backend/internal/http/handlers/identity"
	"github.com/bengobox/ordering-backend/internal/modules/identity"
	"github.com/bengobox/ordering-backend/internal/modules/payments"
	"github.com/bengobox/ordering-backend/internal/platform/treasury"
)

// PaymentMethodHandler exposes payment method HTTP endpoints.
type PaymentMethodHandler struct {
	log      *zap.Logger
	service  *payments.PaymentMethodService
	treasury *treasury.Client
}

// NewPaymentMethodHandler constructs a PaymentMethodHandler instance.
// treasuryClient is used by the checkout payment-methods aggregate to fetch the
// tenant's enabled gateways and the user's wallet balance; it may be nil in
// contexts that do not need the aggregate (those fields are then omitted).
func NewPaymentMethodHandler(log *zap.Logger, service *payments.PaymentMethodService, treasuryClient *treasury.Client) *PaymentMethodHandler {
	return &PaymentMethodHandler{
		log:      log.Named("payments.PaymentMethodHandler"),
		service:  service,
		treasury: treasuryClient,
	}
}

// Register mounts payment method routes on the supplied router.
func (h *PaymentMethodHandler) Register(r chi.Router, auth *identityhandler.Authenticator) {
	r.Route("/payment-methods", func(methodRouter chi.Router) {
		methodRouter.Use(auth.RequireAuth)

		methodRouter.Post("/", h.CreatePaymentMethod)
		methodRouter.Get("/", h.ListPaymentMethods)
		methodRouter.Get("/default", h.GetDefaultPaymentMethod)
		methodRouter.Get("/{methodId}", h.GetPaymentMethod)
		methodRouter.Put("/{methodId}", h.UpdatePaymentMethod)
		methodRouter.Put("/{methodId}/default", h.SetDefaultPaymentMethod)
		methodRouter.Delete("/{methodId}", h.DeletePaymentMethod)
	})
}

// --- Request/Response Types ---

// CreatePaymentMethodRequest represents a request to create a payment method.
type CreatePaymentMethodRequest struct {
	Provider      string `json:"provider"`
	Type          string `json:"type"`
	Mask          string `json:"mask,omitempty"`
	Label         string `json:"label,omitempty"`
	ExpMonth      *int   `json:"expMonth,omitempty"`
	ExpYear       *int   `json:"expYear,omitempty"`
	ProviderToken string `json:"providerToken,omitempty"`
	SetAsDefault  bool   `json:"setAsDefault"`
}

// UpdatePaymentMethodRequest represents a request to update a payment method.
type UpdatePaymentMethodRequest struct {
	Label string `json:"label,omitempty"`
}

// PaymentMethodResponse represents a payment method response.
type PaymentMethodResponse struct {
	ID        string `json:"id"`
	Provider  string `json:"provider"`
	Type      string `json:"type"`
	Mask      string `json:"mask,omitempty"`
	Label     string `json:"label,omitempty"`
	ExpMonth  *int   `json:"expMonth,omitempty"`
	ExpYear   *int   `json:"expYear,omitempty"`
	IsDefault bool   `json:"isDefault"`
	CreatedAt string `json:"createdAt"`
}

// PaymentGatewayResponse describes one payment gateway the tenant has enabled. Field names
// mirror the frontend PaymentGateway type (ordering-frontend/src/lib/api/payment-methods.ts).
type PaymentGatewayResponse struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Icon    string `json:"icon"`
	Enabled bool   `json:"enabled"`
	Reason  string `json:"reason,omitempty"`
}

// WalletSummaryResponse is the checkout wallet summary; nil when the request is a guest or
// the wallet lookup fails. Mirrors the frontend wallet shape: { balance, currency }.
type WalletSummaryResponse struct {
	Balance  float64 `json:"balance"`
	Currency string  `json:"currency"`
}

// PaymentMethodsAggregateResponse is the checkout payment-methods payload consumed by
// ordering-frontend's getPaymentMethods(): { gateways, wallet, saved_methods }.
type PaymentMethodsAggregateResponse struct {
	Gateways     []PaymentGatewayResponse `json:"gateways"`
	Wallet       *WalletSummaryResponse   `json:"wallet"`
	SavedMethods []PaymentMethodResponse  `json:"saved_methods"`
}

// gatewayDisplay maps a treasury gateway type key to its checkout presentation metadata
// (label + icon). Keys correspond to treasury PublicListActiveGateways output.
var gatewayDisplay = map[string]struct {
	Name string
	Icon string
}{
	"paystack": {Name: "Card / Paystack", Icon: "paystack"},
	"mpesa":    {Name: "M-Pesa", Icon: "mpesa"},
	"wallet":   {Name: "Pay with Wallet", Icon: "wallet"},
	"cod":      {Name: "Cash on Delivery", Icon: "cash"},
}

// --- Helper Functions ---

func getUserFromContext(r *http.Request) (*identity.User, uuid.UUID, error) {
	ctx := r.Context()
	if httpware.IsPlatformOwner(ctx) {
		if q := r.URL.Query().Get("tenantId"); q != "" {
			if id, err := uuid.Parse(q); err == nil {
				user, _ := identity.UserFromContext(ctx)
				return user, id, nil
			}
		}
	}
	user, ok := identity.UserFromContext(ctx)
	if !ok {
		return nil, uuid.Nil, errors.New("user not found in context")
	}
	tenantID, err := uuid.Parse(user.TenantID)
	if err != nil {
		return nil, uuid.Nil, errors.New("invalid tenant ID")
	}
	return user, tenantID, nil
}

func (h *PaymentMethodHandler) handleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, payments.ErrPaymentMethodNotFound):
		handlers.RespondError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, payments.ErrDuplicatePaymentMethod):
		handlers.RespondError(w, http.StatusConflict, err.Error())
	case errors.Is(err, payments.ErrUnauthorized):
		handlers.RespondError(w, http.StatusForbidden, err.Error())
	default:
		h.log.Error("unexpected error", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "internal server error")
	}
}

func mapPaymentMethodToResponse(m *payments.PaymentMethod) PaymentMethodResponse {
	return PaymentMethodResponse{
		ID:        m.ID.String(),
		Provider:  string(m.Provider),
		Type:      string(m.Type),
		Mask:      m.Mask,
		Label:     m.Label,
		ExpMonth:  m.ExpMonth,
		ExpYear:   m.ExpYear,
		IsDefault: m.IsDefault,
		CreatedAt: m.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

// --- Endpoint Handlers ---

// CreatePaymentMethod creates a new payment method.
// @Summary Create payment method
// @Tags Payment Methods
// @Accept json
// @Produce json
// @Param request body CreatePaymentMethodRequest true "Payment method request"
// @Success 201 {object} PaymentMethodResponse
// @Router /payment-methods [post]
func (h *PaymentMethodHandler) CreatePaymentMethod(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, tenantID, err := getUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req CreatePaymentMethodRequest
	if err := handlers.DecodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	method, err := h.service.CreatePaymentMethod(ctx, payments.CreatePaymentMethodRequest{
		TenantID:      tenantID,
		UserID:        user.ID,
		Provider:      payments.PaymentProvider(req.Provider),
		Type:          payments.PaymentMethodType(req.Type),
		Mask:          req.Mask,
		Label:         req.Label,
		ExpMonth:      req.ExpMonth,
		ExpYear:       req.ExpYear,
		ProviderToken: req.ProviderToken,
		SetAsDefault:  req.SetAsDefault,
	})
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusCreated, mapPaymentMethodToResponse(method))
}

// ListPaymentMethods returns the checkout payment-methods aggregate: the tenant's enabled
// payment gateways, the signed-in user's wallet summary (null for guests or on lookup
// failure), and the user's saved cards. The shape matches ordering-frontend's
// getPaymentMethods() so the checkout can render "Pay with Wallet" plus the enabled gateways.
// @Summary List checkout payment methods
// @Description Aggregate of enabled gateways, wallet summary, and saved cards for checkout.
// @Tags Payment Methods
// @Produce json
// @Success 200 {object} PaymentMethodsAggregateResponse
// @Router /payment-methods [get]
func (h *PaymentMethodHandler) ListPaymentMethods(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, tenantID, err := getUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	resp := PaymentMethodsAggregateResponse{
		Gateways:     []PaymentGatewayResponse{},
		Wallet:       nil,
		SavedMethods: []PaymentMethodResponse{},
	}

	// Enabled gateways for the tenant (best-effort: a treasury hiccup must not break checkout
	// rendering of saved cards). treasury may be nil in contexts without the aggregate wired.
	if h.treasury != nil {
		gateways, gwErr := h.treasury.ListEnabledGateways(ctx, tenantID)
		if gwErr != nil {
			h.log.Warn("failed to list enabled gateways", zap.Error(gwErr), zap.String("tenant_id", tenantID.String()))
		} else {
			resp.Gateways = mapEnabledGateways(gateways)
		}
	}

	// Wallet summary: only for an authenticated user, best-effort. Guests (no user in context)
	// or any lookup error leave wallet null rather than failing the whole response.
	if h.treasury != nil && user != nil {
		bal, balErr := h.treasury.GetUserWalletBalance(ctx, tenantID, user.ID)
		if balErr != nil {
			h.log.Warn("failed to fetch wallet balance", zap.Error(balErr), zap.String("tenant_id", tenantID.String()))
		} else {
			resp.Wallet = &WalletSummaryResponse{
				Balance:  float64(bal.Balance),
				Currency: bal.Currency,
			}
		}
	}

	// Saved cards: existing per-user logic (unchanged).
	if user != nil {
		methods, methodsErr := h.service.ListPaymentMethods(ctx, tenantID, user.ID)
		if methodsErr != nil {
			h.handleError(w, methodsErr)
			return
		}
		saved := make([]PaymentMethodResponse, len(methods))
		for i, m := range methods {
			saved[i] = mapPaymentMethodToResponse(&m)
		}
		resp.SavedMethods = saved
	}

	handlers.RespondJSON(w, http.StatusOK, resp)
}

// mapEnabledGateways converts treasury gateway type keys into the frontend gateway shape,
// attaching display metadata. Unknown keys fall back to the raw key as the name with no icon.
func mapEnabledGateways(keys []treasury.EnabledGateway) []PaymentGatewayResponse {
	out := make([]PaymentGatewayResponse, 0, len(keys))
	for _, k := range keys {
		key := string(k)
		gw := PaymentGatewayResponse{Type: key, Enabled: true}
		if disp, ok := gatewayDisplay[key]; ok {
			gw.Name = disp.Name
			gw.Icon = disp.Icon
		} else {
			gw.Name = key
		}
		out = append(out, gw)
	}
	return out
}

// GetDefaultPaymentMethod retrieves the user's default payment method.
// @Summary Get default payment method
// @Tags Payment Methods
// @Produce json
// @Success 200 {object} PaymentMethodResponse
// @Router /payment-methods/default [get]
func (h *PaymentMethodHandler) GetDefaultPaymentMethod(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, tenantID, err := getUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	method, err := h.service.GetDefaultPaymentMethod(ctx, tenantID, user.ID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, mapPaymentMethodToResponse(method))
}

// GetPaymentMethod retrieves a payment method by ID.
// @Summary Get payment method
// @Tags Payment Methods
// @Produce json
// @Param methodId path string true "Payment Method ID"
// @Success 200 {object} PaymentMethodResponse
// @Router /payment-methods/{methodId} [get]
func (h *PaymentMethodHandler) GetPaymentMethod(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, tenantID, err := getUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	methodID, err := uuid.Parse(chi.URLParam(r, "methodId"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid methodId")
		return
	}

	method, err := h.service.GetPaymentMethod(ctx, tenantID, methodID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	// Verify ownership
	if method.UserID != user.ID {
		handlers.RespondError(w, http.StatusForbidden, "access denied")
		return
	}

	handlers.RespondJSON(w, http.StatusOK, mapPaymentMethodToResponse(method))
}

// UpdatePaymentMethod updates a payment method.
// @Summary Update payment method
// @Tags Payment Methods
// @Accept json
// @Produce json
// @Param methodId path string true "Payment Method ID"
// @Param request body UpdatePaymentMethodRequest true "Update request"
// @Success 200 {object} PaymentMethodResponse
// @Router /payment-methods/{methodId} [put]
func (h *PaymentMethodHandler) UpdatePaymentMethod(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, tenantID, err := getUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	methodID, err := uuid.Parse(chi.URLParam(r, "methodId"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid methodId")
		return
	}

	// Verify ownership
	existing, err := h.service.GetPaymentMethod(ctx, tenantID, methodID)
	if err != nil {
		h.handleError(w, err)
		return
	}
	if existing.UserID != user.ID {
		handlers.RespondError(w, http.StatusForbidden, "access denied")
		return
	}

	var req UpdatePaymentMethodRequest
	if err := handlers.DecodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	method, err := h.service.UpdatePaymentMethod(ctx, payments.UpdatePaymentMethodRequest{
		TenantID: tenantID,
		ID:       methodID,
		Label:    req.Label,
	})
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, mapPaymentMethodToResponse(method))
}

// SetDefaultPaymentMethod sets a payment method as the default.
// @Summary Set default payment method
// @Tags Payment Methods
// @Param methodId path string true "Payment Method ID"
// @Success 204
// @Router /payment-methods/{methodId}/default [put]
func (h *PaymentMethodHandler) SetDefaultPaymentMethod(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, tenantID, err := getUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	methodID, err := uuid.Parse(chi.URLParam(r, "methodId"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid methodId")
		return
	}

	if err := h.service.SetDefaultPaymentMethod(ctx, tenantID, user.ID, methodID); err != nil {
		h.handleError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// DeletePaymentMethod deletes a payment method.
// @Summary Delete payment method
// @Tags Payment Methods
// @Param methodId path string true "Payment Method ID"
// @Success 204
// @Router /payment-methods/{methodId} [delete]
func (h *PaymentMethodHandler) DeletePaymentMethod(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, tenantID, err := getUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	methodID, err := uuid.Parse(chi.URLParam(r, "methodId"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid methodId")
		return
	}

	if err := h.service.DeletePaymentMethod(ctx, tenantID, user.ID, methodID); err != nil {
		h.handleError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
