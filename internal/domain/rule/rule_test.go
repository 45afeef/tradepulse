package rule

import (
	"testing"
	"time"

	"tradepulse/internal/domain/event"
	"tradepulse/internal/domain/user"
)

func matchAll(e event.Event, uc UserContext) bool {
	for _, r := range Default() {
		if !r.Matches(e, uc) {
			return false
		}
	}
	return true
}

func TestDefaultRules(t *testing.T) {
	p := user.DefaultProfile()
	p.Watchlist = []string{"HDFCBANK"}
	uc := NewUserContext("u1", p)
	ev := func(typ event.Type, entity string, pl map[string]any) event.Event {
		return event.New(typ, entity, "test", time.Now(), pl)
	}
	if !matchAll(ev(event.VolumeSpike, "HDFCBANK", map[string]any{"relative_volume": 2.4}), uc) {
		t.Fatal("expected match")
	}
	if matchAll(ev(event.VolumeSpike, "HDFCBANK", map[string]any{"relative_volume": 1.6}), uc) {
		t.Fatal("below threshold should not match")
	}
	if matchAll(ev(event.VolumeSpike, "TCS", map[string]any{"relative_volume": 5.0}), uc) {
		t.Fatal("not on watchlist should not match")
	}
}
