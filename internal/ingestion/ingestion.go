// Package ingestion runs source listeners and pushes validated events to the bus.
package ingestion

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"tradepulse/internal/domain/event"
	"tradepulse/internal/processing"
	"tradepulse/internal/source"
)

// Run supervises every listener (restarting with backoff if its feed ends) and
// blocks until ctx is cancelled and all listeners have stopped.
func Run(ctx context.Context, log *slog.Logger, bus event.Bus, reg *event.Registry, listeners []source.Listener, procs map[string]processing.Processor) {
	var wg sync.WaitGroup
	for _, l := range listeners {
		p, ok := procs[l.Name()]
		if !ok {
			log.Error("no processor registered for source", "source", l.Name())
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			supervise(ctx, log, bus, reg, l, p)
		}()
	}
	wg.Wait()
}

func supervise(ctx context.Context, log *slog.Logger, bus event.Bus, reg *event.Registry, l source.Listener, p processing.Processor) {
	backoff := time.Second
	for ctx.Err() == nil {
		started := time.Now()
		consume(ctx, log, bus, reg, l, p)
		if ctx.Err() != nil {
			return
		}
		if time.Since(started) > time.Minute {
			backoff = time.Second
		}
		log.Warn("source feed ended; restarting", "source", l.Name(), "in", backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff *= 2; backoff > 30*time.Second {
			backoff = 30 * time.Second
		}
	}
}

func consume(ctx context.Context, log *slog.Logger, bus event.Bus, reg *event.Registry, l source.Listener, p processing.Processor) {
	for msg := range l.Listen(ctx) {
		events, err := p.Process(msg)
		if err != nil {
			log.Warn("dropping malformed message", "source", l.Name(), "err", err)
			continue
		}
		for _, e := range events {
			if err := reg.Validate(e); err != nil {
				log.Warn("dropping invalid event", "source", l.Name(), "type", e.Type, "err", err)
				continue
			}
			if err := bus.Publish(ctx, e); err != nil {
				return
			}
		}
	}
}
