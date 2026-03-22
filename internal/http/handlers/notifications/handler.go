package notificationshandler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/Bengo-Hub/httpware"
	"github.com/bengobox/ordering-backend/internal/http/handlers"
	identityhandler "github.com/bengobox/ordering-backend/internal/http/handlers/identity"
	"github.com/bengobox/ordering-backend/internal/modules/identity"
	"github.com/bengobox/ordering-backend/internal/modules/notifications"
)

// Handler handles notification HTTP requests.
type Handler struct {
	logger  *zap.Logger
	service *notifications.Service
}

// NewHandler creates a new notifications handler.
func NewHandler(logger *zap.Logger, service *notifications.Service) *Handler {
	return &Handler{
		logger:  logger.Named("notifications.handler"),
		service: service,
	}
}

// Register registers the notification routes.
func (h *Handler) Register(r chi.Router, auth *identityhandler.Authenticator) {
	r.Route("/notifications", func(notif chi.Router) {
		notif.Use(auth.RequireAuth)

		// Templates
		notif.With(auth.RequirePermissions(identity.PermissionNotificationsManage)).
			Post("/templates", h.CreateTemplate)
		notif.With(auth.RequirePermissions(identity.PermissionNotificationsView)).
			Get("/templates", h.ListTemplates)
		notif.With(auth.RequirePermissions(identity.PermissionNotificationsView)).
			Get("/templates/{templateId}", h.GetTemplate)
		notif.With(auth.RequirePermissions(identity.PermissionNotificationsManage)).
			Put("/templates/{templateId}", h.UpdateTemplate)
		notif.With(auth.RequirePermissions(identity.PermissionNotificationsManage)).
			Delete("/templates/{templateId}", h.DeleteTemplate)

		// Events
		notif.With(auth.RequirePermissions(identity.PermissionNotificationsView)).
			Get("/events", h.ListEvents)
		notif.With(auth.RequirePermissions(identity.PermissionNotificationsView)).
			Get("/events/{eventId}", h.GetEvent)

		// User preferences
		notif.Get("/preferences", h.GetPreferences)
		notif.Put("/preferences", h.UpdatePreferences)
	})
}

// --- Template Handlers ---

// CreateTemplate creates a new notification template.
func (h *Handler) CreateTemplate(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req notifications.CreateTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.TenantID = tenantID

	template, err := h.service.CreateTemplate(r.Context(), &req)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusCreated, template)
}

// ListTemplates lists notification templates.
func (h *Handler) ListTemplates(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	filter := notifications.TemplateFilter{
		TenantID: tenantID,
		Limit:    50,
	}

	if v := r.URL.Query().Get("channel"); v != "" {
		channel := notifications.NotificationChannel(v)
		filter.Channel = &channel
	}
	if v := r.URL.Query().Get("event_key"); v != "" {
		filter.EventKey = &v
	}
	if v := r.URL.Query().Get("locale"); v != "" {
		filter.Locale = &v
	}
	if v := r.URL.Query().Get("active"); v != "" {
		isActive := v == "true"
		filter.IsActive = &isActive
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if offset, err := strconv.Atoi(v); err == nil {
			filter.Offset = offset
		}
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if limit, err := strconv.Atoi(v); err == nil && limit > 0 && limit <= 100 {
			filter.Limit = limit
		}
	}

	templates, total, err := h.service.ListTemplates(r.Context(), tenantID, filter)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"templates": templates,
		"total":     total,
	})
}

// GetTemplate retrieves a template by ID.
func (h *Handler) GetTemplate(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	templateID, err := uuid.Parse(chi.URLParam(r, "templateId"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid template ID")
		return
	}

	template, err := h.service.GetTemplate(r.Context(), tenantID, templateID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, template)
}

// UpdateTemplate updates a template.
func (h *Handler) UpdateTemplate(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	templateID, err := uuid.Parse(chi.URLParam(r, "templateId"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid template ID")
		return
	}

	var req notifications.UpdateTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.service.UpdateTemplate(r.Context(), tenantID, templateID, &req); err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// DeleteTemplate deletes a template.
func (h *Handler) DeleteTemplate(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	templateID, err := uuid.Parse(chi.URLParam(r, "templateId"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid template ID")
		return
	}

	if err := h.service.DeleteTemplate(r.Context(), tenantID, templateID); err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// --- Event Handlers ---

// ListEvents lists notification events.
func (h *Handler) ListEvents(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	filter := notifications.NotificationEventFilter{
		TenantID: tenantID,
		Limit:    50,
	}

	if v := r.URL.Query().Get("user_id"); v != "" {
		if id, err := uuid.Parse(v); err == nil {
			filter.UserID = &id
		}
	}
	if v := r.URL.Query().Get("event_key"); v != "" {
		filter.EventKey = &v
	}
	if v := r.URL.Query().Get("status"); v != "" {
		status := notifications.EventStatus(v)
		filter.Status = &status
	}
	if v := r.URL.Query().Get("order_id"); v != "" {
		if id, err := uuid.Parse(v); err == nil {
			filter.OrderID = &id
		}
	}
	if v := r.URL.Query().Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.DateFrom = &t
		}
	}
	if v := r.URL.Query().Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.DateTo = &t
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if offset, err := strconv.Atoi(v); err == nil {
			filter.Offset = offset
		}
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if limit, err := strconv.Atoi(v); err == nil && limit > 0 && limit <= 100 {
			filter.Limit = limit
		}
	}

	events, total, err := h.service.ListEvents(r.Context(), tenantID, filter)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"events": events,
		"total":  total,
	})
}

// GetEvent retrieves an event by ID.
func (h *Handler) GetEvent(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	eventID, err := uuid.Parse(chi.URLParam(r, "eventId"))
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid event ID")
		return
	}

	event, err := h.service.GetEvent(r.Context(), tenantID, eventID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, event)
}

// --- Preference Handlers ---

// GetPreferences retrieves user's notification preferences.
func (h *Handler) GetPreferences(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	user, ok := identity.UserFromContext(r.Context())
	if !ok {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	prefs, err := h.service.GetUserPreferences(r.Context(), tenantID, user.ID)
	if err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, prefs)
}

// UpdatePreferences updates user's notification preferences.
func (h *Handler) UpdatePreferences(w http.ResponseWriter, r *http.Request) {
	tenantID, err := getTenantID(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	user, ok := identity.UserFromContext(r.Context())
	if !ok {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req notifications.UpdatePreferencesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.service.UpdateUserPreferences(r.Context(), tenantID, user.ID, &req); err != nil {
		h.handleError(w, err)
		return
	}

	handlers.RespondJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// --- Helper Functions ---

func (h *Handler) handleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, notifications.ErrTemplateNotFound),
		errors.Is(err, notifications.ErrEventNotFound):
		handlers.RespondError(w, http.StatusNotFound, err.Error())

	case errors.Is(err, notifications.ErrTemplateExists):
		handlers.RespondError(w, http.StatusConflict, err.Error())

	case errors.Is(err, notifications.ErrInvalidChannel),
		errors.Is(err, notifications.ErrInvalidEventKey):
		handlers.RespondError(w, http.StatusBadRequest, err.Error())

	default:
		h.logger.Error("internal error", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "internal server error")
	}
}

func getTenantID(r *http.Request) (uuid.UUID, error) {
	ctx := r.Context()
	if httpware.IsPlatformOwner(ctx) {
		if q := r.URL.Query().Get("tenantId"); q != "" {
			return uuid.Parse(q)
		}
	}
	user, ok := identity.UserFromContext(ctx)
	if !ok {
		return uuid.Nil, errors.New("user not found in context")
	}
	return uuid.Parse(user.TenantID)
}
