// Package simulated is a stand-in market-data source for development. Replace
// it with a real NSE/BSE listener implementing source.Listener.
package simulated

import (
	"context"
	"encoding/json"
	"math"
	"math/rand/v2"
	"time"

	"tradepulse/internal/source"
)

const Name = "NSE_SIM"

type state struct {
	symbol string
	price  float64
	avgVol float64
}

type Source struct {
	interval time.Duration
	rng      *rand.Rand
	states   []state
}

func New(symbols []string, interval time.Duration) *Source {
	rng := rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 1))
	s := &Source{interval: interval, rng: rng}
	for _, sym := range symbols {
		s.states = append(s.states, state{
			symbol: sym,
			price:  500 + rng.Float64()*2500,
			avgVol: 200_000 + rng.Float64()*2_000_000,
		})
	}
	return s
}

func (s *Source) Name() string { return Name }

func (s *Source) Listen(ctx context.Context) <-chan source.RawMessage {
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

func (s *Source) tick(st *state, now time.Time) source.RawMessage {
	ret := s.rng.NormFloat64() * 0.004
	if s.rng.Float64() < 0.05 { // occasional shock
		sign := 1.0
		if s.rng.Float64() < 0.5 {
			sign = -1
		}
		ret += sign * (0.01 + s.rng.Float64()*0.02)
	}
	prev := st.price
	st.price = math.Max(1, prev*(1+ret))
	vol := st.avgVol * (0.6 + 0.8*s.rng.Float64())
	if s.rng.Float64() < 0.08 { // occasional volume spike
		vol = st.avgVol * (2 + 2*s.rng.Float64())
	}
	payload, _ := json.Marshal(map[string]any{
		"symbol": st.symbol, "ltp": st.price, "prev_ltp": prev,
		"volume": vol, "avg_volume": st.avgVol, "ts": now.UnixMilli(),
	})
	return source.RawMessage{Source: Name, ReceivedAt: now, Payload: payload}
}
