package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"tradepulse/internal/domain/event"
	"tradepulse/internal/safefetch"
)

type fakeNet struct {
	pages map[string]*safefetch.Response
	errs  map[string]error
}

var lastNet *fakeNet

func (f *fakeNet) Get(_ context.Context, u string, _ http.Header) (*safefetch.Response, error) {
	if e, ok := f.errs[u]; ok {
		return nil, e
	}
	if r, ok := f.pages[u]; ok {
		cp := *r
		cp.FinalURL = u
		return &cp, nil
	}
	return &safefetch.Response{Status: 404, FinalURL: u}, nil
}

const feedXML = `<rss version="2.0"><channel><item><title>Markets rally</title><link>https://example.com/a</link><guid>1</guid><description>&lt;b&gt;Up&lt;/b&gt;</description></item></channel></rss>`

func rssBody(name string) string {
	return `{"name":"` + name + `","type":"rss","url":"https://example.com/feed.xml","interval_seconds":300,"entities":["Markets"]}`
}

func createSrc(t *testing.T, h http.Handler, admin, body string) (id string) {
	t.Helper()
	rec := do(h, "POST", "/api/v1/admin/sources", admin, body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create source: %d %s", rec.Code, rec.Body)
	}
	var v struct{ ID string }
	_ = json.Unmarshal(rec.Body.Bytes(), &v)
	return v.ID
}

func TestSourceAdminRoutesRequireAdmin(t *testing.T) {
	_, h := newServer(t, nil)
	member := register(t, h, "m@example.com")
	routes := []struct{ m, p string }{
		{"POST", "/api/v1/admin/sources"}, {"GET", "/api/v1/admin/sources"}, {"GET", "/api/v1/admin/sources/x"},
		{"PUT", "/api/v1/admin/sources/x"}, {"DELETE", "/api/v1/admin/sources/x"}, {"POST", "/api/v1/admin/sources/x/run"},
		{"POST", "/api/v1/admin/sources/test"}, {"POST", "/api/v1/admin/items"},
	}
	for _, r := range routes {
		if rec := do(h, r.m, r.p, member, `{}`); rec.Code != http.StatusForbidden {
			t.Errorf("member %s %s: %d, want 403", r.m, r.p, rec.Code)
		}
		if rec := do(h, r.m, r.p, "", `{}`); rec.Code != http.StatusUnauthorized {
			t.Errorf("anonymous %s %s: %d, want 401", r.m, r.p, rec.Code)
		}
	}
	if rec := do(h, "GET", "/api/v1/sources", member, ""); rec.Code != http.StatusOK {
		t.Fatalf("members may read the catalog: %d", rec.Code)
	}
}

func TestCreateSourceRejectsDangerousAndInvalidInput(t *testing.T) {
	_, h := newServer(t, nil)
	_, admin := adminToken(t)
	bad := map[string]string{
		"metadata ip":      `{"name":"a1","type":"rss","url":"http://169.254.169.254/latest/","interval_seconds":300}`,
		"localhost":        `{"name":"a2","type":"rss","url":"http://localhost/feed","interval_seconds":300}`,
		"private ip":       `{"name":"a3","type":"rss","url":"http://10.0.0.5/feed","interval_seconds":300}`,
		"file scheme":      `{"name":"a4","type":"rss","url":"file:///etc/passwd","interval_seconds":300}`,
		"odd port":         `{"name":"a5","type":"rss","url":"http://example.com:8080/f","interval_seconds":300}`,
		"credentials":      `{"name":"a6","type":"rss","url":"https://u:p@example.com/f","interval_seconds":300}`,
		"interval low":     `{"name":"a7","type":"rss","url":"https://example.com/f","interval_seconds":5}`,
		"interval high":    `{"name":"a8","type":"rss","url":"https://example.com/f","interval_seconds":604800}`,
		"bad name":         `{"name":"Bad Name","type":"rss","url":"https://example.com/f","interval_seconds":300}`,
		"unknown type":     `{"name":"a9","type":"ftp","url":"https://example.com/f","interval_seconds":300}`,
		"web no config":    `{"name":"b1","type":"webmonitor","url":"https://example.com/p","interval_seconds":300}`,
		"bad selector":     `{"name":"b2","type":"webmonitor","url":"https://example.com/p","interval_seconds":300,"web":{"item_selector":"div["}}`,
		"robots no reason": `{"name":"b3","type":"webmonitor","url":"https://example.com/p","interval_seconds":300,"web":{"item_selector":"a","ignore_robots":true}}`,
		"unknown field":    `{"name":"b4","type":"rss","url":"https://example.com/f","interval_seconds":300,"bogus":1}`,
	}
	for name, body := range bad {
		if rec := do(h, "POST", "/api/v1/admin/sources", admin, body); rec.Code < 400 || rec.Code >= 500 {
			t.Errorf("%s: got %d, want 4xx (%s)", name, rec.Code, rec.Body)
		}
	}
}

func TestSourceLifecycleAndNameImmutability(t *testing.T) {
	_, h := newServer(t, nil)
	_, admin := adminToken(t)
	id := createSrc(t, h, admin, rssBody("cnbc-top"))
	if rec := do(h, "POST", "/api/v1/admin/sources", admin, rssBody("cnbc-top")); rec.Code != http.StatusConflict {
		t.Fatalf("duplicate name: %d", rec.Code)
	}
	get := do(h, "GET", "/api/v1/admin/sources/"+id, admin, "")
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), `"enabled":false`) || !strings.Contains(get.Body.String(), `"health":"disabled"`) {
		t.Fatalf("new sources must start disabled: %d %s", get.Code, get.Body)
	}
	upd := do(h, "PUT", "/api/v1/admin/sources/"+id, admin, `{"url":"https://example.com/other.xml","enabled":true,"interval_seconds":600,"entities":["markets","india"]}`)
	if upd.Code != http.StatusOK || !strings.Contains(upd.Body.String(), `"name":"cnbc-top"`) || !strings.Contains(upd.Body.String(), `"health":"pending"`) {
		t.Fatalf("update: %d %s", upd.Code, upd.Body)
	}
	if rec := do(h, "PUT", "/api/v1/admin/sources/"+id, admin, `{"url":"http://127.0.0.1/x","enabled":true,"interval_seconds":600}`); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("update must re-run SSRF checks: %d", rec.Code)
	}
	if rec := do(h, "PUT", "/api/v1/admin/sources/"+id, admin, `{"name":"renamed","url":"https://example.com/x","enabled":true,"interval_seconds":600}`); rec.Code < 400 {
		t.Fatalf("name must not be changeable: %d", rec.Code)
	}
	if rec := do(h, "PUT", "/api/v1/admin/sources/nope", admin, `{"url":"https://example.com/x","enabled":true,"interval_seconds":600}`); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown source: %d", rec.Code)
	}
	if rec := do(h, "DELETE", "/api/v1/admin/sources/"+id, admin, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", rec.Code)
	}
	if rec := do(h, "GET", "/api/v1/admin/sources/"+id, admin, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("deleted source still readable: %d", rec.Code)
	}
}

func TestRunNowRequiresEnabledPollingSource(t *testing.T) {
	_, h := newServer(t, nil)
	_, admin := adminToken(t)
	id := createSrc(t, h, admin, rssBody("poll-me"))
	if rec := do(h, "POST", "/api/v1/admin/sources/"+id+"/run", admin, ""); rec.Code != http.StatusConflict {
		t.Fatalf("disabled source: %d, want 409", rec.Code)
	}
	do(h, "PUT", "/api/v1/admin/sources/"+id, admin, `{"url":"https://example.com/feed.xml","enabled":true,"interval_seconds":300}`)
	rec := do(h, "POST", "/api/v1/admin/sources/"+id+"/run", admin, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "next_run_at") {
		t.Fatalf("run now: %d %s", rec.Code, rec.Body)
	}
}

func TestTestEndpointPreviewsWithoutSaving(t *testing.T) {
	s, h := newServer(t, nil)
	_, admin := adminToken(t)
	lastNet.pages["https://example.com/feed.xml"] = &safefetch.Response{Status: 200, Body: []byte(feedXML), Header: http.Header{}}
	rec := do(h, "POST", "/api/v1/admin/sources/test", admin, rssBody("draft"))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ok":true`) || !strings.Contains(rec.Body.String(), "Markets rally") {
		t.Fatalf("preview: %d %s", rec.Code, rec.Body)
	}
	if list, _ := s.Sources.List(context.Background()); len(list) != 0 {
		t.Fatal("testing a draft must not save it")
	}
	lastNet.errs = map[string]error{"https://example.com/feed.xml": safefetch.ErrBlocked}
	rec = do(h, "POST", "/api/v1/admin/sources/test", admin, rssBody("draft"))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ok":false`) || !strings.Contains(rec.Body.String(), "not allowed") {
		t.Fatalf("fetch failure must be reported in the body: %d %s", rec.Code, rec.Body)
	}
	if rec := do(h, "POST", "/api/v1/admin/sources/test", admin, `{"name":"x1","type":"rss","url":"http://169.254.169.254/","interval_seconds":300}`); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("draft test must also reject SSRF targets: %d", rec.Code)
	}
}

func TestCatalogShowsOnlyEnabledSourcesWithoutURLs(t *testing.T) {
	_, h := newServer(t, nil)
	_, admin := adminToken(t)
	member := register(t, h, "cat@example.com")
	on := createSrc(t, h, admin, rssBody("visible-one"))
	createSrc(t, h, admin, rssBody("hidden-one"))
	do(h, "PUT", "/api/v1/admin/sources/"+on, admin, `{"url":"https://example.com/feed.xml","enabled":true,"interval_seconds":300,"entities":["markets"]}`)
	rec := do(h, "GET", "/api/v1/sources", member, "")
	body := rec.Body.String()
	if !strings.Contains(body, "visible-one") || strings.Contains(body, "hidden-one") {
		t.Fatalf("catalog must list only enabled sources: %s", body)
	}
	if strings.Contains(body, "example.com") {
		t.Fatalf("catalog must not expose source URLs: %s", body)
	}
}

func TestManualPostPublishesAndIsAudited(t *testing.T) {
	s, h := newServer(t, nil)
	_, admin := adminToken(t)
	var mu sync.Mutex
	var got []event.Event
	s.Bus.Subscribe("test", func(_ context.Context, e event.Event) { mu.Lock(); got = append(got, e); mu.Unlock() })

	id := createSrc(t, h, admin, `{"name":"admin-posts","type":"manual","entities":["announcements"]}`)
	post := `{"source":"admin-posts","title":"  Maintenance   tonight ","summary":"Short outage","url":"https://example.com/status","entities":["Infra"]}`
	if rec := do(h, "POST", "/api/v1/admin/items", admin, post); rec.Code != http.StatusConflict {
		t.Fatalf("disabled manual source: %d, want 409", rec.Code)
	}
	do(h, "PUT", "/api/v1/admin/sources/"+id, admin, `{"enabled":true}`)
	rec := do(h, "POST", "/api/v1/admin/items", admin, post)
	if rec.Code != http.StatusCreated {
		t.Fatalf("post: %d %s", rec.Code, rec.Body)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 || got[0].Title != "Maintenance tonight" || got[0].Source != "admin-posts" || got[0].Type != "content_item" ||
		len(got[0].Entities) != 2 || got[0].Entities[1] != "infra" {
		t.Fatalf("unexpected event: %+v", got)
	}
	if audit := do(h, "GET", "/api/v1/admin/audit", admin, ""); !strings.Contains(audit.Body.String(), "item.manual") || !strings.Contains(audit.Body.String(), "source.create") {
		t.Fatalf("audit missing entries: %s", audit.Body)
	}
}

func TestManualPostValidation(t *testing.T) {
	_, h := newServer(t, nil)
	_, admin := adminToken(t)
	rssID := createSrc(t, h, admin, rssBody("a-feed"))
	do(h, "PUT", "/api/v1/admin/sources/"+rssID, admin, `{"url":"https://example.com/feed.xml","enabled":true,"interval_seconds":300}`)
	manID := createSrc(t, h, admin, `{"name":"manual-1","type":"manual"}`)
	do(h, "PUT", "/api/v1/admin/sources/"+manID, admin, `{"enabled":true}`)
	cases := map[string]struct {
		body string
		want int
	}{
		"unknown source":  {`{"source":"nope","title":"t"}`, 404},
		"not manual":      {`{"source":"a-feed","title":"t"}`, 409},
		"missing title":   {`{"source":"manual-1","title":"   "}`, 422},
		"js link":         {`{"source":"manual-1","title":"t","url":"javascript:alert(1)"}`, 422},
		"link credential": {`{"source":"manual-1","title":"t","url":"https://u:p@example.com/"}`, 422},
	}
	for name, c := range cases {
		if rec := do(h, "POST", "/api/v1/admin/items", admin, c.body); rec.Code != c.want {
			t.Errorf("%s: got %d want %d (%s)", name, rec.Code, c.want, rec.Body)
		}
	}
}
