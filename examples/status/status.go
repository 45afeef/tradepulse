// Package status is a second EXAMPLE domain (service health) proving the core is
// domain-agnostic: same pipeline, different event type, zero core changes.
package status

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	"tradepulse/internal/domain/alert"
	"tradepulse/internal/domain/event"
	"tradepulse/internal/domain/rule"
	"tradepulse/internal/source"
)

const SourceName = "STATUS_SIM"

func RegisterTypes(reg *event.Registry) error {
	return reg.Register(event.TypeDef{
		ID: "service_status", Description: "A service's health status",
		Fields: []event.FieldDef{
			{Name: "status", Kind: event.KindString, Doc: "up | degraded | down"},
			{Name: "latency_ms", Kind: event.KindNumber},
		},
		Title: "{entity} is {status}", Body: "Latency {latency_ms:%.0f} ms", Severity: "notable",
	})
}

// RegisterAlertTemplates adds a ready-made alert rule for the status example.
func RegisterAlertTemplates(t *alert.Templates) error {
	return t.Register(alert.Template{ID: "service-not-up", Name: "Service not up", Description: "A watched service reports degraded or down.", ScopeKind: "entity",
		ValuesHint: "service names, e.g. checkout", Types: []string{"service_status"},
		Conditions: []rule.Condition{{Field: "status", Op: rule.Neq, Value: "up"}}, CooldownSeconds: 300, Severity: "high"})
}

type Processor struct{}

func NewProcessor() Processor       { return Processor{} }
func (Processor) Source() string    { return SourceName }

func (Processor) Process(msg source.RawMessage) ([]event.Event, error) {
	var m struct {
		Service   string  `json:"service"`
		Status    string  `json:"status"`
		LatencyMS float64 `json:"latency_ms"`
		TS        int64   `json:"ts"`
	}
	if err := json.Unmarshal(msg.Payload, &m); err != nil {
		return nil, fmt.Errorf("decode status: %w", err)
	}
	svc := strings.TrimSpace(m.Service)
	st := strings.ToLower(strings.TrimSpace(m.Status))
	if svc == "" || (st != "up" && st != "degraded" && st != "down") || m.LatencyMS < 0 {
		return nil, errors.New("invalid status message")
	}
	ts := msg.ReceivedAt
	if m.TS > 0 {
		ts = time.UnixMilli(m.TS).UTC()
	}
	return []event.Event{event.New("service_status", SourceName, []string{svc}, ts,
		map[string]any{"status": st, "latency_ms": m.LatencyMS})}, nil
}

type Simulator struct {
	services []string
	interval time.Duration
	rng      *rand.Rand
}

func NewSimulator(interval time.Duration) *Simulator {
	return &Simulator{
		services: []string{"checkout", "search", "payments", "auth"},
		interval: interval,
		rng:      rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 2)),
	}
}

func (s *Simulator) Name() string { return SourceName }

func (s *Simulator) Listen(ctx context.Context) <-chan source.RawMessage {
	out := make(chan source.RawMessage, 64)
	go func() {
		defer close(out)
		t := time.NewTicker(s.interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-t.C:
				st := "up"
				switch r := s.rng.Float64(); {
				case r > 0.95:
					st = "down"
				case r > 0.85:
					st = "degraded"
				}
				payload, _ := json.Marshal(map[string]any{
					"service": s.services[s.rng.IntN(len(s.services))], "status": st,
					"latency_ms": 40 + s.rng.Float64()*900, "ts": now.UnixMilli(),
				})
				select {
				case out <- source.RawMessage{Source: SourceName, ReceivedAt: now, Payload: payload}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out
}
