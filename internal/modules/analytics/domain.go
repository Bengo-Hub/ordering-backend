package analytics

import (
	"time"

	"github.com/google/uuid"
)

// DashboardModule represents the type of analytics dashboard.
type DashboardModule string

const (
	DashboardModuleOrders       DashboardModule = "orders"
	DashboardModuleRevenue      DashboardModule = "revenue"
	DashboardModuleCustomers    DashboardModule = "customers"
	DashboardModuleOperations   DashboardModule = "operations"
	DashboardModuleSubscription DashboardModule = "subscription"
)

// DashboardEmbed represents an embedded dashboard with its embed URL.
type DashboardEmbed struct {
	Module    DashboardModule `json:"module"`
	URL       string          `json:"url"`
	Token     string          `json:"token"`
	ExpiresAt time.Time       `json:"expires_at"`
}

// DashboardInfo represents information about a dashboard.
type DashboardInfo struct {
	ID          int             `json:"id"`
	Module      DashboardModule `json:"module"`
	Title       string          `json:"title"`
	Description string          `json:"description,omitempty"`
	URL         string          `json:"url,omitempty"`
}

// ReportJob represents a report generation job.
type ReportJob struct {
	ID          uuid.UUID              `json:"id"`
	TenantID    uuid.UUID              `json:"tenant_id"`
	ReportType  string                 `json:"report_type"`
	Status      ReportJobStatus        `json:"status"`
	RequestedBy uuid.UUID              `json:"requested_by"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
	ResultURL   string                 `json:"result_url,omitempty"`
	RequestedAt time.Time              `json:"requested_at"`
	CompletedAt *time.Time             `json:"completed_at,omitempty"`
	ErrorMsg    string                 `json:"error_message,omitempty"`
}

// ReportJobStatus represents the status of a report job.
type ReportJobStatus string

const (
	ReportJobStatusPending    ReportJobStatus = "pending"
	ReportJobStatusProcessing ReportJobStatus = "processing"
	ReportJobStatusCompleted  ReportJobStatus = "completed"
	ReportJobStatusFailed     ReportJobStatus = "failed"
)

// ReportType represents the type of report.
type ReportType string

const (
	ReportTypeOrderAnalytics    ReportType = "order_analytics"
	ReportTypeRevenueAnalytics  ReportType = "revenue_analytics"
	ReportTypeCustomerAnalytics ReportType = "customer_analytics"
	ReportTypeOperations        ReportType = "operations"
)

// CreateReportJobRequest represents a request to create a report job.
type CreateReportJobRequest struct {
	TenantID    uuid.UUID
	ReportType  ReportType
	RequestedBy uuid.UUID
	Parameters  map[string]interface{}
	Format      string // csv, pdf
}

// ReportFilter represents filters for listing reports.
type ReportFilter struct {
	TenantID    uuid.UUID
	ReportType  *ReportType
	Status      *ReportJobStatus
	RequestedBy *uuid.UUID
	DateFrom    *time.Time
	DateTo      *time.Time
	Limit       int
	Offset      int
}
