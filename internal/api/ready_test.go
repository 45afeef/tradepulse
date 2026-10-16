package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestReadyzReflectsDependencyHealth(t *testing.T) {
	s, h := newServer(t, nil)
	if rec := do(h, "GET", "/readyz", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("no dependency configured: %d", rec.Code)
	}
	s.Ready = func(context.Context) error { return nil }
	if rec := do(h, "GET", "/readyz", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("healthy dependency: %d", rec.Code)
	}
	s.Ready = func(context.Context) error { return errors.New("dial tcp: password=hunter2 refused") }
	rec := do(h, "GET", "/readyz", "", "")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("unhealthy dependency: %d, want 503", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "hunter2") || strings.Contains(rec.Body.String(), "refused") {
		t.Fatalf("driver error details leaked to the client: %s", rec.Body)
	}
	if rec := do(h, "GET", "/healthz", "", ""); rec.Code != http.StatusOK {
		t.Fatal("liveness must not depend on the database")
	}
}
