package superset

import (
	"context"

	"github.com/google/uuid"
)

// ClientInterface defines the interface for Superset client operations.
// This interface allows for mocking in tests.
type ClientInterface interface {
	// GetEmbedURL generates an embed URL for a dashboard with RLS.
	GetEmbedURL(ctx context.Context, dashboardID int, tenantID, userID uuid.UUID) (*EmbedURL, error)

	// GetDashboard retrieves a dashboard by ID.
	GetDashboard(ctx context.Context, dashboardID int) (*Dashboard, error)

	// ListDashboards retrieves all dashboards.
	ListDashboards(ctx context.Context) ([]Dashboard, error)

	// GetBaseURL returns the Superset base URL.
	GetBaseURL() string
}

// Ensure Client implements ClientInterface
var _ ClientInterface = (*Client)(nil)
