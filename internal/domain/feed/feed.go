// Package feed defines FeedItem: what a particular user should see.
package feed

import (
	"context"
	"time"
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
}

type Repository interface {
	Add(ctx context.Context, item FeedItem) error
	// Recent returns the newest items first, for ONE user.
	Recent(ctx context.Context, userID string, limit int) ([]FeedItem, error)
}
