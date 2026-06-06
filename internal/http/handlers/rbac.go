package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	authclient "github.com/Bengo-Hub/shared-auth-client"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/modules/identity"
	"github.com/bengobox/ordering-backend/internal/modules/rbac"
)

// RBACHandler handles RBAC-related operations.
type RBACHandler struct {
	logger       *zap.Logger
	rbacService  *rbac.Service
	rbacRepo     rbac.Repository
	identityRepo identity.Repository
}

// NewRBACHandler creates a new RBAC handler.
func NewRBACHandler(logger *zap.Logger, rbacService *rbac.Service, rbacRepo rbac.Repository, identityRepo identity.Repository) *RBACHandler {
	return &RBACHandler{
		logger:       logger,
		rbacService:  rbacService,
		rbacRepo:     rbacRepo,
		identityRepo: identityRepo,
	}
}

// TenantUserResponse is a lightweight projection of a tenant user, used to back
// searchable user pickers in admin UIs (e.g. role assignment).
type TenantUserResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// maxTenantUsersLimit caps the ?limit query parameter for ListTenantUsers.
const maxTenantUsersLimit = 200

// ListTenantUsers returns the tenant's synced users as a searchable list.
//
// @Summary List tenant users
// @Description Returns the calling tenant's synced ordering users as a lightweight {id,name,email} list, intended to back searchable user pickers (e.g. role assignment). Supports an optional case-insensitive name/email filter and a result cap.
// @Tags RBAC
// @Security bearerAuth
// @Produce json
// @Param tenant path string true "Tenant slug"
// @Param q query string false "Case-insensitive name/email search filter"
// @Param limit query int false "Maximum number of users to return (default 50, max 200)"
// @Success 200 {object} map[string][]TenantUserResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /{tenant}/admin/users [get]
func (h *RBACHandler) ListTenantUsers(w http.ResponseWriter, r *http.Request) {
	claims, ok := authclient.ClaimsFromContext(r.Context())
	if !ok {
		RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	tenantID, err := uuid.Parse(claims.TenantID)
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid tenant ID in claims")
		return
	}

	q := r.URL.Query().Get("q")

	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, perr := strconv.Atoi(raw); perr == nil {
			limit = parsed
		}
	}
	if limit > maxTenantUsersLimit {
		limit = maxTenantUsersLimit
	}

	users, err := h.identityRepo.ListTenantUsers(r.Context(), tenantID, q, limit)
	if err != nil {
		h.logger.Error("failed to list tenant users", zap.Error(err))
		RespondError(w, http.StatusInternalServerError, "failed to list users")
		return
	}

	out := make([]TenantUserResponse, 0, len(users))
	for _, u := range users {
		name := u.FullName
		if name == "" {
			name = u.Email
		}
		out = append(out, TenantUserResponse{
			ID:    u.ID.String(),
			Name:  name,
			Email: u.Email,
		})
	}

	RespondJSON(w, http.StatusOK, map[string]interface{}{"users": out})
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

// rbacAuthenticator is the subset of *identityhandler.Authenticator that the RBAC
// routes depend on. It is declared as an interface here to avoid an import cycle
// (the handlers package is imported by the identity handler package).
type rbacAuthenticator interface {
	RequirePermissions(perms ...identity.Permission) func(http.Handler) http.Handler
}

// RegisterRoutes registers RBAC routes on the provided router, guarding every
// route with the RBAC-management permission so only privileged users (and the
// admin/superuser bypass) can read or mutate role assignments.
func (h *RBACHandler) RegisterRoutes(r chi.Router, auth rbacAuthenticator) {
	r.Group(func(rbacRouter chi.Router) {
		rbacRouter.Use(auth.RequirePermissions(identity.PermissionRbacManage))
		rbacRouter.Post("/rbac/assignments", h.AssignRole)
		rbacRouter.Get("/rbac/assignments", h.ListAssignments)
		rbacRouter.Delete("/rbac/assignments/{id}", h.RevokeRole)
		rbacRouter.Get("/rbac/roles", h.ListOrderingRoles)
		rbacRouter.Get("/rbac/permissions", h.ListOrderingPermissions)
		// Tenant users directory — backs the searchable user picker in admin UIs
		// (e.g. role assignment). Gated by the same RBAC-management permission.
		rbacRouter.Get("/admin/users", h.ListTenantUsers)
	})
}
