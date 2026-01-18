package analytics

import "errors"

var (
	// ErrDashboardNotFound is returned when a dashboard is not found.
	ErrDashboardNotFound = errors.New("analytics: dashboard not found")

	// ErrDashboardNotConfigured is returned when a dashboard module is not configured.
	ErrDashboardNotConfigured = errors.New("analytics: dashboard not configured for this module")

	// ErrSupersetUnavailable is returned when Superset is unavailable.
	ErrSupersetUnavailable = errors.New("analytics: superset service unavailable")

	// ErrReportJobNotFound is returned when a report job is not found.
	ErrReportJobNotFound = errors.New("analytics: report job not found")

	// ErrInvalidReportType is returned when an invalid report type is specified.
	ErrInvalidReportType = errors.New("analytics: invalid report type")

	// ErrReportGenerationFailed is returned when report generation fails.
	ErrReportGenerationFailed = errors.New("analytics: report generation failed")
)
