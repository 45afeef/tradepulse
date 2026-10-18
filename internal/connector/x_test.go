package connector

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"tradepulse/internal/domain/sourcedef"
	"tradepulse/internal/safefetch"
	"tradepulse/internal/storage/memory"
)

const xToken = "super-secret-token"

func xSetup(budget float64) (*X, *fakeHTTP, *memory.UsageRepo) {
	fh := &fakeHTTP{pages: map[string]*safefetch.Response{}, prefixes: map[string]*safefetch.Response{}}
	u := memory.NewUsageRepo()
	return NewX(fh, XSettings{Token: xToken, BaseURL: "https://api.x.test", BudgetUSD: budget, CostPerResourceUSD: 0.005, MaxPosts: 20}, u), fh, u
}

func xSource(cursor string) sourcedef.Source {
	return sourcedef.Source{Name: "modi", Type: sourcedef.TypeX, X: &sourcedef.XConfig{Handle: "narendramodi", UserID: "42"}, State: sourcedef.State{Cursor: cursor}}
}

const tweetsJSON = `{"data":[{"id":"200","text":"Growth &amp; jobs &lt;3\n  today","created_at":"2026-10-17T10:00:00.000Z"},
{"id":"199","text":"  ","created_at":"2026-10-17T09:00:00.000Z"},{"id":"198","text":"Older post","created_at":"2026-10-17T08:00:00.000Z"}],
"meta":{"newest_id":"200","oldest_id":"198","result_count":3}}`

func TestXFetchParsesPostsAndUsesCursor(t *testing.T) {
	x, fh, usage := xSetup(1)
	fh.prefixes["https://api.x.test/2/users/42/tweets"] = page(200, tweetsJSON)
	res, err := x.Fetch(context.Background(), xSource("100"))
	if err != nil || len(res.Items) != 2 || res.Cursor != "200" {
		t.Fatalf("%+v %v", res, err)
	}
	it := res.Items[0]
	if it.Title != "Growth & jobs <3 today" || it.URL != "https://x.com/narendramodi/status/200" || it.Key != "x:200" ||
		len(it.Entities) != 1 || it.Entities[0] != "narendramodi" || it.PublishedAt.IsZero() {
		t.Fatalf("item: %+v", it)
	}
	u := fh.calls[0]
	for _, want := range []string{"since_id=100", "max_results=20", "exclude=retweets%2Creplies", "tweet.fields=created_at"} {
		if !strings.Contains(u, want) {
			t.Errorf("request %s missing %s", u, want)
		}
	}
	if got := fh.hdrs[0].Get("Authorization"); got != "Bearer "+xToken {
		t.Fatalf("auth header: %q", got)
	}
	if spent, _ := usage.Month(context.Background(), "x", time.Now()); spent != 3*5000 {
		t.Fatalf("spend recorded = %d micro-USD, want 15000 (3 posts returned)", spent)
	}
	x2, fh2, _ := xSetup(1)
	fh2.prefixes["https://api.x.test/2/users/42/tweets"] = page(200, `{"meta":{"result_count":0}}`)
	if res, err := x2.Fetch(context.Background(), xSource("")); err != nil || len(res.Items) != 0 || res.Cursor != "" || strings.Contains(fh2.calls[0], "since_id") {
		t.Fatalf("empty page / first run: %+v %v %s", res, err, fh2.calls[0])
	}
}

func TestXBudgetIsEnforcedBeforeEveryCall(t *testing.T) {
	for _, c := range []struct {
		budget float64
		wantN  string
		denied bool
	}{{0, "", true}, {0.02, "", true}, {0.06, "max_results=12", false}, {10, "max_results=20", false}} {
		x, fh, _ := xSetup(c.budget)
		fh.prefixes["https://api.x.test/2/users/42/tweets"] = page(200, `{"meta":{"result_count":0}}`)
		_, err := x.Fetch(context.Background(), xSource(""))
		if c.denied {
			if !errors.Is(err, ErrXBudget) || len(fh.calls) != 0 {
				t.Errorf("budget %.2f: want ErrXBudget without calling X, got %v (%d calls)", c.budget, err, len(fh.calls))
			}
			continue
		}
		if err != nil || !strings.Contains(fh.calls[0], c.wantN) {
			t.Errorf("budget %.2f: %v %v", c.budget, err, fh.calls)
		}
	}
}

func TestXSpendAccumulatesAndStopsAtCap(t *testing.T) {
	x, fh, usage := xSetup(0.04) // $0.04 = 8 posts at $0.005
	three := `{"data":[{"id":"3","text":"a"},{"id":"2","text":"b"},{"id":"1","text":"c"}],"meta":{"newest_id":"3","result_count":3}}`
	fh.prefixes["https://api.x.test/2/users/42/tweets"] = page(200, three)
	if _, err := x.Fetch(context.Background(), xSource("")); err != nil { // 8 affordable -> max 8, buys 3
		t.Fatal(err)
	}
	if _, err := x.Fetch(context.Background(), xSource("3")); err != nil { // 5 affordable -> max 5, buys 3
		t.Fatalf("second call: %v", err)
	}
	if !strings.Contains(fh.calls[1], "max_results=5") {
		t.Fatalf("page size must shrink to the remaining budget: %s", fh.calls[1])
	}
	if _, err := x.Fetch(context.Background(), xSource("3")); !errors.Is(err, ErrXBudget) { // 2 left < 5
		t.Fatalf("third call must be refused, got %v", err)
	}
	if len(fh.calls) != 2 {
		t.Fatalf("refused call must not reach X: %d calls", len(fh.calls))
	}
	if spent, _ := usage.Month(context.Background(), "x", time.Now()); spent != 6*5000 {
		t.Fatalf("spent %d", spent)
	}
}

func TestXErrorsAreSafeAndNeverLeakTheToken(t *testing.T) {
	for status, want := range map[int]string{401: "rejected", 403: "rejected", 402: "credits", 429: "rate limit", 500: "unexpected"} {
		x, fh, _ := xSetup(1)
		fh.prefixes["https://api.x.test/"] = page(status, `{"detail":"`+xToken+`"}`)
		_, err := x.Fetch(context.Background(), xSource(""))
		if err == nil || !strings.Contains(err.Error(), want) || strings.Contains(err.Error(), xToken) {
			t.Errorf("status %d: %v", status, err)
		}
	}
	x, _, _ := xSetup(1)
	x.http = errHTTP{errors.New("dial failed using Bearer " + xToken)}
	if _, err := x.Fetch(context.Background(), xSource("")); err == nil || strings.Contains(err.Error(), xToken) {
		t.Fatalf("token leaked through a transport error: %v", err)
	}
}

type errHTTP struct{ err error }

func (e errHTTP) Get(context.Context, string, http.Header) (*safefetch.Response, error) { return nil, e.err }

func TestXResolveLooksUpUserAndChargesOneRead(t *testing.T) {
	x, fh, usage := xSetup(1)
	fh.prefixes["https://api.x.test/2/users/by/username/narendramodi"] = page(200, `{"data":{"id":"123","username":"narendramodi"}}`)
	src := sourcedef.Source{Type: sourcedef.TypeX, X: &sourcedef.XConfig{Handle: "narendramodi"}}
	out, err := x.Resolve(context.Background(), src)
	if err != nil || out.X.UserID != "123" || src.X.UserID != "" {
		t.Fatalf("%+v %v (input must not be mutated)", out.X, err)
	}
	if spent, _ := usage.Month(context.Background(), "x", time.Now()); spent != 5000 {
		t.Fatalf("lookup must be charged as one resource, got %d", spent)
	}
	fh.prefixes["https://api.x.test/2/users/by/username/ghost"] = page(200, `{"errors":[{"title":"Not Found Error"}]}`)
	if _, err := x.Resolve(context.Background(), sourcedef.Source{Type: sourcedef.TypeX, X: &sourcedef.XConfig{Handle: "ghost"}}); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unknown account: %v", err)
	}
	broke, fh2, _ := xSetup(0)
	if _, err := broke.Resolve(context.Background(), src); !errors.Is(err, ErrXBudget) || len(fh2.calls) != 0 {
		t.Fatalf("no budget means no lookup: %v", err)
	}
}

func TestXValidateAndUnresolvedSource(t *testing.T) {
	x, _, _ := xSetup(1)
	if err := x.Validate(xSource("")); err != nil {
		t.Fatal(err)
	}
	if err := x.Validate(sourcedef.Source{Type: sourcedef.TypeX}); err == nil {
		t.Fatal("missing handle accepted")
	}
	noTok := NewX(&fakeHTTP{}, XSettings{BudgetUSD: 1}, memory.NewUsageRepo())
	if err := noTok.Validate(xSource("")); err == nil || !strings.Contains(err.Error(), "X_BEARER_TOKEN") {
		t.Fatalf("missing token: %v", err)
	}
	src := xSource("")
	src.X.UserID = ""
	if _, err := x.Fetch(context.Background(), src); err == nil {
		t.Fatal("unresolved source must not be fetched")
	}
}
