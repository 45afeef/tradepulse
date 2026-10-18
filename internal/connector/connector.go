// Package connector turns configured sources into normalized items. Connectors never
// touch the network directly: they use an HTTP implementation backed by
// internal/safefetch (SSRF guard), which tests replace with a fake.
package connector

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"time"

	"tradepulse/internal/domain/event"
	"tradepulse/internal/domain/sourcedef"
	"tradepulse/internal/safefetch"
)

// HTTP is what connectors need from the network layer (*safefetch.Fetcher satisfies it).
type HTTP interface {
	Get(ctx context.Context, rawURL string, extra http.Header) (*safefetch.Response, error)
}

// Item is one normalized piece of content found at a source.
type Item struct {
	Key         string // stable dedupe key within the source
	Title       string
	Summary     string
	URL         string
	PublishedAt time.Time // zero when unknown
	Entities    []string  // extra topics (merged with the source's own)
	Raw         []byte    // normalized JSON snapshot kept in the raw store
}

type Result struct {
	Items        []Item
	NotModified  bool
	ETag         string
	LastModified string
	HTTPStatus   int
	Cursor       string // new cursor to persist (e.g. X since_id); empty keeps the old one
}

type Connector interface {
	Type() sourcedef.Type
	Fetch(ctx context.Context, src sourcedef.Source) (Result, error)
}

// Resolver is optionally implemented by connectors that must look something up when a
// source is saved (e.g. X handle -> user id). The returned source carries the result.
type Resolver interface {
	Resolve(ctx context.Context, src sourcedef.Source) (sourcedef.Source, error)
}

// Validator is optionally implemented by connectors to check type-specific config.
type Validator interface {
	Validate(src sourcedef.Source) error
}

type Registry struct{ byType map[sourcedef.Type]Connector }

func NewRegistry(cs ...Connector) *Registry {
	r := &Registry{byType: map[sourcedef.Type]Connector{}}
	for _, c := range cs {
		r.byType[c.Type()] = c
	}
	return r
}

func (r *Registry) Get(t sourcedef.Type) (Connector, bool) {
	c, ok := r.byType[t]
	return c, ok
}

// Validate runs the connector's own config validation (no-op for types without one).
func (r *Registry) Validate(src sourcedef.Source) error {
	if src.Type == sourcedef.TypeManual {
		return nil
	}
	c, ok := r.byType[src.Type]
	if !ok {
		return fmt.Errorf("no connector available for type %q (is it configured on this server?)", src.Type)
	}
	if v, ok := c.(Validator); ok {
		return v.Validate(src)
	}
	return nil
}

// ItemEvent converts an item into a content_item event for the source. Publication
// times in the future are clamped to now (feeds with wrong clocks).
func ItemEvent(src sourcedef.Source, it Item, now time.Time) event.Event {
	ts := it.PublishedAt
	if ts.IsZero() || ts.After(now.Add(5*time.Minute)) {
		ts = now
	}
	ents := append([]string(nil), src.Entities...)
	seen := map[string]bool{}
	for _, e := range ents {
		seen[e] = true
	}
	for _, e := range it.Entities {
		if n := event.NormEntity(e); n != "" && !seen[n] {
			seen[n] = true
			ents = append(ents, n)
		}
	}
	ev := event.New("content_item", src.Name, ents, ts.UTC(), nil)
	ev.Title, ev.Summary, ev.URL, ev.DedupeKey = it.Title, it.Summary, it.URL, it.Key
	return ev
}

var ErrNotFeed = errors.New("response is not an RSS or Atom feed")

// HashKey builds a stable dedupe key from the given parts.
func HashKey(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}
