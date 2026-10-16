// Package processing defines how source-specific RawMessages become Events.
// Concrete processors live next to their source (see examples/ and internal/connector).
package processing

import (
	"tradepulse/internal/domain/event"
	"tradepulse/internal/source"
)

// Processor converts a source-specific RawMessage into zero or more Events.
type Processor interface {
	Source() string
	Process(msg source.RawMessage) ([]event.Event, error)
}
