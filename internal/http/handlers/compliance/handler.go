package compliance

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	handlers "github.com/bengobox/ordering-backend/internal/http/handlers"
	identityhandler "github.com/bengobox/ordering-backend/internal/http/handlers/identity"
	"github.com/bengobox/ordering-backend/internal/modules/compliance"
	"github.com/bengobox/ordering-backend/internal/modules/identity"
)

// Handler handles compliance HTTP requests.
type Handler struct {
	logger         *zap.Logger
	complianceSvc  *compliance.Service
}

// NewHandler creates a new compliance handler.
func NewHandler(logger *zap.Logger, complianceSvc *compliance.Service) *Handler {
	return &Handler{
		logger:        logger.Named("compliance.handler"),
		complianceSvc: complianceSvc,
	}
}

// Register registers the compliance routes.
func (h *Handler) Register(r chi.Router, auth *identityhandler.Authenticator) {
	r.Route("/{tenant}/compliance", func(cr chi.Router) {
		cr.Use(auth.RequireAuth)

		// Data Subject Requests
		cr.Post("/data-subject-requests", h.CreateDataSubjectRequest)
		cr.Get("/data-subject-requests", h.ListDataSubjectRequests)
		cr.Get("/data-subject-requests/{id}", h.GetDataSubjectRequest)

		// Data Export
		cr.Post("/data-export", h.RequestDataExport)
		cr.Get("/data-export/{id}", h.GetExportJob)

		// Data Deletion
		cr.Post("/data-deletion", h.RequestDataDeletion)
		cr.Get("/data-deletion/{id}", h.GetDeletionJob)
	})
}

// --- Data Subject Request Handlers ---

// CreateDataSubjectRequestRequest represents a request to create a data subject request.
type CreateDataSubjectRequestRequest struct {
	RequestType string `json:"request_type"`
	Description string `json:"description,omitempty"`
}

// CreateDataSubjectRequest creates a new data subject request.
// @Summary Create data subject request
// @Tags Compliance
// @Accept json
// @Produce json
// @Param tenant path string true "Tenant slug"
// @Param request body CreateDataSubjectRequestRequest true "Request"
// @Success 201 {object} compliance.DataSubjectRequest
// @Router /api/v1/{tenant}/compliance/data-subject-requests [post]
func (h *Handler) CreateDataSubjectRequest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, tenantID, err := getUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req CreateDataSubjectRequestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	requestType := compliance.DataSubjectRequestType(req.RequestType)

	dsRequest, err := h.complianceSvc.CreateDataSubjectRequest(ctx, tenantID, user.ID, requestType, req.Description)
	if err != nil {
		if errors.Is(err, compliance.ErrInvalidRequestType) {
			handlers.RespondError(w, http.StatusBadRequest, "invalid request type")
			return
		}
		h.logger.Error("Failed to create data subject request", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "failed to create request")
		return
	}

	handlers.RespondJSON(w, http.StatusCreated, dsRequest)
}

// ListDataSubjectRequests lists data subject requests.
// @Summary List data subject requests
// @Tags Compliance
// @Produce json
// @Param tenant path string true "Tenant slug"
// @Param type query string false "Request type filter"
// @Param status query string false "Status filter"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/{tenant}/compliance/data-subject-requests [get]
func (h *Handler) ListDataSubjectRequests(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, tenantID, err := getUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	filter := compliance.DataSubjectRequestFilter{
		TenantID: tenantID,
		UserID:   &user.ID,
		Limit:    handlers.ParseInt(r.URL.Query().Get("limit"), 20),
		Offset:   handlers.ParseInt(r.URL.Query().Get("offset"), 0),
	}

	if reqType := r.URL.Query().Get("type"); reqType != "" {
		rt := compliance.DataSubjectRequestType(reqType)
		filter.RequestType = &rt
	}

	if status := r.URL.Query().Get("status"); status != "" {
		st := compliance.DataSubjectRequestStatus(status)
		filter.Status = &st
	}

	requests, total, err := h.complianceSvc.ListDataSubjectRequests(ctx, filter)
	if err != nil {
		h.logger.Error("Failed to list data subject requests", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "failed to list requests")
		return
	}

	handlers.RespondJSON(w, http.StatusOK, map[string]interface{}{
		"data":   requests,
		"total":  total,
		"limit":  filter.Limit,
		"offset": filter.Offset,
	})
}

// GetDataSubjectRequest retrieves a data subject request by ID.
// @Summary Get data subject request
// @Tags Compliance
// @Produce json
// @Param tenant path string true "Tenant slug"
// @Param id path string true "Request ID"
// @Success 200 {object} compliance.DataSubjectRequest
// @Router /api/v1/{tenant}/compliance/data-subject-requests/{id} [get]
func (h *Handler) GetDataSubjectRequest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, tenantID, err := getUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request ID")
		return
	}

	dsRequest, err := h.complianceSvc.GetDataSubjectRequest(ctx, tenantID, id)
	if err != nil {
		if errors.Is(err, compliance.ErrRequestNotFound) {
			handlers.RespondError(w, http.StatusNotFound, "request not found")
			return
		}
		h.logger.Error("Failed to get data subject request", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "failed to get request")
		return
	}

	handlers.RespondJSON(w, http.StatusOK, dsRequest)
}

// --- Data Export Handlers ---

// RequestDataExportRequest represents a request to export user data.
type RequestDataExportRequest struct {
	Format       string   `json:"format,omitempty"` // json or csv
	IncludedData []string `json:"included_data,omitempty"`
}

// RequestDataExport creates a data export request.
// @Summary Request data export
// @Tags Compliance
// @Accept json
// @Produce json
// @Param tenant path string true "Tenant slug"
// @Param request body RequestDataExportRequest true "Export request"
// @Success 201 {object} compliance.DataExportJob
// @Router /api/v1/{tenant}/compliance/data-export [post]
func (h *Handler) RequestDataExport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, tenantID, err := getUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req RequestDataExportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	format := compliance.ExportFormatJSON
	if req.Format == "csv" {
		format = compliance.ExportFormatCSV
	}

	exportJob, err := h.complianceSvc.CreateExportJob(ctx, compliance.CreateExportRequest{
		TenantID:     tenantID,
		UserID:       user.ID,
		Format:       format,
		IncludedData: req.IncludedData,
	})
	if err != nil {
		if errors.Is(err, compliance.ErrExportInProgress) {
			handlers.RespondError(w, http.StatusConflict, "export already in progress")
			return
		}
		if errors.Is(err, compliance.ErrInvalidExportFormat) {
			handlers.RespondError(w, http.StatusBadRequest, "invalid export format")
			return
		}
		h.logger.Error("Failed to create export job", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "failed to create export request")
		return
	}

	handlers.RespondJSON(w, http.StatusCreated, exportJob)
}

// GetExportJob retrieves an export job by ID.
// @Summary Get export job
// @Tags Compliance
// @Produce json
// @Param tenant path string true "Tenant slug"
// @Param id path string true "Export job ID"
// @Success 200 {object} compliance.DataExportJob
// @Router /api/v1/{tenant}/compliance/data-export/{id} [get]
func (h *Handler) GetExportJob(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, tenantID, err := getUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid job ID")
		return
	}

	job, err := h.complianceSvc.GetExportJob(ctx, tenantID, id)
	if err != nil {
		if errors.Is(err, compliance.ErrExportJobNotFound) {
			handlers.RespondError(w, http.StatusNotFound, "export job not found")
			return
		}
		h.logger.Error("Failed to get export job", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "failed to get export job")
		return
	}

	handlers.RespondJSON(w, http.StatusOK, job)
}

// --- Data Deletion Handlers ---

// RequestDataDeletionRequest represents a request to delete user data.
type RequestDataDeletionRequest struct {
	DeletionType  string `json:"deletion_type,omitempty"` // soft, permanent, anonymize
	Reason        string `json:"reason,omitempty"`
	Confirmed     bool   `json:"confirmed"`
	RetentionDays int    `json:"retention_days,omitempty"`
}

// RequestDataDeletion creates a data deletion request.
// @Summary Request data deletion
// @Tags Compliance
// @Accept json
// @Produce json
// @Param tenant path string true "Tenant slug"
// @Param request body RequestDataDeletionRequest true "Deletion request"
// @Success 201 {object} compliance.DataDeletionJob
// @Router /api/v1/{tenant}/compliance/data-deletion [post]
func (h *Handler) RequestDataDeletion(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user, tenantID, err := getUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req RequestDataDeletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	deletionType := compliance.DeletionTypeSoft
	if req.DeletionType == "permanent" {
		deletionType = compliance.DeletionTypePermanent
	} else if req.DeletionType == "anonymize" {
		deletionType = compliance.DeletionTypeAnonymize
	}

	deletionJob, err := h.complianceSvc.CreateDeletionJob(ctx, compliance.CreateDeletionRequest{
		TenantID:      tenantID,
		UserID:        user.ID,
		DeletionType:  deletionType,
		Reason:        req.Reason,
		Confirmed:     req.Confirmed,
		RetentionDays: req.RetentionDays,
	})
	if err != nil {
		if errors.Is(err, compliance.ErrDeletionInProgress) {
			handlers.RespondError(w, http.StatusConflict, "deletion already in progress")
			return
		}
		if errors.Is(err, compliance.ErrDeletionNotConfirmed) {
			handlers.RespondError(w, http.StatusBadRequest, "deletion must be confirmed")
			return
		}
		h.logger.Error("Failed to create deletion job", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "failed to create deletion request")
		return
	}

	handlers.RespondJSON(w, http.StatusCreated, deletionJob)
}

// GetDeletionJob retrieves a deletion job by ID.
// @Summary Get deletion job
// @Tags Compliance
// @Produce json
// @Param tenant path string true "Tenant slug"
// @Param id path string true "Deletion job ID"
// @Success 200 {object} compliance.DataDeletionJob
// @Router /api/v1/{tenant}/compliance/data-deletion/{id} [get]
func (h *Handler) GetDeletionJob(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, tenantID, err := getUserFromContext(r)
	if err != nil {
		handlers.RespondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		handlers.RespondError(w, http.StatusBadRequest, "invalid job ID")
		return
	}

	job, err := h.complianceSvc.GetDeletionJob(ctx, tenantID, id)
	if err != nil {
		if errors.Is(err, compliance.ErrDeletionJobNotFound) {
			handlers.RespondError(w, http.StatusNotFound, "deletion job not found")
			return
		}
		h.logger.Error("Failed to get deletion job", zap.Error(err))
		handlers.RespondError(w, http.StatusInternalServerError, "failed to get deletion job")
		return
	}

	handlers.RespondJSON(w, http.StatusOK, job)
}

// Helper function to get user from context
func getUserFromContext(r *http.Request) (*identity.User, uuid.UUID, error) {
	user, ok := identity.UserFromContext(r.Context())
	if !ok {
		return nil, uuid.Nil, errors.New("user not found in context")
	}
	tenantID, err := uuid.Parse(user.TenantID)
	if err != nil {
		return nil, uuid.Nil, errors.New("invalid tenant ID")
	}
	return user, tenantID, nil
}
