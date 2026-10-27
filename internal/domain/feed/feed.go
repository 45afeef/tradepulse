// Package feed defines FeedItem: what a particular user should see.
package feed

import (
	"context"
	"time"

	"tradepulse/internal/page"
)

type Severity string

const (
	Info    Severity = "info"
	Notable Severity = "notable"
	High    Severity = "high"
)

type FeedItem struct {
	ID        string    `json:"id"`
	UserID    string    `json:"-"`
	EventID   string    `json:"event_id"`
	EventType string    `json:"event_type"`
	Source    string    `json:"source"`
	Entities  []string  `json:"entities"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	URL       string    `json:"url,omitempty"`
	Severity  Severity  `json:"severity"`
	Reason    string    `json:"reason" doc:"Why this item matched (explainability)"`
	CreatedAt time.Time `json:"created_at"`
	Seq       int64     `json:"-"`
	Cursor    string    `json:"cursor" doc:"Opaque position in your feed; use as before/after"`
}

type Repository interface {
	Add(ctx context.Context, item FeedItem) error
	// AddItem stores the item and returns it with its Seq/Cursor assigned.
	AddItem(ctx context.Context, item FeedItem) (FeedItem, error)
	// Page lists ONE user's items per the cursor query (see package page).
	Page(ctx context.Context, userID string, q page.Query) ([]FeedItem, error)
	// Recent returns the newest items first, for ONE user.
	Recent(ctx context.Context, userID string, limit int) ([]FeedItem, error)
}
