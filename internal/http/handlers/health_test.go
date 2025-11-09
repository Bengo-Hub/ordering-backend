package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	testing "testing"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type stubDB struct{ err error }

func (s stubDB) Ping(context.Context) error { return s.err }
func (s stubDB) Close()                     {}

func TestHealthHandler_Liveness(t *testing.T) {
	h := NewHealthHandler(zap.NewNop(), nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	h.Liveness(rec, req)

	if status := rec.Code; status != http.StatusOK {
		t.Fatalf("expected status 200, got %d", status)
	}
}

func TestHealthHandler_Status_WithIssues(t *testing.T) {
	h := NewHealthHandler(zap.NewNop(), stubDB{err: context.DeadlineExceeded}, redis.NewClient(&redis.Options{Addr: "localhost:0"}), nil)
	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	rec := httptest.NewRecorder()

	h.Status(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", rec.Code)
	}
}
