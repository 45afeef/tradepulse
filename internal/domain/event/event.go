// Package event defines the central domain object: something that happened.
// Events know nothing about users, UI or delivery, and nothing about any
// particular domain: meaning comes from the type registry (registry.go).
package event

import (
	"context"
	"strings"
	"time"
	"unicode"

	"tradepulse/internal/idgen"
)

type Event struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	Source    string         `json:"source"`
	Entities  []string       `json:"entities"`
	Title     string         `json:"title,omitempty"`
	Summary   string         `json:"summary,omitempty"`
	URL       string         `json:"url,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
	Payload   map[string]any `json:"payload,omitempty"`
	DedupeKey string         `json:"dedupe_key,omitempty"`
}

func New(typ, source string, entities []string, ts time.Time, payload map[string]any) Event {
	return Event{ID: idgen.New(), Type: typ, Source: source, Entities: entities, Timestamp: ts, Payload: payload}
}

// Field resolves a built-in field (type, source, title, summary, url, entity)
// or a payload field. Rules and templates read events only through this.
func (e Event) Field(name string) (any, bool) {
	switch name {
	case "type":
		return e.Type, true
	case "source":
		return e.Source, true
	case "title":
		return e.Title, true
	case "summary":
		return e.Summary, true
	case "url":
		return e.URL, true
	case "entity":
		if len(e.Entities) > 0 {
			return e.Entities[0], true
		}
		return "", true
	}
	v, ok := e.Payload[name]
	return v, ok
}

// Float reads a numeric payload value (0 if absent or not numeric).
func (e Event) Float(key string) float64 {
	f, _ := ToFloat(e.Payload[key])
	return f
}

// ToFloat converts any Go/JSON numeric value to float64.
func ToFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

// NormEntity canonicalizes entity/source names for matching (case/space-insensitive).
func NormEntity(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }

// Tokens splits text into lowercase alphanumeric words.
func Tokens(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

// Phrase canonicalizes a keyword phrase: tokens joined by single spaces.
func Phrase(s string) string { return strings.Join(Tokens(s), " ") }

type Handler func(ctx context.Context, e Event)

// Bus publishes and consumes events. The in-memory implementation can later be
// replaced by NATS/Kafka without touching the domain.
type Bus interface {
	Publish(ctx context.Context, e Event) error
	// Subscribe registers a handler; the returned func unsubscribes it.
	Subscribe(name string, h Handler) (unsubscribe func())
	Close()
}
