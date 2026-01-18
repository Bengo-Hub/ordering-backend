package compliance

import (
	"time"

	"github.com/google/uuid"
)

// DataSubjectRequestType represents the type of data subject request.
type DataSubjectRequestType string

const (
	RequestTypeExport DataSubjectRequestType = "export"
	RequestTypeDelete DataSubjectRequestType = "delete"
	RequestTypeAccess DataSubjectRequestType = "access"
	RequestTypeRectify DataSubjectRequestType = "rectify"
)

// DataSubjectRequestStatus represents the status of a request.
type DataSubjectRequestStatus string

const (
	RequestStatusPending    DataSubjectRequestStatus = "pending"
	RequestStatusProcessing DataSubjectRequestStatus = "processing"
	RequestStatusCompleted  DataSubjectRequestStatus = "completed"
	RequestStatusFailed     DataSubjectRequestStatus = "failed"
	RequestStatusCancelled  DataSubjectRequestStatus = "cancelled"
)

// DataSubjectRequest represents a GDPR/DPA data subject request.
type DataSubjectRequest struct {
	ID            uuid.UUID                `json:"id"`
	TenantID      uuid.UUID                `json:"tenant_id"`
	UserID        uuid.UUID                `json:"user_id"`
	RequestType   DataSubjectRequestType   `json:"request_type"`
	Status        DataSubjectRequestStatus `json:"status"`
	Description   string                   `json:"description,omitempty"`
	ResultURL     string                   `json:"result_url,omitempty"`
	ErrorMessage  string                   `json:"error_message,omitempty"`
	ProcessedBy   *uuid.UUID               `json:"processed_by,omitempty"`
	Notes         string                   `json:"notes,omitempty"`
	SubmittedAt   time.Time                `json:"submitted_at"`
	ProcessedAt   *time.Time               `json:"processed_at,omitempty"`
	ExpiresAt     *time.Time               `json:"expires_at,omitempty"`
	CreatedAt     time.Time                `json:"created_at"`
	UpdatedAt     time.Time                `json:"updated_at"`
}

// ExportFormat represents the format of data export.
type ExportFormat string

const (
	ExportFormatJSON ExportFormat = "json"
	ExportFormatCSV  ExportFormat = "csv"
)

// ExportJobStatus represents the status of an export job.
type ExportJobStatus string

const (
	ExportStatusPending    ExportJobStatus = "pending"
	ExportStatusInProgress ExportJobStatus = "in_progress"
	ExportStatusCompleted  ExportJobStatus = "completed"
	ExportStatusFailed     ExportJobStatus = "failed"
	ExportStatusExpired    ExportJobStatus = "expired"
)

// DataExportJob represents a data export job.
type DataExportJob struct {
	ID              uuid.UUID       `json:"id"`
	TenantID        uuid.UUID       `json:"tenant_id"`
	UserID          uuid.UUID       `json:"user_id"`
	Format          ExportFormat    `json:"format"`
	Status          ExportJobStatus `json:"status"`
	IncludedData    []string        `json:"included_data"` // orders, addresses, loyalty, etc.
	StorageURL      string          `json:"storage_url,omitempty"`
	ErrorMessage    string          `json:"error_message,omitempty"`
	FileSizeBytes   *int            `json:"file_size_bytes,omitempty"`
	RecordsExported *int            `json:"records_exported,omitempty"`
	RequestedAt     time.Time       `json:"requested_at"`
	StartedAt       *time.Time      `json:"started_at,omitempty"`
	CompletedAt     *time.Time      `json:"completed_at,omitempty"`
	ExpiresAt       *time.Time      `json:"expires_at,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// DeletionJobStatus represents the status of a deletion job.
type DeletionJobStatus string

const (
	DeletionStatusPending    DeletionJobStatus = "pending"
	DeletionStatusScheduled  DeletionJobStatus = "scheduled"
	DeletionStatusInProgress DeletionJobStatus = "in_progress"
	DeletionStatusCompleted  DeletionJobStatus = "completed"
	DeletionStatusFailed     DeletionJobStatus = "failed"
	DeletionStatusCancelled  DeletionJobStatus = "cancelled"
)

// DataDeletionJob represents a data deletion job.
type DataDeletionJob struct {
	ID              uuid.UUID         `json:"id"`
	TenantID        uuid.UUID         `json:"tenant_id"`
	UserID          uuid.UUID         `json:"user_id"`
	DeletionType    DeletionType      `json:"deletion_type"`
	Status          DeletionJobStatus `json:"status"`
	Reason          string            `json:"reason,omitempty"`
	Confirmed       bool              `json:"confirmed"`
	RetentionDays   int               `json:"retention_days"` // Days before permanent deletion
	ErrorMessage    string            `json:"error_message,omitempty"`
	DeletionSummary map[string]int    `json:"deletion_summary,omitempty"` // Entity type -> count
	RequestedAt     time.Time         `json:"requested_at"`
	ScheduledFor    *time.Time        `json:"scheduled_for,omitempty"`
	StartedAt       *time.Time        `json:"started_at,omitempty"`
	CompletedAt     *time.Time        `json:"completed_at,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

// DeletionType represents the type of deletion.
type DeletionType string

const (
	DeletionTypeSoft      DeletionType = "soft"      // Soft delete with retention
	DeletionTypePermanent DeletionType = "permanent" // Immediate permanent deletion
	DeletionTypeAnonymize DeletionType = "anonymize" // Replace PII with anonymized data
)

// UserExportData represents all exportable data for a user.
type UserExportData struct {
	Profile       map[string]interface{}   `json:"profile"`
	Orders        []map[string]interface{} `json:"orders,omitempty"`
	Addresses     []map[string]interface{} `json:"addresses,omitempty"`
	Carts         []map[string]interface{} `json:"carts,omitempty"`
	LoyaltyData   map[string]interface{}   `json:"loyalty_data,omitempty"`
	Preferences   map[string]interface{}   `json:"preferences,omitempty"`
	PaymentMethods []map[string]interface{} `json:"payment_methods,omitempty"`
	ExportedAt    time.Time                `json:"exported_at"`
}

// CreateExportRequest represents a request to create a data export.
type CreateExportRequest struct {
	TenantID     uuid.UUID
	UserID       uuid.UUID
	Format       ExportFormat
	IncludedData []string // If empty, export all data
}

// CreateDeletionRequest represents a request to create a data deletion.
type CreateDeletionRequest struct {
	TenantID      uuid.UUID
	UserID        uuid.UUID
	DeletionType  DeletionType
	Reason        string
	Confirmed     bool
	RetentionDays int
}

// DataSubjectRequestFilter represents filters for listing requests.
type DataSubjectRequestFilter struct {
	TenantID    uuid.UUID
	UserID      *uuid.UUID
	RequestType *DataSubjectRequestType
	Status      *DataSubjectRequestStatus
	DateFrom    *time.Time
	DateTo      *time.Time
	Limit       int
	Offset      int
}
