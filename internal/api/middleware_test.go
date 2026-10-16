package api

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tradepulse/internal/config"
)

func TestLimiterTokenBucket(t *testing.T) {
	l := newLimiter(0, 2) // no refill
	if !l.allow("ip") || !l.allow("ip") {
		t.Fatal("burst not honored")
	}
	if l.allow("ip") {
		t.Fatal("limit not enforced")
	}
	if !l.allow("other") {
		t.Fatal("limits must be per key")
	}
}

func TestClientIPTrustsProxyHeaderOnlyWhenConfigured(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:5555"
	req.Header.Set("X-Forwarded-For", "6.6.6.6, 5.6.7.8")

	untrusted := &Server{Deps: Deps{Cfg: config.Config{TrustProxy: false}}}
	if got := untrusted.clientIP(req); got != "10.0.0.1" {
		t.Errorf("spoofed X-Forwarded-For honored: %q", got)
	}
	trusted := &Server{Deps: Deps{Cfg: config.Config{TrustProxy: true}}}
	if got := trusted.clientIP(req); got != "5.6.7.8" {
		t.Errorf("trusted proxy entry not used: %q", got)
	}
}

func TestPanicIsRecoveredWithoutLeakingDetails(t *testing.T) {
	s := &Server{Deps: Deps{Log: slog.New(slog.NewTextHandler(io.Discard, nil))}}
	h := s.recoverer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("secret internal detail") }))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "secret internal detail") {
		t.Fatal("panic detail leaked to client")
	}
}

func TestBodyLimitMiddleware(t *testing.T) {
	var readErr error
	h := bodyLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, readErr = io.Copy(io.Discard, r.Body)
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("a", 2<<20))))
	if readErr == nil {
		t.Fatal("2 MB body was not rejected")
	}
}
