// Package source defines how external information enters the system.
// Sources hold identity/config only; they contain no business logic and know
// nothing about users.
package source

import (
	"context"
	"time"
)

// RawMessage is unprocessed source data. It is never exposed to users.
type RawMessage struct {
	Source     string
	ReceivedAt time.Time
	Payload    []byte
	Metadata   map[string]string
}

// Listener receives information from a source (WebSocket, polling, RSS, ...).
// The channel is closed when ctx is cancelled or the underlying feed ends.
type Listener interface {
	Name() string
	Listen(ctx context.Context) <-chan RawMessage
}
