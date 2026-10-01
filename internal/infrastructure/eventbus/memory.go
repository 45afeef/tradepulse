// Package eventbus contains event.Bus implementations.
package eventbus

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"

	"tradepulse/internal/domain/event"
)

var ErrClosed = errors.New("event bus closed")

type subscription struct {
	name string
	ch   chan event.Event
}

// Memory is an in-process bus. Every subscriber has its own bounded queue and
// goroutine, so a slow subscriber never blocks publishers or other subscribers;
// when a queue is full the event is dropped for that subscriber and counted.
type Memory struct {
	log     *slog.Logger
	buf     int
	mu      sync.RWMutex
	subs    map[int]*subscription
	next    int
	closed  bool
	wg      sync.WaitGroup
	dropped atomic.Uint64
}

func NewMemory(buffer int, log *slog.Logger) *Memory {
	return &Memory{log: log, buf: buffer, subs: map[int]*subscription{}}
}

func (b *Memory) Publish(_ context.Context, e event.Event) error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.closed {
		return ErrClosed
	}
	for _, s := range b.subs {
		select {
		case s.ch <- e:
		default:
			n := b.dropped.Add(1)
			b.log.Warn("event dropped: subscriber queue full", "subscriber", s.name, "total_dropped", n)
		}
	}
	return nil
}

func (b *Memory) Subscribe(name string, h event.Handler) func() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return func() {}
	}
	id := b.next
	b.next++
	s := &subscription{name: name, ch: make(chan event.Event, b.buf)}
	b.subs[id] = s
	b.wg.Add(1)
	b.mu.Unlock()

	go func() {
		defer b.wg.Done()
		for e := range s.ch {
			b.call(s, h, e)
		}
	}()

	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if _, ok := b.subs[id]; ok {
			delete(b.subs, id)
			close(s.ch)
		}
	}
}

func (b *Memory) call(s *subscription, h event.Handler, e event.Event) {
	defer func() {
		if r := recover(); r != nil {
			b.log.Error("subscriber panic", "subscriber", s.name, "panic", r)
		}
	}()
	h(context.Background(), e)
}

// Close stops accepting events and waits for subscribers to drain.
func (b *Memory) Close() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	for id, s := range b.subs {
		delete(b.subs, id)
		close(s.ch)
	}
	b.mu.Unlock()
	b.wg.Wait()
}
