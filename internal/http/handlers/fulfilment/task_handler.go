package fulfilment

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/Bengo-Hub/httpware"
	handlers "github.com/bengobox/ordering-backend/internal/http/handlers"
	identityhandler "github.com/bengobox/ordering-backend/internal/http/handlers/identity"
	"github.com/bengobox/ordering-backend/internal/modules/fulfilment"
	"github.com/bengobox/ordering-backend/internal/modules/identity"
	"github.com/bengobox/ordering-backend/internal/modules/ordering"
	"github.com/bengobox/ordering-backend/internal/platform/subscriptions"
)

// OrderFetcher is the minimal interface needed to look up order details for task creation.
type OrderFetcher interface {
	GetOrder(ctx context.Context, tenantID, orderID uuid.UUID) (*ordering.Order, error)
}

// TaskHandler handles delivery task HTTP requests.
type TaskHandler struct {
	logger   *zap.Logger
	taskSvc  *fulfilment.TaskService
	orderSvc OrderFetcher
}

// NewTaskHandler creates a new task handler.
func NewTaskHandler(logger *zap.Logger, taskSvc *fulfilment.TaskService, orderSvc OrderFetcher) *TaskHandler {
	return &TaskHandler{
		logger:   logger,
		taskSvc:  taskSvc,
		orderSvc: orderSvc,
	}
}

// Register registers the task routes.
func (h *TaskHandler) Register(r chi.Router, auth *identityhandler.Authenticator) {
	r.Route("/orders/{orderId}/delivery", func(delivery chi.Router) {
		delivery.Use(auth.RequireAuth)
		// Delivery tasks only apply to outlet types that handle physical delivery
		delivery.With(subscriptions.RequireUseCase("hospitality", "quick_service", "food_delivery")).Post("/create-task", h.CreateDeliveryTask)
		delivery.Get("/task", h.GetDeliveryTask)
		delivery.Post("/cancel-task", h.CancelDeliveryTask)
		delivery.Get("/tracking", h.GetTracking)
	})

	r.Route("/assignments", func(assignments chi.Router) {
		assignments.Use(auth.RequireAuth)
		assignments.Get("/", h.ListAssignments)
		assignments.Get("/{id}", h.GetAssignment)
	})
}

// CreateDeliveryTaskRequest represents a request to create a delivery task.
type CreateDeliveryTaskRequest struct {
	Priority       string `json:"priority,omitempty"`
	Instructions   string `json:"instructions,omitempty"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

// CreateDeliveryTask creates a delivery task for an order.
// @Summary Create delivery task
// @Tags Delivery
// @Accept json
// @Produce json
// @Param tenant path string true "Tenant slug"
// @Param orderId path string true "Order ID"
// @Param request body CreateDeliveryTaskRequest true "Task request"
// @Success 201 {object} fulfilment.OrderAssignment
// @Router /api/v1/{tenant}/orders/{orderId}/delivery/create-task [post]
func (h *TaskHandler) CreateDeliveryTask(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, tenantID, err := getUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	orderIDStr := chi.URLParam(r, "orderId")
	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid order ID")
		return
	}

	var req CreateDeliveryTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	priority := fulfilment.PriorityNormal
	if req.Priority != "" {
		priority = fulfilment.TaskPriority(req.Priority)
	}

	tenantSlug := chi.URLParam(r, "tenant")

	// Build OrderInfo from stored order details.
	orderInfo := fulfilment.OrderInfo{
		ID:         orderID,
		TenantID:   tenantID,
		TenantSlug: tenantSlug,
	}
	if h.orderSvc != nil {
		if order, oErr := h.orderSvc.GetOrder(ctx, tenantID, orderID); oErr == nil {
			orderInfo.OrderNumber = order.OrderNumber
			orderInfo.CustomerName = order.CustomerName
			orderInfo.CustomerPhone = order.CustomerPhone
			orderInfo.Instructions = order.Instructions
			orderInfo.DeliveryFee = order.DeliveryFee
			orderInfo.ItemCount = len(order.Items)
			if len(order.Items) > 0 {
				names := make([]string, 0, len(order.Items))
				for _, item := range order.Items {
					names = append(names, item.NameSnapshot)
				}
				orderInfo.ItemsDescription = strings.Join(names, ", ")
			}
			if order.DeliveryAddress != nil {
				addr := order.DeliveryAddress
				parts := []string{addr.AddressLine1}
				if addr.AddressLine2 != "" {
					parts = append(parts, addr.AddressLine2)
				}
				if addr.City != "" {
					parts = append(parts, addr.City)
				}
				orderInfo.DeliveryAddress = strings.Join(parts, ", ")
				if addr.Latitude != nil {
					orderInfo.DeliveryLat = *addr.Latitude
				}
				if addr.Longitude != nil {
					orderInfo.DeliveryLng = *addr.Longitude
				}
				orderInfo.DeliveryNotes = addr.Instructions
				if addr.ContactName != "" {
					orderInfo.CustomerName = addr.ContactName
				}
				if addr.ContactPhone != "" {
					orderInfo.CustomerPhone = addr.ContactPhone
				}
			}
		}
	}
	// Allow request-body instructions to override
	if req.Instructions != "" {
		orderInfo.Instructions = req.Instructions
	}

	taskReq := fulfilment.CreateDeliveryTaskRequest{
		OrderInfo:      orderInfo,
		Priority:       priority,
		IdempotencyKey: req.IdempotencyKey,
	}

	assignment, err := h.taskSvc.CreateDeliveryTask(ctx, taskReq)
	if err != nil {
		h.logger.Error("failed to create delivery task",
			zap.Error(err),
			zap.String("order_id", orderID.String()),
			zap.String("user_id", user.ID.String()))

		if errors.Is(err, fulfilment.ErrTaskCreationFailed) {
			handlers.RespondError(w, http.StatusServiceUnavailable, "logistics service unavailable")
			return
		}
		handlers.RespondError(w, http.StatusInternalServerError, "failed to create delivery task")
		return
	}

	handlers.RespondJSON(w, http.StatusCreated, assignment)
}

// GetDeliveryTask retrieves the delivery task for an order.
// @Summary Get delivery task
// @Tags Delivery
// @Produce json
// @Param tenant path string true "Tenant slug"
// @Param orderId path string true "Order ID"
// @Success 200 {object} fulfilment.OrderAssignment
// @Router /api/v1/{tenant}/orders/{orderId}/delivery/task [get]
func (h *TaskHandler) GetDeliveryTask(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, tenantID, err := getUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	orderIDStr := chi.URLParam(r, "orderId")
	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid order ID")
		return
	}

	assignment, err := h.taskSvc.GetAssignmentByOrderID(ctx, tenantID, orderID)
	if err != nil {
		if errors.Is(err, fulfilment.ErrAssignmentNotFound) {
			handlers.RespondError(w, http.StatusNotFound, "delivery task not found")
			return
		}
		handlers.RespondError(w, http.StatusInternalServerError, "failed to get delivery task")
		return
	}

	handlers.RespondJSON(w, http.StatusOK, assignment)
}

// CancelDeliveryTaskRequest represents a request to cancel a delivery task.
type CancelDeliveryTaskRequest struct {
	Reason string `json:"reason"`
}

// CancelDeliveryTask cancels the delivery task for an order.
// @Summary Cancel delivery task
// @Tags Delivery
// @Accept json
// @Produce json
// @Param tenant path string true "Tenant slug"
// @Param orderId path string true "Order ID"
// @Param request body CancelDeliveryTaskRequest true "Cancel request"
// @Success 200 {object} map[string]string
// @Router /api/v1/{tenant}/orders/{orderId}/delivery/cancel-task [post]
func (h *TaskHandler) CancelDeliveryTask(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, tenantID, err := getUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	tenantSlug := chi.URLParam(r, "tenant")
	orderIDStr := chi.URLParam(r, "orderId")
	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid order ID")
		return
	}

	var req CancelDeliveryTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Reason == "" {
		handlers.RespondError(w, http.StatusBadRequest, "cancellation reason is required")
		return
	}

	if err := h.taskSvc.CancelDeliveryTask(ctx, tenantSlug, tenantID, orderID, req.Reason); err != nil {
		h.logger.Error("failed to cancel delivery task",
			zap.Error(err),
			zap.String("order_id", orderID.String()),
			zap.String("user_id", user.ID.String()))

		if errors.Is(err, fulfilment.ErrAssignmentNotFound) {
			handlers.RespondError(w, http.StatusNotFound, "delivery task not found")
			return
		}
		if errors.Is(err, fulfilment.ErrAssignmentNotCancellable) {
			handlers.RespondError(w, http.StatusBadRequest, "delivery task cannot be cancelled")
			return
		}
		handlers.RespondError(w, http.StatusInternalServerError, "failed to cancel delivery task")
		return
	}

	handlers.RespondJSON(w, http.StatusOK, map[string]string{"message": "delivery task cancelled"})
}

// GetTracking retrieves real-time tracking information.
// @Summary Get tracking info
// @Tags Delivery
// @Produce json
// @Param tenant path string true "Tenant slug"
// @Param orderId path string true "Order ID"
// @Success 200 {object} fulfilment.TrackingInfo
// @Router /api/v1/{tenant}/orders/{orderId}/delivery/tracking [get]
func (h *TaskHandler) GetTracking(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, tenantID, err := getUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	tenantSlug := chi.URLParam(r, "tenant")
	orderIDStr := chi.URLParam(r, "orderId")
	orderID, err := uuid.Parse(orderIDStr)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid order ID")
		return
	}

	tracking, err := h.taskSvc.GetTracking(ctx, tenantSlug, tenantID, orderID)
	if err != nil {
		if errors.Is(err, fulfilment.ErrAssignmentNotFound) {
			handlers.RespondError(w, http.StatusNotFound, "delivery task not found")
			return
		}
		if errors.Is(err, fulfilment.ErrTrackingNotAvailable) {
			handlers.RespondError(w, http.StatusServiceUnavailable, "tracking not available")
			return
		}
		handlers.RespondError(w, http.StatusInternalServerError, "failed to get tracking")
		return
	}

	handlers.RespondJSON(w, http.StatusOK, tracking)
}

// GetAssignment retrieves an assignment by ID.
// @Summary Get assignment
// @Tags Assignments
// @Produce json
// @Param tenant path string true "Tenant slug"
// @Param id path string true "Assignment ID"
// @Success 200 {object} fulfilment.OrderAssignment
// @Router /api/v1/{tenant}/assignments/{id} [get]
func (h *TaskHandler) GetAssignment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, tenantID, err := getUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid assignment ID")
		return
	}

	assignment, err := h.taskSvc.GetAssignment(ctx, tenantID, id)
	if err != nil {
		if errors.Is(err, fulfilment.ErrAssignmentNotFound) {
			handlers.RespondError(w, http.StatusNotFound, "assignment not found")
			return
		}
		handlers.RespondError(w, http.StatusInternalServerError, "failed to get assignment")
		return
	}

	handlers.RespondJSON(w, http.StatusOK, assignment)
}

// ListAssignments lists assignments with filters.
// @Summary List assignments
// @Tags Assignments
// @Produce json
// @Param tenant path string true "Tenant slug"
// @Param order_id query string false "Filter by order ID"
// @Param status query string false "Filter by status"
// @Param limit query int false "Limit"
// @Param offset query int false "Offset"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/{tenant}/assignments [get]
func (h *TaskHandler) ListAssignments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, tenantID, err := getUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	filter := fulfilment.AssignmentFilter{
		TenantID: tenantID,
		Limit:    handlers.ParseInt(r.URL.Query().Get("limit"), 20),
		Offset:   handlers.ParseInt(r.URL.Query().Get("offset"), 0),
	}

	if orderIDStr := r.URL.Query().Get("order_id"); orderIDStr != "" {
		orderID, err := uuid.Parse(orderIDStr)
		if err == nil {
			filter.OrderID = &orderID
		}
	}

	if status := r.URL.Query().Get("status"); status != "" {
		filter.Status = []fulfilment.AssignmentStatus{fulfilment.AssignmentStatus(status)}
	}

	assignments, total, err := h.taskSvc.ListAssignments(ctx, filter)
	if err != nil {
		handlers.RespondError(w, http.StatusInternalServerError, "failed to list assignments")
		return
	}

	handlers.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"data":   assignments,
		"total":  total,
		"limit":  filter.Limit,
		"offset": filter.Offset,
	})
}

// Helper function to get user from context with platform-owner tenant override.
func getUserFromContext(r *http.Request) (*identity.User, uuid.UUID, error) {
	ctx := r.Context()
	// Platform owner query-param override
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
