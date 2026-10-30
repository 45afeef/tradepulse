// Package view models saved feed views: named filters over sources, topics, keywords,
// event types and time. A view only narrows what is already in the member's feed.
package view

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"tradepulse/internal/domain/event"
	"tradepulse/internal/domain/feed"
)

const MaxPerUser = 20

var (
	ErrNotFound  = errors.New("view not found")
	ErrDuplicate = errors.New("a view with this name already exists")
)

// Filter: every non-empty criterion must hold; within one list any value matches.
type Filter struct {
	Sources     []string `json:"sources,omitempty" maxItems:"20" doc:"Items from any of these sources"`
	Entities    []string `json:"entities,omitempty" maxItems:"20" doc:"Items about any of these topics"`
	Keywords    []string `json:"keywords,omitempty" maxItems:"20" doc:"Whole words or phrases in the title or text"`
	Types       []string `json:"types,omitempty" maxItems:"20" doc:"Event types (see GET /event-types)"`
	WithinHours int      `json:"within_hours,omitempty" minimum:"0" maximum:"720" doc:"Only items from the last N hours"`
}

type Spec struct {
	Name   string `json:"name" minLength:"1" maxLength:"60" example:"Market movers today"`
	Filter Filter `json:"filter"`
}

type View struct {
	ID        string
	UserID    string
	Spec      Spec
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Repository interface {
	Create(ctx context.Context, v View) error // ErrDuplicate when the member already has that name
	List(ctx context.Context, userID string) ([]View, error)
	Get(ctx context.Context, userID, id string) (View, error)
	Update(ctx context.Context, v View) error
	Delete(ctx context.Context, userID, id string) error
}

func printable(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func clean(vals []string, norm func(string) string, what string) ([]string, error) {
	if len(vals) > 20 {
		return nil, fmt.Errorf("%s: at most 20 values", what)
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		if !printable(v) {
			return nil, fmt.Errorf("%s: control characters are not allowed", what)
		}
		n := norm(v)
		if n == "" || len(n) > 100 {
			return nil, fmt.Errorf("%s: each value must be 1-100 characters", what)
		}
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	return out, nil
}

func (f Filter) Empty() bool {
	return len(f.Sources) == 0 && len(f.Entities) == 0 && len(f.Keywords) == 0 && len(f.Types) == 0 && f.WithinHours == 0
}

// Normalize validates and canonicalizes the spec.
func (s Spec) Normalize(reg *event.Registry) (Spec, error) {
	s.Name = strings.Join(strings.Fields(s.Name), " ")
	if s.Name == "" || len([]rune(s.Name)) > 60 || !printable(s.Name) {
		return s, errors.New("name must be 1-60 printable characters")
	}
	var err error
	f := s.Filter
	if f.Sources, err = clean(f.Sources, event.NormEntity, "sources"); err != nil {
		return s, err
	}
	if f.Entities, err = clean(f.Entities, event.NormEntity, "entities"); err != nil {
		return s, err
	}
	if f.Keywords, err = clean(f.Keywords, event.Phrase, "keywords"); err != nil {
		return s, err
	}
	if f.Types, err = clean(f.Types, func(x string) string { return strings.ToLower(strings.TrimSpace(x)) }, "types"); err != nil {
		return s, err
	}
	for _, t := range f.Types {
		if _, ok := reg.Get(t); !ok {
			return s, fmt.Errorf("unknown event type %q", t)
		}
	}
	if f.WithinHours < 0 || f.WithinHours > 720 {
		return s, errors.New("within_hours must be between 1 and 720")
	}
	if f.Empty() {
		return s, errors.New("a view needs at least one filter (sources, entities, keywords, types or within_hours)")
	}
	s.Filter = f
	return s, nil
}

// Match reports whether a (normalized) filter accepts the feed item.
func (f Filter) Match(it feed.FeedItem, now time.Time) bool {
	if len(f.Sources) > 0 && !contains(f.Sources, event.NormEntity(it.Source)) {
		return false
	}
	if len(f.Types) > 0 && !contains(f.Types, strings.ToLower(it.EventType)) {
		return false
	}
	if len(f.Entities) > 0 {
		hit := false
		for _, e := range it.Entities {
			if contains(f.Entities, event.NormEntity(e)) {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	if len(f.Keywords) > 0 {
		text := " " + strings.Join(event.Tokens(it.Title+" "+it.Body), " ") + " "
		hit := false
		for _, k := range f.Keywords {
			if strings.Contains(text, " "+k+" ") {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	if f.WithinHours > 0 && now.Sub(it.CreatedAt) > time.Duration(f.WithinHours)*time.Hour {
		return false
	}
	return true
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
