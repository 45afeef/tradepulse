package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"tradepulse/internal/alerting"
	"tradepulse/internal/auth"
	"tradepulse/internal/connector"
	"tradepulse/internal/config"
	"tradepulse/internal/delivery"
	"tradepulse/internal/domain/alert"
	"tradepulse/internal/domain/event"
	"tradepulse/internal/domain/invite"
	"tradepulse/internal/domain/user"
	"tradepulse/internal/idgen"
	"tradepulse/internal/infrastructure/eventbus"
	"tradepulse/internal/safefetch"
	"tradepulse/internal/storage/memory"
)

type nopPersonalizer struct{}

func (nopPersonalizer) UpsertUser(string, user.Profile) {}
func (nopPersonalizer) RemoveUser(string)               {}

// lastServer lets helpers reach the repositories of the server under test (tests do not run in parallel).
var lastServer *Server

func newServer(t *testing.T, mutate func(*config.Config)) (*Server, http.Handler) {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := config.Config{Env: "development", DocsEnabled: true, RateRPS: 1000, RateBurst: 1000,
		SourceMinInterval: 60 * time.Second, SourceMaxInterval: 24 * time.Hour,
		WebhookSigningKey: strings.Repeat("w", 40), WebhookRatePerMinute: 600,
		XBearerToken: "test-x-token", XMonthlyBudgetUSD: 1, XCostPerResourceUSD: 0.005, XMaxPostsPerFetch: 20, AllowedOrigins: []string{"http://allowed.example"}}
	if mutate != nil {
		mutate(&cfg)
	}
	jwt, err := auth.NewJWT(strings.Repeat("k", 32), "test", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakeNet{pages: map[string]*safefetch.Response{}, prefixes: map[string]*safefetch.Response{}}
	lastNet = fake
	usage := memory.NewUsageRepo()
	conns := []connector.Connector{connector.NewRSS(fake), connector.NewWebMonitor(fake, connector.NewRobots(fake, time.Hour, nil))}
	if cfg.XBearerToken != "" {
		conns = append(conns, connector.NewX(fake, connector.XSettings{Token: cfg.XBearerToken, BaseURL: "https://api.x.test",
			BudgetUSD: cfg.XMonthlyBudgetUSD, CostPerResourceUSD: cfg.XCostPerResourceUSD, MaxPosts: 20}, usage))
	}
	reg := event.NewRegistry()
	hub := delivery.NewHub(log, nil, 2)
	aRules, aFires, chans, dels := memory.NewAlertRuleRepo(), memory.NewAlertFireRepo(), memory.NewChannelRepo(), memory.NewDeliveryRepo()
	out := &fakeOut{}
	lastOut = out
	testClockOffset = 0
	engine := alerting.NewEngine(log, aRules, aFires, reg, hub, out, alerting.Config{MaxFiresPerHour: 3,
		Now: func() time.Time { return time.Now().Add(testClockOffset) }})
	s, err := New(Deps{
		Cfg: cfg, Log: log, Users: memory.NewUserRepo(), Feeds: memory.NewFeedRepo(10), JWT: jwt,
		Invites: memory.NewInviteRepo(), Audit: memory.NewAuditRepo(100),
		Sources: memory.NewSourceRepo(), Runs: memory.NewRunRepo(), Bus: eventbus.NewMemory(64, log),
		Connectors: connector.NewRegistry(conns...), Items: memory.NewItemStore(), Usage: usage,
		Tickets: auth.NewTicketStore(time.Minute), Personalizer: nopPersonalizer{},
		Hub: hub, Registry: reg, Alerts: engine, AlertRules: aRules, AlertFires: aFires, Channels: chans, Deliveries: dels,
		Templates: alert.NewTemplates(), Outbound: out,
	})
	if err != nil {
		t.Fatal(err)
	}
	s.authLim = newLimiter(1000, 1000) // rate-limit tests re-tighten this
	lastServer = s
	s.Bus.Subscribe("alerts", engine.Handle)
	return s, s.Handler()
}

func do(h http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// inviteCode creates a valid single-use invite directly in the repository.
func inviteCode(t *testing.T) string {
	t.Helper()
	code, hash, err := invite.NewCode()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	err = lastServer.Invites.Create(context.Background(), invite.Invite{
		ID: idgen.New(), CodeHash: hash, CreatedBy: "test", CreatedAt: now, ExpiresAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	return code
}

// adminToken creates an admin account directly and returns a valid token for it.
func adminToken(t *testing.T) (id, token string) {
	t.Helper()
	id = idgen.New()
	err := lastServer.Users.Create(context.Background(), user.User{ID: id, Email: id + "@admin.example", Role: user.RoleAdmin,
		Status: user.StatusActive, CreatedAt: time.Now().UTC(), Profile: user.DefaultProfile()})
	if err != nil {
		t.Fatal(err)
	}
	token, err = lastServer.JWT.Issue(id)
	if err != nil {
		t.Fatal(err)
	}
	return id, token
}

func register(t *testing.T, h http.Handler, email string) string {
	t.Helper()
	rec := do(h, http.MethodPost, "/api/v1/auth/register", "", `{"email":"`+email+`","password":"correct-horse-battery","invite_code":"`+inviteCode(t)+`"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register %s: %d %s", email, rec.Code, rec.Body)
	}
	var out struct{ Token string }
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.Token == "" {
		t.Fatalf("no token in %s", rec.Body)
	}
	return out.Token
}

func TestPrivateRoutesRequireValidToken(t *testing.T) {
	_, h := newServer(t, nil)
	paths := []struct{ method, path string }{
		{"GET", "/api/v1/me/subscriptions"}, {"PUT", "/api/v1/me/subscriptions"},
		{"GET", "/api/v1/me/feed"}, {"POST", "/api/v1/ws-ticket"}, {"GET", "/api/v1/event-types"},
	}
	good := register(t, h, "a@example.com")
	for _, p := range paths {
		for name, tok := range map[string]string{"none": "", "garbage": "abc.def.ghi", "tampered": good + "x"} {
			if rec := do(h, p.method, p.path, tok, `{}`); rec.Code != http.StatusUnauthorized {
				t.Errorf("%s %s with %s token: got %d, want 401", p.method, p.path, name, rec.Code)
			}
		}
	}
}

func TestSubscriptionsAreIsolatedBetweenUsers(t *testing.T) {
	_, h := newServer(t, nil)
	alice := register(t, h, "alice@example.com")
	bob := register(t, h, "bob@example.com")
	put := do(h, "PUT", "/api/v1/me/subscriptions", alice, `{"subscriptions":[{"kind":"entity","value":"Secret Topic"}]}`)
	if put.Code != http.StatusOK {
		t.Fatalf("put failed: %d %s", put.Code, put.Body)
	}
	var p user.Profile
	_ = json.Unmarshal(do(h, "GET", "/api/v1/me/subscriptions", alice, "").Body.Bytes(), &p)
	if len(p.Subscriptions) != 1 || p.Subscriptions[0].Value != "secret topic" {
		t.Fatalf("alice lost her subscription: %+v", p)
	}
	var bobP user.Profile
	rec := do(h, "GET", "/api/v1/me/subscriptions", bob, "")
	_ = json.Unmarshal(rec.Body.Bytes(), &bobP)
	if len(bobP.Subscriptions) != 0 || strings.Contains(rec.Body.String(), "secret topic") {
		t.Fatalf("cross-user data leak: %s", rec.Body)
	}
}

func TestSubscriptionValidation(t *testing.T) {
	_, h := newServer(t, nil)
	tok := register(t, h, "v@example.com")
	bad := map[string]string{
		"unknown field in condition": `{"subscriptions":[{"kind":"entity","value":"x","conditions":[{"field":"nope","op":"==","value":"a"}]}]}`,
		"bad kind":                   `{"subscriptions":[{"kind":"weird","value":"x"}]}`,
		"unknown property":           `{"subscriptions":[],"bogus":1}`,
		"unknown type":               `{"subscriptions":[{"kind":"entity","value":"x","types":["nope"]}]}`,
		"malformed json":             `{"subscriptions":`,
	}
	for name, body := range bad {
		if rec := do(h, "PUT", "/api/v1/me/subscriptions", tok, body); rec.Code < 400 || rec.Code >= 500 {
			t.Errorf("%s: got %d, want 4xx", name, rec.Code)
		}
	}
}

func TestDocsOnlyWhenEnabled(t *testing.T) {
	_, dev := newServer(t, nil)
	if rec := do(dev, "GET", "/openapi.json", "", ""); rec.Code != http.StatusOK {
		t.Errorf("dev /openapi.json: %d", rec.Code)
	}
	rec := do(dev, "GET", "/docs", "", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Content-Security-Policy"), "unpkg.com") {
		t.Errorf("dev /docs: %d, CSP %q", rec.Code, rec.Header().Get("Content-Security-Policy"))
	}
	_, prod := newServer(t, func(c *config.Config) { c.Env = "production"; c.DocsEnabled = false })
	for _, p := range []string{"/docs", "/openapi.json", "/openapi.yaml"} {
		if rec := do(prod, "GET", p, "", ""); rec.Code != http.StatusNotFound {
			t.Errorf("prod %s must be 404, got %d", p, rec.Code)
		}
	}
	if csp := do(prod, "GET", "/healthz", "", "").Header().Get("Content-Security-Policy"); strings.Contains(csp, "unpkg") {
		t.Error("relaxed CSP leaked to non-docs route")
	}
}

func TestSecurityHeaders(t *testing.T) {
	_, h := newServer(t, func(c *config.Config) { c.Env = "production"; c.DocsEnabled = false })
	rec := do(h, "GET", "/healthz", "", "")
	want := map[string]string{
		"X-Content-Type-Options": "nosniff", "X-Frame-Options": "DENY",
		"Referrer-Policy": "no-referrer", "Cache-Control": "no-store",
	}
	for k, v := range want {
		if got := rec.Header().Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
	if !strings.Contains(rec.Header().Get("Content-Security-Policy"), "default-src 'none'") {
		t.Error("strict CSP missing")
	}
	if rec.Header().Get("Strict-Transport-Security") == "" || rec.Header().Get("X-Request-ID") == "" {
		t.Error("HSTS (production) or request id missing")
	}
}

func TestCORSAllowlist(t *testing.T) {
	_, h := newServer(t, nil)
	req := func(origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodOptions, "/api/v1/auth/login", nil)
		r.Header.Set("Origin", origin)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec
	}
	if got := req("http://allowed.example").Header().Get("Access-Control-Allow-Origin"); got != "http://allowed.example" {
		t.Errorf("allowed origin not echoed: %q", got)
	}
	if got := req("http://evil.example").Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("foreign origin must not get CORS headers, got %q", got)
	}
}

func TestLoginDoesNotRevealWhetherEmailExists(t *testing.T) {
	_, h := newServer(t, nil)
	register(t, h, "real@example.com")
	wrongPw := do(h, "POST", "/api/v1/auth/login", "", `{"email":"real@example.com","password":"definitely-wrong-pw"}`)
	unknown := do(h, "POST", "/api/v1/auth/login", "", `{"email":"ghost@example.com","password":"definitely-wrong-pw"}`)
	if wrongPw.Code != http.StatusUnauthorized || unknown.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401/401, got %d/%d", wrongPw.Code, unknown.Code)
	}
	if wrongPw.Body.String() != unknown.Body.String() {
		t.Fatalf("responses differ and leak account existence:\n%s\n%s", wrongPw.Body, unknown.Body)
	}
}

func TestAuthEndpointsAreRateLimited(t *testing.T) {
	s, h := newServer(t, nil)
	s.authLim = newLimiter(0, 5) // no refill: the test must not depend on how slow password hashing is
	limited := false
	for i := 0; i < 8; i++ {
		if do(h, "POST", "/api/v1/auth/login", "", `{"email":"x@example.com","password":"wrong-password-1"}`).Code == http.StatusTooManyRequests {
			limited = true
			break
		}
	}
	if !limited {
		t.Fatal("login brute force not rate limited")
	}
}

func TestOversizedBodyRejected(t *testing.T) {
	_, h := newServer(t, nil)
	body := `{"email":"` + strings.Repeat("a", 80<<10) + `@x.com","password":"correct-horse-battery"}`
	if rec := do(h, "POST", "/api/v1/auth/register", "", body); rec.Code < 400 || rec.Code == http.StatusUnauthorized {
		t.Fatalf("oversized body not rejected: %d", rec.Code)
	}
}

func TestRegisterValidationAndDuplicates(t *testing.T) {
	_, h := newServer(t, nil)
	for name, body := range map[string]string{
		"short password": `{"invite_code":"tp_inv_0123456789","email":"a@example.com","password":"short"}`,
		"bad email":      `{"invite_code":"tp_inv_0123456789","email":"not-an-email","password":"correct-horse-battery"}`,
		"name in email":  `{"invite_code":"tp_inv_0123456789","email":"Bob <b@example.com>","password":"correct-horse-battery"}`,
	} {
		if rec := do(h, "POST", "/api/v1/auth/register", "", body); rec.Code < 400 || rec.Code >= 500 {
			t.Errorf("%s: got %d, want 4xx", name, rec.Code)
		}
	}
	register(t, h, "dup@example.com")
	if rec := do(h, "POST", "/api/v1/auth/register", "", `{"email":"DUP@example.com","password":"correct-horse-battery","invite_code":"`+inviteCode(t)+`"}`); rec.Code != http.StatusConflict {
		t.Fatalf("duplicate email: got %d, want 409", rec.Code)
	}
}

func TestWebSocketTicketIsSingleUseEvenOnFailedHandshake(t *testing.T) {
	_, h := newServer(t, nil)
	tok := register(t, h, "ws@example.com")
	var out struct{ Ticket string }
	_ = json.Unmarshal(do(h, "POST", "/api/v1/ws-ticket", tok, "").Body.Bytes(), &out)
	if out.Ticket == "" {
		t.Fatal("no ticket issued")
	}
	if rec := do(h, "GET", "/ws?ticket=nope", "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad ticket: %d", rec.Code)
	}
	first := do(h, "GET", "/ws?ticket="+out.Ticket, "", "") // no upgrade headers -> 400, ticket consumed
	if first.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for non-upgrade request, got %d", first.Code)
	}
	if again := do(h, "GET", "/ws?ticket="+out.Ticket, "", ""); again.Code != http.StatusUnauthorized {
		t.Fatalf("ticket reusable: %d", again.Code)
	}
	if rec := do(h, "GET", "/ws", "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing ticket: %d", rec.Code)
	}
}

func TestEventTypesEndpoint(t *testing.T) {
	_, h := newServer(t, nil)
	tok := register(t, h, "et@example.com")
	rec := do(h, "GET", "/api/v1/event-types", tok, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "content_item") || !strings.Contains(rec.Body.String(), "builtin_fields") {
		t.Fatalf("unexpected: %d %s", rec.Code, rec.Body)
	}
}
