package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/html/charset"

	"tradepulse/internal/domain/sourcedef"
)

const maxFeedItems = 100

type RSS struct{ http HTTP }

func NewRSS(h HTTP) *RSS { return &RSS{http: h} }

func (*RSS) Type() sourcedef.Type { return sourcedef.TypeRSS }

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	GUID        string `xml:"guid"`
	PubDate     string `xml:"pubDate"`
	DCDate      string `xml:"http://purl.org/dc/elements/1.1/ date"`
}
type rssDoc struct {
	Channel struct {
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
}
type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}
type atomEntry struct {
	ID        string     `xml:"id"`
	Title     string     `xml:"title"`
	Summary   string     `xml:"summary"`
	Content   string     `xml:"content"`
	Updated   string     `xml:"updated"`
	Published string     `xml:"published"`
	Links     []atomLink `xml:"link"`
}
type atomDoc struct {
	Entries []atomEntry `xml:"entry"`
}

func (r *RSS) Fetch(ctx context.Context, src sourcedef.Source) (Result, error) {
	hdr := http.Header{}
	hdr.Set("Accept", "application/rss+xml, application/atom+xml, application/xml;q=0.9, text/xml;q=0.9, */*;q=0.5")
	if src.State.ETag != "" {
		hdr.Set("If-None-Match", src.State.ETag)
	}
	if src.State.LastModified != "" {
		hdr.Set("If-Modified-Since", src.State.LastModified)
	}
	resp, err := r.http.Get(ctx, src.URL, hdr)
	if err != nil {
		return Result{}, err
	}
	switch {
	case resp.Status == http.StatusNotModified:
		return Result{NotModified: true, HTTPStatus: resp.Status}, nil
	case resp.Status < 200 || resp.Status >= 300:
		return Result{HTTPStatus: resp.Status}, fmt.Errorf("unexpected HTTP status %d", resp.Status)
	}
	items, err := ParseFeed(resp.Body, resp.FinalURL)
	if err != nil {
		return Result{HTTPStatus: resp.Status}, err
	}
	return Result{
		Items: items, HTTPStatus: resp.Status,
		ETag: truncate(resp.Header.Get("ETag"), 200), LastModified: truncate(resp.Header.Get("Last-Modified"), 100),
	}, nil
}

// ParseFeed parses RSS 2.0 and Atom. XML entities/DTDs are not expanded (encoding/xml
// ignores DTDs), the body is already size-capped, and all text is reduced to plain text.
func ParseFeed(body []byte, base string) ([]Item, error) {
	dec := xml.NewDecoder(bytes.NewReader(body))
	dec.Strict = false
	dec.Entity = xml.HTMLEntity
	dec.CharsetReader = charset.NewReaderLabel
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil, ErrNotFeed
		}
		if err != nil {
			return nil, fmt.Errorf("invalid XML: %w", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch strings.ToLower(se.Name.Local) {
		case "rss":
			var d rssDoc
			if err := dec.DecodeElement(&d, &se); err != nil {
				return nil, fmt.Errorf("invalid RSS: %w", err)
			}
			return rssItems(d, base), nil
		case "feed":
			var d atomDoc
			if err := dec.DecodeElement(&d, &se); err != nil {
				return nil, fmt.Errorf("invalid Atom: %w", err)
			}
			return atomItems(d, base), nil
		default:
			return nil, ErrNotFeed
		}
	}
}

func build(key, title, summary, link string, published time.Time) (Item, bool) {
	title = truncate(stripTags(title), 300)
	if title == "" {
		return Item{}, false
	}
	summary = truncate(stripTags(summary), 500)
	if key == "" {
		if link != "" {
			key = link
		} else {
			key = HashKey(title, published.String())
		}
	}
	if len(key) > 200 {
		key = HashKey(key)
	}
	raw, _ := json.Marshal(map[string]any{"title": title, "summary": summary, "url": link, "published": published})
	return Item{Key: "rss:" + key, Title: title, Summary: summary, URL: link, PublishedAt: published, Raw: raw}, true
}

func rssItems(d rssDoc, base string) []Item {
	var out []Item
	for _, it := range d.Channel.Items {
		if len(out) >= maxFeedItems {
			break
		}
		link, _ := safeLink(base, it.Link)
		date := it.PubDate
		if date == "" {
			date = it.DCDate
		}
		if item, ok := build(strings.TrimSpace(it.GUID), it.Title, it.Description, link, parseDate(date)); ok {
			out = append(out, item)
		}
	}
	return out
}

func atomItems(d atomDoc, base string) []Item {
	var out []Item
	for _, e := range d.Entries {
		if len(out) >= maxFeedItems {
			break
		}
		href := ""
		for _, l := range e.Links {
			if l.Rel == "" || l.Rel == "alternate" {
				href = l.Href
				break
			}
		}
		link, _ := safeLink(base, href)
		summary := e.Summary
		if summary == "" {
			summary = e.Content
		}
		date := e.Published
		if date == "" {
			date = e.Updated
		}
		if item, ok := build(strings.TrimSpace(e.ID), e.Title, summary, link, parseDate(date)); ok {
			out = append(out, item)
		}
	}
	return out
}
