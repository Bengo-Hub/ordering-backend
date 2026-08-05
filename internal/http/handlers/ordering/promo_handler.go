package orderinghandler

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/http/handlers"
	identityhandler "github.com/bengobox/ordering-backend/internal/http/handlers/identity"
	"github.com/bengobox/ordering-backend/internal/modules/ordering"
	"github.com/bengobox/ordering-backend/internal/platform/subscriptions"
)

// PromoHandler exposes promo code HTTP endpoints.
type PromoHandler struct {
	log         *zap.Logger
	promoSvc    *ordering.PromoService
	cartService *ordering.CartService
}

// NewPromoHandler constructs a PromoHandler instance.
func NewPromoHandler(log *zap.Logger, promoSvc *ordering.PromoService, cartService *ordering.CartService) *PromoHandler {
	return &PromoHandler{
		log:         log.Named("ordering.PromoHandler"),
		promoSvc:    promoSvc,
		cartService: cartService,
	}
}

// Register mounts promo routes on the supplied router.
func (h *PromoHandler) Register(r chi.Router, auth *identityhandler.Authenticator) {
	r.Route("/promo-codes", func(promoRouter chi.Router) {
		// Public validation endpoint (auth optional but recommended)
		promoRouter.Post("/validate", h.ValidatePromoCode)

		// Cart-related promo endpoints (requires auth)
		promoRouter.Group(func(cartPromoRouter chi.Router) {
			cartPromoRouter.Use(auth.RequireAuth)
			cartPromoRouter.Use(subscriptions.RequireFeature("promo_codes"))
			cartPromoRouter.Post("/apply", h.ApplyPromoToCart)
			cartPromoRouter.Delete("/remove", h.RemovePromoFromCart)
		})
	})
}

// --- Request/Response Types ---

// ValidatePromoLineRequest is one cart line the storefront sends so a code can be evaluated
// against the SAME schedule/meal_period/item-or-category scope/BOGO rules the POS terminal uses,
// instead of a flat subtotal.
type ValidatePromoLineRequest struct {
	SKU       string  `json:"sku"`
	Category  string  `json:"category,omitempty"`
	Quantity  float64 `json:"quantity"`
	UnitPrice float64 `json:"unitPrice"`
}

// ValidatePromoRequest represents a request to validate a promo code.
type ValidatePromoRequest struct {
	Code   string                      `json:"code"`
	CafeID string                      `json:"cafeId"`
	Items  []ValidatePromoLineRequest `json:"items"`
	// Subtotal is kept for backward compatibility with any caller that hasn't been updated to
	// send real items yet; when Items is non-empty it's ignored (recomputed from Items instead).
	Subtotal float64 `json:"subtotal"`
}

// ApplyPromoRequest represents a request to apply a promo code to a cart.
type ApplyPromoRequest struct {
	Code   string `json:"code"`
	CartID string `json:"cartId"`
}

// RemovePromoRequest represents a request to remove a promo code from a cart.
type RemovePromoRequest struct {
	CartID string `json:"cartId"`
}

// --- Helper Functions ---

func (h *PromoHandler) handleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ordering.ErrPromoCodeNotFound):
		handlers.RespondError(w, http.StatusNotFound, err.Error())

	case errors.Is(err, ordering.ErrCartNotFound):
		handlers.RespondError(w, http.StatusNotFound, err.Error())

	case errors.Is(err, ordering.ErrCartAlreadyCheckedOut):
		handlers.RespondError(w, http.StatusConflict, err.Error())

	case errors.Is(err, ordering.ErrPromoCodeExpired),
		errors.Is(err, ordering.ErrPromoCodeNotStarted),
		errors.Is(err, ordering.ErrPromoCodeMaxUses),
		errors.Is(err, ordering.ErrPromoCodeUserLimit),
		errors.Is(err, ordering.ErrPromoCodeMinSubtotal),
		errors.Is(err, ordering.ErrPromoCodeInactive),
		errors.Is(err, ordering.ErrPromoCodeInvalidCafe),
		errors.Is(err, ordering.ErrPromoCodeAlreadyUsed):
		handlers.RespondError(w, http.StatusBadRequest, err.Error())

	case errors.Is(err, ordering.ErrUnauthorized):
		handlers.RespondError(w, http.StatusForbidden, err.Error())

	default:
		h.log.Error("internal error", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "internal server error")
	}
}

// --- Handlers ---

// ValidatePromoCode validates a promo code without applying it.
// @Summary Validate promo code
// @Description Validates a promo code and returns discount information
// @Tags Promo Codes
// @Accept json
// @Produce json
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param payload body ValidatePromoRequest true "Promo code to validate"
// @Success 200 {object} ordering.PromoValidationResult
// @Failure 400 {object} handlers.ErrorResponse
// @Router /promo-codes/validate [post]
func (h *PromoHandler) ValidatePromoCode(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}

	var req ValidatePromoRequest
	if err := decodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Code == "" {
		handlers.RespondError(w, http.StatusBadRequest, "promo code is required")
		return
	}

	cafeID, err := uuid.Parse(req.CafeID)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid cafe ID")
		return
	}

	// Get user ID if authenticated (optional for validation)
	var userID *uuid.UUID
	if user, err := getUserFromContext(r); err == nil {
		userID = &user.ID
	}

	// Real cart lines let the SoT evaluator (pos-api) enforce scope/schedule/BOGO; a caller that
	// hasn't been updated yet (or a guest with an empty cart) falls back to a single synthetic
	// storewide line built from req.Subtotal so min-subtotal/percentage-off codes still resolve.
	items := make([]ordering.CartItem, 0, len(req.Items))
	for _, l := range req.Items {
		items = append(items, ordering.CartItem{
			InventorySKU: l.SKU,
			Quantity:     int(l.Quantity),
			UnitPrice:    l.UnitPrice,
			TotalPrice:   l.UnitPrice * l.Quantity,
		})
	}
	if len(items) == 0 && req.Subtotal > 0 {
		items = append(items, ordering.CartItem{Quantity: 1, UnitPrice: req.Subtotal, TotalPrice: req.Subtotal})
	}

	result, err := h.promoSvc.ValidatePromoCode(r.Context(), tenantID, cafeID, req.Code, items, userID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, result)
}

// ApplyPromoToCart applies a promo code to a cart.
// @Summary Apply promo code to cart
// @Description Applies a promo code to the specified cart
// @Tags Promo Codes
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param payload body ApplyPromoRequest true "Promo code and cart"
// @Success 200 {object} ordering.Cart
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Failure 404 {object} handlers.ErrorResponse
// @Router /promo-codes/apply [post]
func (h *PromoHandler) ApplyPromoToCart(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}

	user, err := getUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req ApplyPromoRequest
	if err := decodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Code == "" {
		handlers.RespondError(w, http.StatusBadRequest, "promo code is required")
		return
	}

	cartID, err := uuid.Parse(req.CartID)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid cart ID")
		return
	}

	// Verify cart belongs to user
	cart, err := h.cartService.GetCart(r.Context(), tenantID, cartID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	if cart.UserID == nil || *cart.UserID != user.ID {
		handlers.RespondError(w, http.StatusForbidden, "access denied")
		return
	}

	updatedCart, err := h.promoSvc.ApplyPromoToCart(r.Context(), tenantID, cartID, req.Code, &user.ID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, updatedCart)
}

// RemovePromoFromCart removes a promo code from a cart.
// @Summary Remove promo code from cart
// @Description Removes the applied promo code from the specified cart
// @Tags Promo Codes
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param payload body RemovePromoRequest true "Cart ID"
// @Success 200 {object} ordering.Cart
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Failure 404 {object} handlers.ErrorResponse
// @Router /promo-codes/remove [delete]
func (h *PromoHandler) RemovePromoFromCart(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}

	user, err := getUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req RemovePromoRequest
	if err := decodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	cartID, err := uuid.Parse(req.CartID)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid cart ID")
		return
	}

	// Verify cart belongs to user
	cart, err := h.cartService.GetCart(r.Context(), tenantID, cartID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	if cart.UserID == nil || *cart.UserID != user.ID {
		handlers.RespondError(w, http.StatusForbidden, "access denied")
		return
	}

	updatedCart, err := h.promoSvc.RemovePromoFromCart(r.Context(), tenantID, cartID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, updatedCart)
}
