package compliance

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Service provides compliance and data subject request business logic.
type Service struct {
	repo   Repository
	logger *zap.Logger

	// Configuration
	defaultRetentionDays int
	exportExpiryDays     int
}

// NewService creates a new compliance service.
func NewService(repo Repository, logger *zap.Logger) *Service {
	return &Service{
		repo:                 repo,
		logger:               logger.Named("compliance.service"),
		defaultRetentionDays: 30, // 30 days retention before permanent deletion
		exportExpiryDays:     7,  // Export download links expire after 7 days
	}
}

// --- Data Subject Requests ---

// CreateDataSubjectRequest creates a new data subject request.
func (s *Service) CreateDataSubjectRequest(ctx context.Context, tenantID, userID uuid.UUID, requestType DataSubjectRequestType, description string) (*DataSubjectRequest, error) {
	// Validate request type
	if !isValidRequestType(requestType) {
		return nil, ErrInvalidRequestType
	}

	req := &DataSubjectRequest{
		ID:          uuid.New(),
		TenantID:    tenantID,
		UserID:      userID,
		RequestType: requestType,
		Status:      RequestStatusPending,
		Description: description,
		SubmittedAt: time.Now(),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := s.repo.CreateRequest(ctx, req); err != nil {
		return nil, fmt.Errorf("compliance: create request: %w", err)
	}

	s.logger.Info("Data subject request created",
		zap.String("request_id", req.ID.String()),
		zap.String("user_id", userID.String()),
		zap.String("type", string(requestType)))

	// If it's an export or delete request, auto-create the job
	switch requestType {
	case RequestTypeExport:
		_, err := s.CreateExportJob(ctx, CreateExportRequest{
			TenantID: tenantID,
			UserID:   userID,
			Format:   ExportFormatJSON,
		})
		if err != nil {
			s.logger.Warn("Failed to auto-create export job for request",
				zap.Error(err),
				zap.String("request_id", req.ID.String()))
		}
	}

	return req, nil
}

// GetDataSubjectRequest retrieves a data subject request by ID.
func (s *Service) GetDataSubjectRequest(ctx context.Context, tenantID, id uuid.UUID) (*DataSubjectRequest, error) {
	return s.repo.GetRequest(ctx, tenantID, id)
}

// ListDataSubjectRequests lists data subject requests with filters.
func (s *Service) ListDataSubjectRequests(ctx context.Context, filter DataSubjectRequestFilter) ([]DataSubjectRequest, int, error) {
	return s.repo.ListRequests(ctx, filter)
}

// --- Data Export ---

// CreateExportJob creates a new data export job.
func (s *Service) CreateExportJob(ctx context.Context, req CreateExportRequest) (*DataExportJob, error) {
	// Validate format
	if req.Format != ExportFormatJSON && req.Format != ExportFormatCSV {
		return nil, ErrInvalidExportFormat
	}

	// Check if export already in progress
	existing, err := s.repo.GetExportJobByUserID(ctx, req.TenantID, req.UserID)
	if err == nil && existing != nil {
		if existing.Status == ExportStatusPending || existing.Status == ExportStatusInProgress {
			return nil, ErrExportInProgress
		}
	}

	// Default to all data if not specified
	includedData := req.IncludedData
	if len(includedData) == 0 {
		includedData = []string{"profile", "orders", "addresses", "carts", "loyalty", "payment_methods", "preferences"}
	}

	expiresAt := time.Now().Add(time.Duration(s.exportExpiryDays) * 24 * time.Hour)

	job := &DataExportJob{
		ID:           uuid.New(),
		TenantID:     req.TenantID,
		UserID:       req.UserID,
		Format:       req.Format,
		Status:       ExportStatusPending,
		IncludedData: includedData,
		RequestedAt:  time.Now(),
		ExpiresAt:    &expiresAt,
	}

	if err := s.repo.CreateExportJob(ctx, job); err != nil {
		return nil, fmt.Errorf("compliance: create export job: %w", err)
	}

	s.logger.Info("Data export job created",
		zap.String("job_id", job.ID.String()),
		zap.String("user_id", req.UserID.String()),
		zap.String("format", string(req.Format)))

	return job, nil
}

// GetExportJob retrieves an export job by ID.
func (s *Service) GetExportJob(ctx context.Context, tenantID, id uuid.UUID) (*DataExportJob, error) {
	return s.repo.GetExportJob(ctx, tenantID, id)
}

// ProcessExportJob processes an export job and generates the export file.
func (s *Service) ProcessExportJob(ctx context.Context, tenantID, jobID uuid.UUID) error {
	job, err := s.repo.GetExportJob(ctx, tenantID, jobID)
	if err != nil {
		return err
	}

	// Update status to processing
	startedAt := time.Now()
	job.Status = ExportStatusInProgress
	job.StartedAt = &startedAt
	if err := s.repo.UpdateExportJob(ctx, job); err != nil {
		return err
	}

	// Collect user data
	exportData, err := s.collectUserData(ctx, tenantID, job.UserID, job.IncludedData)
	if err != nil {
		job.Status = ExportStatusFailed
		job.ErrorMessage = err.Error()
		_ = s.repo.UpdateExportJob(ctx, job)
		return fmt.Errorf("%w: %v", ErrExportFailed, err)
	}

	// Generate export file (in production, this would upload to S3)
	exportJSON, err := json.MarshalIndent(exportData, "", "  ")
	if err != nil {
		job.Status = ExportStatusFailed
		job.ErrorMessage = err.Error()
		_ = s.repo.UpdateExportJob(ctx, job)
		return fmt.Errorf("%w: marshal export data: %v", ErrExportFailed, err)
	}

	// In production, upload to S3 and set StorageURL
	// For now, we just log the success and store the size
	fileSize := len(exportJSON)
	job.FileSizeBytes = &fileSize
	job.Status = ExportStatusCompleted
	now := time.Now()
	job.CompletedAt = &now
	// job.StorageURL = "s3://bucket/exports/" + job.ID.String() + ".json"

	if err := s.repo.UpdateExportJob(ctx, job); err != nil {
		return err
	}

	s.logger.Info("Data export completed",
		zap.String("job_id", job.ID.String()),
		zap.String("user_id", job.UserID.String()),
		zap.Intp("size_bytes", job.FileSizeBytes))

	return nil
}

// collectUserData collects all user data for export.
func (s *Service) collectUserData(ctx context.Context, tenantID, userID uuid.UUID, includedData []string) (*UserExportData, error) {
	data := &UserExportData{
		ExportedAt: time.Now(),
	}

	for _, dataType := range includedData {
		switch dataType {
		case "profile":
			profile, err := s.repo.GetUserProfile(ctx, tenantID, userID)
			if err == nil {
				data.Profile = profile
			}
		case "orders":
			orders, err := s.repo.GetUserOrders(ctx, tenantID, userID)
			if err == nil {
				data.Orders = orders
			}
		case "addresses":
			addresses, err := s.repo.GetUserAddresses(ctx, tenantID, userID)
			if err == nil {
				data.Addresses = addresses
			}
		case "carts":
			carts, err := s.repo.GetUserCarts(ctx, tenantID, userID)
			if err == nil {
				data.Carts = carts
			}
		case "loyalty":
			loyalty, err := s.repo.GetUserLoyaltyData(ctx, tenantID, userID)
			if err == nil {
				data.LoyaltyData = loyalty
			}
		case "payment_methods":
			methods, err := s.repo.GetUserPaymentMethods(ctx, tenantID, userID)
			if err == nil {
				data.PaymentMethods = methods
			}
		case "preferences":
			profile, err := s.repo.GetUserProfile(ctx, tenantID, userID)
			if err == nil {
				if prefs, ok := profile["preferences"]; ok {
					if prefsMap, ok := prefs.(map[string]interface{}); ok {
						data.Preferences = prefsMap
					}
				}
			}
		}
	}

	return data, nil
}

// --- Data Deletion ---

// CreateDeletionJob creates a new data deletion job.
func (s *Service) CreateDeletionJob(ctx context.Context, req CreateDeletionRequest) (*DataDeletionJob, error) {
	// Check if deletion already in progress
	existing, err := s.repo.GetDeletionJobByUserID(ctx, req.TenantID, req.UserID)
	if err == nil && existing != nil {
		if existing.Status == DeletionStatusPending || existing.Status == DeletionStatusScheduled || existing.Status == DeletionStatusInProgress {
			return nil, ErrDeletionInProgress
		}
	}

	// Require confirmation for deletion
	if !req.Confirmed {
		return nil, ErrDeletionNotConfirmed
	}

	retentionDays := req.RetentionDays
	if retentionDays <= 0 {
		retentionDays = s.defaultRetentionDays
	}

	deletionType := req.DeletionType
	if deletionType == "" {
		deletionType = DeletionTypeSoft
	}

	job := &DataDeletionJob{
		ID:            uuid.New(),
		TenantID:      req.TenantID,
		UserID:        req.UserID,
		Status:        DeletionStatusPending,
		DeletionType:  deletionType,
		Reason:        req.Reason,
		Confirmed:     req.Confirmed,
		RetentionDays: retentionDays,
		RequestedAt:   time.Now(),
	}

	// Set scheduled deletion date for soft deletes
	if deletionType == DeletionTypeSoft {
		scheduledFor := time.Now().Add(time.Duration(retentionDays) * 24 * time.Hour)
		job.ScheduledFor = &scheduledFor
		job.Status = DeletionStatusScheduled
	}

	if err := s.repo.CreateDeletionJob(ctx, job); err != nil {
		return nil, fmt.Errorf("compliance: create deletion job: %w", err)
	}

	s.logger.Info("Data deletion job created",
		zap.String("job_id", job.ID.String()),
		zap.String("user_id", req.UserID.String()),
		zap.String("deletion_type", string(deletionType)),
		zap.Int("retention_days", retentionDays))

	return job, nil
}

// GetDeletionJob retrieves a deletion job by ID.
func (s *Service) GetDeletionJob(ctx context.Context, tenantID, id uuid.UUID) (*DataDeletionJob, error) {
	return s.repo.GetDeletionJob(ctx, tenantID, id)
}

// ProcessDeletionJob processes a deletion job.
func (s *Service) ProcessDeletionJob(ctx context.Context, tenantID, jobID uuid.UUID) error {
	job, err := s.repo.GetDeletionJob(ctx, tenantID, jobID)
	if err != nil {
		return err
	}

	// For soft deletes, check if scheduled time has passed
	if job.DeletionType == DeletionTypeSoft && job.ScheduledFor != nil {
		if time.Now().Before(*job.ScheduledFor) {
			return fmt.Errorf("deletion scheduled for %s", job.ScheduledFor.Format(time.RFC3339))
		}
	}

	// Update status to processing
	startedAt := time.Now()
	job.Status = DeletionStatusInProgress
	job.StartedAt = &startedAt
	if err := s.repo.UpdateDeletionJob(ctx, job); err != nil {
		return err
	}

	var affectedRecords map[string]int

	switch job.DeletionType {
	case DeletionTypeSoft:
		if err := s.repo.SoftDeleteUser(ctx, tenantID, job.UserID); err != nil {
			job.Status = DeletionStatusFailed
			job.ErrorMessage = err.Error()
			_ = s.repo.UpdateDeletionJob(ctx, job)
			return fmt.Errorf("%w: %v", ErrDeletionFailed, err)
		}
		affectedRecords = map[string]int{"user": 1}

	case DeletionTypeAnonymize:
		if err := s.repo.AnonymizeUser(ctx, tenantID, job.UserID); err != nil {
			job.Status = DeletionStatusFailed
			job.ErrorMessage = err.Error()
			_ = s.repo.UpdateDeletionJob(ctx, job)
			return fmt.Errorf("%w: %v", ErrDeletionFailed, err)
		}
		affectedRecords = map[string]int{"user": 1}

	case DeletionTypePermanent:
		affectedRecords, err = s.repo.HardDeleteUser(ctx, tenantID, job.UserID)
		if err != nil {
			job.Status = DeletionStatusFailed
			job.ErrorMessage = err.Error()
			_ = s.repo.UpdateDeletionJob(ctx, job)
			return fmt.Errorf("%w: %v", ErrDeletionFailed, err)
		}
	}

	job.DeletionSummary = affectedRecords
	job.Status = DeletionStatusCompleted
	now := time.Now()
	job.CompletedAt = &now

	if err := s.repo.UpdateDeletionJob(ctx, job); err != nil {
		return err
	}

	s.logger.Info("Data deletion completed",
		zap.String("job_id", job.ID.String()),
		zap.String("user_id", job.UserID.String()),
		zap.String("deletion_type", string(job.DeletionType)),
		zap.Any("affected_records", affectedRecords))

	return nil
}

// --- Helper Functions ---

func isValidRequestType(t DataSubjectRequestType) bool {
	switch t {
	case RequestTypeExport, RequestTypeDelete, RequestTypeAccess, RequestTypeRectify:
		return true
	default:
		return false
	}
}
