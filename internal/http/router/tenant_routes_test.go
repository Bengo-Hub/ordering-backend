package router_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	analyticshandler "github.com/bengobox/ordering-backend/internal/http/handlers/analytics"
	compliancehandler "github.com/bengobox/ordering-backend/internal/http/handlers/compliance"
	googlebusinesshandler "github.com/bengobox/ordering-backend/internal/http/handlers/googlebusiness"
)

// TestTenantHandlersMountOnce guards the paths the admin pages call. These handlers are
// registered on the tenant group that router.go already mounts at /api/v1/{tenant}; when they
// added their own /{tenant} prefix the real paths became /api/v1/{tenant}/{tenant}/... and every
// analytics, compliance and Google Business call from the frontend got a 404.
func TestTenantHandlersMountOnce(t *testing.T) {
	r := chi.NewRouter()
	r.Route("/api/v1/{tenant}", func(tenant chi.Router) {
		analyticshandler.NewHandler(zap.NewNop(), nil).Register(tenant, nil)
		compliancehandler.NewHandler(zap.NewNop(), nil).Register(tenant, nil)
		googlebusinesshandler.NewHandler(zap.NewNop(), nil).Register(tenant, nil)
	})

	routes := map[string]bool{}
	err := chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		routes[method+" "+strings.TrimSuffix(route, "/")] = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"GET /api/v1/{tenant}/analytics/dashboards",
		"GET /api/v1/{tenant}/analytics/status",
		"GET /api/v1/{tenant}/compliance/data-subject-requests",
		"POST /api/v1/{tenant}/compliance/data-export",
		"GET /api/v1/{tenant}/admin/integrations/google/status",
		"GET /api/v1/{tenant}/integrations/google/callback",
	} {
		if !routes[want] {
			t.Errorf("missing route %s", want)
		}
	}
	for route := range routes {
		if strings.Count(route, "{tenant}") > 1 {
			t.Errorf("route mounts the tenant twice: %s", route)
		}
	}
}
