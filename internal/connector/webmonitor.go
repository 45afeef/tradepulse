package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/andybalholm/cascadia"
	"golang.org/x/net/html/charset"

	"tradepulse/internal/domain/sourcedef"
)

// WebMonitor extracts items from an HTML page with admin-configured CSS selectors.
// No JavaScript is executed. robots.txt is respected unless explicitly overridden
// (with a recorded reason). Extracted values are plain text and validated links.
type WebMonitor struct {
	http   HTTP
	robots *Robots
}

func NewWebMonitor(h HTTP, robots *Robots) *WebMonitor { return &WebMonitor{http: h, robots: robots} }

func (*WebMonitor) Type() sourcedef.Type { return sourcedef.TypeWebMonitor }

// Validate checks that every configured selector is valid CSS.
func (*WebMonitor) Validate(src sourcedef.Source) error {
	if src.Web == nil {
		return errors.New("web configuration is required")
	}
	for name, sel := range map[string]string{
		"item_selector": src.Web.ItemSelector, "title_selector": src.Web.TitleSelector,
		"link_selector": src.Web.LinkSelector, "summary_selector": src.Web.SummarySelector,
	} {
		if sel == "" {
			continue
		}
		if _, err := cascadia.Compile(sel); err != nil {
			return fmt.Errorf("%s is not a valid CSS selector", name)
		}
	}
	return nil
}

func (w *WebMonitor) Fetch(ctx context.Context, src sourcedef.Source) (Result, error) {
	if err := w.Validate(src); err != nil {
		return Result{}, err
	}
	if !src.Web.IgnoreRobots {
		ok, err := w.robots.Allowed(ctx, src.URL)
		if err != nil {
			return Result{}, err
		}
		if !ok {
			return Result{}, errors.New("blocked by robots.txt")
		}
	}
	hdr := http.Header{}
	hdr.Set("Accept", "text/html,application/xhtml+xml;q=0.9,*/*;q=0.5")
	if src.State.ETag != "" {
		hdr.Set("If-None-Match", src.State.ETag)
	}
	if src.State.LastModified != "" {
		hdr.Set("If-Modified-Since", src.State.LastModified)
	}
	resp, err := w.http.Get(ctx, src.URL, hdr)
	if err != nil {
		return Result{}, err
	}
	switch {
	case resp.Status == http.StatusNotModified:
		return Result{NotModified: true, HTTPStatus: resp.Status}, nil
	case resp.Status < 200 || resp.Status >= 300:
		return Result{HTTPStatus: resp.Status}, fmt.Errorf("unexpected HTTP status %d", resp.Status)
	}
	items, err := ParseHTML(resp.Body, resp.Header.Get("Content-Type"), resp.FinalURL, *src.Web)
	if err != nil {
		return Result{HTTPStatus: resp.Status}, err
	}
	return Result{
		Items: items, HTTPStatus: resp.Status,
		ETag: truncate(resp.Header.Get("ETag"), 200), LastModified: truncate(resp.Header.Get("Last-Modified"), 100),
	}, nil
}

// ParseHTML applies the selectors to a page.
func ParseHTML(body []byte, contentType, base string, cfg sourcedef.WebConfig) ([]Item, error) {
	r, err := charset.NewReader(bytes.NewReader(body), contentType)
	if err != nil {
		r = bytes.NewReader(body)
	}
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, fmt.Errorf("cannot parse HTML: %w", err)
	}
	max := cfg.MaxItems
	if max < 1 || max > 100 {
		max = 30
	}
	var out []Item
	doc.Find(cfg.ItemSelector).EachWithBreak(func(_ int, s *goquery.Selection) bool {
		if len(out) >= max {
			return false
		}
		if it, ok := extractItem(s, base, cfg); ok {
			out = append(out, it)
		}
		return true
	})
	return out, nil
}

func extractItem(s *goquery.Selection, base string, cfg sourcedef.WebConfig) (Item, bool) {
	// link element
	var linkSel *goquery.Selection
	switch {
	case cfg.LinkSelector != "":
		linkSel = s.Find(cfg.LinkSelector).First()
		if linkSel.Length() == 0 && s.Is(cfg.LinkSelector) {
			linkSel = s
		}
	case s.Is("a[href]"):
		linkSel = s
	default:
		linkSel = s.Find("a[href]").First()
	}
	href, _ := linkSel.Attr("href")
	link, _ := safeLink(base, href)

	// title
	var title string
	switch {
	case cfg.TitleSelector != "":
		title = s.Find(cfg.TitleSelector).First().Text()
	case linkSel.Length() > 0 && strings.TrimSpace(linkSel.Text()) != "":
		title = linkSel.Text()
	default:
		title = s.Text()
	}
	title = truncate(collapse(title), 300)
	if title == "" {
		return Item{}, false
	}
	summary := ""
	if cfg.SummarySelector != "" {
		summary = truncate(collapse(s.Find(cfg.SummarySelector).First().Text()), 500)
	}
	key := link
	if key == "" {
		key = hashKey(title)
	}
	if len(key) > 200 {
		key = hashKey(key)
	}
	raw, _ := json.Marshal(map[string]any{"title": title, "summary": summary, "url": link})
	return Item{Key: "web:" + key, Title: title, Summary: summary, URL: link, Raw: raw}, true
}
