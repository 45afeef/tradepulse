package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"tradepulse/internal/config"
	"tradepulse/internal/domain/alert"
)

type fakeOut struct {
	mu    sync.Mutex
	enq   []string // "user|channel"
	tests []string
}

func (f *fakeOut) Enqueue(userID, channelID string, _ alert.Fire) {
	f.mu.Lock()
	f.enq = append(f.enq, userID+"|"+channelID)
	f.mu.Unlock()
}
func (f *fakeOut) EnqueueTest(userID, channelID string) {
	f.mu.Lock()
	f.tests = append(f.tests, userID+"|"+channelID)
	f.mu.Unlock()
}

var (
	lastOut         *fakeOut
	testClockOffset time.Duration
)

func idOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var v struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &v)
	if v.ID == "" {
		t.Fatalf("no id in response (%d): %s", rec.Code, rec.Body)
	}
	return v.ID
}

func userOf(t *testing.T, s *Server, token string) string {
	t.Helper()
	c, err := s.JWT.Verify(token)
	if err != nil {
		t.Fatal(err)
	}
	return c.Sub
}

func newRule(t *testing.T, h http.Handler, tok, body string) string {
	t.Helper()
	rec := do(h, "POST", "/api/v1/me/alerts", tok, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create alert: %d %s", rec.Code, rec.Body)
	}
	return idOf(t, rec)
}

func kwRule(name, kw string) string {
	return `{"name":"` + name + `","scope":{"kind":"keyword","values":["` + kw + `"]},"cooldown_seconds":10,"enabled":true}`
}

func newChan(t *testing.T, h http.Handler, tok, name string) (id, secret string) {
	t.Helper()
	rec := do(h, "POST", "/api/v1/me/channels", tok, `{"type":"webhook","name":"`+name+`","url":"https://hooks.example.com/tp"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create channel: %d %s", rec.Code, rec.Body)
	}
	var v struct {
		ID     string `json:"id"`
		Secret string `json:"secret"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &v)
	return v.ID, v.Secret
}

func manualFeed(t *testing.T, h http.Handler, admin string) {
	t.Helper()
	id := createSrc(t, h, admin, `{"name":"feed","type":"manual","entities":["news"]}`)
	if rec := do(h, "PUT", "/api/v1/admin/sources/"+id, admin, `{"enabled":true,"entities":["news"]}`); rec.Code != http.StatusOK {
		t.Fatalf("enable feed: %d", rec.Code)
	}
}

func postItem(t *testing.T, h http.Handler, admin, title string) {
	t.Helper()
	if rec := do(h, "POST", "/api/v1/admin/items", admin, `{"source":"feed","title":"`+title+`"}`); rec.Code != http.StatusCreated {
		t.Fatalf("post item: %d %s", rec.Code, rec.Body)
	}
}

type fireJSON struct {
	Title    string `json:"title"`
	Reason   string `json:"reason"`
	RuleName string `json:"rule_name"`
}

func fires(h http.Handler, tok string) []fireJSON {
	var v struct {
		Fires []fireJSON `json:"fires"`
	}
	_ = json.Unmarshal(do(h, "GET", "/api/v1/me/alerts/fires", tok, "").Body.Bytes(), &v)
	return v.Fires
}

func waitFires(t *testing.T, h http.Handler, tok string, n int) []fireJSON {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if f := fires(h, tok); len(f) >= n {
			return f
		}
		time.Sleep(15 * time.Millisecond)
	}
	t.Fatalf("expected %d fires, got %d", n, len(fires(h, tok)))
	return nil
}

func settle() { time.Sleep(120 * time.Millisecond) }

func TestAlertEndpointsRequireAuthentication(t *testing.T) {
	_, h := newServer(t, nil)
	for _, r := range []struct{ m, p string }{
		{"GET", "/api/v1/alert-templates"}, {"GET", "/api/v1/me/alerts"}, {"POST", "/api/v1/me/alerts"}, {"POST", "/api/v1/me/alerts/from-template"},
		{"POST", "/api/v1/me/alerts/preview"}, {"GET", "/api/v1/me/alerts/fires"}, {"GET", "/api/v1/me/alerts/x"}, {"PUT", "/api/v1/me/alerts/x"},
		{"DELETE", "/api/v1/me/alerts/x"}, {"GET", "/api/v1/me/channels"}, {"POST", "/api/v1/me/channels"}, {"GET", "/api/v1/me/channels/deliveries"},
		{"DELETE", "/api/v1/me/channels/x"}, {"POST", "/api/v1/me/channels/x/rotate-secret"}, {"POST", "/api/v1/me/channels/x/enable"}, {"POST", "/api/v1/me/channels/x/test"},
	} {
		if rec := do(h, r.m, r.p, "", `{}`); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s anonymous: %d, want 401", r.m, r.p, rec.Code)
		}
	}
}

func TestAlertTemplatesListed(t *testing.T) {
	_, h := newServer(t, nil)
	tok := register(t, h, "t@example.com")
	rec := do(h, "GET", "/api/v1/alert-templates", tok, "")
	for _, want := range []string{"keyword-mention", "topic-updates", "source-new-items", "values_hint"} {
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("templates: %d missing %s: %s", rec.Code, want, rec.Body)
		}
	}
}

func TestCreateAlertValidation(t *testing.T) {
	_, h := newServer(t, nil)
	alice, bob := register(t, h, "a@example.com"), register(t, h, "b@example.com")
	bobChan, _ := newChan(t, h, bob, "bobs")
	base := `"scope":{"kind":"entity","values":["tcs"]},"enabled":true`
	for name, body := range map[string]string{
		"empty name":         `{"name":"","` + strings.TrimPrefix(base, `"`) + `}`,
		"bad kind":           `{"name":"x","scope":{"kind":"weird","values":["a"]},"enabled":true}`,
		"no values":          `{"name":"x","scope":{"kind":"entity","values":[]},"enabled":true}`,
		"unknown condition":  `{"name":"x",` + base + `,"conditions":[{"field":"nope","op":"==","value":"a"}]}`,
		"cooldown too small": `{"name":"x",` + base + `,"cooldown_seconds":5}`,
		"bad severity":       `{"name":"x",` + base + `,"severity":"critical"}`,
		"unknown channel":    `{"name":"x",` + base + `,"channels":["no-such-channel"]}`,
		"someone elses chan": `{"name":"x",` + base + `,"channels":["` + bobChan + `"]}`,
		"unknown type":       `{"name":"x",` + base + `,"types":["nope"]}`,
		"unknown property":   `{"name":"x",` + base + `,"bogus":1}`,
	} {
		if rec := do(h, "POST", "/api/v1/me/alerts", alice, body); rec.Code < 400 || rec.Code >= 500 {
			t.Errorf("%s: got %d (%s)", name, rec.Code, rec.Body)
		}
	}
}

func TestAlertLifecycleAndDefaults(t *testing.T) {
	_, h := newServer(t, nil)
	tok := register(t, h, "life@example.com")
	rec := do(h, "POST", "/api/v1/me/alerts", tok, `{"name":"  Outages ","scope":{"kind":"keyword","values":["Outage"]},"enabled":true}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	for _, want := range []string{`"name":"Outages"`, `"cooldown_seconds":300`, `"severity":"notable"`, `"channels":["in_app"]`, `"values":["outage"]`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("defaults/normalization missing %s: %s", want, rec.Body)
		}
	}
	id := idOf(t, rec)
	if g := do(h, "GET", "/api/v1/me/alerts/"+id, tok, ""); g.Code != http.StatusOK {
		t.Fatalf("get: %d", g.Code)
	}
	up := do(h, "PUT", "/api/v1/me/alerts/"+id, tok, `{"name":"Big outages","scope":{"kind":"keyword","values":["outage","downtime"]},"severity":"high","cooldown_seconds":60,"enabled":true}`)
	if up.Code != http.StatusOK || !strings.Contains(up.Body.String(), `"severity":"high"`) || !strings.Contains(up.Body.String(), "downtime") {
		t.Fatalf("update: %d %s", up.Code, up.Body)
	}
	if l := do(h, "GET", "/api/v1/me/alerts", tok, ""); !strings.Contains(l.Body.String(), "Big outages") {
		t.Fatalf("list: %s", l.Body)
	}
	if d := do(h, "DELETE", "/api/v1/me/alerts/"+id, tok, ""); d.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", d.Code)
	}
	if g := do(h, "GET", "/api/v1/me/alerts/"+id, tok, ""); g.Code != http.StatusNotFound {
		t.Fatalf("deleted rule: %d", g.Code)
	}
}

func TestAlertOwnershipIsolation(t *testing.T) {
	_, h := newServer(t, nil)
	alice, bob := register(t, h, "a@example.com"), register(t, h, "b@example.com")
	id := newRule(t, h, alice, kwRule("private", "secret"))
	for name, rec := range map[string]*httptest.ResponseRecorder{
		"get":    do(h, "GET", "/api/v1/me/alerts/"+id, bob, ""),
		"put":    do(h, "PUT", "/api/v1/me/alerts/"+id, bob, kwRule("hijack", "x")),
		"delete": do(h, "DELETE", "/api/v1/me/alerts/"+id, bob, ""),
	} {
		if rec.Code != http.StatusNotFound {
			t.Errorf("bob %s alice's rule: %d, want 404", name, rec.Code)
		}
	}
	if l := do(h, "GET", "/api/v1/me/alerts", bob, ""); strings.Contains(l.Body.String(), "private") {
		t.Fatal("list leaks another user's rules")
	}
	if g := do(h, "GET", "/api/v1/me/alerts/"+id, alice, ""); g.Code != http.StatusOK {
		t.Fatal("alice lost her rule")
	}
}

func TestAlertRuleLimit(t *testing.T) {
	_, h := newServer(t, nil)
	tok := register(t, h, "many@example.com")
	for i := 0; i < alert.MaxRulesPerUser; i++ {
		newRule(t, h, tok, fmt.Sprintf(`{"name":"r%d","scope":{"kind":"entity","values":["e%d"]},"enabled":false}`, i, i))
	}
	if rec := do(h, "POST", "/api/v1/me/alerts", tok, kwRule("one too many", "x")); rec.Code != http.StatusConflict {
		t.Fatalf("limit: %d, want 409", rec.Code)
	}
}

func TestCreateAlertFromTemplate(t *testing.T) {
	_, h := newServer(t, nil)
	tok := register(t, h, "tpl@example.com")
	rec := do(h, "POST", "/api/v1/me/alerts/from-template", tok, `{"template_id":"keyword-mention","values":["Fraud"]}`)
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"name":"Keyword mention"`) || !strings.Contains(rec.Body.String(), `"values":["fraud"]`) {
		t.Fatalf("from template: %d %s", rec.Code, rec.Body)
	}
	if rec := do(h, "POST", "/api/v1/me/alerts/from-template", tok, `{"template_id":"keyword-mention","values":["x"],"name":"My name"}`); !strings.Contains(rec.Body.String(), "My name") {
		t.Fatalf("name override: %s", rec.Body)
	}
	for name, c := range map[string]struct {
		body string
		want int
	}{
		"unknown template": {`{"template_id":"nope","values":["x"]}`, 404},
		"no values":        {`{"template_id":"keyword-mention","values":[]}`, 422},
		"bad channel":      {`{"template_id":"keyword-mention","values":["x"],"channels":["nope"]}`, 422},
	} {
		if rec := do(h, "POST", "/api/v1/me/alerts/from-template", tok, c.body); rec.Code != c.want {
			t.Errorf("%s: got %d want %d (%s)", name, rec.Code, c.want, rec.Body)
		}
	}
}

func TestAlertFiresEndToEndWithExplanationAndIsolation(t *testing.T) {
	s, h := newServer(t, nil)
	_, admin := adminToken(t)
	alice, bob := register(t, h, "a@example.com"), register(t, h, "b@example.com")
	manualFeed(t, h, admin)
	chID, _ := newChan(t, h, alice, "n8n")
	newRule(t, h, alice, `{"name":"Outage watch","scope":{"kind":"keyword","values":["outage"]},"cooldown_seconds":10,"severity":"high","channels":["in_app","`+chID+`"],"enabled":true}`)
	newRule(t, h, bob, kwRule("bobs rule", "unrelated"))

	postItem(t, h, admin, "Major outage at provider")
	f := waitFires(t, h, alice, 1)
	if f[0].Title != "Major outage at provider" || f[0].RuleName != "Outage watch" || !strings.Contains(f[0].Reason, "Outage watch") {
		t.Fatalf("fire: %+v", f[0])
	}
	settle()
	if len(fires(h, bob)) != 0 {
		t.Fatal("bob must not receive alice's alerts")
	}
	lastOut.mu.Lock()
	defer lastOut.mu.Unlock()
	if len(lastOut.enq) != 1 || lastOut.enq[0] != userOf(t, s, alice)+"|"+chID {
		t.Fatalf("webhook channel dispatch: %v", lastOut.enq)
	}
}

func TestAutoPauseIsVisibleAndReversibleViaAPI(t *testing.T) {
	_, h := newServer(t, nil)
	_, admin := adminToken(t)
	tok := register(t, h, "noisy@example.com")
	manualFeed(t, h, admin)
	spec := kwRule("noisy", "alpha")
	id := newRule(t, h, tok, spec)
	for i := 1; i <= 3; i++ {
		testClockOffset += 11 * time.Second
		postItem(t, h, admin, fmt.Sprintf("alpha news %d", i))
		waitFires(t, h, tok, i)
	}
	testClockOffset += 11 * time.Second
	postItem(t, h, admin, "alpha news 4") // exceeds the hourly cap of 3 -> auto-pause
	deadline := time.Now().Add(2 * time.Second)
	var rule string
	for time.Now().Before(deadline) {
		rule = do(h, "GET", "/api/v1/me/alerts/"+id, tok, "").Body.String()
		if strings.Contains(rule, "paused_reason") {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}
	if !strings.Contains(rule, `"enabled":false`) || !strings.Contains(rule, "auto-paused") {
		t.Fatalf("rule must be visibly paused with a reason: %s", rule)
	}
	testClockOffset += 11 * time.Second
	postItem(t, h, admin, "alpha news 5")
	settle()
	if n := len(fires(h, tok)); n != 3 {
		t.Fatalf("a paused rule must not fire, got %d fires", n)
	}
	up := do(h, "PUT", "/api/v1/me/alerts/"+id, tok, spec)
	if up.Code != http.StatusOK || strings.Contains(up.Body.String(), "paused_reason") {
		t.Fatalf("re-enabling must clear the paused reason: %d %s", up.Code, up.Body)
	}
	testClockOffset += 11 * time.Second
	postItem(t, h, admin, "alpha news 6")
	waitFires(t, h, tok, 4)
}

func TestSuspendedMemberAlertsStopAndResume(t *testing.T) {
	s, h := newServer(t, nil)
	_, admin := adminToken(t)
	tok := register(t, h, "sus@example.com")
	uid := userOf(t, s, tok)
	manualFeed(t, h, admin)
	newRule(t, h, tok, kwRule("delta", "delta"))
	if rec := do(h, "POST", "/api/v1/admin/users/"+uid+"/suspend", admin, ""); rec.Code != http.StatusOK {
		t.Fatalf("suspend: %d", rec.Code)
	}
	postItem(t, h, admin, "delta news while suspended")
	settle()
	if rec := do(h, "POST", "/api/v1/admin/users/"+uid+"/reactivate", admin, ""); rec.Code != http.StatusOK {
		t.Fatalf("reactivate: %d", rec.Code)
	}
	if len(fires(h, tok)) != 0 {
		t.Fatal("a suspended member's rules must not fire")
	}
	testClockOffset += 11 * time.Second
	postItem(t, h, admin, "delta news after")
	if f := waitFires(t, h, tok, 1); f[0].Title != "delta news after" {
		t.Fatalf("rules must resume after reactivation: %+v", f)
	}
}

func TestPreviewEndpointHasNoSideEffects(t *testing.T) {
	_, h := newServer(t, nil)
	_, admin := adminToken(t)
	tok := register(t, h, "prev@example.com")
	manualFeed(t, h, admin)
	for i := 1; i <= 3; i++ {
		postItem(t, h, admin, fmt.Sprintf("gamma news %d", i))
	}
	settle()
	rec := do(h, "POST", "/api/v1/me/alerts/preview", tok, `{"name":"p","scope":{"kind":"keyword","values":["gamma"]},"enabled":true}`)
	var p struct {
		Examined   int  `json:"examined"`
		WouldMatch int  `json:"would_match"`
		WouldFire  int  `json:"would_fire"`
		WouldPause bool `json:"would_pause"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &p)
	if rec.Code != http.StatusOK || p.Examined < 3 || p.WouldMatch != 3 || p.WouldFire != 1 || p.WouldPause {
		t.Fatalf("preview: %d %s", rec.Code, rec.Body)
	}
	if len(fires(h, tok)) != 0 || strings.Contains(do(h, "GET", "/api/v1/me/alerts", tok, "").Body.String(), "gamma") {
		t.Fatal("preview must not save a rule or fire anything")
	}
	if rec := do(h, "POST", "/api/v1/me/alerts/preview", tok, `{"name":"p","scope":{"kind":"weird","values":["x"]}}`); rec.Code < 400 {
		t.Fatal("invalid drafts must be rejected")
	}
}

func TestChannelCreationRules(t *testing.T) {
	_, h := newServer(t, nil)
	alice, bob := register(t, h, "a@example.com"), register(t, h, "b@example.com")
	for name, body := range map[string]string{
		"http":        `{"type":"webhook","name":"x","url":"http://hooks.example.com/x"}`,
		"localhost":   `{"type":"webhook","name":"x","url":"https://localhost/x"}`,
		"metadata ip": `{"type":"webhook","name":"x","url":"https://169.254.169.254/x"}`,
		"private ip":  `{"type":"webhook","name":"x","url":"https://10.0.0.5/x"}`,
		"odd port":    `{"type":"webhook","name":"x","url":"https://example.com:8443/x"}`,
		"credentials": `{"type":"webhook","name":"x","url":"https://u:p@example.com/x"}`,
		"blank name":  `{"type":"webhook","name":"   ","url":"https://example.com/x"}`,
		"other type":  `{"type":"sms","name":"x","url":"https://example.com/x"}`,
	} {
		if rec := do(h, "POST", "/api/v1/me/channels", alice, body); rec.Code < 400 || rec.Code >= 500 {
			t.Errorf("%s: got %d (%s)", name, rec.Code, rec.Body)
		}
	}
	id, secret := newChan(t, h, alice, "first")
	if !strings.HasPrefix(secret, "whsec_") {
		t.Fatalf("secret must be returned at creation: %q", secret)
	}
	list := do(h, "GET", "/api/v1/me/channels", alice, "")
	if !strings.Contains(list.Body.String(), id) || strings.Contains(list.Body.String(), "whsec_") || strings.Contains(list.Body.String(), secret) {
		t.Fatalf("list must show channels but never secrets: %s", list.Body)
	}
	if strings.Contains(do(h, "GET", "/api/v1/me/channels", bob, "").Body.String(), id) {
		t.Fatal("channels leak across users")
	}
	if rec := do(h, "DELETE", "/api/v1/me/channels/"+id, bob, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("another user deleting my channel: %d", rec.Code)
	}
	for i := 2; i <= alert.MaxChannelsPerUser; i++ {
		newChan(t, h, alice, fmt.Sprintf("c%d", i))
	}
	if rec := do(h, "POST", "/api/v1/me/channels", alice, `{"type":"webhook","name":"extra","url":"https://hooks.example.com/tp"}`); rec.Code != http.StatusConflict {
		t.Fatalf("channel limit: %d", rec.Code)
	}
}

func TestChannelSecretRotationEnableAndTest(t *testing.T) {
	s, h := newServer(t, nil)
	alice, bob := register(t, h, "a@example.com"), register(t, h, "b@example.com")
	id, old := newChan(t, h, alice, "n8n")
	rec := do(h, "POST", "/api/v1/me/channels/"+id+"/rotate-secret", alice, "")
	var v struct {
		Secret string `json:"secret"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &v)
	if rec.Code != http.StatusOK || v.Secret == "" || v.Secret == old {
		t.Fatalf("rotate: %d %s", rec.Code, rec.Body)
	}
	if rec := do(h, "POST", "/api/v1/me/channels/"+id+"/rotate-secret", bob, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("another user rotating: %d", rec.Code)
	}
	if rec := do(h, "POST", "/api/v1/me/channels/"+id+"/test", alice, ""); rec.Code != http.StatusOK {
		t.Fatalf("test: %d", rec.Code)
	}
	if rec := do(h, "POST", "/api/v1/me/channels/"+id+"/test", bob, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("another user testing: %d", rec.Code)
	}
	uid := userOf(t, s, alice)
	lastOut.mu.Lock()
	if len(lastOut.tests) != 1 || lastOut.tests[0] != uid+"|"+id {
		t.Fatalf("test delivery not queued for the owner only: %v", lastOut.tests)
	}
	lastOut.mu.Unlock()
	_ = s.Channels.SetEnabled(context.Background(), uid, id, false, "too many failures")
	if rec := do(h, "POST", "/api/v1/me/channels/"+id+"/test", alice, ""); rec.Code != http.StatusConflict {
		t.Fatalf("disabled channel test: %d, want 409", rec.Code)
	}
	if rec := do(h, "POST", "/api/v1/me/channels/"+id+"/enable", alice, ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"enabled":true`) {
		t.Fatalf("enable: %d %s", rec.Code, rec.Body)
	}
}

func TestDeleteChannelDetachesItFromRules(t *testing.T) {
	_, h := newServer(t, nil)
	tok := register(t, h, "detach@example.com")
	chID, _ := newChan(t, h, tok, "temp")
	ruleID := newRule(t, h, tok, `{"name":"r","scope":{"kind":"entity","values":["x"]},"channels":["`+chID+`"],"enabled":true}`)
	if rec := do(h, "DELETE", "/api/v1/me/channels/"+chID, tok, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", rec.Code)
	}
	g := do(h, "GET", "/api/v1/me/alerts/"+ruleID, tok, "")
	if !strings.Contains(g.Body.String(), `"channels":["in_app"]`) || strings.Contains(g.Body.String(), chID) {
		t.Fatalf("rule must fall back to in_app: %s", g.Body)
	}
}

func TestChannelsNeedSigningKey(t *testing.T) {
	_, h := newServer(t, func(c *config.Config) { c.WebhookSigningKey = "" })
	tok := register(t, h, "nokey@example.com")
	if rec := do(h, "POST", "/api/v1/me/channels", tok, `{"type":"webhook","name":"x","url":"https://hooks.example.com/tp"}`); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("without a signing key: %d", rec.Code)
	}
}

func TestDeliveriesAreVisibleOnlyToTheOwner(t *testing.T) {
	s, h := newServer(t, nil)
	alice, bob := register(t, h, "a@example.com"), register(t, h, "b@example.com")
	now := time.Now().UTC()
	ctx := context.Background()
	_ = s.Deliveries.Add(ctx, alert.Delivery{ID: "alice-delivery", ChannelID: "c", UserID: userOf(t, s, alice), FireID: "f", At: now, OK: true, Status: 200, Attempts: 1})
	_ = s.Deliveries.Add(ctx, alert.Delivery{ID: "bob-delivery", ChannelID: "c", UserID: userOf(t, s, bob), FireID: "f", At: now, Attempts: 4, Error: "boom"})
	a := do(h, "GET", "/api/v1/me/channels/deliveries", alice, "").Body.String()
	if !strings.Contains(a, "alice-delivery") || strings.Contains(a, "bob-delivery") || strings.Contains(a, "boom") {
		t.Fatalf("deliveries must be scoped to their owner: %s", a)
	}
}
