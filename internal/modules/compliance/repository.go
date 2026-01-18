package compliance

import (
	"context"

	"github.com/google/uuid"
)

// Repository abstracts persistence for compliance entities.
type Repository interface {
	// Data Subject Requests
	CreateRequest(ctx context.Context, req *DataSubjectRequest) error
	GetRequest(ctx context.Context, tenantID, id uuid.UUID) (*DataSubjectRequest, error)
	UpdateRequest(ctx context.Context, req *DataSubjectRequest) error
	ListRequests(ctx context.Context, filter DataSubjectRequestFilter) ([]DataSubjectRequest, int, error)

	// Export Jobs
	CreateExportJob(ctx context.Context, job *DataExportJob) error
	GetExportJob(ctx context.Context, tenantID, id uuid.UUID) (*DataExportJob, error)
	GetExportJobByUserID(ctx context.Context, tenantID, userID uuid.UUID) (*DataExportJob, error)
	UpdateExportJob(ctx context.Context, job *DataExportJob) error
	ListExportJobs(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]DataExportJob, int, error)

	// Deletion Jobs
	CreateDeletionJob(ctx context.Context, job *DataDeletionJob) error
	GetDeletionJob(ctx context.Context, tenantID, id uuid.UUID) (*DataDeletionJob, error)
	GetDeletionJobByUserID(ctx context.Context, tenantID, userID uuid.UUID) (*DataDeletionJob, error)
	UpdateDeletionJob(ctx context.Context, job *DataDeletionJob) error
	ListDeletionJobs(ctx context.Context, tenantID uuid.UUID, limit, offset int) ([]DataDeletionJob, int, error)

	// User Data Access (for export)
	GetUserProfile(ctx context.Context, tenantID, userID uuid.UUID) (map[string]interface{}, error)
	GetUserOrders(ctx context.Context, tenantID, userID uuid.UUID) ([]map[string]interface{}, error)
	GetUserAddresses(ctx context.Context, tenantID, userID uuid.UUID) ([]map[string]interface{}, error)
	GetUserCarts(ctx context.Context, tenantID, userID uuid.UUID) ([]map[string]interface{}, error)
	GetUserLoyaltyData(ctx context.Context, tenantID, userID uuid.UUID) (map[string]interface{}, error)
	GetUserPaymentMethods(ctx context.Context, tenantID, userID uuid.UUID) ([]map[string]interface{}, error)

	// User Data Deletion
	SoftDeleteUser(ctx context.Context, tenantID, userID uuid.UUID) error
	AnonymizeUser(ctx context.Context, tenantID, userID uuid.UUID) error
	HardDeleteUser(ctx context.Context, tenantID, userID uuid.UUID) (map[string]int, error)
}
