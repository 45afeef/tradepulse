package ingestion

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"tradepulse/internal/domain/event"
	"tradepulse/internal/infrastructure/eventbus"
	"tradepulse/internal/processing"
	"tradepulse/internal/source"
)

type fakeListener struct{ msgs []string }

func (f fakeListener) Name() string { return "FAKE" }
func (f fakeListener) Listen(context.Context) <-chan source.RawMessage {
	ch := make(chan source.RawMessage, len(f.msgs))
	for _, m := range f.msgs {
		ch <- source.RawMessage{Source: "FAKE", Payload: []byte(m)}
	}
	close(ch)
	return ch
}

type fakeProc struct{}

func (fakeProc) Source() string { return "FAKE" }
func (fakeProc) Process(m source.RawMessage) ([]event.Event, error) {
	switch string(m.Payload) {
	case "ok":
		return []event.Event{event.New("content_item", "FAKE", []string{"x"}, time.Now(), nil)}, nil
	case "badtype":
		return []event.Event{event.New("nope", "FAKE", nil, time.Now(), nil)}, nil
	}
	return nil, errors.New("malformed")
}

func TestMalformedAndInvalidEventsAreDropped(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	bus := eventbus.NewMemory(16, log)
	defer bus.Close()
	got := make(chan event.Event, 8)
	bus.Subscribe("t", func(_ context.Context, e event.Event) { got <- e })

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		Run(ctx, log, bus, event.NewRegistry(),
			[]source.Listener{fakeListener{msgs: []string{"ok", "badtype", "garbage", "ok"}}},
			map[string]processing.Processor{"FAKE": fakeProc{}})
		close(done)
	}()

	for i := 0; i < 2; i++ {
		select {
		case <-got:
		case <-time.After(2 * time.Second):
			t.Fatal("expected two valid events")
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after cancel")
	}
	if len(got) != 0 {
		t.Fatalf("invalid events leaked through: %d extra", len(got))
	}
}
