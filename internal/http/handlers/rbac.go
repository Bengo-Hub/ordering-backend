package handlers

import (
	"encoding/json"
	"net/http"

	authclient "github.com/Bengo-Hub/shared-auth-client"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/modules/rbac"
)

// RBACHandler handles RBAC-related operations.
type RBACHandler struct {
	logger      *zap.Logger
	rbacService *rbac.Service
	rbacRepo    rbac.Repository
}

// NewRBACHandler creates a new RBAC handler.
func NewRBACHandler(logger *zap.Logger, rbacService *rbac.Service, rbacRepo rbac.Repository) *RBACHandler {
	return &RBACHandler{
		logger:      logger,
		rbacService: rbacService,
		rbacRepo:    rbacRepo,
	}
}

// AssignRoleRequest represents a request to assign a role.
type AssignRoleRequest struct {
	UserID uuid.UUID `json:"user_id"`
	RoleID uuid.UUID `json:"role_id"`
}

// AssignRole assigns a role to a user.
func (h *RBACHandler) AssignRole(w http.ResponseWriter, r *http.Request) {
	claims, ok := authclient.ClaimsFromContext(r.Context())
	if !ok {
		RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	tenantIDStr := claims.TenantID
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid tenant ID in claims")
		return
	}

	var req AssignRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	assignedBy, err := claims.UserID()
	if err != nil || assignedBy == uuid.Nil {
		RespondError(w, http.StatusUnauthorized, "invalid user ID")
		return
	}

	if err := h.rbacService.AssignRole(r.Context(), tenantID, req.UserID, req.RoleID, assignedBy); err != nil {
		h.logger.Error("failed to assign role", zap.Error(err))
		RespondError(w, http.StatusInternalServerError, "failed to assign role")
		return
	}

	RespondJSON(w, http.StatusCreated, map[string]string{"message": "role assigned successfully"})
}

// RevokeRole revokes a role from a user.
func (h *RBACHandler) RevokeRole(w http.ResponseWriter, r *http.Request) {
	claims, ok := authclient.ClaimsFromContext(r.Context())
	if !ok {
		RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	tenantIDStr := claims.TenantID
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid tenant ID in claims")
		return
	}

	assignmentIDStr := chi.URLParam(r, "id")
	assignmentID, err := uuid.Parse(assignmentIDStr)
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid assignment ID")
		return
	}

	// Get assignment to extract user ID and role ID
	assignments, err := h.rbacRepo.ListUserAssignments(r.Context(), tenantID, rbac.AssignmentFilters{})
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to get assignment")
		return
	}

	var assignment *rbac.UserRoleAssignment
	for _, a := range assignments {
		if a.ID == assignmentID {
			assignment = a
			break
		}
	}

	if assignment == nil {
		RespondError(w, http.StatusNotFound, "assignment not found")
		return
	}

	if err := h.rbacService.RevokeRole(r.Context(), tenantID, assignment.UserID, assignment.RoleID); err != nil {
		h.logger.Error("failed to revoke role", zap.Error(err))
		RespondError(w, http.StatusInternalServerError, "failed to revoke role")
		return
	}

	RespondJSON(w, http.StatusOK, map[string]string{"message": "role revoked successfully"})
}

// ListAssignments lists all role assignments.
func (h *RBACHandler) ListAssignments(w http.ResponseWriter, r *http.Request) {
	claims, ok := authclient.ClaimsFromContext(r.Context())
	if !ok {
		RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	tenantIDStr := claims.TenantID
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid tenant ID in claims")
		return
	}

	assignments, err := h.rbacRepo.ListUserAssignments(r.Context(), tenantID, rbac.AssignmentFilters{})
	if err != nil {
		h.logger.Error("failed to list assignments", zap.Error(err))
		RespondError(w, http.StatusInternalServerError, "failed to list assignments")
		return
	}

	RespondJSON(w, http.StatusOK, map[string]interface{}{"assignments": assignments})
}

// ListOrderingRoles lists all roles.
func (h *RBACHandler) ListOrderingRoles(w http.ResponseWriter, r *http.Request) {
	claims, ok := authclient.ClaimsFromContext(r.Context())
	if !ok {
		RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	tenantIDStr := claims.TenantID
	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid tenant ID in claims")
		return
	}

	roles, err := h.rbacRepo.ListRoles(r.Context(), tenantID)
	if err != nil {
		h.logger.Error("failed to list roles", zap.Error(err))
		RespondError(w, http.StatusInternalServerError, "failed to list roles")
		return
	}

	RespondJSON(w, http.StatusOK, map[string]interface{}{"roles": roles})
}

// ListOrderingPermissions lists all permissions.
func (h *RBACHandler) ListOrderingPermissions(w http.ResponseWriter, r *http.Request) {
	permissions, err := h.rbacRepo.ListPermissions(r.Context(), rbac.PermissionFilters{})
	if err != nil {
		h.logger.Error("failed to list permissions", zap.Error(err))
		RespondError(w, http.StatusInternalServerError, "failed to list permissions")
		return
	}

	RespondJSON(w, http.StatusOK, map[string]interface{}{"permissions": permissions})
}

// RegisterRoutes registers RBAC routes on the provided router.
func (h *RBACHandler) RegisterRoutes(r chi.Router) {
	r.Post("/rbac/assignments", h.AssignRole)
	r.Get("/rbac/assignments", h.ListAssignments)
	r.Delete("/rbac/assignments/{id}", h.RevokeRole)
	r.Get("/rbac/roles", h.ListOrderingRoles)
	r.Get("/rbac/permissions", h.ListOrderingPermissions)
}
