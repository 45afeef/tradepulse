package eventbus

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"tradepulse/internal/domain/event"
)

func newBus(buf int) *Memory { return NewMemory(buf, slog.New(slog.NewTextHandler(io.Discard, nil))) }

func ev(typ string) event.Event { return event.New(typ, "S", nil, time.Now(), nil) }

func TestPublishDeliversToAllSubscribers(t *testing.T) {
	b := newBus(8)
	defer b.Close()
	var n atomic.Int32
	for i := 0; i < 3; i++ {
		b.Subscribe("s", func(context.Context, event.Event) { n.Add(1) })
	}
	_ = b.Publish(context.Background(), ev("content_item"))
	deadline := time.Now().Add(time.Second)
	for n.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if n.Load() != 3 {
		t.Fatalf("delivered to %d subscribers", n.Load())
	}
}

func TestSlowSubscriberNeverBlocksPublisher(t *testing.T) {
	b := newBus(1)
	release := make(chan struct{})
	b.Subscribe("slow", func(context.Context, event.Event) { <-release })
	done := make(chan struct{})
	go func() {
		for i := 0; i < 20; i++ {
			_ = b.Publish(context.Background(), ev("content_item"))
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("publisher blocked by slow subscriber")
	}
	if b.dropped.Load() == 0 {
		t.Error("expected drops to be counted")
	}
	close(release)
	b.Close()
}

func TestSubscriberPanicIsIsolated(t *testing.T) {
	b := newBus(8)
	defer b.Close()
	var ok atomic.Int32
	b.Subscribe("panicky", func(_ context.Context, e event.Event) {
		if e.Type == "boom" {
			panic("boom")
		}
		ok.Add(1)
	})
	_ = b.Publish(context.Background(), ev("boom"))
	_ = b.Publish(context.Background(), ev("fine"))
	deadline := time.Now().Add(time.Second)
	for ok.Load() < 1 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if ok.Load() != 1 {
		t.Fatal("subscriber died after a panic")
	}
}

func TestUnsubscribeAndClose(t *testing.T) {
	b := newBus(8)
	var n atomic.Int32
	unsub := b.Subscribe("s", func(context.Context, event.Event) { n.Add(1) })
	unsub()
	unsub() // idempotent
	_ = b.Publish(context.Background(), ev("content_item"))
	time.Sleep(50 * time.Millisecond)
	if n.Load() != 0 {
		t.Fatal("unsubscribed handler still called")
	}
	b.Close()
	b.Close() // idempotent
	if err := b.Publish(context.Background(), ev("content_item")); err != ErrClosed {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}
