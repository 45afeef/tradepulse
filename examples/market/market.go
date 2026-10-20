// Package market is an EXAMPLE domain (equity ticks) built purely on the generic
// core: it registers event types, a processor, a simulated source and a signal.
// Nothing here is imported by internal/ packages.
package market

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"strings"
	"sync"
	"time"

	"tradepulse/internal/domain/alert"
	"tradepulse/internal/domain/event"
	"tradepulse/internal/domain/rule"
	"tradepulse/internal/source"
)

const SourceName = "NSE_SIM"

// RegisterTypes adds the market event types to the registry.
func RegisterTypes(reg *event.Registry) error {
	num := func(n string) event.FieldDef { return event.FieldDef{Name: n, Kind: event.KindNumber} }
	defs := []event.TypeDef{
		{
			ID: "price_changed", Description: "Last traded price moved",
			Fields: []event.FieldDef{num("ltp"), num("prev_ltp"), num("change_pct"), num("abs_change_pct"), {Name: "direction", Kind: event.KindString}},
			Title:  "{entity} {direction} {abs_change_pct:%.2f}%", Body: "Last price {ltp:%.2f}", Severity: "info",
		},
		{
			ID: "volume_spike", Description: "Volume far above its average",
			Fields: []event.FieldDef{num("relative_volume"), num("volume"), num("avg_volume")},
			Title:  "⚠ {entity} unusual volume", Body: "Volume is {relative_volume:%.1f}x normal", Severity: "notable",
		},
		{
			ID: "unusual_activity", Description: "Derived signal: big price move and volume spike together",
			Fields: []event.FieldDef{num("change_pct"), num("relative_volume")},
			Title:  "🔥 {entity} unusual activity", Body: "Price {change_pct:%+.2f}% with volume {relative_volume:%.1f}x normal", Severity: "high",
		},
	}
	for _, d := range defs {
		if err := reg.Register(d); err != nil {
			return err
		}
	}
	return nil
}

// RegisterAlertTemplates adds ready-made alert rules for the market example.
func RegisterAlertTemplates(t *alert.Templates) error {
	gte := func(field string, v float64) []rule.Condition { return []rule.Condition{{Field: field, Op: rule.Gte, Value: v}} }
	for _, x := range []alert.Template{
		{ID: "big-mover", Name: "Big mover", Description: "A watched symbol moves 2% or more between ticks.", ScopeKind: "entity", ValuesHint: "symbols, e.g. TCS",
			Types: []string{"price_changed"}, Conditions: gte("abs_change_pct", 2), CooldownSeconds: 600, Severity: "notable"},
		{ID: "volume-spike", Name: "Volume spike", Description: "Volume is at least 3x its average.", ScopeKind: "entity", ValuesHint: "symbols",
			Types: []string{"volume_spike"}, Conditions: gte("relative_volume", 3), CooldownSeconds: 900, Severity: "notable"},
		{ID: "unusual-activity", Name: "Unusual activity", Description: "A big move together with a volume spike.", ScopeKind: "entity", ValuesHint: "symbols",
			Types: []string{"unusual_activity"}, CooldownSeconds: 1800, Severity: "high"},
	} {
		if err := t.Register(x); err != nil {
			return err
		}
	}
	return nil
}

type nseTick struct {
	Symbol    string  `json:"symbol"`
	LTP       float64 `json:"ltp"`
	PrevLTP   float64 `json:"prev_ltp"`
	Volume    float64 `json:"volume"`
	AvgVolume float64 `json:"avg_volume"`
	TS        int64   `json:"ts"`
}

// NSETick normalizes tick messages into price_changed / volume_spike events.
type NSETick struct{ name string }

func NewNSETick(sourceName string) NSETick { return NSETick{name: sourceName} }
func (p NSETick) Source() string           { return p.name }

func (p NSETick) Process(msg source.RawMessage) ([]event.Event, error) {
	var t nseTick
	if err := json.Unmarshal(msg.Payload, &t); err != nil {
		return nil, fmt.Errorf("decode tick: %w", err)
	}
	sym := strings.ToUpper(strings.TrimSpace(t.Symbol))
	if sym == "" || t.LTP <= 0 || t.PrevLTP <= 0 || t.AvgVolume <= 0 {
		return nil, errors.New("invalid tick values")
	}
	ts := msg.ReceivedAt
	if t.TS > 0 {
		ts = time.UnixMilli(t.TS).UTC()
	}
	ents := []string{sym}
	var out []event.Event
	if chg := (t.LTP - t.PrevLTP) / t.PrevLTP * 100; chg != 0 {
		dir := "up"
		if chg < 0 {
			dir = "down"
		}
		out = append(out, event.New("price_changed", p.name, ents, ts, map[string]any{
			"ltp": t.LTP, "prev_ltp": t.PrevLTP, "change_pct": chg, "abs_change_pct": math.Abs(chg), "direction": dir,
		}))
	}
	if rel := t.Volume / t.AvgVolume; rel >= 1.5 {
		out = append(out, event.New("volume_spike", p.name, ents, ts, map[string]any{
			"relative_volume": rel, "volume": t.Volume, "avg_volume": t.AvgVolume,
		}))
	}
	return out, nil
}

type entityState struct {
	priceAt, volumeAt, signalAt time.Time
	changePct, relVolume        float64
}

// UnusualActivityDetector derives a signal from several events: a >=1% move plus
// a volume spike on the same entity within a window, with a cooldown.
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
	if len(e.Entities) == 0 {
		return
	}
	key := e.Entities[0]
	d.mu.Lock()
	st := d.state[key]
	if st == nil {
		st = &entityState{}
		d.state[key] = st
	}
	now := time.Now()
	switch e.Type {
	case "price_changed":
		if c := e.Float("change_pct"); math.Abs(c) >= 1.0 {
			st.priceAt, st.changePct = now, c
		}
	case "volume_spike":
		st.volumeAt, st.relVolume = now, e.Float("relative_volume")
	default:
		d.mu.Unlock()
		return
	}
	fire := now.Sub(st.priceAt) <= d.window && now.Sub(st.volumeAt) <= d.window && now.Sub(st.signalAt) >= d.cooldown
	var sig event.Event
	if fire {
		st.signalAt = now
		sig = event.New("unusual_activity", "signal", e.Entities, now.UTC(), map[string]any{
			"change_pct": st.changePct, "relative_volume": st.relVolume,
		})
	}
	d.mu.Unlock()
	if fire {
		_ = d.bus.Publish(ctx, sig)
	}
}

type symState struct {
	symbol string
	price  float64
	avgVol float64
}

// Simulator is a stand-in tick source for development.
type Simulator struct {
	interval time.Duration
	rng      *rand.Rand
	states   []symState
}

func NewSimulator(symbols []string, interval time.Duration) *Simulator {
	rng := rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 1))
	s := &Simulator{interval: interval, rng: rng}
	for _, sym := range symbols {
		s.states = append(s.states, symState{symbol: sym, price: 500 + rng.Float64()*2500, avgVol: 200_000 + rng.Float64()*2_000_000})
	}
	return s
}

func (s *Simulator) Name() string { return SourceName }

func (s *Simulator) Listen(ctx context.Context) <-chan source.RawMessage {
	out := make(chan source.RawMessage, 256)
	go func() {
		defer close(out)
		t := time.NewTicker(s.interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-t.C:
				for i := range s.states {
					msg := s.tick(&s.states[i], now)
					select {
					case out <- msg:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()
	return out
}

func (s *Simulator) tick(st *symState, now time.Time) source.RawMessage {
	ret := s.rng.NormFloat64() * 0.004
	if s.rng.Float64() < 0.05 {
		sign := 1.0
		if s.rng.Float64() < 0.5 {
			sign = -1
		}
		ret += sign * (0.01 + s.rng.Float64()*0.02)
	}
	prev := st.price
	st.price = math.Max(1, prev*(1+ret))
	vol := st.avgVol * (0.6 + 0.8*s.rng.Float64())
	if s.rng.Float64() < 0.08 {
		vol = st.avgVol * (2 + 2*s.rng.Float64())
	}
	payload, _ := json.Marshal(map[string]any{
		"symbol": st.symbol, "ltp": st.price, "prev_ltp": prev, "volume": vol, "avg_volume": st.avgVol, "ts": now.UnixMilli(),
	})
	return source.RawMessage{Source: SourceName, ReceivedAt: now, Payload: payload}
}
