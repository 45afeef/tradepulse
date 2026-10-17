package connector

import (
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/net/html"
)

// stripTags returns the visible text of an HTML fragment: scripts, styles and
// frames are dropped entirely, entities are decoded, whitespace is collapsed.
// Output is plain text and is never interpreted as HTML downstream.
func stripTags(s string) string {
	z := html.NewTokenizer(strings.NewReader(s))
	var b strings.Builder
	skipDepth := 0
	isSkip := func(name []byte) bool {
		switch string(name) {
		case "script", "style", "iframe", "object", "embed", "noscript", "template":
			return true
		}
		return false
	}
	for {
		switch z.Next() {
		case html.ErrorToken:
			return collapse(b.String())
		case html.StartTagToken:
			name, _ := z.TagName()
			if isSkip(name) {
				skipDepth++
			}
			b.WriteByte(' ')
		case html.EndTagToken:
			name, _ := z.TagName()
			if isSkip(name) && skipDepth > 0 {
				skipDepth--
			}
			b.WriteByte(' ')
		case html.TextToken:
			if skipDepth == 0 {
				b.Write(z.Text())
				b.WriteByte(' ')
			}
		}
	}
}

func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }

func truncate(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:max-1])) + "…"
}

// safeLink resolves raw against base and accepts only plain http(s) URLs
// (no javascript:, data:, credentials); fragments are dropped.
func safeLink(base, raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 2000 {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	if b, err := url.Parse(base); err == nil && base != "" {
		u = b.ResolveReference(u)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return "", false
	}
	u.Fragment = ""
	out := u.String()
	if len(out) > 2000 {
		return "", false
	}
	return out, true
}

var dateLayouts = []string{
	time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC822, time.RFC3339, time.RFC3339Nano,
	"Mon, 2 Jan 2006 15:04:05 -0700", "Mon, 2 Jan 2006 15:04:05 MST", "2 Jan 2006 15:04:05 -0700",
	"2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02",
}

func parseDate(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, l := range dateLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}
