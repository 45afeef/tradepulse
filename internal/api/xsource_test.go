package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"tradepulse/internal/config"
	"tradepulse/internal/safefetch"
)

func xUser(id string) *safefetch.Response {
	return &safefetch.Response{Status: 200, Header: http.Header{}, Body: []byte(`{"data":{"id":"` + id + `","username":"x"}}`)}
}

func xBody(name, handle string) string {
	return `{"name":"` + name + `","type":"x","interval_seconds":300,"x":{"handle":"` + handle + `","user_id":"999"}}`
}

func TestXSourceNeedsServerConfiguration(t *testing.T) {
	_, h := newServer(t, func(c *config.Config) { c.XBearerToken = "" })
	_, admin := adminToken(t)
	rec := do(h, "POST", "/api/v1/admin/sources", admin, xBody("modi", "narendramodi"))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "configured") {
		t.Fatalf("X without a token must be refused with a clear message: %d %s", rec.Code, rec.Body)
	}
}

func TestCreateXSourceResolvesUserAndIgnoresClientUserID(t *testing.T) {
	_, h := newServer(t, nil)
	_, admin := adminToken(t)
	lastNet.prefixes["https://api.x.test/2/users/by/username/NarendraModi"] = xUser("123")
	id := createSrc(t, h, admin, xBody("modi", "@NarendraModi"))
	rec := do(h, "GET", "/api/v1/admin/sources/"+id, admin, "")
	if !strings.Contains(rec.Body.String(), `"user_id":"123"`) || strings.Contains(rec.Body.String(), "999") || !strings.Contains(rec.Body.String(), `"handle":"NarendraModi"`) {
		t.Fatalf("user id must come from X, not the client: %s", rec.Body)
	}
	usage := do(h, "GET", "/api/v1/admin/usage", admin, "")
	var u struct {
		XConfigured bool    `json:"x_configured"`
		XSpentUSD   float64 `json:"x_spent_usd"`
		XBudgetUSD  float64 `json:"x_budget_usd"`
	}
	_ = json.Unmarshal(usage.Body.Bytes(), &u)
	if !u.XConfigured || u.XSpentUSD != 0.005 || u.XBudgetUSD != 1 {
		t.Fatalf("usage: %s", usage.Body)
	}
}

func TestXSourceValidation(t *testing.T) {
	_, h := newServer(t, nil)
	_, admin := adminToken(t)
	lastNet.prefixes["https://api.x.test/2/users/by/username/ghost"] = &safefetch.Response{Status: 200, Header: http.Header{}, Body: []byte(`{"errors":[{"title":"Not Found Error"}]}`)}
	for name, body := range map[string]string{
		"bad handle":        xBody("x1", "not valid!"),
		"handle too long":   xBody("x2", "abcdefghijklmnop"),
		"no x block":        `{"name":"x3","type":"x","interval_seconds":300}`,
		"interval too low":  `{"name":"x4","type":"x","interval_seconds":5,"x":{"handle":"abc"}}`,
		"unknown account":   xBody("x5", "ghost"),
		"url on x source":   `{"name":"x6","type":"x","url":"http://127.0.0.1/","interval_seconds":300,"x":{"handle":"ghost"}}`,
	} {
		if rec := do(h, "POST", "/api/v1/admin/sources", admin, body); rec.Code < 400 || rec.Code >= 500 {
			t.Errorf("%s: got %d (%s)", name, rec.Code, rec.Body)
		}
	}
}

func TestXUpdateKeepsResolvedIDForSameHandleAndReResolvesOnChange(t *testing.T) {
	_, h := newServer(t, nil)
	_, admin := adminToken(t)
	lastNet.prefixes["https://api.x.test/2/users/by/username/elonmusk"] = xUser("44")
	lastNet.prefixes["https://api.x.test/2/users/by/username/narendramodi"] = xUser("55")
	id := createSrc(t, h, admin, xBody("watch", "elonmusk"))
	before := len(lastNet.urls)
	if rec := do(h, "PUT", "/api/v1/admin/sources/"+id, admin, `{"enabled":true,"interval_seconds":600,"x":{"handle":"@ElonMusk"}}`); rec.Code != http.StatusOK {
		t.Fatalf("update: %d %s", rec.Code, rec.Body)
	}
	if len(lastNet.urls) != before {
		t.Fatal("updating with the same handle must not trigger another (billed) lookup")
	}
	rec := do(h, "PUT", "/api/v1/admin/sources/"+id, admin, `{"enabled":true,"interval_seconds":600,"x":{"handle":"narendramodi"}}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"user_id":"55"`) || len(lastNet.urls) != before+1 {
		t.Fatalf("changing the handle must re-resolve: %d %s", rec.Code, rec.Body)
	}
}

func TestXDraftTestShowsPostsWithinBudget(t *testing.T) {
	_, h := newServer(t, nil)
	_, admin := adminToken(t)
	lastNet.prefixes["https://api.x.test/2/users/by/username/elonmusk"] = xUser("44")
	lastNet.prefixes["https://api.x.test/2/users/44/tweets"] = &safefetch.Response{Status: 200, Header: http.Header{},
		Body: []byte(`{"data":[{"id":"9","text":"Hello &amp; welcome","created_at":"2026-10-17T10:00:00.000Z"}],"meta":{"newest_id":"9","result_count":1}}`)}
	rec := do(h, "POST", "/api/v1/admin/sources/test", admin, xBody("draftx", "elonmusk"))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ok":true`) || !strings.Contains(rec.Body.String(), "Hello & welcome") {
		t.Fatalf("draft test: %d %s", rec.Code, rec.Body)
	}
}

func TestUsageEndpointIsAdminOnly(t *testing.T) {
	_, h := newServer(t, nil)
	member := register(t, h, "usage@example.com")
	if rec := do(h, "GET", "/api/v1/admin/usage", member, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("member: %d", rec.Code)
	}
	if rec := do(h, "GET", "/api/v1/admin/usage", "", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: %d", rec.Code)
	}
}
