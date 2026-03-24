package orderinghandler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/Bengo-Hub/httpware"
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
		// Guest cart routes (no auth required, identified by session_id)
		cartRouter.Route("/guest", func(guestRouter chi.Router) {
			guestRouter.Get("/", h.GetGuestCart)
			guestRouter.Post("/items", h.AddGuestItem)
			guestRouter.Put("/items/{itemId}", h.UpdateGuestItem)
			guestRouter.Delete("/items/{itemId}", h.RemoveGuestItem)
		})

		// Authenticated cart operations (grouped to avoid middleware-after-routes panic)
		cartRouter.Group(func(authedCart chi.Router) {
			authedCart.Use(auth.RequireAuth)
			authedCart.Get("/", h.GetCurrentCart)
			authedCart.Post("/items", h.AddItem)
			authedCart.Put("/items/{itemId}", h.UpdateItem)
			authedCart.Delete("/items/{itemId}", h.RemoveItem)
			authedCart.Delete("/", h.ClearCart)
			authedCart.Get("/summary", h.GetCartSummary)
			authedCart.Post("/merge", h.MergeGuestCart)
		})
	})
}

// --- Request/Response Types ---

// AddItemRequest represents a request to add an item to cart.
type AddItemRequest struct {
	OutletID     string  `json:"outletId"`
	InventorySKU string  `json:"inventorySku"`
	VariantID    *string `json:"variantId,omitempty"`
	Quantity     int     `json:"quantity"`
	Notes        string  `json:"notes,omitempty"`
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
	ctx := r.Context()

	// Platform owner query-param override
	if httpware.IsPlatformOwner(ctx) {
		if q := r.URL.Query().Get("tenantId"); q != "" {
			return uuid.Parse(q)
		}
	}

	// httpware context (from TenantV2 middleware)
	if tenantIDStr := httpware.GetTenantID(ctx); tenantIDStr != "" {
		return uuid.Parse(tenantIDStr)
	}

	// Fallback: header or context value
	tenantIDStr := r.Header.Get("X-Tenant-ID")
	if tenantIDStr == "" {
		if val := ctx.Value("tenant_id"); val != nil {
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
		handlers.RespondError(w, http.StatusBadRequest, "invalid outlet ID")
		return
	}

	if req.InventorySKU == "" {
		handlers.RespondError(w, http.StatusBadRequest, "inventorySku is required")
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
		TenantID:     tenantID,
		OutletID:     outletID,
		UserID:       &user.ID,
		InventorySKU: req.InventorySKU,
		VariantID:    variantID,
		Quantity:     req.Quantity,
		Notes:        req.Notes,
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

// --- Guest Cart Helpers ---

// getSessionID extracts the session ID from the X-Session-ID header or session_id cookie.
func getSessionID(r *http.Request) string {
	if sid := r.Header.Get("X-Session-ID"); sid != "" {
		return sid
	}
	if cookie, err := r.Cookie("session_id"); err == nil && cookie.Value != "" {
		return cookie.Value
	}
	return ""
}

// --- Guest Cart Handlers ---

// GetGuestCart retrieves the guest cart identified by session ID.
// @Summary Get guest cart
// @Description Retrieves the guest cart identified by session ID (no auth required)
// @Tags Cart
// @Produce json
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param X-Session-ID header string true "Session ID"
// @Param outlet_id query string true "Outlet ID"
// @Success 200 {object} ordering.Cart
// @Failure 400 {object} handlers.ErrorResponse
// @Router /cart/guest [get]
func (h *CartHandler) GetGuestCart(w http.ResponseWriter, r *http.Request) {
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

	sessionID := getSessionID(r)
	if sessionID == "" {
		handlers.RespondError(w, http.StatusBadRequest, "session ID is required (X-Session-ID header or session_id cookie)")
		return
	}

	cart, err := h.cartService.GetOrCreateCart(r.Context(), tenantID, outletID, nil, sessionID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, cart)
}

// AddGuestItem adds an item to a guest cart identified by session ID.
// @Summary Add item to guest cart
// @Description Adds a menu item to a guest cart (no auth required)
// @Tags Cart
// @Accept json
// @Produce json
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param X-Session-ID header string true "Session ID"
// @Param payload body AddItemRequest true "Item to add"
// @Success 200 {object} ordering.Cart
// @Failure 400 {object} handlers.ErrorResponse
// @Router /cart/guest/items [post]
func (h *CartHandler) AddGuestItem(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}

	sessionID := getSessionID(r)
	if sessionID == "" {
		handlers.RespondError(w, http.StatusBadRequest, "session ID is required (X-Session-ID header or session_id cookie)")
		return
	}

	var req AddItemRequest
	if err := decodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	outletID, err := uuid.Parse(req.OutletID)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid outlet ID")
		return
	}

	if req.InventorySKU == "" {
		handlers.RespondError(w, http.StatusBadRequest, "inventorySku is required")
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
		TenantID:     tenantID,
		OutletID:     outletID,
		UserID:       nil,
		SessionID:    sessionID,
		InventorySKU: req.InventorySKU,
		VariantID:    variantID,
		Quantity:     req.Quantity,
		Notes:        req.Notes,
	})
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, cart)
}

// UpdateGuestItem updates a guest cart item.
// @Summary Update guest cart item
// @Description Updates the quantity or notes of a guest cart item (no auth required)
// @Tags Cart
// @Accept json
// @Produce json
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param X-Session-ID header string true "Session ID"
// @Param itemId path string true "Cart item ID"
// @Param outlet_id query string true "Outlet ID"
// @Param payload body UpdateItemRequest true "Update data"
// @Success 200 {object} ordering.Cart
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 404 {object} handlers.ErrorResponse
// @Router /cart/guest/items/{itemId} [put]
func (h *CartHandler) UpdateGuestItem(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}

	sessionID := getSessionID(r)
	if sessionID == "" {
		handlers.RespondError(w, http.StatusBadRequest, "session ID is required (X-Session-ID header or session_id cookie)")
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

	var req UpdateItemRequest
	if err := decodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Get guest cart
	existingCart, err := h.cartService.GetOrCreateCart(r.Context(), tenantID, outletID, nil, sessionID)
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

// RemoveGuestItem removes an item from a guest cart.
// @Summary Remove guest cart item
// @Description Removes an item from a guest cart (no auth required)
// @Tags Cart
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param X-Session-ID header string true "Session ID"
// @Param itemId path string true "Cart item ID"
// @Param outlet_id query string true "Outlet ID"
// @Success 200 {object} ordering.Cart
// @Failure 400 {object} handlers.ErrorResponse
// @Failure 404 {object} handlers.ErrorResponse
// @Router /cart/guest/items/{itemId} [delete]
func (h *CartHandler) RemoveGuestItem(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid tenant")
		return
	}

	sessionID := getSessionID(r)
	if sessionID == "" {
		handlers.RespondError(w, http.StatusBadRequest, "session ID is required (X-Session-ID header or session_id cookie)")
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

	// Get guest cart
	existingCart, err := h.cartService.GetOrCreateCart(r.Context(), tenantID, outletID, nil, sessionID)
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
