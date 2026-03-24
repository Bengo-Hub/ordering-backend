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
)

// GroupOrderHandler exposes group ordering HTTP endpoints.
type GroupOrderHandler struct {
	log     *zap.Logger
	service *ordering.GroupOrderService
}

// NewGroupOrderHandler constructs a GroupOrderHandler instance.
func NewGroupOrderHandler(log *zap.Logger, service *ordering.GroupOrderService) *GroupOrderHandler {
	return &GroupOrderHandler{
		log:     log.Named("ordering.GroupOrderHandler"),
		service: service,
	}
}

// Register mounts group order routes on the supplied router.
func (h *GroupOrderHandler) Register(r chi.Router, auth *identityhandler.Authenticator) {
	r.Route("/group-orders", func(goRouter chi.Router) {
		goRouter.Use(auth.RequireAuth)

		goRouter.Post("/", h.CreateGroupOrder)
		goRouter.Post("/join", h.JoinGroupOrder)
		goRouter.Put("/{id}/lock", h.LockGroupOrder)
		goRouter.Get("/{id}", h.GetGroupOrder)
	})
}

// --- Request Types ---

type createGroupOrderRequest struct {
	CartID string `json:"cartId"`
}

type joinGroupOrderRequest struct {
	InviteCode string `json:"inviteCode"`
	UserName   string `json:"userName"`
}

// --- Handlers ---

// CreateGroupOrder creates a new group ordering session.
func (h *GroupOrderHandler) CreateGroupOrder(w http.ResponseWriter, r *http.Request) {
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

	var req createGroupOrderRequest
	if err := handlers.DecodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	cartID, err := uuid.Parse(req.CartID)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid cart ID")
		return
	}

	groupOrder, err := h.service.CreateGroupOrder(r.Context(), tenantID, user.ID, cartID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusCreated, groupOrder)
}

// JoinGroupOrder joins a group order by invite code.
func (h *GroupOrderHandler) JoinGroupOrder(w http.ResponseWriter, r *http.Request) {
	user, err := getUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req joinGroupOrderRequest
	if err := handlers.DecodeJSON(r, &req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.InviteCode == "" {
		handlers.RespondError(w, http.StatusBadRequest, "invite code is required")
		return
	}

	userName := req.UserName
	if userName == "" {
		userName = user.FullName
		if userName == "" {
			userName = "Guest"
		}
	}

	groupOrder, err := h.service.JoinGroupOrder(r.Context(), req.InviteCode, user.ID, userName)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, groupOrder)
}

// LockGroupOrder locks a group order for checkout (host only).
func (h *GroupOrderHandler) LockGroupOrder(w http.ResponseWriter, r *http.Request) {
	user, err := getUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	groupOrderID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid group order ID")
		return
	}

	groupOrder, err := h.service.LockGroupOrder(r.Context(), groupOrderID, user.ID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, groupOrder)
}

// GetGroupOrder retrieves a group order by ID.
func (h *GroupOrderHandler) GetGroupOrder(w http.ResponseWriter, r *http.Request) {
	groupOrderID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid group order ID")
		return
	}

	groupOrder, err := h.service.GetGroupOrder(r.Context(), groupOrderID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, groupOrder)
}

func (h *GroupOrderHandler) handleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ordering.ErrGroupOrderNotFound):
		handlers.RespondError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ordering.ErrInvalidInviteCode):
		handlers.RespondError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ordering.ErrGroupOrderExpired):
		handlers.RespondError(w, http.StatusGone, err.Error())
	case errors.Is(err, ordering.ErrGroupOrderNotOpen):
		handlers.RespondError(w, http.StatusConflict, err.Error())
	case errors.Is(err, ordering.ErrGroupOrderFull):
		handlers.RespondError(w, http.StatusConflict, err.Error())
	case errors.Is(err, ordering.ErrAlreadyParticipant):
		handlers.RespondError(w, http.StatusConflict, err.Error())
	case errors.Is(err, ordering.ErrNotGroupOrderHost):
		handlers.RespondError(w, http.StatusForbidden, err.Error())
	default:
		h.log.Error("internal error", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "internal server error")
	}
}
