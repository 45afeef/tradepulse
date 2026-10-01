// Package event defines the central domain object: something that happened.
// Events know nothing about users, UI or delivery.
package event

import (
	"context"
	"time"

	"tradepulse/internal/idgen"
)

type Type string

const (
	PriceChanged    Type = "price_changed"
	VolumeSpike     Type = "volume_spike"
	NewsPublished   Type = "news_published"
	UnusualActivity Type = "unusual_activity" // derived signal
)

var known = map[Type]struct{}{
	PriceChanged: {}, VolumeSpike: {}, NewsPublished: {}, UnusualActivity: {},
}

// IsKnown reports whether s is a supported event type.
func IsKnown(s string) bool { _, ok := known[Type(s)]; return ok }

type Event struct {
	ID        string         `json:"id"`
	Type      Type           `json:"type"`
	Entity    string         `json:"entity"`
	Timestamp time.Time      `json:"timestamp"`
	Payload   map[string]any `json:"payload"`
	Source    string         `json:"source"`
}

func New(t Type, entity, source string, ts time.Time, payload map[string]any) Event {
	return Event{ID: idgen.New(), Type: t, Entity: entity, Source: source, Timestamp: ts, Payload: payload}
}

// Float reads a numeric payload value (0 if absent or not numeric).
func (e Event) Float(key string) float64 {
	switch n := e.Payload[key].(type) {
	case float64:
		return n
	case int:
		return float64(n)
	}
	return 0
}

// String reads a string payload value.
func (e Event) String(key string) string {
	s, _ := e.Payload[key].(string)
	return s
}

type Handler func(ctx context.Context, e Event)

// Bus publishes and consumes events. The in-memory implementation can later be
// replaced by NATS/Kafka without touching the domain.
type Bus interface {
	Publish(ctx context.Context, e Event) error
	// Subscribe registers a handler; the returned func unsubscribes it.
	Subscribe(name string, h Handler) (unsubscribe func())
	Close()
}
