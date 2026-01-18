package analytics

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/config"
	"github.com/bengobox/ordering-backend/internal/platform/superset"
)

// Service provides analytics and dashboard business logic.
type Service struct {
	supersetClient  *superset.Client
	dashboardConfig map[DashboardModule]int
	logger          *zap.Logger
}

// NewService creates a new analytics service.
func NewService(supersetClient *superset.Client, cfg config.SupersetConfig, logger *zap.Logger) *Service {
	// Map dashboard modules to their IDs from config
	dashboardConfig := make(map[DashboardModule]int)

	if cfg.OrderAnalyticsDashboardID > 0 {
		dashboardConfig[DashboardModuleOrders] = cfg.OrderAnalyticsDashboardID
	}
	if cfg.RevenueDashboardID > 0 {
		dashboardConfig[DashboardModuleRevenue] = cfg.RevenueDashboardID
	}
	if cfg.CustomerAnalyticsDashboardID > 0 {
		dashboardConfig[DashboardModuleCustomers] = cfg.CustomerAnalyticsDashboardID
	}
	if cfg.OperationsDashboardID > 0 {
		dashboardConfig[DashboardModuleOperations] = cfg.OperationsDashboardID
	}
	if cfg.SubscriptionDashboardID > 0 {
		dashboardConfig[DashboardModuleSubscription] = cfg.SubscriptionDashboardID
	}

	return &Service{
		supersetClient:  supersetClient,
		dashboardConfig: dashboardConfig,
		logger:          logger.Named("analytics.service"),
	}
}

// GetDashboardEmbed returns an embed URL for the specified dashboard module.
func (s *Service) GetDashboardEmbed(ctx context.Context, module DashboardModule, tenantID, userID uuid.UUID) (*DashboardEmbed, error) {
	dashboardID, ok := s.dashboardConfig[module]
	if !ok || dashboardID == 0 {
		s.logger.Warn("Dashboard not configured for module",
			zap.String("module", string(module)))
		return nil, ErrDashboardNotConfigured
	}

	// Get embed URL from Superset
	embedURL, err := s.supersetClient.GetEmbedURL(ctx, dashboardID, tenantID, userID)
	if err != nil {
		s.logger.Error("Failed to get embed URL from Superset",
			zap.Error(err),
			zap.String("module", string(module)),
			zap.Int("dashboard_id", dashboardID))

		if err == superset.ErrDashboardNotFound {
			return nil, ErrDashboardNotFound
		}
		return nil, fmt.Errorf("%w: %v", ErrSupersetUnavailable, err)
	}

	return &DashboardEmbed{
		Module:    module,
		URL:       embedURL.URL,
		Token:     embedURL.Token,
		ExpiresAt: embedURL.ExpiresAt,
	}, nil
}

// ListAvailableDashboards returns a list of available dashboards for the tenant.
func (s *Service) ListAvailableDashboards(ctx context.Context, tenantID uuid.UUID) ([]DashboardInfo, error) {
	var dashboards []DashboardInfo

	// Return configured dashboards with descriptions
	moduleDescriptions := map[DashboardModule]string{
		DashboardModuleOrders:       "Order Analytics - Orders by status, daily volume, top items",
		DashboardModuleRevenue:      "Revenue Dashboard - Revenue by period, payment methods, refunds",
		DashboardModuleCustomers:    "Customer Analytics - Acquisition, lifetime value, loyalty",
		DashboardModuleOperations:   "Operations Dashboard - Kitchen tickets, prep time, capacity",
		DashboardModuleSubscription: "Subscription Dashboard - Active plans, usage, renewals",
	}

	for module, dashboardID := range s.dashboardConfig {
		if dashboardID > 0 {
			dashboards = append(dashboards, DashboardInfo{
				ID:          dashboardID,
				Module:      module,
				Title:       string(module),
				Description: moduleDescriptions[module],
				URL:         fmt.Sprintf("/api/v1/analytics/dashboards/%s/embed", module),
			})
		}
	}

	return dashboards, nil
}

// GetDashboardInfo returns information about a specific dashboard.
func (s *Service) GetDashboardInfo(ctx context.Context, module DashboardModule) (*DashboardInfo, error) {
	dashboardID, ok := s.dashboardConfig[module]
	if !ok || dashboardID == 0 {
		return nil, ErrDashboardNotConfigured
	}

	// Try to fetch dashboard details from Superset
	dashboard, err := s.supersetClient.GetDashboard(ctx, dashboardID)
	if err != nil {
		// Return basic info if Superset is unavailable
		s.logger.Warn("Failed to fetch dashboard details from Superset, returning basic info",
			zap.Error(err),
			zap.String("module", string(module)))

		return &DashboardInfo{
			ID:     dashboardID,
			Module: module,
			Title:  string(module),
		}, nil
	}

	return &DashboardInfo{
		ID:     dashboard.ID,
		Module: module,
		Title:  dashboard.DashboardTitle,
		URL:    dashboard.URL,
	}, nil
}

// ValidateModule checks if a module string is valid.
func ValidateModule(module string) (DashboardModule, bool) {
	switch DashboardModule(module) {
	case DashboardModuleOrders,
		DashboardModuleRevenue,
		DashboardModuleCustomers,
		DashboardModuleOperations,
		DashboardModuleSubscription:
		return DashboardModule(module), true
	default:
		return "", false
	}
}

// IsEnabled returns true if the analytics service is enabled (Superset configured).
func (s *Service) IsEnabled() bool {
	return s.supersetClient != nil && len(s.dashboardConfig) > 0
}

// GetSupersetBaseURL returns the Superset base URL.
func (s *Service) GetSupersetBaseURL() string {
	if s.supersetClient == nil {
		return ""
	}
	return s.supersetClient.GetBaseURL()
}
