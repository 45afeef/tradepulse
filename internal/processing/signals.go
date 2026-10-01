package processing

import (
	"context"
	"math"
	"sync"
	"time"

	"tradepulse/internal/domain/event"
)

type entityState struct {
	priceAt, volumeAt, signalAt time.Time
	changePct, relVolume        float64
}

// UnusualActivityDetector derives a Signal from several Events: a large price
// move plus a volume spike on the same entity within a window. The signal is
// published back to the bus as an Event of type unusual_activity. It is not
// user-specific; personalization happens downstream.
type UnusualActivityDetector struct {
	bus      event.Bus
	window   time.Duration
	cooldown time.Duration
	mu       sync.Mutex
	state    map[string]*entityState
}

func NewUnusualActivityDetector(bus event.Bus, window, cooldown time.Duration) *UnusualActivityDetector {
	return &UnusualActivityDetector{bus: bus, window: window, cooldown: cooldown, state: map[string]*entityState{}}
}

func (d *UnusualActivityDetector) Handle(ctx context.Context, e event.Event) {
	if e.Entity == "" {
		return
	}
	d.mu.Lock()
	st := d.state[e.Entity]
	if st == nil {
		st = &entityState{}
		d.state[e.Entity] = st
	}
	now := time.Now()
	switch e.Type {
	case event.PriceChanged:
		if c := e.Float("change_pct"); math.Abs(c) >= 1.0 {
			st.priceAt, st.changePct = now, c
		}
	case event.VolumeSpike:
		st.volumeAt, st.relVolume = now, e.Float("relative_volume")
	default:
		d.mu.Unlock()
		return
	}
	fire := now.Sub(st.priceAt) <= d.window && now.Sub(st.volumeAt) <= d.window &&
		now.Sub(st.signalAt) >= d.cooldown
	var sig event.Event
	if fire {
		st.signalAt = now
		sig = event.New(event.UnusualActivity, e.Entity, "signal", now.UTC(), map[string]any{
			"change_pct": st.changePct, "relative_volume": st.relVolume,
		})
	}
	d.mu.Unlock()
	if fire {
		_ = d.bus.Publish(ctx, sig)
	}
}
