package market

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"tradepulse/internal/domain/event"
	"tradepulse/internal/infrastructure/eventbus"
	"tradepulse/internal/source"
)

func TestRegisterTypes(t *testing.T) {
	if err := RegisterTypes(event.NewRegistry()); err != nil {
		t.Fatal(err)
	}
}

func TestNSETickProcess(t *testing.T) {
	reg := event.NewRegistry()
	if err := RegisterTypes(reg); err != nil {
		t.Fatal(err)
	}
	msg := source.RawMessage{Source: SourceName, ReceivedAt: time.Now(),
		Payload: []byte(`{"symbol":"reliance","ltp":101,"prev_ltp":100,"volume":3000,"avg_volume":1000}`)}
	evs, err := NewNSETick(SourceName).Process(msg)
	if err != nil || len(evs) != 2 {
		t.Fatalf("expected 2 events, got %d (%v)", len(evs), err)
	}
	for _, e := range evs {
		if err := reg.Validate(e); err != nil {
			t.Fatalf("event %s invalid: %v", e.Type, err)
		}
	}
	title, body, _ := reg.Render(evs[0])
	if title != "RELIANCE up 1.00%" || body != "Last price 101.00" {
		t.Fatalf("unexpected render: %q / %q", title, body)
	}
}

func TestNSETickRejectsInvalid(t *testing.T) {
	bad := []string{
		`not json`,
		`{"symbol":"","ltp":1,"prev_ltp":1,"avg_volume":1}`,
		`{"symbol":"X","ltp":0,"prev_ltp":1,"avg_volume":1}`,
		`{"symbol":"X","ltp":1,"prev_ltp":0,"avg_volume":1}`,
		`{"symbol":"X","ltp":1,"prev_ltp":1,"avg_volume":0}`,
	}
	for _, b := range bad {
		if _, err := NewNSETick(SourceName).Process(source.RawMessage{Payload: []byte(b)}); err == nil {
			t.Errorf("expected error for %s", b)
		}
	}
}

func TestDetectorFiresOnceWithCooldown(t *testing.T) {
	bus := eventbus.NewMemory(16, slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer bus.Close()
	got := make(chan event.Event, 8)
	bus.Subscribe("test", func(_ context.Context, e event.Event) { got <- e })

	d := NewUnusualActivityDetector(bus, time.Minute, time.Hour)
	price := event.New("price_changed", "S", []string{"TCS"}, time.Now(), map[string]any{"change_pct": 2.5})
	vol := event.New("volume_spike", "S", []string{"TCS"}, time.Now(), map[string]any{"relative_volume": 3.0})
	d.Handle(context.Background(), price)
	d.Handle(context.Background(), vol)

	select {
	case e := <-got:
		if e.Type != "unusual_activity" || e.Entities[0] != "TCS" {
			t.Fatalf("unexpected signal %+v", e)
		}
	case <-time.After(time.Second):
		t.Fatal("signal not published")
	}
	d.Handle(context.Background(), price)
	d.Handle(context.Background(), vol)
	select {
	case e := <-got:
		t.Fatalf("cooldown violated: %+v", e)
	case <-time.After(150 * time.Millisecond):
	}
}
