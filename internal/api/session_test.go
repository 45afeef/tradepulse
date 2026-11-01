package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"tradepulse/internal/config"
	"tradepulse/internal/domain/session"
)

type pair struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	User         struct {
		ID string `json:"id"`
	} `json:"user"`
}

func parsePair(t *testing.T, rec *httptest.ResponseRecorder) pair {
	t.Helper()
	var p pair
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil || p.Token == "" {
		t.Fatalf("no token pair (%d): %s", rec.Code, rec.Body)
	}
	return p
}

func signupFull(t *testing.T, h http.Handler, email string) pair {
	t.Helper()
	rec := do(h, "POST", "/api/v1/auth/register", "", `{"email":"`+email+`","password":"correct-horse-battery","invite_code":"`+inviteCode(t)+`"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("signup: %d %s", rec.Code, rec.Body)
	}
	return parsePair(t, rec)
}

func loginPair(t *testing.T, h http.Handler, email, pw string) pair {
	t.Helper()
	rec := do(h, "POST", "/api/v1/auth/login", "", `{"email":"`+email+`","password":"`+pw+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("login: %d %s", rec.Code, rec.Body)
	}
	return parsePair(t, rec)
}

func refreshWith(h http.Handler, rt string) *httptest.ResponseRecorder {
	return do(h, "POST", "/api/v1/auth/refresh", "", `{"refresh_token":"`+rt+`"}`)
}

func feedCode(h http.Handler, tok string) int { return do(h, "GET", "/api/v1/me/feed", tok, "").Code }

func TestSignupAndLoginReturnRefreshTokens(t *testing.T) {
	_, h := newServer(t, nil)
	p := signupFull(t, h, "a@example.com")
	if !strings.HasPrefix(p.RefreshToken, session.TokenPrefix) || p.ExpiresIn != 60 || p.User.ID == "" {
		t.Fatalf("signup pair: %+v", p)
	}
	l := loginPair(t, h, "a@example.com", "correct-horse-battery")
	if l.RefreshToken == p.RefreshToken || l.Token == "" {
		t.Fatal("every login must get its own refresh token")
	}
}

func TestRefreshRotatesAndReuseRevokesTheWholeSession(t *testing.T) {
	_, h := newServer(t, nil)
	p0 := signupFull(t, h, "a@example.com")
	rec := refreshWith(h, p0.RefreshToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("refresh: %d %s", rec.Code, rec.Body)
	}
	p1 := parsePair(t, rec)
	if p1.RefreshToken == p0.RefreshToken || p1.Token == p0.Token || feedCode(h, p1.Token) != http.StatusOK {
		t.Fatalf("rotation must yield a new working pair: %+v", p1)
	}
	if r := refreshWith(h, p0.RefreshToken); r.Code != http.StatusUnauthorized {
		t.Fatalf("a used refresh token must be refused: %d", r.Code)
	}
	if r := refreshWith(h, p1.RefreshToken); r.Code != http.StatusUnauthorized {
		t.Fatalf("reuse must revoke the whole family (theft response): %d", r.Code)
	}
}

func TestRefreshRejectsBadTokensAndSuspendedUsers(t *testing.T) {
	s, h := newServer(t, nil)
	p := signupFull(t, h, "a@example.com")
	uid := p.User.ID
	for name, tok := range map[string]string{"garbage": "not-a-token-at-all", "unknown": "tp_rt_" + strings.Repeat("A", 43)} {
		if r := refreshWith(h, tok); r.Code != http.StatusUnauthorized {
			t.Errorf("%s: %d", name, r.Code)
		}
	}
	expired, hash, _ := session.New()
	_ = s.Sessions.Create(context.Background(), session.Token{ID: "e", UserID: uid, FamilyID: "fam-e", Hash: hash, CreatedAt: time.Now().Add(-48 * time.Hour), ExpiresAt: time.Now().Add(-time.Hour)})
	if r := refreshWith(h, expired); r.Code != http.StatusUnauthorized {
		t.Errorf("expired: %d", r.Code)
	}
	_, admin := adminToken(t)
	do(h, "POST", "/api/v1/admin/users/"+uid+"/suspend", admin, "")
	if r := refreshWith(h, p.RefreshToken); r.Code != http.StatusUnauthorized {
		t.Fatalf("a suspended member must not refresh: %d", r.Code)
	}
	do(h, "POST", "/api/v1/admin/users/"+uid+"/reactivate", admin, "")
	if r := refreshWith(h, p.RefreshToken); r.Code != http.StatusUnauthorized {
		t.Fatalf("the session revoked during suspension must stay dead: %d", r.Code)
	}
}

func TestLogoutIsUniformAndRevokesTheSession(t *testing.T) {
	_, h := newServer(t, nil)
	p := signupFull(t, h, "a@example.com")
	if r := do(h, "POST", "/api/v1/auth/logout", "", `{"refresh_token":"`+p.RefreshToken+`"}`); r.Code != http.StatusNoContent {
		t.Fatalf("logout: %d", r.Code)
	}
	if r := refreshWith(h, p.RefreshToken); r.Code != http.StatusUnauthorized {
		t.Fatalf("logged-out session must not refresh: %d", r.Code)
	}
	if r := do(h, "POST", "/api/v1/auth/logout", "", `{"refresh_token":"tp_rt_`+strings.Repeat("Z", 43)+`"}`); r.Code != http.StatusNoContent {
		t.Fatalf("unknown tokens must get the same answer: %d", r.Code)
	}
}

func TestLogoutAllInvalidatesEverySessionImmediately(t *testing.T) {
	_, h := newServer(t, nil)
	a := signupFull(t, h, "a@example.com")
	b := loginPair(t, h, "a@example.com", "correct-horse-battery")
	if feedCode(h, a.Token) != http.StatusOK || feedCode(h, b.Token) != http.StatusOK {
		t.Fatal("both sessions work before")
	}
	if r := do(h, "POST", "/api/v1/me/logout-all", a.Token, ""); r.Code != http.StatusNoContent {
		t.Fatalf("logout-all: %d", r.Code)
	}
	for name, tok := range map[string]string{"A access": a.Token, "B access": b.Token} {
		if feedCode(h, tok) != http.StatusUnauthorized {
			t.Errorf("%s must be dead at once (not at expiry)", name)
		}
	}
	for name, rt := range map[string]string{"A refresh": a.RefreshToken, "B refresh": b.RefreshToken} {
		if refreshWith(h, rt).Code != http.StatusUnauthorized {
			t.Errorf("%s must be dead", name)
		}
	}
	fresh := loginPair(t, h, "a@example.com", "correct-horse-battery")
	if feedCode(h, fresh.Token) != http.StatusOK {
		t.Fatal("a login after logout-all must work immediately")
	}
}

func TestChangePasswordSignsOutOtherSessionsAndIsThrottled(t *testing.T) {
	s, h := newServer(t, nil)
	a := signupFull(t, h, "a@example.com")
	b := loginPair(t, h, "a@example.com", "correct-horse-battery")
	body := func(cur, nw string) string { return `{"current_password":"` + cur + `","new_password":"` + nw + `"}` }
	if r := do(h, "POST", "/api/v1/me/password", a.Token, body("wrong-password-1", "brand-new-password-1")); r.Code != http.StatusForbidden {
		t.Fatalf("wrong current password: %d", r.Code)
	}
	for name, b := range map[string]string{"same": body("correct-horse-battery", "correct-horse-battery"), "short": body("correct-horse-battery", "short")} {
		if r := do(h, "POST", "/api/v1/me/password", a.Token, b); r.Code < 400 || r.Code >= 500 {
			t.Errorf("%s new password: %d", name, r.Code)
		}
	}
	ok := do(h, "POST", "/api/v1/me/password", a.Token, body("correct-horse-battery", "brand-new-password-1"))
	if ok.Code != http.StatusOK {
		t.Fatalf("change: %d %s", ok.Code, ok.Body)
	}
	fresh := parsePair(t, ok)
	if feedCode(h, fresh.Token) != http.StatusOK {
		t.Fatal("the response must carry a working token pair for this device")
	}
	if feedCode(h, b.Token) != http.StatusUnauthorized || refreshWith(h, b.RefreshToken).Code != http.StatusUnauthorized || refreshWith(h, a.RefreshToken).Code != http.StatusUnauthorized {
		t.Fatal("every other session must be signed out")
	}
	if r := do(h, "POST", "/api/v1/auth/login", "", `{"email":"a@example.com","password":"correct-horse-battery"}`); r.Code != http.StatusUnauthorized {
		t.Fatalf("old password must stop working: %d", r.Code)
	}
	loginPair(t, h, "a@example.com", "brand-new-password-1")
	for i := 0; i < 10; i++ {
		s.throttle.fail("pw:" + fresh.User.ID)
	}
	if r := do(h, "POST", "/api/v1/me/password", fresh.Token, body("brand-new-password-1", "yet-another-password-2")); r.Code != http.StatusTooManyRequests {
		t.Fatalf("repeated wrong current passwords must lock the endpoint: %d", r.Code)
	}
}

func TestAdminResetPasswordIssuesOneTimeTemporaryPassword(t *testing.T) {
	_, h := newServer(t, nil)
	_, admin := adminToken(t)
	p := signupFull(t, h, "a@example.com")
	other := register(t, h, "other@example.com")
	rec := do(h, "POST", "/api/v1/admin/users/"+p.User.ID+"/reset-password", admin, "")
	var v struct {
		Temp string `json:"temporary_password"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &v)
	if rec.Code != http.StatusOK || len(v.Temp) < 20 {
		t.Fatalf("reset: %d %s", rec.Code, rec.Body)
	}
	if feedCode(h, p.Token) != http.StatusUnauthorized || refreshWith(h, p.RefreshToken).Code != http.StatusUnauthorized {
		t.Fatal("the member's sessions must end at once")
	}
	if r := do(h, "POST", "/api/v1/auth/login", "", `{"email":"a@example.com","password":"correct-horse-battery"}`); r.Code != http.StatusUnauthorized {
		t.Fatalf("old password: %d", r.Code)
	}
	loginPair(t, h, "a@example.com", v.Temp)
	if r := do(h, "POST", "/api/v1/admin/users/"+p.User.ID+"/reset-password", other, ""); r.Code != http.StatusForbidden {
		t.Fatalf("members cannot reset passwords: %d", r.Code)
	}
	if r := do(h, "POST", "/api/v1/admin/users/nope/reset-password", admin, ""); r.Code != http.StatusNotFound {
		t.Fatalf("unknown user: %d", r.Code)
	}
	if audit := do(h, "GET", "/api/v1/admin/audit", admin, "").Body.String(); !strings.Contains(audit, "user.reset_password") || strings.Contains(audit, v.Temp) {
		t.Fatal("the reset must be audited without the password")
	}
}

func TestLoginLockoutIsIdenticalForKnownAndUnknownAccounts(t *testing.T) {
	s, h := newServer(t, nil)
	signupFull(t, h, "victim@example.com")
	signupFull(t, h, "bystander@example.com")
	for i := 0; i < 10; i++ {
		s.throttle.fail("victim@example.com")
		s.throttle.fail("ghost@example.com")
	}
	known := do(h, "POST", "/api/v1/auth/login", "", `{"email":"Victim@example.com","password":"correct-horse-battery"}`)
	ghost := do(h, "POST", "/api/v1/auth/login", "", `{"email":"ghost@example.com","password":"whatever-password"}`)
	if known.Code != http.StatusTooManyRequests || ghost.Code != http.StatusTooManyRequests || known.Body.String() != ghost.Body.String() {
		t.Fatalf("lockout must not reveal whether the account exists:\n%d %s\n%d %s", known.Code, known.Body, ghost.Code, ghost.Body)
	}
	loginPair(t, h, "bystander@example.com", "correct-horse-battery")
	s.throttle.reset("victim@example.com")
	loginPair(t, h, "victim@example.com", "correct-horse-battery")
}

func TestMetricsEndpointIsTokenProtectedAndLeaksNothing(t *testing.T) {
	token := strings.Repeat("m", 32)
	s, h := newServer(t, func(c *config.Config) { c.MetricsToken = token })
	s.RegisterMetrics(func() uint64 { return 7 })
	signupFull(t, h, "private.person@example.com")
	do(h, "GET", "/api/v1/me/feed", "", "") // a 401 to populate the 4xx class
	bad1 := do(h, "GET", "/metrics", "", "")
	bad2 := do(h, "GET", "/metrics", "wrong-token", "")
	if bad1.Code != http.StatusUnauthorized || bad2.Code != http.StatusUnauthorized || bad1.Body.String() != bad2.Body.String() {
		t.Fatalf("missing/wrong token: %d %d", bad1.Code, bad2.Code)
	}
	rec := do(h, "GET", "/metrics", token, "")
	out := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/plain") {
		t.Fatalf("scrape: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	for _, want := range []string{`http_requests_total{class="2xx"}`, `http_requests_total{class="4xx"}`, "http_request_duration_seconds_count", "bus_dropped_events 7", "ws_connections 0", `sources_by_health{health="ok"} 0`, "x_budget_usd 1"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, secret := range []string{"private.person", "@", token, "tp_rt_", "whsec_"} {
		if strings.Contains(out, secret) {
			t.Fatalf("metrics must not contain %q", secret)
		}
	}
	_, off := newServer(t, nil)
	if r := do(off, "GET", "/metrics", token, ""); r.Code != http.StatusNotFound {
		t.Fatalf("metrics must be a 404 unless METRICS_TOKEN is set: %d", r.Code)
	}
}

func TestMeReturnsProfileAndRoleAndLoginCarriesTheRole(t *testing.T) {
	_, h := newServer(t, nil)
	_, adminTok := adminToken(t)
	p := signupFull(t, h, "me@example.com")
	var m struct {
		ID, Email, Role string
	}
	rec := do(h, "GET", "/api/v1/me", p.Token, "")
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	if rec.Code != http.StatusOK || m.Email != "me@example.com" || m.Role != "member" || m.ID != p.User.ID {
		t.Fatalf("member: %d %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "password") {
		t.Fatal("never expose credentials")
	}
	if rec := do(h, "GET", "/api/v1/me", adminTok, ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"role":"admin"`) {
		t.Fatalf("admin: %d %s", rec.Code, rec.Body)
	}
	if rec := do(h, "GET", "/api/v1/me", "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", rec.Code)
	}
	if !strings.Contains(do(h, "POST", "/api/v1/auth/login", "", `{"email":"me@example.com","password":"correct-horse-battery"}`).Body.String(), `"role":"member"`) {
		t.Fatal("login must carry the role so clients can route members and admins")
	}
}
