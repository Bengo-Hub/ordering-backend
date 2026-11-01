package googlebusiness

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/bengobox/ordering-backend/internal/modules/googlebusiness"
)

const frontend = "https://ordering.example.com/{tenant}/dashboard/staff/integrations"

func callbackRouter(svc *googlebusiness.Service) http.Handler {
	h := NewHandler(zap.NewNop(), svc)
	r := chi.NewRouter()
	r.Route("/api/v1", func(v1 chi.Router) { h.RegisterPublicCallback(v1) })
	return r
}

func TestCallbackNotConfiguredLandsOnTenantPage(t *testing.T) {
	signer := googlebusiness.NewService(nil, googlebusiness.OAuthConfig{}, frontend, zap.NewNop(), nil)
	state, err := signer.SignState(uuid.New(), "codevertex-demo", nil)
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/integrations/google/callback?code=c&state="+state, nil)
	callbackRouter(signer).ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	want := "https://ordering.example.com/codevertex-demo/dashboard/staff/integrations?integration=google&status=not_configured"
	if loc != want {
		t.Fatalf("Location %q, want %q", loc, want)
	}
}

func TestCallbackDeniedAndInvalidState(t *testing.T) {
	oauth := googlebusiness.NewOAuthConfig("id", "secret", "https://api.example.com/api/v1/integrations/google/callback")
	svc := googlebusiness.NewService(nil, oauth, frontend, zap.NewNop(), nil)
	state, _ := svc.SignState(uuid.New(), "demo", nil)

	// The person declined on Google's consent screen.
	rec := httptest.NewRecorder()
	callbackRouter(svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/api/v1/integrations/google/callback?error=access_denied&state="+state, nil))
	if loc := rec.Header().Get("Location"); !strings.HasSuffix(loc, "/demo/dashboard/staff/integrations?integration=google&status=denied") {
		t.Fatalf("denied Location %q", loc)
	}

	// A state that does not verify never stores anything.
	rec = httptest.NewRecorder()
	callbackRouter(svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/api/v1/integrations/google/callback?code=c&state=bad.state", nil))
	if loc := rec.Header().Get("Location"); !strings.HasSuffix(loc, "status=error") {
		t.Fatalf("invalid state Location %q", loc)
	}
}
