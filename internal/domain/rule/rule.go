// Package rule decides whether an Event is relevant for a given user.
package rule

import (
	"math"

	"tradepulse/internal/domain/event"
	"tradepulse/internal/domain/user"
)

// UserContext is an immutable, lookup-optimised view of a user's profile.
type UserContext struct {
	UserID  string
	Profile user.Profile
	Watch   map[string]struct{}
	Muted   map[string]struct{}
	Types   map[string]struct{}
}

func NewUserContext(id string, p user.Profile) UserContext {
	set := func(in []string) map[string]struct{} {
		m := make(map[string]struct{}, len(in))
		for _, s := range in {
			m[s] = struct{}{}
		}
		return m
	}
	return UserContext{UserID: id, Profile: p, Watch: set(p.Watchlist), Muted: set(p.Muted), Types: set(p.EventTypes)}
}

type Rule interface {
	Name() string
	Matches(e event.Event, uc UserContext) bool
}

// Default is the standard chain; all rules must match.
func Default() []Rule {
	return []Rule{WatchlistRule{}, MutedRule{}, EventTypeRule{}, ThresholdRule{}}
}

type WatchlistRule struct{}

func (WatchlistRule) Name() string { return "watchlist" }
func (WatchlistRule) Matches(e event.Event, uc UserContext) bool {
	_, ok := uc.Watch[e.Entity]
	return ok
}

type MutedRule struct{}

func (MutedRule) Name() string { return "not_muted" }
func (MutedRule) Matches(e event.Event, uc UserContext) bool {
	_, muted := uc.Muted[e.Entity]
	return !muted
}

type EventTypeRule struct{}

func (EventTypeRule) Name() string { return "event_type" }
func (EventTypeRule) Matches(e event.Event, uc UserContext) bool {
	if len(uc.Types) == 0 {
		return true
	}
	_, ok := uc.Types[string(e.Type)]
	return ok
}

type ThresholdRule struct{}

func (ThresholdRule) Name() string { return "threshold" }
func (ThresholdRule) Matches(e event.Event, uc UserContext) bool {
	switch e.Type {
	case event.VolumeSpike:
		return e.Float("relative_volume") >= uc.Profile.VolumeSpikeThreshold
	case event.PriceChanged:
		return math.Abs(e.Float("change_pct")) >= uc.Profile.PriceMoveThresholdPct
	}
	return true
}
