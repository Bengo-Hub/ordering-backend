package orderinghandler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/http/handlers"
	identityhandler "github.com/bengobox/ordering-backend/internal/http/handlers/identity"
	"github.com/bengobox/ordering-backend/internal/modules/identity"
	"github.com/bengobox/ordering-backend/internal/modules/ordering"
)

// CartHandler exposes cart-related HTTP endpoints.
type CartHandler struct {
	log         *zap.Logger
	cartService *ordering.CartService
}

// NewCartHandler constructs a CartHandler instance.
func NewCartHandler(log *zap.Logger, cartService *ordering.CartService) *CartHandler {
	return &CartHandler{
		log:         log.Named("ordering.CartHandler"),
		cartService: cartService,
	}
}

// Register mounts cart routes on the supplied router.
func (h *CartHandler) Register(r chi.Router, auth *identityhandler.Authenticator) {
	r.Route("/cart", func(cartRouter chi.Router) {
		// Cart operations require authentication
		cartRouter.Use(auth.RequireAuth)

		cartRouter.Get("/", h.GetCurrentCart)
		cartRouter.Post("/items", h.AddItem)
		cartRouter.Put("/items/{itemId}", h.UpdateItem)
		cartRouter.Delete("/items/{itemId}", h.RemoveItem)
		cartRouter.Delete("/", h.ClearCart)
		cartRouter.Get("/summary", h.GetCartSummary)
		cartRouter.Post("/merge", h.MergeGuestCart)
	})
}

// --- Request/Response Types ---

// AddItemRequest represents a request to add an item to cart.
type AddItemRequest struct {
	OutletID      string  `json:"outletId"`
	CatalogItemID string  `json:"catalogItemId"`
	VariantID     *string `json:"variantId,omitempty"`
	Quantity      int     `json:"quantity"`
	Notes         string  `json:"notes,omitempty"`
}

// UpdateItemRequest represents a request to update a cart item.
type UpdateItemRequest struct {
	Quantity *int    `json:"quantity,omitempty"`
	Notes    *string `json:"notes,omitempty"`
}

// MergeCartRequest represents a request to merge a guest cart.
type MergeCartRequest struct {
	OutletID  string `json:"outletId"`
	SessionID string `json:"sessionId"`
}

// --- Helper Functions ---

func decodeJSON(r *http.Request, dst interface{}) error {
	return json.NewDecoder(r.Body).Decode(dst)
}

func (h *CartHandler) handleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ordering.ErrCartNotFound):
		handlers.RespondError(w, http.StatusNotFound, err.Error())

	case errors.Is(err, ordering.ErrCartItemNotFound):
		handlers.RespondError(w, http.StatusNotFound, err.Error())

	case errors.Is(err, ordering.ErrCartExpired),
		errors.Is(err, ordering.ErrCartAlreadyCheckedOut):
		handlers.RespondError(w, http.StatusConflict, err.Error())

	case errors.Is(err, ordering.ErrInvalidQuantity),
		errors.Is(err, ordering.ErrCatalogItemUnavailable),
		errors.Is(err, ordering.ErrVariantUnavailable),
		errors.Is(err, ordering.ErrInvalidCartStatus),
		errors.Is(err, ordering.ErrCartEmpty):
		handlers.RespondError(w, http.StatusBadRequest, err.Error())

	case errors.Is(err, ordering.ErrUnauthorized):
		handlers.RespondError(w, http.StatusForbidden, err.Error())

	default:
		h.log.Error("internal error", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "internal server error")
	}
}

func getTenantID(r *http.Request) (uuid.UUID, error) {
	tenantIDStr := r.Header.Get("X-Tenant-ID")
	if tenantIDStr == "" {
		if val := r.Context().Value("tenant_id"); val != nil {
			if id, ok := val.(uuid.UUID); ok {
				return id, nil
			}
			if str, ok := val.(string); ok {
				return uuid.Parse(str)
			}
		}
		return uuid.Nil, errors.New("tenant ID not found")
	}
	return uuid.Parse(tenantIDStr)
}

func getOutletID(r *http.Request) (uuid.UUID, error) {
	outletIDStr := r.URL.Query().Get("outlet_id")
	if outletIDStr == "" {
		outletIDStr = r.Header.Get("X-Outlet-ID")
	}
	if outletIDStr == "" {
		return uuid.Nil, errors.New("outlet ID required")
	}
	return uuid.Parse(outletIDStr)
}

func getUserFromContext(r *http.Request) (*identity.User, error) {
	user, ok := identity.UserFromContext(r.Context())
	if !ok {
		return nil, errors.New("user not found in context")
	}
	return user, nil
}

// --- Handlers ---

// GetCurrentCart retrieves the current user's active cart.
// @Summary Get current cart
// @Description Retrieves the current user's active cart for a cafe
// @Tags Cart
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param outlet_id query string true "Cafe ID"
// @Success 200 {object} ordering.Cart
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Failure 404 {object} handlers.ErrorResponse
// @Router /cart [get]
func (h *CartHandler) GetCurrentCart(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}

	outletID, err := getOutletID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "outlet_id is required")
		return
	}

	user, err := getUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	cart, err := h.cartService.GetOrCreateCart(r.Context(), tenantID, outletID, &user.ID, "")
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, cart)
}

// AddItem adds an item to the current cart.
// @Summary Add item to cart
// @Description Adds a menu item to the current user's cart
// @Tags Cart
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param payload body AddItemRequest true "Item to add"
// @Success 200 {object} ordering.Cart
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Router /cart/items [post]
func (h *CartHandler) AddItem(w http.ResponseWriter, r *http.Request) {
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

	var req AddItemRequest
	if err := decodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	outletID, err := uuid.Parse(req.OutletID)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid cafe ID")
		return
	}

	catalogItemID, err := uuid.Parse(req.CatalogItemID)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid catalog item ID")
		return
	}

	var variantID *uuid.UUID
	if req.VariantID != nil && *req.VariantID != "" {
		id, err := uuid.Parse(*req.VariantID)
		if err != nil {
			handlers.RespondError(w, http.StatusBadRequest, "invalid variant ID")
			return
		}
		variantID = &id
	}

	cart, err := h.cartService.AddItem(r.Context(), ordering.AddItemRequest{
		TenantID:      tenantID,
		OutletID:      outletID,
		UserID:        &user.ID,
		CatalogItemID: catalogItemID,
		VariantID:     variantID,
		Quantity:      req.Quantity,
		Notes:         req.Notes,
	})
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, cart)
}

// UpdateItem updates a cart item.
// @Summary Update cart item
// @Description Updates the quantity or notes of a cart item
// @Tags Cart
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param itemId path string true "Cart item ID"
// @Param payload body UpdateItemRequest true "Update data"
// @Success 200 {object} ordering.Cart
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Failure 404 {object} handlers.ErrorResponse
// @Router /cart/items/{itemId} [put]
func (h *CartHandler) UpdateItem(w http.ResponseWriter, r *http.Request) {
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

	itemID, err := uuid.Parse(chi.URLParam(r, "itemId"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid item ID")
		return
	}

	var req UpdateItemRequest
	if err := decodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	outletID, err := getOutletID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "outlet_id is required")
		return
	}

	// Get user's cart first
	existingCart, err := h.cartService.GetOrCreateCart(r.Context(), tenantID, outletID, &user.ID, "")
	if err != nil {
		h.handleError(w, err)
		return
	}

	cart, err := h.cartService.UpdateItem(r.Context(), ordering.UpdateItemRequest{
		TenantID: tenantID,
		CartID:   existingCart.ID,
		ItemID:   itemID,
		Quantity: req.Quantity,
		Notes:    req.Notes,
	})
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, cart)
}

// RemoveItem removes an item from the cart.
// @Summary Remove cart item
// @Description Removes an item from the current user's cart
// @Tags Cart
// @Param Authorization header string true "Bearer token"
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param itemId path string true "Cart item ID"
// @Success 200 {object} ordering.Cart
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Failure 404 {object} handlers.ErrorResponse
// @Router /cart/items/{itemId} [delete]
func (h *CartHandler) RemoveItem(w http.ResponseWriter, r *http.Request) {
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

	itemID, err := uuid.Parse(chi.URLParam(r, "itemId"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid item ID")
		return
	}

	outletID, err := getOutletID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "outlet_id is required")
		return
	}

	// Get user's cart first
	existingCart, err := h.cartService.GetOrCreateCart(r.Context(), tenantID, outletID, &user.ID, "")
	if err != nil {
		h.handleError(w, err)
		return
	}

	cart, err := h.cartService.RemoveItem(r.Context(), tenantID, existingCart.ID, itemID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, cart)
}

// ClearCart clears all items from the cart.
// @Summary Clear cart
// @Description Removes all items from the current user's cart
// @Tags Cart
// @Param Authorization header string true "Bearer token"
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param outlet_id query string true "Cafe ID"
// @Success 200 {object} ordering.Cart
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Router /cart [delete]
func (h *CartHandler) ClearCart(w http.ResponseWriter, r *http.Request) {
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

	outletID, err := getOutletID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "outlet_id is required")
		return
	}

	// Get user's cart first
	existingCart, err := h.cartService.GetOrCreateCart(r.Context(), tenantID, outletID, &user.ID, "")
	if err != nil {
		h.handleError(w, err)
		return
	}

	cart, err := h.cartService.ClearCart(r.Context(), tenantID, existingCart.ID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, cart)
}

// GetCartSummary retrieves the cart summary with calculated totals.
// @Summary Get cart summary
// @Description Retrieves the cart summary with subtotal, discounts, taxes, and grand total
// @Tags Cart
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param outlet_id query string true "Outlet ID"
// @Success 200 {object} ordering.CartSummary
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Failure 404 {object} handlers.ErrorResponse
// @Router /cart/summary [get]
func (h *CartHandler) GetCartSummary(w http.ResponseWriter, r *http.Request) {
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

	outletID, err := getOutletID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "outlet_id is required")
		return
	}

	// Get user's cart first
	existingCart, err := h.cartService.GetOrCreateCart(r.Context(), tenantID, outletID, &user.ID, "")
	if err != nil {
		h.handleError(w, err)
		return
	}

	summary, err := h.cartService.GetCartSummary(r.Context(), tenantID, existingCart.ID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, summary)
}

// MergeGuestCart merges a guest cart into the user's cart.
// @Summary Merge guest cart
// @Description Merges items from a guest (session-based) cart into the authenticated user's cart
// @Tags Cart
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer token"
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param payload body MergeCartRequest true "Guest cart session info"
// @Success 200 {object} ordering.Cart
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 401 {object} handlers.ErrorResponse
// @Router /cart/merge [post]
func (h *CartHandler) MergeGuestCart(w http.ResponseWriter, r *http.Request) {
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

	var req MergeCartRequest
	if err := decodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.SessionID == "" {
		handlers.RespondError(w, http.StatusBadRequest, "sessionId is required")
		return
	}

	outletID, err := uuid.Parse(req.OutletID)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid cafe ID")
		return
	}

	cart, err := h.cartService.MergeGuestCart(r.Context(), tenantID, outletID, req.SessionID, user.ID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, cart)
}
