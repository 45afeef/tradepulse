// Package processing turns RawMessages into normalized Events and derives signals.
package processing

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"tradepulse/internal/domain/event"
	"tradepulse/internal/source"
)

// Processor converts a source-specific RawMessage into zero or more Events.
type Processor interface {
	Source() string
	Process(msg source.RawMessage) ([]event.Event, error)
}

type nseTick struct {
	Symbol    string  `json:"symbol"`
	LTP       float64 `json:"ltp"`
	PrevLTP   float64 `json:"prev_ltp"`
	Volume    float64 `json:"volume"`
	AvgVolume float64 `json:"avg_volume"`
	TS        int64   `json:"ts"`
}

// NSETick normalizes equity tick messages.
type NSETick struct{ name string }

func NewNSETick(sourceName string) NSETick { return NSETick{name: sourceName} }

func (p NSETick) Source() string { return p.name }

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
	var out []event.Event
	if chg := (t.LTP - t.PrevLTP) / t.PrevLTP * 100; chg != 0 {
		out = append(out, event.New(event.PriceChanged, sym, p.name, ts, map[string]any{
			"ltp": t.LTP, "prev_ltp": t.PrevLTP, "change_pct": chg,
		}))
	}
	if rel := t.Volume / t.AvgVolume; rel >= 1.5 {
		out = append(out, event.New(event.VolumeSpike, sym, p.name, ts, map[string]any{
			"relative_volume": rel, "volume": t.Volume, "avg_volume": t.AvgVolume,
		}))
	}
	return out, nil
}
