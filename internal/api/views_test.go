package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"tradepulse/internal/domain/feed"
)

func seedItem(t *testing.T, s *Server, uid, id, source, title string, entities []string, age time.Duration) {
	t.Helper()
	_, err := s.Feeds.AddItem(context.Background(), feed.FeedItem{ID: id, UserID: uid, EventID: id, EventType: "content_item", Source: source, Entities: entities,
		Title: title, Severity: feed.Info, Reason: "test", CreatedAt: time.Now().Add(-age)})
	if err != nil {
		t.Fatal(err)
	}
}

func newView(t *testing.T, h http.Handler, tok, body string) string {
	t.Helper()
	rec := do(h, "POST", "/api/v1/me/views", tok, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create view: %d %s", rec.Code, rec.Body)
	}
	return idOf(t, rec)
}

func TestSavedViewsCRUDValidationLimitAndIsolation(t *testing.T) {
	_, h := newServer(t, nil)
	alice, bob := register(t, h, "a@example.com"), register(t, h, "b@example.com")
	rec := do(h, "POST", "/api/v1/me/views", alice, `{"name":"  CNBC   markets ","filter":{"sources":["CNBC","cnbc"],"entities":["Markets"]}}`)
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"name":"CNBC markets"`) || !strings.Contains(rec.Body.String(), `"sources":["cnbc"]`) {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	id := idOf(t, rec)
	if up := do(h, "PUT", "/api/v1/me/views/"+id, alice, `{"name":"CNBC markets","filter":{"sources":["cnbc"],"within_hours":24}}`); up.Code != http.StatusOK || !strings.Contains(up.Body.String(), `"within_hours":24`) {
		t.Fatalf("update: %d %s", up.Code, up.Body)
	}
	if l := do(h, "GET", "/api/v1/me/views", alice, ""); !strings.Contains(l.Body.String(), id) {
		t.Fatalf("list: %s", l.Body)
	}
	if d := do(h, "POST", "/api/v1/me/views", alice, `{"name":"cnbc MARKETS","filter":{"sources":["x"]}}`); d.Code != http.StatusConflict {
		t.Fatalf("duplicate name (case-insensitive): %d", d.Code)
	}
	for name, body := range map[string]string{
		"empty filter": `{"name":"x","filter":{}}`, "blank name": `{"name":"  ","filter":{"sources":["a"]}}`,
		"unknown type": `{"name":"x","filter":{"types":["nope"]}}`, "too many": `{"name":"x","filter":{"sources":["` + strings.Repeat(`a","`, 25) + `a"]}}`,
		"unknown field": `{"name":"x","filter":{"sources":["a"]},"bogus":1}`, "hours": `{"name":"x","filter":{"within_hours":9999}}`,
	} {
		if r := do(h, "POST", "/api/v1/me/views", alice, body); r.Code < 400 || r.Code >= 500 {
			t.Errorf("%s: %d (%s)", name, r.Code, r.Body)
		}
	}
	for name, r := range map[string]int{
		"get-as-list": do(h, "PUT", "/api/v1/me/views/"+id, bob, `{"name":"mine now","filter":{"sources":["a"]}}`).Code,
		"delete":      do(h, "DELETE", "/api/v1/me/views/"+id, bob, "").Code,
	} {
		if r != http.StatusNotFound {
			t.Errorf("bob %s alice's view: %d, want 404", name, r)
		}
	}
	if strings.Contains(do(h, "GET", "/api/v1/me/views", bob, "").Body.String(), id) {
		t.Fatal("views leak across members")
	}
	for i := 2; i <= 20; i++ {
		newView(t, h, alice, fmt.Sprintf(`{"name":"v%d","filter":{"entities":["e%d"]}}`, i, i))
	}
	if r := do(h, "POST", "/api/v1/me/views", alice, `{"name":"one too many","filter":{"entities":["z"]}}`); r.Code != http.StatusConflict {
		t.Fatalf("view limit: %d", r.Code)
	}
	if r := do(h, "DELETE", "/api/v1/me/views/"+id, alice, ""); r.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", r.Code)
	}
}

func TestFeedThroughAViewFiltersAndPagesLikeTheFeed(t *testing.T) {
	s, h := newServer(t, nil)
	alice, bob := register(t, h, "a@example.com"), register(t, h, "b@example.com")
	uid := userOf(t, s, alice)
	seedItem(t, s, uid, "i1", "cnbc", "Oil jumps", []string{"markets"}, 5*time.Hour)
	seedItem(t, s, uid, "i2", "moneycontrol", "Sensex falls", []string{"markets"}, 4*time.Hour)
	seedItem(t, s, uid, "i3", "cnbc", "Chip news", []string{"tech"}, 3*time.Hour)
	seedItem(t, s, uid, "i4", "cnbc", "Rates held", []string{"markets"}, 2*time.Hour)
	seedItem(t, s, uid, "i5", "x", "Old news", []string{"markets"}, 72*time.Hour)
	seedItem(t, s, uid, "i6", "cnbc", "Bonds rally", []string{"markets"}, time.Hour)

	if all := getList(t, h, "/api/v1/me/feed?limit=50", alice); len(all.Items) != 6 {
		t.Fatalf("plain feed: %d", len(all.Items))
	}
	cnbc := newView(t, h, alice, `{"name":"cnbc markets","filter":{"sources":["CNBC"],"entities":["markets"]}}`)
	p1 := getList(t, h, "/api/v1/me/feed?limit=2&view="+cnbc, alice)
	if !eq(titles(p1), "Bonds rally", "Rates held") || p1.NextCursor == "" || p1.Order != "desc" {
		t.Fatalf("view page 1: %+v", p1)
	}
	p2 := getList(t, h, "/api/v1/me/feed?limit=2&view="+cnbc+"&before="+p1.NextCursor, alice)
	if !eq(titles(p2), "Oil jumps") || p2.NextCursor != "" {
		t.Fatalf("view page 2 skips non-matching items and ends cleanly: %+v", p2)
	}
	up := getList(t, h, "/api/v1/me/feed?limit=5&view="+cnbc+"&after="+p2.Items[0].Cursor, alice)
	if !eq(titles(up), "Rates held", "Bonds rally") || up.Order != "asc" {
		t.Fatalf("view catch-up: %+v", up)
	}
	kw := newView(t, h, alice, `{"name":"rates","filter":{"keywords":["rates"]}}`)
	if got := getList(t, h, "/api/v1/me/feed?view="+kw, alice); !eq(titles(got), "Rates held") {
		t.Fatalf("keyword view: %+v", got)
	}
	recent := newView(t, h, alice, `{"name":"recent markets","filter":{"entities":["markets"],"within_hours":24}}`)
	if got := getList(t, h, "/api/v1/me/feed?view="+recent, alice); !eq(titles(got), "Bonds rally", "Rates held", "Sensex falls", "Oil jumps") {
		t.Fatalf("time window view: %+v", got)
	}
	typed := newView(t, h, alice, `{"name":"typed","filter":{"types":["content_item"]}}`)
	if got := getList(t, h, "/api/v1/me/feed?view="+typed+"&limit=200", alice); len(got.Items) != 6 {
		t.Fatalf("type view: %d", len(got.Items))
	}
	for name, q := range map[string]string{"unknown view": "?view=nope", "another member's view": "?view=" + cnbc} {
		tok := alice
		if name == "another member's view" {
			tok = bob
		}
		if rec := do(h, "GET", "/api/v1/me/feed"+q, tok, ""); rec.Code != http.StatusNotFound {
			t.Errorf("%s: %d, want 404", name, rec.Code)
		}
	}
}

func TestFeedbackOnFeedItemsAndAlertsAndTheOperatorSummary(t *testing.T) {
	s, h := newServer(t, nil)
	_, admin := adminToken(t)
	alice, bob := register(t, h, "a@example.com"), register(t, h, "b@example.com")
	aid, bid := userOf(t, s, alice), userOf(t, s, bob)
	seedItem(t, s, aid, "a1", "cnbc", "Oil jumps", []string{"markets"}, time.Hour)
	seedItem(t, s, aid, "a2", "moneycontrol", "Sensex falls", []string{"markets"}, time.Hour)
	seedItem(t, s, bid, "b1", "cnbc", "Bonds rally", []string{"markets"}, time.Hour)

	rate := func(tok, path, body string) int { return do(h, "POST", path, tok, body).Code }
	if c := rate(alice, "/api/v1/me/feed/a1/feedback", `{"rating":"useful"}`); c != http.StatusOK {
		t.Fatalf("rate: %d", c)
	}
	if feed := do(h, "GET", "/api/v1/me/feed", alice, "").Body.String(); !strings.Contains(feed, `"feedback":"useful"`) {
		t.Fatalf("the member's rating must show on the item: %s", feed)
	}
	if c := rate(alice, "/api/v1/me/feed/a1/feedback", `{"rating":"not_useful","note":"seen it already"}`); c != http.StatusOK {
		t.Fatalf("re-rate: %d", c)
	}
	if feed := do(h, "GET", "/api/v1/me/feed", alice, "").Body.String(); strings.Contains(feed, `"feedback":"useful"`) || !strings.Contains(feed, `"feedback":"not_useful"`) {
		t.Fatalf("rating replaces the earlier one: %s", feed)
	}
	if strings.Contains(do(h, "GET", "/api/v1/me/feed", bob, "").Body.String(), `"feedback"`) {
		t.Fatal("ratings are private to the member")
	}
	if c := rate(alice, "/api/v1/me/feed/a2/feedback", `{"rating":"useful"}`); c != http.StatusOK {
		t.Fatal(c)
	}
	if c := rate(bob, "/api/v1/me/feed/b1/feedback", `{"rating":"useful"}`); c != http.StatusOK {
		t.Fatal(c)
	}
	if rec := do(h, "DELETE", "/api/v1/me/feed/a2/feedback", alice, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("clear: %d", rec.Code)
	}
	if rec := do(h, "DELETE", "/api/v1/me/feed/never-rated/feedback", alice, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("clearing nothing is still 204: %d", rec.Code)
	}
	for name, c := range map[string]int{
		"another member's item": rate(bob, "/api/v1/me/feed/a1/feedback", `{"rating":"useful"}`),
		"unknown item":          rate(alice, "/api/v1/me/feed/nope/feedback", `{"rating":"useful"}`),
	} {
		if c != http.StatusNotFound {
			t.Errorf("%s: %d, want 404", name, c)
		}
	}
	for name, body := range map[string]string{
		"bad rating": `{"rating":"meh"}`, "no rating": `{}`, "long note": `{"rating":"useful","note":"` + strings.Repeat("a", 281) + `"}`,
		"control char": `{"rating":"useful","note":"bad\u0007note"}`, "unknown field": `{"rating":"useful","x":1}`,
	} {
		if c := rate(alice, "/api/v1/me/feed/a1/feedback", body); c < 400 || c >= 500 {
			t.Errorf("%s: %d", name, c)
		}
	}

	// alerts: a rule fires, the member rates the alert
	manualFeed(t, h, admin)
	newRule(t, h, alice, kwRule("fb rule", "fbkw"))
	testClockOffset += 11 * time.Second
	postItem(t, h, admin, "fbkw story")
	waitFires(t, h, alice, 1)
	fireID := getList(t, h, "/api/v1/me/alerts/fires", alice).Fires[0].ID
	if c := rate(alice, "/api/v1/me/alerts/fires/"+fireID+"/feedback", `{"rating":"not_useful"}`); c != http.StatusOK {
		t.Fatalf("rate alert: %d", c)
	}
	if body := do(h, "GET", "/api/v1/me/alerts/fires", alice, "").Body.String(); !strings.Contains(body, `"feedback":"not_useful"`) {
		t.Fatalf("alert rating must show on the fire: %s", body)
	}
	if c := rate(bob, "/api/v1/me/alerts/fires/"+fireID+"/feedback", `{"rating":"useful"}`); c != http.StatusNotFound {
		t.Fatalf("another member's alert: %d", c)
	}

	// operator summary: totals, precision, groups, no identities
	rec := do(h, "GET", "/api/v1/admin/feedback/summary", admin, "")
	var out struct {
		FeedItems struct {
			Useful    int      `json:"useful"`
			NotUseful int      `json:"not_useful"`
			Precision *float64 `json:"precision"`
		} `json:"feed_items"`
		AlertFires struct {
			Useful    int      `json:"useful"`
			NotUseful int      `json:"not_useful"`
			Precision *float64 `json:"precision"`
		} `json:"alert_fires"`
		Groups []struct {
			Group     string `json:"group"`
			Label     string `json:"label"`
			Useful    int    `json:"useful"`
			NotUseful int    `json:"not_useful"`
		} `json:"groups"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("summary: %d %s", rec.Code, rec.Body)
	}
	// feed: alice a1 not_useful (cleared a2), bob b1 useful -> 1 useful, 1 not useful; alerts: 1 not useful
	if out.FeedItems.Useful != 1 || out.FeedItems.NotUseful != 1 || out.FeedItems.Precision == nil || *out.FeedItems.Precision != 0.5 {
		t.Fatalf("feed totals: %+v", out.FeedItems)
	}
	if out.AlertFires.NotUseful != 1 || out.AlertFires.Useful != 0 || out.AlertFires.Precision == nil || *out.AlertFires.Precision != 0 {
		t.Fatalf("alert totals: %+v", out.AlertFires)
	}
	var sawSource, sawRule bool
	for _, g := range out.Groups {
		sawSource = sawSource || (g.Group == "source:cnbc" && g.Label == "cnbc")
		sawRule = sawRule || (strings.HasPrefix(g.Group, "rule:") && g.Label == "fb rule")
	}
	if !sawSource || !sawRule {
		t.Fatalf("groups: %+v", out.Groups)
	}
	if body := rec.Body.String(); strings.Contains(body, "@") || strings.Contains(body, aid) || strings.Contains(body, bid) {
		t.Fatal("the summary must not contain member identities")
	}
	if c := do(h, "GET", "/api/v1/admin/feedback/summary", alice, "").Code; c != http.StatusForbidden {
		t.Fatalf("members cannot read the summary: %d", c)
	}
	if c := do(h, "GET", "/api/v1/admin/feedback/summary?days=0", admin, "").Code; c < 400 || c >= 500 {
		t.Fatalf("days=0: %d", c)
	}
}

func TestViewsAndFeedbackEndpointsRequireAuthentication(t *testing.T) {
	_, h := newServer(t, nil)
	for _, r := range []struct{ m, p string }{
		{"GET", "/api/v1/me/views"}, {"POST", "/api/v1/me/views"}, {"PUT", "/api/v1/me/views/x"}, {"DELETE", "/api/v1/me/views/x"},
		{"POST", "/api/v1/me/feed/x/feedback"}, {"DELETE", "/api/v1/me/feed/x/feedback"},
		{"POST", "/api/v1/me/alerts/fires/x/feedback"}, {"DELETE", "/api/v1/me/alerts/fires/x/feedback"}, {"GET", "/api/v1/admin/feedback/summary"},
	} {
		if rec := do(h, r.m, r.p, "", `{}`); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s anonymous: %d, want 401", r.m, r.p, rec.Code)
		}
	}
}
