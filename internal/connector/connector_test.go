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
)

type fakeHTTP struct {
	pages map[string]*safefetch.Response
	errs  map[string]error
	calls []string
	hdrs  []http.Header
}

func (f *fakeHTTP) Get(_ context.Context, u string, h http.Header) (*safefetch.Response, error) {
	f.calls = append(f.calls, u)
	f.hdrs = append(f.hdrs, h)
	if e, ok := f.errs[u]; ok {
		return nil, e
	}
	if r, ok := f.pages[u]; ok {
		cp := *r
		if cp.FinalURL == "" {
			cp.FinalURL = u
		}
		return &cp, nil
	}
	return &safefetch.Response{Status: 404, FinalURL: u}, nil
}

func page(status int, body string, hdr ...string) *safefetch.Response {
	h := http.Header{}
	for i := 0; i+1 < len(hdr); i += 2 {
		h.Set(hdr[i], hdr[i+1])
	}
	return &safefetch.Response{Status: status, Body: []byte(body), Header: h}
}

const rssSample = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:dc="http://purl.org/dc/elements/1.1/"><channel><title>T</title>
<item><title>Sensex jumps 500 points</title><link>/news/sensex</link><guid>g-1</guid>
 <description>&lt;p&gt;Markets &lt;b&gt;rally&lt;/b&gt;&lt;script&gt;alert(1)&lt;/script&gt; today&amp;nbsp;ok&lt;/p&gt;</description>
 <pubDate>Mon, 02 Jan 2006 15:04:05 +0000</pubDate></item>
<item><title><![CDATA[Fed <i>holds</i> rates]]></title><link>javascript:alert(1)</link><dc:date>2026-10-16T10:00:00Z</dc:date></item>
<item><title></title><link>https://example.com/empty-title</link></item>
</channel></rss>`

const atomSample = `<feed xmlns="http://www.w3.org/2005/Atom"><entry><id>tag:x,1</id><title>Atom story</title>
<link rel="self" href="https://example.com/self"/><link rel="alternate" href="https://example.com/story#frag"/>
<summary>Short</summary><updated>2026-10-16T09:00:00Z</updated></entry></feed>`

func TestParseRSSSanitizesAndKeys(t *testing.T) {
	items, err := ParseFeed([]byte(rssSample), "https://feeds.example.com/rss.xml")
	if err != nil || len(items) != 2 {
		t.Fatalf("got %d items, %v", len(items), err)
	}
	a := items[0]
	if a.Title != "Sensex jumps 500 points" || a.URL != "https://feeds.example.com/news/sensex" || a.Key != "rss:g-1" {
		t.Fatalf("item 0: %+v", a)
	}
	if strings.Contains(a.Summary, "alert") || strings.Contains(a.Summary, "<") || !strings.Contains(a.Summary, "Markets rally") {
		t.Fatalf("summary not sanitized: %q", a.Summary)
	}
	if a.PublishedAt.Year() != 2006 {
		t.Fatalf("date not parsed: %v", a.PublishedAt)
	}
	b := items[1]
	if b.URL != "" {
		t.Fatalf("javascript: link must be dropped, got %q", b.URL)
	}
	if b.Title != "Fed holds rates" || b.PublishedAt.IsZero() || !strings.HasPrefix(b.Key, "rss:") {
		t.Fatalf("item 1: %+v", b)
	}
}

func TestParseAtom(t *testing.T) {
	items, err := ParseFeed([]byte(atomSample), "")
	if err != nil || len(items) != 1 {
		t.Fatalf("%v %d", err, len(items))
	}
	if items[0].URL != "https://example.com/story" || items[0].Key != "rss:tag:x,1" || items[0].Summary != "Short" {
		t.Fatalf("%+v", items[0])
	}
}

func TestParseFeedRejectsGarbageAndHTML(t *testing.T) {
	for name, body := range map[string]string{
		"empty": "", "html page": "<html><body>hi</body></html>", "json": `{"a":1}`, "broken xml": "<rss><channel><item>",
	} {
		if _, err := ParseFeed([]byte(body), ""); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestParseFeedHandlesLatin1AndIsBounded(t *testing.T) {
	latin := "<?xml version=\"1.0\" encoding=\"ISO-8859-1\"?><rss><channel><item><title>Caf\xe9 news</title><guid>1</guid></item></channel></rss>"
	items, err := ParseFeed([]byte(latin), "")
	if err != nil || len(items) != 1 || items[0].Title != "Café news" {
		t.Fatalf("latin1: %v %+v", err, items)
	}
	var b strings.Builder
	b.WriteString("<rss><channel>")
	for i := 0; i < 500; i++ {
		b.WriteString("<item><title>t</title><guid>g")
		b.WriteString(strings.Repeat("x", i%7))
		b.WriteString(string(rune('a' + i%26)))
		b.WriteString("</guid></item>")
	}
	b.WriteString("</channel></rss>")
	if items, _ := ParseFeed([]byte(b.String()), ""); len(items) > maxFeedItems {
		t.Fatalf("item cap not enforced: %d", len(items))
	}
}

func TestParseFeedDoesNotExpandEntities(t *testing.T) {
	bomb := `<?xml version="1.0"?><!DOCTYPE r [<!ENTITY a "AAAAAAAAAA"><!ENTITY b "&a;&a;&a;&a;&a;&a;&a;&a;"><!ENTITY c "&b;&b;&b;&b;&b;&b;&b;&b;">]>
<rss><channel><item><title>&c;&c;&c;&c;</title><guid>1</guid></item></channel></rss>`
	items, err := ParseFeed([]byte(bomb), "")
	if err == nil {
		for _, it := range items {
			if len(it.Title) > 1000 {
				t.Fatalf("entity expansion happened: %d bytes", len(it.Title))
			}
		}
	}
}

func TestStripTagsAndSafeLink(t *testing.T) {
	if got := stripTags(`<div onclick="x()">Hi <b>there</b><style>p{}</style><iframe src=x>bad</iframe>&amp; bye</div>`); got != "Hi there & bye" {
		t.Fatalf("got %q", got)
	}
	cases := map[string]bool{
		"https://example.com/a": true, "/rel/path": true, "javascript:alert(1)": false, "data:text/html,x": false,
		"ftp://example.com/x": false, "https://user:pw@example.com/": false, "": false, "//evil.example/x": true,
	}
	for in, want := range cases {
		if _, ok := safeLink("https://example.com/base/", in); ok != want {
			t.Errorf("safeLink(%q) = %v, want %v", in, ok, want)
		}
	}
	if u, _ := safeLink("https://example.com/", "/a#top"); u != "https://example.com/a" {
		t.Fatalf("fragment kept: %s", u)
	}
	if got := truncate(strings.Repeat("é", 400), 300); len([]rune(got)) != 300 {
		t.Fatalf("truncate not rune-safe: %d", len([]rune(got)))
	}
}

func TestRSSFetchConditionalAndStatuses(t *testing.T) {
	fh := &fakeHTTP{pages: map[string]*safefetch.Response{
		"https://example.com/ok":  page(200, atomSample, "ETag", `"v2"`, "Last-Modified", "Mon, 02 Jan 2006 15:04:05 GMT"),
		"https://example.com/304": page(304, ""),
		"https://example.com/500": page(500, "boom"),
	}}
	c := NewRSS(fh)
	src := sourcedef.Source{Type: sourcedef.TypeRSS, URL: "https://example.com/ok", State: sourcedef.State{ETag: `"v1"`, LastModified: "yesterday"}}
	res, err := c.Fetch(context.Background(), src)
	if err != nil || len(res.Items) != 1 || res.ETag != `"v2"` || res.LastModified == "" {
		t.Fatalf("%+v %v", res, err)
	}
	if fh.hdrs[0].Get("If-None-Match") != `"v1"` || fh.hdrs[0].Get("If-Modified-Since") != "yesterday" {
		t.Fatalf("conditional headers not sent: %v", fh.hdrs[0])
	}
	src.URL = "https://example.com/304"
	if res, err := c.Fetch(context.Background(), src); err != nil || !res.NotModified {
		t.Fatalf("304: %+v %v", res, err)
	}
	src.URL = "https://example.com/500"
	if _, err := c.Fetch(context.Background(), src); err == nil {
		t.Fatal("HTTP 500 must be an error")
	}
	fh.errs = map[string]error{"https://example.com/blocked": safefetch.ErrBlocked}
	src.URL = "https://example.com/blocked"
	if _, err := c.Fetch(context.Background(), src); !errors.Is(err, safefetch.ErrBlocked) {
		t.Fatalf("safefetch errors must propagate: %v", err)
	}
}

const htmlSample = `<html><body><ul class="news">
<li class="item"><a href="/a/one?x=1#c"><span>First headline</span></a><p class="sum">Summary one</p></li>
<li class="item"><a href="javascript:alert(1)">Second headline</a></li>
<li class="item"><a href="https://other.example/three">Third headline</a></li>
<li class="item"><span>No link headline</span></li>
<li class="item"><a href="/empty"> </a></li>
</ul><script>var evil=1</script></body></html>`

func TestParseHTMLWithSelectors(t *testing.T) {
	items, err := ParseHTML([]byte(htmlSample), "text/html; charset=utf-8", "https://news.example.com/list/",
		sourcedef.WebConfig{ItemSelector: "li.item", SummarySelector: "p.sum"})
	if err != nil || len(items) != 4 {
		t.Fatalf("%v %d %+v", err, len(items), items)
	}
	if items[0].Title != "First headline" || items[0].URL != "https://news.example.com/a/one?x=1" || items[0].Summary != "Summary one" || items[0].Key != "web:https://news.example.com/a/one?x=1" {
		t.Fatalf("item 0: %+v", items[0])
	}
	if items[1].URL != "" || !strings.HasPrefix(items[1].Key, "web:") {
		t.Fatalf("javascript: link must be dropped: %+v", items[1])
	}
	if items[3].Title != "No link headline" {
		t.Fatalf("item 3: %+v", items[3])
	}
	capped, _ := ParseHTML([]byte(htmlSample), "", "https://x.example/", sourcedef.WebConfig{ItemSelector: "li.item", MaxItems: 2})
	if len(capped) != 2 {
		t.Fatalf("max_items ignored: %d", len(capped))
	}
	link, _ := ParseHTML([]byte(htmlSample), "", "https://x.example/", sourcedef.WebConfig{ItemSelector: "ul.news a[href]"})
	if len(link) < 2 || link[0].URL != "https://x.example/a/one?x=1" {
		t.Fatalf("item selector on links: %+v", link)
	}
}

func TestWebMonitorValidateSelectors(t *testing.T) {
	w := NewWebMonitor(&fakeHTTP{}, NewRobots(&fakeHTTP{}, time.Hour, nil))
	good := sourcedef.Source{Web: &sourcedef.WebConfig{ItemSelector: "div.a > h3 a", TitleSelector: "h3"}}
	if err := w.Validate(good); err != nil {
		t.Fatal(err)
	}
	for _, sel := range []string{"div[", ":::", "a >>> b"} {
		if err := w.Validate(sourcedef.Source{Web: &sourcedef.WebConfig{ItemSelector: sel}}); err == nil {
			t.Errorf("invalid selector %q accepted", sel)
		}
	}
	if err := w.Validate(sourcedef.Source{}); err == nil {
		t.Error("missing web config accepted")
	}
}

func TestWebMonitorRespectsRobotsAndOverride(t *testing.T) {
	fh := &fakeHTTP{pages: map[string]*safefetch.Response{
		"https://blocked.example/robots.txt": page(200, "User-agent: *\nDisallow: /news\n"),
		"https://blocked.example/news":       page(200, htmlSample),
		"https://open.example/robots.txt":    page(404, ""),
		"https://open.example/news":          page(200, htmlSample),
	}}
	w := NewWebMonitor(fh, NewRobots(fh, time.Hour, nil))
	cfg := &sourcedef.WebConfig{ItemSelector: "li.item"}
	if _, err := w.Fetch(context.Background(), sourcedef.Source{URL: "https://blocked.example/news", Web: cfg}); err == nil || !strings.Contains(err.Error(), "robots") {
		t.Fatalf("robots disallow not enforced: %v", err)
	}
	for _, c := range fh.calls {
		if c == "https://blocked.example/news" {
			t.Fatal("the page was fetched despite robots.txt")
		}
	}
	over := &sourcedef.WebConfig{ItemSelector: "li.item", IgnoreRobots: true, IgnoreRobotsReason: "written permission from owner"}
	if res, err := w.Fetch(context.Background(), sourcedef.Source{URL: "https://blocked.example/news", Web: over}); err != nil || len(res.Items) == 0 {
		t.Fatalf("override failed: %v", err)
	}
	if res, err := w.Fetch(context.Background(), sourcedef.Source{URL: "https://open.example/news", Web: cfg}); err != nil || len(res.Items) == 0 {
		t.Fatalf("404 robots means allowed: %v", err)
	}
}

func TestRobotsFailsClosedOnServerErrorAndCaches(t *testing.T) {
	fh := &fakeHTTP{pages: map[string]*safefetch.Response{
		"https://down.example/robots.txt": page(503, ""),
		"https://ok.example/robots.txt":   page(200, "User-agent: *\nDisallow: /private\n"),
	}}
	now := time.Now()
	r := NewRobots(fh, time.Hour, func() time.Time { return now })
	if ok, err := r.Allowed(context.Background(), "https://down.example/x"); ok || err == nil {
		t.Fatalf("5xx must fail closed: %v %v", ok, err)
	}
	for i := 0; i < 3; i++ {
		if ok, _ := r.Allowed(context.Background(), "https://ok.example/public"); !ok {
			t.Fatal("public path blocked")
		}
	}
	if ok, _ := r.Allowed(context.Background(), "https://ok.example/private/x"); ok {
		t.Fatal("private path allowed")
	}
	n := 0
	for _, c := range fh.calls {
		if c == "https://ok.example/robots.txt" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("robots.txt fetched %d times, want 1 (cache)", n)
	}
	now = now.Add(2 * time.Hour)
	_, _ = r.Allowed(context.Background(), "https://ok.example/public")
}

func TestParseRobotsRules(t *testing.T) {
	body := "# comment\nUser-agent: googlebot\nDisallow: /g\n\nUser-agent: *\nDisallow: /private\nAllow: /private/open\nDisallow: /*.pdf$\nDisallow:\n\nUser-agent: tradepulse\nDisallow: /only-us\n"
	star := parseRobots(strings.Replace(body, "tradepulse", "other", 1), robotsToken)
	cases := []struct {
		path string
		want bool
	}{{"/private/x", false}, {"/private/open/y", true}, {"/a/b.pdf", false}, {"/a/b.pdf?x", true}, {"/public", true}, {"/g", true}}
	for _, c := range cases {
		if got := robotsAllowed(star, c.path); got != c.want {
			t.Errorf("star group %s: got %v want %v", c.path, got, c.want)
		}
	}
	specific := parseRobots(body, robotsToken)
	if robotsAllowed(specific, "/only-us/x") || !robotsAllowed(specific, "/private/x") {
		t.Fatal("our own group must win over *")
	}
}
