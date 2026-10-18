package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"tradepulse/internal/config"
	"tradepulse/internal/domain/event"
	"tradepulse/internal/webhook"
)

func mkHook(t *testing.T, h http.Handler, admin, name, auth string) (id, secret string) {
	t.Helper()
	body := `{"name":"` + name + `","type":"webhook","enabled":true,"entities":["alerts"],"webhook":{"auth":"` + auth + `"}}`
	rec := do(h, "POST", "/api/v1/admin/sources", admin, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create webhook: %d %s", rec.Code, rec.Body)
	}
	var v struct{ ID, WebhookSecret, WebhookPath string }
	_ = json.Unmarshal(rec.Body.Bytes(), &v)
	if !strings.HasPrefix(v.WebhookSecret, webhook.SecretPrefix) || v.WebhookPath != "/api/v1/hooks/"+name {
		t.Fatalf("secret/path missing in create response: %s", rec.Body)
	}
	return v.ID, v.WebhookSecret
}

func postHook(h http.Handler, name, body string, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/api/v1/hooks/"+name, strings.NewReader(body))
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func signed(secret, body string, at time.Time) map[string]string {
	ts := strconv.FormatInt(at.Unix(), 10)
	return map[string]string{"X-TP-Timestamp": ts, "X-TP-Signature": webhook.Sign(secret, ts, []byte(body))}
}

type collector struct {
	mu sync.Mutex
	ev []event.Event
}

func collect(s *Server) *collector {
	c := &collector{}
	s.Bus.Subscribe("hooks-test", func(_ context.Context, e event.Event) { c.mu.Lock(); c.ev = append(c.ev, e); c.mu.Unlock() })
	return c
}

func (c *collector) wait(n int) []event.Event {
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		l := len(c.ev)
		c.mu.Unlock()
		if l >= n {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(60 * time.Millisecond)
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]event.Event(nil), c.ev...)
}

func TestWebhookAcceptsSignedPayloadAndDeduplicates(t *testing.T) {
	s, h := newServer(t, nil)
	_, admin := adminToken(t)
	_, secret := mkHook(t, h, admin, "n8n-alerts", "hmac")
	c := collect(s)

	body := `{"id":"evt-1","title":"  Disk   usage high ","summary":"92%","url":"https://example.com/a","entities":["Infra"],"published_at":"2026-10-17T10:00:00Z"}`
	rec := postHook(h, "n8n-alerts", body, signed(secret, body, time.Now()))
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), "accepted") {
		t.Fatalf("first delivery: %d %s", rec.Code, rec.Body)
	}
	again := postHook(h, "n8n-alerts", body, signed(secret, body, time.Now()))
	if again.Code != http.StatusOK || !strings.Contains(again.Body.String(), "duplicate") {
		t.Fatalf("retry must be an idempotent duplicate: %d %s", again.Code, again.Body)
	}
	noID := `{"title":"No id here","url":"https://example.com/b"}`
	postHook(h, "n8n-alerts", noID, signed(secret, noID, time.Now()))
	if dup := postHook(h, "n8n-alerts", noID, signed(secret, noID, time.Now())); dup.Code != http.StatusOK {
		t.Fatalf("identical payload without id must dedupe: %d", dup.Code)
	}
	ev := c.wait(2)
	if len(ev) != 2 || ev[0].Title != "Disk usage high" || ev[0].Source != "n8n-alerts" || ev[0].Type != "content_item" ||
		len(ev[0].Entities) != 2 || ev[0].Entities[0] != "alerts" || ev[0].Entities[1] != "infra" || ev[0].URL != "https://example.com/a" {
		t.Fatalf("unexpected events: %+v", ev)
	}
}

func TestWebhookRejectsBadCredentialsIdentically(t *testing.T) {
	s, h := newServer(t, nil)
	_, admin := adminToken(t)
	_, secret := mkHook(t, h, admin, "strict-hook", "hmac")
	offID, _ := mkHook(t, h, admin, "off-hook", "hmac")
	do(h, "PUT", "/api/v1/admin/sources/"+offID, admin, `{"enabled":false,"webhook":{"auth":"hmac"}}`)
	c := collect(s)
	body := `{"title":"x"}`
	good := signed(secret, body, time.Now())
	cases := map[string]*httptest.ResponseRecorder{
		"wrong signature":     postHook(h, "strict-hook", body, map[string]string{"X-TP-Timestamp": good["X-TP-Timestamp"], "X-TP-Signature": "sha256=" + strings.Repeat("0", 64)}),
		"stale timestamp":     postHook(h, "strict-hook", body, signed(secret, body, time.Now().Add(-10*time.Minute))),
		"future timestamp":    postHook(h, "strict-hook", body, signed(secret, body, time.Now().Add(10*time.Minute))),
		"no headers":          postHook(h, "strict-hook", body, nil),
		"signed other body":   postHook(h, "strict-hook", `{"title":"y"}`, good),
		"token to hmac":       postHook(h, "strict-hook", body, map[string]string{"X-Webhook-Token": secret}),
		"bearer to hmac":      postHook(h, "strict-hook", body, map[string]string{"Authorization": "Bearer " + secret}),
		"unknown source":      postHook(h, "no-such-hook", body, good),
		"disabled source":     postHook(h, "off-hook", body, good),
		"wrong secret":        postHook(h, "strict-hook", body, signed("whsec_wrong", body, time.Now())),
	}
	var first string
	for name, rec := range cases {
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: got %d, want 401", name, rec.Code)
		}
		if first == "" {
			first = rec.Body.String()
		} else if rec.Body.String() != first {
			t.Errorf("%s: response differs and could leak which check failed: %s", name, rec.Body)
		}
	}
	if ev := c.wait(1); len(ev) != 0 {
		t.Fatalf("unauthenticated requests published %d events", len(ev))
	}
}

func TestWebhookTokenMode(t *testing.T) {
	s, h := newServer(t, nil)
	_, admin := adminToken(t)
	_, secret := mkHook(t, h, admin, "zapier-hook", "token")
	c := collect(s)
	b1, b2 := `{"title":"via header"}`, `{"title":"via bearer"}`
	if rec := postHook(h, "zapier-hook", b1, map[string]string{"X-Webhook-Token": secret}); rec.Code != http.StatusCreated {
		t.Fatalf("header token: %d", rec.Code)
	}
	if rec := postHook(h, "zapier-hook", b2, map[string]string{"Authorization": "Bearer " + secret}); rec.Code != http.StatusCreated {
		t.Fatalf("bearer token: %d", rec.Code)
	}
	if rec := postHook(h, "zapier-hook", `{"title":"bad"}`, map[string]string{"X-Webhook-Token": secret + "x"}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token: %d", rec.Code)
	}
	if rec := postHook(h, "zapier-hook", `{"title":"hmac"}`, signed(secret, `{"title":"hmac"}`, time.Now())); rec.Code != http.StatusUnauthorized {
		t.Fatalf("signature must not be accepted in token mode: %d", rec.Code)
	}
	if ev := c.wait(2); len(ev) != 2 {
		t.Fatalf("events: %d", len(ev))
	}
}

func TestWebhookPayloadValidation(t *testing.T) {
	_, h := newServer(t, nil)
	_, admin := adminToken(t)
	_, secret := mkHook(t, h, admin, "valid-hook", "hmac")
	for name, body := range map[string]string{
		"not json":        `nope`,
		"unknown field":   `{"title":"t","bogus":1}`,
		"missing title":   `{"summary":"s"}`,
		"blank title":     `{"title":"   "}`,
		"title too long":  `{"title":"` + strings.Repeat("a", 301) + `"}`,
		"javascript link": `{"title":"t","url":"javascript:alert(1)"}`,
		"credential link": `{"title":"t","url":"https://u:p@example.com/"}`,
		"bad date":        `{"title":"t","published_at":"yesterday"}`,
		"too many topics": `{"title":"t","entities":["1","2","3","4","5","6","7","8","9","10","11"]}`,
	} {
		if rec := postHook(h, "valid-hook", body, signed(secret, body, time.Now())); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: got %d, want 422 (%s)", name, rec.Code, rec.Body)
		}
	}
}

func TestWebhookBodyLimitAndRateLimit(t *testing.T) {
	_, h := newServer(t, func(c *config.Config) { c.WebhookRatePerMinute = 3 })
	_, admin := adminToken(t)
	_, secret := mkHook(t, h, admin, "limited-hook", "hmac")
	big := `{"title":"` + strings.Repeat("a", 70<<10) + `"}`
	if rec := postHook(h, "limited-hook", big, signed(secret, big, time.Now())); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized: %d", rec.Code)
	}
	limited := false
	for i := 0; i < 6; i++ {
		body := `{"title":"item ` + strconv.Itoa(i) + `"}`
		if rec := postHook(h, "limited-hook", body, signed(secret, body, time.Now())); rec.Code == http.StatusTooManyRequests {
			limited = true
		}
	}
	if !limited {
		t.Fatal("per-source rate limit not enforced")
	}
}

func TestWebhookSecretNeverExposedAndRotationInvalidatesOld(t *testing.T) {
	s, h := newServer(t, nil)
	_, admin := adminToken(t)
	id, old := mkHook(t, h, admin, "rot-hook", "hmac")
	for _, p := range []string{"/api/v1/admin/sources", "/api/v1/admin/sources/" + id, "/api/v1/sources", "/api/v1/admin/audit"} {
		tok := admin
		if p == "/api/v1/sources" {
			tok = register(t, h, "viewer@example.com")
		}
		if rec := do(h, "GET", p, tok, ""); strings.Contains(rec.Body.String(), "whsec_") || strings.Contains(rec.Body.String(), old) {
			t.Fatalf("secret leaked by GET %s", p)
		}
	}
	rec := do(h, "POST", "/api/v1/admin/sources/"+id+"/webhook-secret", admin, "")
	var v struct{ WebhookSecret string }
	_ = json.Unmarshal(rec.Body.Bytes(), &v)
	if rec.Code != http.StatusOK || v.WebhookSecret == "" || v.WebhookSecret == old {
		t.Fatalf("rotate: %d %s", rec.Code, rec.Body)
	}
	body := `{"title":"after rotation"}`
	if r := postHook(h, "rot-hook", body, signed(old, body, time.Now())); r.Code != http.StatusUnauthorized {
		t.Fatalf("old secret must stop working: %d", r.Code)
	}
	if r := postHook(h, "rot-hook", body, signed(v.WebhookSecret, body, time.Now())); r.Code != http.StatusCreated {
		t.Fatalf("new secret must work: %d", r.Code)
	}
	if audit := do(h, "GET", "/api/v1/admin/audit", admin, ""); !strings.Contains(audit.Body.String(), "source.rotate_secret") || strings.Contains(audit.Body.String(), v.WebhookSecret) {
		t.Fatal("rotation must be audited without the secret")
	}
	rssID := createSrc(t, h, admin, rssBody("not-a-hook"))
	if r := do(h, "POST", "/api/v1/admin/sources/"+rssID+"/webhook-secret", admin, ""); r.Code != http.StatusConflict {
		t.Fatalf("rotate on a non-webhook source: %d", r.Code)
	}
	member := register(t, h, "member2@example.com")
	if r := do(h, "POST", "/api/v1/admin/sources/"+id+"/webhook-secret", member, ""); r.Code != http.StatusForbidden {
		t.Fatalf("members cannot rotate: %d", r.Code)
	}
	_ = s
}

func TestWebhooksDisabledWithoutSigningKey(t *testing.T) {
	_, h := newServer(t, func(c *config.Config) { c.WebhookSigningKey = "" })
	_, admin := adminToken(t)
	if rec := do(h, "POST", "/api/v1/admin/sources", admin, `{"name":"nokey","type":"webhook","enabled":true}`); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("creating a webhook source without a key: %d", rec.Code)
	}
	if rec := postHook(h, "anything", `{"title":"x"}`, nil); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("delivery without a key: %d", rec.Code)
	}
}

func TestWebhookUpdatesSourceStatsAndIsNotPolled(t *testing.T) {
	_, h := newServer(t, nil)
	_, admin := adminToken(t)
	id, secret := mkHook(t, h, admin, "stats-hook", "hmac")
	body := `{"title":"counted"}`
	postHook(h, "stats-hook", body, signed(secret, body, time.Now()))
	rec := do(h, "GET", "/api/v1/admin/sources/"+id, admin, "")
	if !strings.Contains(rec.Body.String(), `"items_total":1`) || !strings.Contains(rec.Body.String(), "last_item_at") || !strings.Contains(rec.Body.String(), `"health":"ok"`) {
		t.Fatalf("stats: %s", rec.Body)
	}
	if r := do(h, "POST", "/api/v1/admin/sources/"+id+"/run", admin, ""); r.Code != http.StatusConflict {
		t.Fatalf("webhook sources cannot be run: %d", r.Code)
	}
	if r := do(h, "POST", "/api/v1/admin/sources/test", admin, `{"name":"x1","type":"webhook","enabled":true}`); r.Code != http.StatusUnprocessableEntity {
		t.Fatalf("webhook sources have nothing to test-fetch: %d", r.Code)
	}
}
