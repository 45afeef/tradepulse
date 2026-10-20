package safefetch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"
)

type fakeResolver map[string][]netip.Addr

func (r fakeResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	if a, ok := r[host]; ok {
		return a, nil
	}
	return nil, errors.New("no such host")
}

// testFetcher allows only 127.0.0.1 (so httptest servers work) and any port.
func testFetcher(o Options) *Fetcher {
	f := New(o)
	f.allow = func(a netip.Addr) bool { return a == netip.MustParseAddr("127.0.0.1") }
	f.allowAnyPort = true
	if o.Timeout <= 0 {
		o.Timeout = 10 * time.Second
	}
	if o.MaxRedirects <= 0 {
		o.MaxRedirects = 3
	}
	f.build(o.Timeout, o.MaxRedirects)
	return f
}

func TestBlockedAddressTable(t *testing.T) {
	blocked := []string{
		"127.0.0.1", "::1", "10.1.2.3", "172.16.0.1", "172.31.255.255", "192.168.1.1", "169.254.169.254",
		"fe80::1", "fc00::1", "fd12:3456::1", "0.0.0.0", "::", "100.64.0.1", "::ffff:127.0.0.1", "::ffff:10.0.0.1",
		"224.0.0.1", "ff02::1", "192.0.2.1", "198.51.100.7", "203.0.113.9", "240.0.0.1", "198.18.0.1", "2001:db8::1",
	}
	for _, s := range blocked {
		if !isBlocked(netip.MustParseAddr(s)) {
			t.Errorf("%s must be blocked", s)
		}
	}
	for _, s := range []string{"8.8.8.8", "1.1.1.1", "93.184.216.34", "2606:4700:4700::1111"} {
		if isBlocked(netip.MustParseAddr(s)) {
			t.Errorf("%s must be allowed", s)
		}
	}
}

func TestRejectsBadURLs(t *testing.T) {
	f := New(Options{})
	cases := map[string]error{
		"file:///etc/passwd":                       ErrScheme,
		"ftp://example.com/x":                      ErrScheme,
		"gopher://example.com":                     ErrScheme,
		"javascript:alert(1)":                      ErrScheme,
		"http://":                                  ErrURL,
		"":                                         ErrURL,
		"http://example.com:8080/":                 ErrPort,
		"http://example.com:22/":                   ErrPort,
		"http://user:pass@example.com/":            ErrURL,
		"http://metadata.google.internal/":         ErrBlocked,
		"http://metadata/computeMetadata/v1/":      ErrBlocked,
		"http://localhost/":                        ErrBlocked,
		"http://anything.internal/":                ErrBlocked,
		"http://METADATA.GOOGLE.INTERNAL./":        ErrBlocked,
		"http://169.254.169.254/latest/meta-data/": ErrBlocked,
		"http://[::1]/":                            ErrBlocked,
		"http://127.0.0.1/":                        ErrBlocked,
		"http://2130706433/":                       ErrBlocked, // decimal form of 127.0.0.1
	}
	for raw, want := range cases {
		_, err := f.Get(context.Background(), raw, nil)
		if err == nil || !errors.Is(err, want) {
			// numeric hosts may surface as a resolve error; that is also a refusal
			if raw == "http://2130706433/" && err != nil {
				continue
			}
			t.Errorf("%q: got %v, want %v", raw, err, want)
		}
	}
}

func TestDNSRebindingToPrivateAddressBlocked(t *testing.T) {
	f := New(Options{Resolver: fakeResolver{
		"evil.example":  {netip.MustParseAddr("10.0.0.5")},
		"meta.example":  {netip.MustParseAddr("169.254.169.254")},
		"mixed.example": {netip.MustParseAddr("192.168.1.1"), netip.MustParseAddr("::1")},
	}})
	for _, h := range []string{"evil.example", "meta.example", "mixed.example"} {
		if _, err := f.Get(context.Background(), "http://"+h+"/", nil); !errors.Is(err, ErrBlocked) {
			t.Errorf("%s: got %v, want ErrBlocked", h, err)
		}
	}
}

func TestPickAllowedDropsPrivateAnswers(t *testing.T) {
	f := New(Options{})
	got := f.pickAllowed([]netip.Addr{netip.MustParseAddr("10.0.0.1"), netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("::ffff:8.8.4.4")})
	if len(got) != 2 || got[0].String() != "8.8.8.8" || got[1].String() != "8.8.4.4" {
		t.Fatalf("unexpected: %v", got)
	}
}

func TestSuccessfulFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" || r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			http.Error(w, "unexpected headers", 400)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()
	resp, err := testFetcher(Options{}).Get(context.Background(), srv.URL, nil)
	if err != nil || resp.Status != 200 || string(resp.Body) != "hello" || resp.Header.Get("ETag") != `"v1"` || resp.FinalURL != srv.URL+"" {
		t.Fatalf("unexpected: %+v %v", resp, err)
	}
}

func TestRedirectToMetadataBlocked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
	}))
	defer srv.Close()
	if _, err := testFetcher(Options{}).Get(context.Background(), srv.URL, nil); !errors.Is(err, ErrBlocked) {
		t.Fatalf("redirect to metadata IP must be blocked, got %v", err)
	}
}

func TestRedirectToDisallowedSchemeBlocked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "file:///etc/passwd", http.StatusFound)
	}))
	defer srv.Close()
	if _, err := testFetcher(Options{}).Get(context.Background(), srv.URL, nil); err == nil {
		t.Fatal("redirect to file:// must fail")
	}
}

func TestTooManyRedirects(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srv.URL+"/again", http.StatusFound)
	}))
	defer srv.Close()
	if _, err := testFetcher(Options{MaxRedirects: 2}).Get(context.Background(), srv.URL, nil); !errors.Is(err, ErrTooManyRedirects) {
		t.Fatalf("got %v", err)
	}
}

func TestBodySizeLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("a", 5000)))
	}))
	defer srv.Close()
	if _, err := testFetcher(Options{MaxBytes: 1024}).Get(context.Background(), srv.URL, nil); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("got %v, want ErrTooLarge", err)
	}
	if resp, err := testFetcher(Options{MaxBytes: 10000}).Get(context.Background(), srv.URL, nil); err != nil || len(resp.Body) != 5000 {
		t.Fatalf("under the cap must succeed: %v", err)
	}
}

func TestTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer srv.Close()
	start := time.Now()
	if _, err := testFetcher(Options{Timeout: 200 * time.Millisecond}).Get(context.Background(), srv.URL, nil); err == nil {
		t.Fatal("expected timeout")
	}
	if time.Since(start) > 1500*time.Millisecond {
		t.Fatal("timeout not enforced")
	}
}

func TestIgnoresProxyEnvironment(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("http_proxy", "http://127.0.0.1:1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) }))
	defer srv.Close()
	if _, err := testFetcher(Options{}).Get(context.Background(), srv.URL, nil); err != nil {
		t.Fatalf("proxy env must be ignored: %v", err)
	}
	_ = os.Getenv("HTTP_PROXY")
}

func TestDoesNotStoreCookies(t *testing.T) {
	var second string
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "secret"})
		} else {
			second = r.Header.Get("Cookie")
		}
	}))
	defer srv.Close()
	f := testFetcher(Options{})
	_, _ = f.Get(context.Background(), srv.URL, nil)
	_, _ = f.Get(context.Background(), srv.URL, nil)
	if second != "" {
		t.Fatalf("cookie was replayed: %q", second)
	}
}

func TestExtraHeadersAreForwarded(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got = r.Header.Get("If-None-Match") }))
	defer srv.Close()
	h := http.Header{}
	h.Set("If-None-Match", `"v1"`)
	if _, err := testFetcher(Options{}).Get(context.Background(), srv.URL, h); err != nil || got != `"v1"` {
		t.Fatalf("header not forwarded: %q %v", got, err)
	}
}

func TestValidateURLStaticChecks(t *testing.T) {
	good := []string{"https://example.com/feed.xml", "http://news.example.org/a?b=c", "https://example.com:443/x"}
	for _, u := range good {
		if err := ValidateURL(u); err != nil {
			t.Errorf("%s should be valid: %v", u, err)
		}
	}
	bad := []string{"file:///etc/passwd", "http://127.0.0.1/", "http://169.254.169.254/", "http://[::1]/", "http://10.0.0.1/x",
		"http://metadata.google.internal/", "https://user:pw@example.com/", "ftp://example.com/", "http://example.com:8080/", "", "not a url"}
	for _, u := range bad {
		if err := ValidateURL(u); err == nil {
			t.Errorf("%q must be rejected", u)
		}
	}
}

func TestPostSendsBodyAndHeadersAndNeverFollowsRedirects(t *testing.T) {
	var got, ctype, sig string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, 100)
		n, _ := r.Body.Read(b)
		got, ctype, sig = string(b[:n]), r.Header.Get("Content-Type"), r.Header.Get("X-TP-Signature")
		if r.URL.Path == "/redir" {
			http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(strings.Repeat("x", 10000)))
	}))
	defer srv.Close()
	f := testFetcher(Options{})
	h := http.Header{}
	h.Set("X-TP-Signature", "sha256=abc")
	resp, err := f.Post(context.Background(), srv.URL+"/hook", []byte(`{"a":1}`), h)
	if err != nil || resp.Status != 202 || got != `{"a":1}` || ctype != "application/json" || sig != "sha256=abc" || len(resp.Body) != 4096 {
		t.Fatalf("post: %+v %v body=%q ctype=%q sig=%q", resp, err, got, ctype, sig)
	}
	resp, err = f.Post(context.Background(), srv.URL+"/redir", []byte(`{}`), nil)
	if err != nil || resp.Status != http.StatusFound {
		t.Fatalf("redirects must be returned, not followed: %+v %v", resp, err)
	}
}

func TestPostIsSubjectToTheSameDestinationRules(t *testing.T) {
	f := New(Options{Resolver: fakeResolver{"evil.example": {netip.MustParseAddr("10.0.0.5")}}})
	for _, u := range []string{"http://169.254.169.254/", "http://127.0.0.1/", "http://evil.example/", "file:///etc/passwd", "https://u:p@example.com/", "http://example.com:8080/"} {
		if _, err := f.Post(context.Background(), u, []byte("{}"), nil); err == nil {
			t.Errorf("POST to %s must be refused", u)
		}
	}
}
