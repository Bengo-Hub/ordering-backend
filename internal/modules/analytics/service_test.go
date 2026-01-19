package analytics

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/bengobox/ordering-backend/internal/config"
	"github.com/bengobox/ordering-backend/internal/platform/superset"
)

// Mock Superset client for testing - implements superset.ClientInterface
type mockSupersetClient struct {
	getEmbedURLFunc    func(ctx context.Context, dashboardID int, tenantID, userID uuid.UUID) (*superset.EmbedURL, error)
	getDashboardFunc   func(ctx context.Context, dashboardID int) (*superset.Dashboard, error)
	listDashboardsFunc func(ctx context.Context) ([]superset.Dashboard, error)
	baseURL            string
}

// Ensure mockSupersetClient implements superset.ClientInterface
var _ superset.ClientInterface = (*mockSupersetClient)(nil)

func (m *mockSupersetClient) GetEmbedURL(ctx context.Context, dashboardID int, tenantID, userID uuid.UUID) (*superset.EmbedURL, error) {
	if m.getEmbedURLFunc != nil {
		return m.getEmbedURLFunc(ctx, dashboardID, tenantID, userID)
	}
	return &superset.EmbedURL{
		URL:       "https://superset.example.com/superset/embedded/123/?guest_token=abc",
		Token:     "abc",
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}, nil
}

func (m *mockSupersetClient) GetDashboard(ctx context.Context, dashboardID int) (*superset.Dashboard, error) {
	if m.getDashboardFunc != nil {
		return m.getDashboardFunc(ctx, dashboardID)
	}
	return &superset.Dashboard{
		ID:              dashboardID,
		DashboardTitle:  "Test Dashboard",
		Slug:            "test-dashboard",
		URL:             "/superset/dashboard/test-dashboard/",
		PublishedStatus: true,
	}, nil
}

func (m *mockSupersetClient) ListDashboards(ctx context.Context) ([]superset.Dashboard, error) {
	if m.listDashboardsFunc != nil {
		return m.listDashboardsFunc(ctx)
	}
	return []superset.Dashboard{}, nil
}

func (m *mockSupersetClient) GetBaseURL() string {
	return m.baseURL
}

func TestService_GetDashboardEmbed(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()

	tests := []struct {
		name           string
		module         DashboardModule
		dashboardID    int
		mockClient     *mockSupersetClient
		wantErr        bool
		errIs          error
		validateResult func(*testing.T, *DashboardEmbed)
	}{
		{
			name:        "successful embed URL generation",
			module:      DashboardModuleOrders,
			dashboardID: 100,
			mockClient: &mockSupersetClient{
				getEmbedURLFunc: func(ctx context.Context, dashboardID int, tID, uID uuid.UUID) (*superset.EmbedURL, error) {
					assert.Equal(t, 100, dashboardID)
					assert.Equal(t, tenantID, tID)
					assert.Equal(t, userID, uID)
					return &superset.EmbedURL{
						URL:       "https://superset.example.com/superset/embedded/100/?guest_token=token123",
						Token:     "token123",
						ExpiresAt: time.Now().Add(5 * time.Minute),
					}, nil
				},
			},
			wantErr: false,
			validateResult: func(t *testing.T, result *DashboardEmbed) {
				assert.Equal(t, DashboardModuleOrders, result.Module)
				assert.Contains(t, result.URL, "superset.example.com")
				assert.Contains(t, result.URL, "guest_token=token123")
				assert.Equal(t, "token123", result.Token)
				assert.True(t, result.ExpiresAt.After(time.Now()))
			},
		},
		{
			name:        "dashboard not configured",
			module:      DashboardModuleOrders,
			dashboardID: 0, // Not configured
			mockClient:  &mockSupersetClient{},
			wantErr:     true,
			errIs:       ErrDashboardNotConfigured,
		},
		{
			name:        "dashboard not found in Superset",
			module:      DashboardModuleRevenue,
			dashboardID: 200,
			mockClient: &mockSupersetClient{
				getEmbedURLFunc: func(ctx context.Context, dashboardID int, tID, uID uuid.UUID) (*superset.EmbedURL, error) {
					return nil, superset.ErrDashboardNotFound
				},
			},
			wantErr: true,
			errIs:   ErrDashboardNotFound,
		},
		{
			name:        "Superset unavailable",
			module:      DashboardModuleCustomers,
			dashboardID: 300,
			mockClient: &mockSupersetClient{
				getEmbedURLFunc: func(ctx context.Context, dashboardID int, tID, uID uuid.UUID) (*superset.EmbedURL, error) {
					return nil, errors.New("connection refused")
				},
			},
			wantErr: true,
			errIs:   ErrSupersetUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.SupersetConfig{
				BaseURL: "https://superset.example.com",
			}

			// Set dashboard IDs based on test case
			if tt.module == DashboardModuleOrders {
				cfg.OrderAnalyticsDashboardID = tt.dashboardID
			} else if tt.module == DashboardModuleRevenue {
				cfg.RevenueDashboardID = tt.dashboardID
			} else if tt.module == DashboardModuleCustomers {
				cfg.CustomerAnalyticsDashboardID = tt.dashboardID
			}

			service := NewService(tt.mockClient, cfg, zaptest.NewLogger(t))

			result, err := service.GetDashboardEmbed(context.Background(), tt.module, tenantID, userID)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errIs != nil {
					assert.ErrorIs(t, err, tt.errIs)
				}
			} else {
				assert.NoError(t, err)
				if tt.validateResult != nil {
					tt.validateResult(t, result)
				}
			}
		})
	}
}

func TestService_ListAvailableDashboards(t *testing.T) {
	tenantID := uuid.New()

	tests := []struct {
		name           string
		cfg            config.SupersetConfig
		wantCount      int
		validateResult func(*testing.T, []DashboardInfo)
	}{
		{
			name: "all dashboards configured",
			cfg: config.SupersetConfig{
				OrderAnalyticsDashboardID:    100,
				RevenueDashboardID:           200,
				CustomerAnalyticsDashboardID: 300,
				OperationsDashboardID:        400,
				SubscriptionDashboardID:      500,
			},
			wantCount: 5,
			validateResult: func(t *testing.T, result []DashboardInfo) {
				modulesSeen := make(map[DashboardModule]bool)
				for _, dashboard := range result {
					modulesSeen[dashboard.Module] = true
					assert.NotEmpty(t, dashboard.Description)
					assert.NotEmpty(t, dashboard.URL)
					assert.Greater(t, dashboard.ID, 0)
				}
				assert.True(t, modulesSeen[DashboardModuleOrders])
				assert.True(t, modulesSeen[DashboardModuleRevenue])
				assert.True(t, modulesSeen[DashboardModuleCustomers])
				assert.True(t, modulesSeen[DashboardModuleOperations])
				assert.True(t, modulesSeen[DashboardModuleSubscription])
			},
		},
		{
			name: "partial dashboards configured",
			cfg: config.SupersetConfig{
				OrderAnalyticsDashboardID: 100,
				RevenueDashboardID:        200,
			},
			wantCount: 2,
			validateResult: func(t *testing.T, result []DashboardInfo) {
				modulesSeen := make(map[DashboardModule]bool)
				for _, dashboard := range result {
					modulesSeen[dashboard.Module] = true
				}
				assert.True(t, modulesSeen[DashboardModuleOrders])
				assert.True(t, modulesSeen[DashboardModuleRevenue])
				assert.False(t, modulesSeen[DashboardModuleCustomers])
			},
		},
		{
			name:      "no dashboards configured",
			cfg:       config.SupersetConfig{},
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &mockSupersetClient{}
			service := NewService(mockClient, tt.cfg, zaptest.NewLogger(t))

			dashboards, err := service.ListAvailableDashboards(context.Background(), tenantID)

			assert.NoError(t, err)
			assert.Len(t, dashboards, tt.wantCount)
			if tt.validateResult != nil {
				tt.validateResult(t, dashboards)
			}
		})
	}
}

func TestService_GetDashboardInfo(t *testing.T) {
	tests := []struct {
		name        string
		module      DashboardModule
		dashboardID int
		mockClient  *mockSupersetClient
		wantErr     bool
		errIs       error
		wantTitle   string
	}{
		{
			name:        "successful dashboard info retrieval",
			module:      DashboardModuleOrders,
			dashboardID: 100,
			mockClient: &mockSupersetClient{
				getDashboardFunc: func(ctx context.Context, dashboardID int) (*superset.Dashboard, error) {
					return &superset.Dashboard{
						ID:              100,
						DashboardTitle:  "Order Analytics Dashboard",
						Slug:            "order-analytics",
						URL:             "/superset/dashboard/order-analytics/",
						PublishedStatus: true,
					}, nil
				},
			},
			wantErr:   false,
			wantTitle: "Order Analytics Dashboard",
		},
		{
			name:        "dashboard not configured",
			module:      DashboardModuleOrders,
			dashboardID: 0,
			mockClient:  &mockSupersetClient{},
			wantErr:     true,
			errIs:       ErrDashboardNotConfigured,
		},
		{
			name:        "Superset unavailable - returns basic info",
			module:      DashboardModuleRevenue,
			dashboardID: 200,
			mockClient: &mockSupersetClient{
				getDashboardFunc: func(ctx context.Context, dashboardID int) (*superset.Dashboard, error) {
					return nil, errors.New("connection refused")
				},
			},
			wantErr:   false,
			wantTitle: string(DashboardModuleRevenue), // Falls back to module name
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.SupersetConfig{}
			if tt.module == DashboardModuleOrders {
				cfg.OrderAnalyticsDashboardID = tt.dashboardID
			} else if tt.module == DashboardModuleRevenue {
				cfg.RevenueDashboardID = tt.dashboardID
			}

			service := NewService(tt.mockClient, cfg, zaptest.NewLogger(t))

			info, err := service.GetDashboardInfo(context.Background(), tt.module)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errIs != nil {
					assert.ErrorIs(t, err, tt.errIs)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantTitle, info.Title)
				assert.Equal(t, tt.dashboardID, info.ID)
				assert.Equal(t, tt.module, info.Module)
			}
		})
	}
}

func TestValidateModule(t *testing.T) {
	tests := []struct {
		name       string
		module     string
		wantValid  bool
		wantModule DashboardModule
	}{
		{
			name:       "valid orders module",
			module:     "orders",
			wantValid:  true,
			wantModule: DashboardModuleOrders,
		},
		{
			name:       "valid revenue module",
			module:     "revenue",
			wantValid:  true,
			wantModule: DashboardModuleRevenue,
		},
		{
			name:       "valid customers module",
			module:     "customers",
			wantValid:  true,
			wantModule: DashboardModuleCustomers,
		},
		{
			name:       "valid operations module",
			module:     "operations",
			wantValid:  true,
			wantModule: DashboardModuleOperations,
		},
		{
			name:       "valid subscription module",
			module:     "subscription",
			wantValid:  true,
			wantModule: DashboardModuleSubscription,
		},
		{
			name:      "invalid module",
			module:    "invalid",
			wantValid: false,
		},
		{
			name:      "empty module",
			module:    "",
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			module, valid := ValidateModule(tt.module)

			assert.Equal(t, tt.wantValid, valid)
			if tt.wantValid {
				assert.Equal(t, tt.wantModule, module)
			}
		})
	}
}

func TestService_IsEnabled(t *testing.T) {
	tests := []struct {
		name        string
		client      *mockSupersetClient
		cfg         config.SupersetConfig
		wantEnabled bool
	}{
		{
			name:   "enabled with client and dashboards",
			client: &mockSupersetClient{},
			cfg: config.SupersetConfig{
				OrderAnalyticsDashboardID: 100,
			},
			wantEnabled: true,
		},
		{
			name:   "disabled with no dashboards",
			client: &mockSupersetClient{},
			cfg:    config.SupersetConfig{},
			wantEnabled: false,
		},
		{
			name:        "disabled with nil client",
			cfg:         config.SupersetConfig{}, // No dashboards configured
			wantEnabled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var client superset.ClientInterface = tt.client
			if client == nil {
				// Pass nil explicitly
				service := &Service{
					supersetClient:  nil,
					dashboardConfig: make(map[DashboardModule]int),
					logger:          zaptest.NewLogger(t).Named("analytics.service"),
				}
				if tt.cfg.OrderAnalyticsDashboardID > 0 {
					service.dashboardConfig[DashboardModuleOrders] = tt.cfg.OrderAnalyticsDashboardID
				}
				assert.Equal(t, tt.wantEnabled, service.IsEnabled())
				return
			}
			service := NewService(client, tt.cfg, zaptest.NewLogger(t))
			assert.Equal(t, tt.wantEnabled, service.IsEnabled())
		})
	}
}

func TestService_GetSupersetBaseURL(t *testing.T) {
	tests := []struct {
		name    string
		client  *mockSupersetClient
		wantURL string
	}{
		{
			name: "returns base URL",
			client: &mockSupersetClient{
				baseURL: "https://superset.example.com",
			},
			wantURL: "https://superset.example.com",
		},
		{
			name:    "returns empty string for nil client",
			client:  nil,
			wantURL: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.client == nil {
				// Create service with nil client
				service := &Service{
					supersetClient:  nil,
					dashboardConfig: make(map[DashboardModule]int),
					logger:          zaptest.NewLogger(t).Named("analytics.service"),
				}
				assert.Equal(t, tt.wantURL, service.GetSupersetBaseURL())
				return
			}
			service := NewService(tt.client, config.SupersetConfig{}, zaptest.NewLogger(t))
			assert.Equal(t, tt.wantURL, service.GetSupersetBaseURL())
		})
	}
}

// Benchmark tests
func BenchmarkService_GetDashboardEmbed(b *testing.B) {
	tenantID := uuid.New()
	userID := uuid.New()

	mockClient := &mockSupersetClient{
		getEmbedURLFunc: func(ctx context.Context, dashboardID int, tID, uID uuid.UUID) (*superset.EmbedURL, error) {
			return &superset.EmbedURL{
				URL:       "https://superset.example.com/superset/embedded/100/?guest_token=token123",
				Token:     "token123",
				ExpiresAt: time.Now().Add(5 * time.Minute),
			}, nil
		},
	}

	service := NewService(mockClient, config.SupersetConfig{
		OrderAnalyticsDashboardID: 100,
	}, zaptest.NewLogger(b))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := service.GetDashboardEmbed(context.Background(), DashboardModuleOrders, tenantID, userID)
		require.NoError(b, err)
	}
}

func BenchmarkService_ListAvailableDashboards(b *testing.B) {
	tenantID := uuid.New()

	mockClient := &mockSupersetClient{}
	service := NewService(mockClient, config.SupersetConfig{
		OrderAnalyticsDashboardID:    100,
		RevenueDashboardID:           200,
		CustomerAnalyticsDashboardID: 300,
		OperationsDashboardID:        400,
		SubscriptionDashboardID:      500,
	}, zaptest.NewLogger(b))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := service.ListAvailableDashboards(context.Background(), tenantID)
		require.NoError(b, err)
	}
}
