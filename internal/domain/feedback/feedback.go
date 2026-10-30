// Package feedback records members' "useful / not useful" ratings on feed items and alert
// fires. It is how alert precision (the share rated useful) is measured.
package feedback

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"
)

const (
	Useful    = "useful"
	NotUseful = "not_useful"

	TargetFeedItem  = "feed_item"
	TargetAlertFire = "alert_fire"

	MaxNote = 280
)

// Entry is one member's rating of one target (a member can change it or clear it).
type Entry struct {
	UserID     string
	TargetType string
	TargetID   string
	Rating     string
	Note       string
	Group      string // aggregation key, e.g. "source:cnbc" or "rule:<id>"
	Label      string // human label for the group
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type Totals struct {
	Useful    int `json:"useful"`
	NotUseful int `json:"not_useful"`
}

// Precision is useful / (useful + not useful); nil when nothing was rated.
func (t Totals) Precision() *float64 {
	n := t.Useful + t.NotUseful
	if n == 0 {
		return nil
	}
	p := float64(t.Useful) / float64(n)
	return &p
}

type GroupStat struct {
	TargetType string
	Group      string
	Label      string
	Totals
}

type Summary struct {
	FeedItems  Totals
	AlertFires Totals
	Groups     []GroupStat // busiest first, at most 20
}

type Repository interface {
	Upsert(ctx context.Context, e Entry) error
	Delete(ctx context.Context, userID, targetType, targetID string) error
	// Ratings returns targetID -> rating for the member's ratings among ids.
	Ratings(ctx context.Context, userID, targetType string, ids []string) (map[string]string, error)
	Summary(ctx context.Context, since time.Time) (Summary, error)
	Prune(ctx context.Context, before time.Time) (int64, error)
}

func ValidRating(r string) bool { return r == Useful || r == NotUseful }

// CleanNote trims and validates the optional free-text note.
func CleanNote(s string) (string, error) {
	s = strings.TrimSpace(s)
	if len([]rune(s)) > MaxNote {
		return "", errors.New("note must be at most 280 characters")
	}
	for _, r := range s {
		if unicode.IsControl(r) && r != '\n' {
			return "", errors.New("note must not contain control characters")
		}
	}
	return s, nil
}
