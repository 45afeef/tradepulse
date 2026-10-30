package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"tradepulse/internal/config"
	"tradepulse/internal/domain/alert"
	"tradepulse/internal/domain/feed"
)

type titled struct {
	Title  string `json:"title"`
	Cursor string `json:"cursor"`
	ID     string `json:"id"`
}
type listPage struct {
	Items      []titled `json:"items"`
	Fires      []titled `json:"fires"`
	Entries    []titled `json:"entries"`
	Deliveries []titled `json:"deliveries"`
	NextCursor string   `json:"next_cursor"`
	Order      string   `json:"order"`
}

func (p listPage) all() []titled {
	return append(append(append(append([]titled{}, p.Items...), p.Fires...), p.Entries...), p.Deliveries...)
}

func getList(t *testing.T, h http.Handler, path, tok string) listPage {
	t.Helper()
	rec := do(h, "GET", path, tok, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: %d %s", path, rec.Code, rec.Body)
	}
	var p listPage
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func titles(p listPage) []string {
	var out []string
	for _, x := range p.all() {
		out = append(out, x.Title)
	}
	return out
}

func eq(a []string, b ...string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestFeedCursorPaginationAndCatchUp(t *testing.T) {
	s, h := newServer(t, nil)
	tok, other := register(t, h, "p@example.com"), register(t, h, "other@example.com")
	uid := userOf(t, s, tok)
	for i := 1; i <= 7; i++ { // seeded directly: this test is about paging, not about matching
		if _, err := s.Feeds.AddItem(context.Background(), feed.FeedItem{ID: fmt.Sprint("fi", i), UserID: uid, Title: fmt.Sprintf("item %d", i), CreatedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}

	p1 := getList(t, h, "/api/v1/me/feed?limit=3", tok)
	if !eq(titles(p1), "item 7", "item 6", "item 5") || p1.Order != "desc" || p1.NextCursor == "" {
		t.Fatalf("page 1: %+v", p1)
	}
	p2 := getList(t, h, "/api/v1/me/feed?limit=3&before="+p1.NextCursor, tok)
	if !eq(titles(p2), "item 4", "item 3", "item 2") || p2.NextCursor == "" {
		t.Fatalf("page 2: %+v", p2)
	}
	p3 := getList(t, h, "/api/v1/me/feed?limit=3&before="+p2.NextCursor, tok)
	if !eq(titles(p3), "item 1") || p3.NextCursor != "" {
		t.Fatalf("the last page is short and has no next cursor: %+v", p3)
	}

	// catch-up after a reconnect: oldest first, from the cursor of "item 3"
	c3 := p2.Items[1].Cursor
	a1 := getList(t, h, "/api/v1/me/feed?limit=2&after="+c3, tok)
	if !eq(titles(a1), "item 4", "item 5") || a1.Order != "asc" || a1.NextCursor == "" {
		t.Fatalf("catch-up 1: %+v", a1)
	}
	a2 := getList(t, h, "/api/v1/me/feed?limit=2&after="+a1.NextCursor, tok)
	if !eq(titles(a2), "item 6", "item 7") {
		t.Fatalf("catch-up 2: %+v", a2)
	}
	a3 := getList(t, h, "/api/v1/me/feed?limit=2&after="+a2.NextCursor, tok)
	if len(a3.Items) != 0 || a3.NextCursor != "" {
		t.Fatalf("catch-up ends with an empty page: %+v", a3)
	}

	for name, q := range map[string]string{
		"garbage cursor": "?before=garbage", "both cursors": "?before=" + p1.NextCursor + "&after=" + p1.NextCursor,
		"limit zero": "?limit=0", "limit too big": "?limit=500", "forged cursor": "?after=c1_" + strings.Repeat("A", 10),
	} {
		if rec := do(h, "GET", "/api/v1/me/feed"+q, tok, ""); rec.Code < 400 || rec.Code >= 500 {
			t.Errorf("%s: got %d, want 4xx", name, rec.Code)
		}
	}
	if leak := getList(t, h, "/api/v1/me/feed?before="+p1.NextCursor, other); len(leak.Items) != 0 {
		t.Fatal("a cursor from another member's feed must reveal nothing")
	}
}

func TestAlertFiresAuditAndDeliveriesPagination(t *testing.T) {
	s, h := newServer(t, nil)
	_, admin := adminToken(t)
	tok, other := register(t, h, "p@example.com"), register(t, h, "other@example.com")
	manualFeed(t, h, admin)
	newRule(t, h, tok, kwRule("pg", "pagekw"))
	for i := 1; i <= 3; i++ {
		testClockOffset += 11 * time.Second
		postItem(t, h, admin, fmt.Sprintf("pagekw story %d", i))
		waitFires(t, h, tok, i)
	}
	f1 := getList(t, h, "/api/v1/me/alerts/fires?limit=2", tok)
	if !eq(titles(f1), "pagekw story 3", "pagekw story 2") || f1.NextCursor == "" || f1.Order != "desc" {
		t.Fatalf("fires page 1: %+v", f1)
	}
	f2 := getList(t, h, "/api/v1/me/alerts/fires?limit=2&before="+f1.NextCursor, tok)
	if !eq(titles(f2), "pagekw story 1") || f2.NextCursor != "" {
		t.Fatalf("fires page 2: %+v", f2)
	}
	up := getList(t, h, "/api/v1/me/alerts/fires?limit=5&after="+f2.Fires[0].Cursor, tok)
	if !eq(titles(up), "pagekw story 2", "pagekw story 3") || up.Order != "asc" {
		t.Fatalf("fires catch-up: %+v", up)
	}
	if len(getList(t, h, "/api/v1/me/alerts/fires", other).Fires) != 0 {
		t.Fatal("fires must be private")
	}

	for i := 0; i < 5; i++ {
		if rec := do(h, "POST", "/api/v1/admin/invites", admin, `{}`); rec.Code != http.StatusCreated {
			t.Fatalf("invite: %d", rec.Code)
		}
	}
	full := getList(t, h, "/api/v1/admin/audit?limit=500", admin).Entries
	var walked []titled
	path := "/api/v1/admin/audit?limit=2"
	for guard := 0; guard < 20; guard++ {
		p := getList(t, h, path, admin)
		walked = append(walked, p.Entries...)
		if p.NextCursor == "" {
			break
		}
		path = "/api/v1/admin/audit?limit=2&before=" + p.NextCursor
	}
	if len(full) < 5 || len(walked) != len(full) {
		t.Fatalf("audit paging must cover everything exactly once: %d vs %d", len(walked), len(full))
	}
	for i := range full {
		if full[i].ID != walked[i].ID {
			t.Fatalf("audit order differs at %d", i)
		}
	}

	uid := userOf(t, s, tok)
	for i := 1; i <= 5; i++ {
		_ = s.Deliveries.Add(context.Background(), alert.Delivery{ID: fmt.Sprint("d", i), ChannelID: "c", UserID: uid, FireID: "f", At: time.Now(), OK: true, Attempts: 1})
	}
	d1 := getList(t, h, "/api/v1/me/channels/deliveries?limit=2", tok)
	d2 := getList(t, h, "/api/v1/me/channels/deliveries?limit=2&before="+d1.NextCursor, tok)
	d3 := getList(t, h, "/api/v1/me/channels/deliveries?limit=2&before="+d2.NextCursor, tok)
	if len(d1.Deliveries) != 2 || len(d2.Deliveries) != 2 || len(d3.Deliveries) != 1 || d3.NextCursor != "" || d1.Deliveries[0].ID != "d5" {
		t.Fatalf("deliveries paging: %d %d %d", len(d1.Deliveries), len(d2.Deliveries), len(d3.Deliveries))
	}
	if len(getList(t, h, "/api/v1/me/channels/deliveries", other).Deliveries) != 0 {
		t.Fatal("deliveries must be private")
	}
}

func assertProblem(t *testing.T, name string, rec *httptest.ResponseRecorder, status int) {
	t.Helper()
	if rec.Code != status {
		t.Errorf("%s: status %d, want %d (%s)", name, rec.Code, status, rec.Body)
		return
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/problem+json") {
		t.Errorf("%s: content type %q, want application/problem+json", name, ct)
	}
	var p struct {
		Title  string `json:"title"`
		Status int    `json:"status"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil || p.Title == "" || p.Status != status {
		t.Errorf("%s: not the uniform error shape: %s", name, rec.Body)
	}
}

func TestEveryErrorUsesTheSameProblemShape(t *testing.T) {
	s, h := newServer(t, nil)
	_ = s
	tok := register(t, h, "shape@example.com")
	dupInvite := inviteCode(t)
	badInvite := `{"email":"x@example.com","password":"correct-horse-battery","invite_code":"tp_inv_not-a-real-code"}`
	assertProblem(t, "401 no token", do(h, "GET", "/api/v1/me/feed", "", ""), 401)
	assertProblem(t, "403 not admin", do(h, "GET", "/api/v1/admin/users", tok, ""), 403)
	assertProblem(t, "404 unknown route", do(h, "GET", "/api/v1/does-not-exist", tok, ""), 404)
	assertProblem(t, "404 wrong method", do(h, "POST", "/healthz", "", `{}`), 404)
	assertProblem(t, "404 unknown resource", do(h, "DELETE", "/api/v1/me/alerts/zzz", tok, ""), 404)
	assertProblem(t, "422 validation", do(h, "POST", "/api/v1/auth/register", "", `{"email":"a@example.com","password":"short","invite_code":"tp_inv_0123456789"}`), 422)
	assertProblem(t, "403 bad invite", do(h, "POST", "/api/v1/auth/register", "", badInvite), 403)
	assertProblem(t, "409 duplicate", do(h, "POST", "/api/v1/auth/register", "", `{"email":"shape@example.com","password":"correct-horse-battery","invite_code":"`+dupInvite+`"}`), 409)
	assertProblem(t, "413 webhook body", postHook(h, "any", `{"title":"`+strings.Repeat("a", 70<<10)+`"}`, nil), 413)

	s2, h2 := newServer(t, nil)
	s2.authLim = newLimiter(0, 1)
	login := `{"email":"nobody@example.com","password":"definitely-wrong-pw"}`
	do(h2, "POST", "/api/v1/auth/login", "", login)
	assertProblem(t, "429 rate limit", do(h2, "POST", "/api/v1/auth/login", "", login), 429)

	_, h3 := newServer(t, func(c *config.Config) { c.WebhookSigningKey = "" })
	assertProblem(t, "503 webhooks off", postHook(h3, "any", `{"title":"x"}`, nil), 503)

	_, h4 := newServer(t, func(c *config.Config) { c.MetricsToken = strings.Repeat("m", 32) })
	assertProblem(t, "401 metrics", do(h4, "GET", "/metrics", "", ""), 401)

	boom := (&Server{Deps: Deps{Log: slog.New(slog.NewTextHandler(io.Discard, nil))}}).recoverer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("x") }))
	rec := httptest.NewRecorder()
	boom.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	assertProblem(t, "500 panic", rec, 500)
}

func TestCORSExposesRequestIDAndRetryAfter(t *testing.T) {
	_, h := newServer(t, nil)
	r := httptest.NewRequest(http.MethodOptions, "/api/v1/auth/login", nil)
	r.Header.Set("Origin", "http://allowed.example")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	exposed := rec.Header().Get("Access-Control-Expose-Headers")
	if !strings.Contains(exposed, "X-Request-ID") || !strings.Contains(exposed, "Retry-After") {
		t.Fatalf("frontends need these headers: %q", exposed)
	}
	if rec.Header().Get("X-Request-ID") == "" {
		t.Fatal("every response carries a request id")
	}
}

func TestSpecDeclaresTheFreezeCandidateVersion(t *testing.T) {
	raw, err := OpenAPISpec()
	if err != nil || !strings.Contains(string(raw), `"version": "1.0.0-rc.1"`) {
		t.Fatalf("spec version: %v", err)
	}
	for _, p := range []string{"/api/v1/me/feed", "/api/v1/me/alerts/fires", "/api/v1/me/channels/deliveries", "/api/v1/admin/audit"} {
		all := string(raw)
		i := strings.Index(all, `"`+p+`"`)
		end := i + 4000
		if end > len(all) {
			end = len(all)
		}
		if i < 0 || !strings.Contains(all[i:end], `"before"`) || !strings.Contains(all[i:end], `"after"`) {
			t.Errorf("%s must document before/after cursor parameters", p)
		}
	}
}
