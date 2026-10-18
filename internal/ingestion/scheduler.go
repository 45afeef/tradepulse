package ingestion

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"net/url"
	"sync"
	"time"

	"tradepulse/internal/connector"
	"tradepulse/internal/domain/event"
	"tradepulse/internal/domain/sourcedef"
	"tradepulse/internal/idgen"
)

type SchedulerConfig struct {
	Tick          time.Duration // how often to look for due sources (default 5s)
	MaxParallel   int           // concurrent fetches (default 4)
	RunTimeout    time.Duration // per fetch (default 30s)
	MaxItemAge    time.Duration // older items are stored but never published (default 48h)
	HostGap       time.Duration // minimum gap between requests to the same host (default 2s)
	Retention     time.Duration // runs and raw items are pruned after this (default 30d)
	FailureRetry  time.Duration // first retry delay after a failure, doubled each time (default 30s)
	Bounds        sourcedef.Bounds
	Now           func() time.Time
	jitterDisable bool
}

func (c *SchedulerConfig) defaults() {
	if c.Tick <= 0 {
		c.Tick = 5 * time.Second
	}
	if c.MaxParallel <= 0 {
		c.MaxParallel = 4
	}
	if c.RunTimeout <= 0 {
		c.RunTimeout = 30 * time.Second
	}
	if c.MaxItemAge <= 0 {
		c.MaxItemAge = 48 * time.Hour
	}
	if c.HostGap < 0 {
		c.HostGap = 0
	} else if c.HostGap == 0 {
		c.HostGap = 2 * time.Second
	}
	if c.Retention <= 0 {
		c.Retention = 30 * 24 * time.Hour
	}
	if c.FailureRetry <= 0 {
		c.FailureRetry = 30 * time.Second
	}
	if c.Bounds.Min <= 0 {
		c.Bounds.Min = 60 * time.Second
	}
	if c.Bounds.Max < c.Bounds.Min {
		c.Bounds.Max = 24 * time.Hour
	}
	if c.Now == nil {
		c.Now = time.Now
	}
}

// Scheduler polls every enabled source on its OWN interval. Sources run
// independently (bounded parallelism, per-host spacing), so one slow or failing
// source never delays the others.
type Scheduler struct {
	log     *slog.Logger
	sources sourcedef.Repository
	runs    sourcedef.RunRepository
	items   sourcedef.ItemStore
	conns   *connector.Registry
	bus     event.Bus
	reg     *event.Registry
	cfg     SchedulerConfig

	mu         sync.Mutex
	inflight   map[string]bool
	lastHealth map[string]string
	hostSlot   map[string]time.Time
	wg         sync.WaitGroup
	sem        chan struct{}
}

func NewScheduler(log *slog.Logger, sources sourcedef.Repository, runs sourcedef.RunRepository, items sourcedef.ItemStore,
	conns *connector.Registry, bus event.Bus, reg *event.Registry, cfg SchedulerConfig) *Scheduler {
	cfg.defaults()
	return &Scheduler{
		log: log, sources: sources, runs: runs, items: items, conns: conns, bus: bus, reg: reg, cfg: cfg,
		inflight: map[string]bool{}, lastHealth: map[string]string{}, hostSlot: map[string]time.Time{},
		sem: make(chan struct{}, cfg.MaxParallel),
	}
}

// Run ticks until ctx is cancelled, then waits for in-flight fetches.
func (s *Scheduler) Run(ctx context.Context) {
	t := time.NewTicker(s.cfg.Tick)
	defer t.Stop()
	prune := time.NewTicker(time.Hour)
	defer prune.Stop()
	s.Tick(ctx)
	for {
		select {
		case <-ctx.Done():
			s.Wait()
			return
		case <-t.C:
			s.Tick(ctx)
		case <-prune.C:
			s.prune(ctx)
		}
	}
}

// Wait blocks until all launched fetches have finished (used on shutdown and in tests).
func (s *Scheduler) Wait() { s.wg.Wait() }

func (s *Scheduler) prune(ctx context.Context) {
	cutoff := s.cfg.Now().Add(-s.cfg.Retention)
	if n, err := s.runs.Prune(ctx, cutoff); err != nil {
		s.log.Error("prune source runs", "err", err)
	} else if n > 0 {
		s.log.Info("pruned source runs", "deleted", n)
	}
	if n, err := s.items.Prune(ctx, cutoff); err != nil {
		s.log.Error("prune source items", "err", err)
	} else if n > 0 {
		s.log.Info("pruned raw items", "deleted", n)
	}
}

// Tick launches every due source (non-blocking) and runs the silence watchdog.
func (s *Scheduler) Tick(ctx context.Context) {
	list, err := s.sources.List(ctx)
	if err != nil {
		s.log.Error("list sources", "err", err)
		return
	}
	now := s.cfg.Now()
	for _, src := range list {
		s.watchdog(src, now)
		if !src.Enabled || src.Type == sourcedef.TypeManual || src.Type == sourcedef.TypeWebhook {
			continue
		}
		if src.State.NextRunAt != nil && now.Before(*src.State.NextRunAt) {
			continue
		}
		if !s.begin(src.ID) {
			continue
		}
		s.wg.Add(1)
		go func(src sourcedef.Source) {
			defer s.wg.Done()
			defer s.end(src.ID)
			select {
			case s.sem <- struct{}{}:
				defer func() { <-s.sem }()
			case <-ctx.Done():
				return
			}
			s.RunOnce(ctx, src)
		}(src)
	}
}

func (s *Scheduler) begin(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inflight[id] {
		return false
	}
	s.inflight[id] = true
	return true
}

func (s *Scheduler) end(id string) {
	s.mu.Lock()
	delete(s.inflight, id)
	s.mu.Unlock()
}

// watchdog logs when a source's health changes to a bad state (once per transition).
func (s *Scheduler) watchdog(src sourcedef.Source, now time.Time) {
	h := src.Health(now)
	s.mu.Lock()
	prev := s.lastHealth[src.ID]
	s.lastHealth[src.ID] = h
	s.mu.Unlock()
	if h != prev && (h == "failing" || h == "overdue") {
		s.log.Warn("source unhealthy", "source", src.Name, "health", h, "failures", src.State.ConsecutiveFailures, "last_error", src.State.LastError)
	}
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// waitHost spaces requests to the same host by HostGap.
func (s *Scheduler) waitHost(ctx context.Context, host string) error {
	if host == "" || s.cfg.HostGap == 0 {
		return nil
	}
	s.mu.Lock()
	now := time.Now()
	slot := s.hostSlot[host]
	if slot.Before(now) {
		slot = now
	}
	s.hostSlot[host] = slot.Add(s.cfg.HostGap)
	s.mu.Unlock()
	d := time.Until(slot)
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// RunOnce fetches one source, dedupes, publishes new items and records the outcome.
func (s *Scheduler) RunOnce(ctx context.Context, src sourcedef.Source) {
	started := s.cfg.Now()
	rctx, cancel := context.WithTimeout(ctx, s.cfg.RunTimeout)
	defer cancel()
	run := sourcedef.Run{ID: idgen.New(), SourceID: src.ID, StartedAt: started}

	conn, ok := s.conns.Get(src.Type)
	if !ok {
		s.finishFailure(ctx, src, run, started, 0, "no connector for type "+string(src.Type))
		return
	}
	if err := s.waitHost(rctx, hostOf(src.URL)); err != nil {
		s.finishFailure(ctx, src, run, started, 0, err.Error())
		return
	}
	res, err := conn.Fetch(rctx, src)
	if err != nil {
		s.finishFailure(ctx, src, run, started, res.HTTPStatus, err.Error())
		return
	}

	baseline := src.State.LastSuccessAt == nil // first success: remember items, do not flood subscribers
	now := s.cfg.Now()
	run.HTTPStatus, run.ItemsSeen = res.HTTPStatus, len(res.Items)
	for _, it := range res.Items {
		inserted, err := s.items.Insert(ctx, src.ID, it.Key, it.Raw, now)
		if err != nil { // storage problem: do not advance state; retry later
			s.finishFailure(ctx, src, run, started, res.HTTPStatus, "storage error: "+err.Error())
			return
		}
		if !inserted {
			continue
		}
		run.ItemsNew++
		if baseline {
			continue
		}
		if !it.PublishedAt.IsZero() && now.Sub(it.PublishedAt) > s.cfg.MaxItemAge {
			continue // stale item that merely appeared on the page
		}
		ev := connector.ItemEvent(src, it, now)
		if err := s.reg.Validate(ev); err != nil {
			s.log.Warn("dropping invalid item event", "source", src.Name, "err", err)
			continue
		}
		if err := s.bus.Publish(ctx, ev); err != nil {
			s.log.Warn("publish failed", "source", src.Name, "err", err)
			continue
		}
		run.ItemsPublished++
	}

	st := src.State
	st.LastRunAt, st.LastSuccessAt = &now, &now
	st.LastError, st.ConsecutiveFailures = "", 0
	if res.Cursor != "" {
		st.Cursor = res.Cursor
	}
	if res.ETag != "" || res.LastModified != "" {
		st.ETag, st.LastModified = res.ETag, res.LastModified
	}
	if run.ItemsNew > 0 {
		st.LastItemAt, st.UnchangedStreak = &now, 0
		st.ItemsTotal += int64(run.ItemsNew)
	} else {
		st.UnchangedStreak++
	}
	next := now.Add(s.jitter(NextInterval(time.Duration(src.IntervalSeconds)*time.Second, st.UnchangedStreak, s.cfg.Bounds.Max)))
	st.NextRunAt = &next
	run.OK, run.DurationMS = true, int(s.cfg.Now().Sub(started).Milliseconds())
	s.save(ctx, src, st, run)
}

func (s *Scheduler) finishFailure(ctx context.Context, src sourcedef.Source, run sourcedef.Run, started time.Time, status int, msg string) {
	now := s.cfg.Now()
	st := src.State
	st.LastRunAt = &now
	st.LastError = clip(msg, 300)
	st.ConsecutiveFailures++
	next := now.Add(s.jitter(FailureDelay(time.Duration(src.IntervalSeconds)*time.Second, st.ConsecutiveFailures, s.cfg.FailureRetry, s.cfg.Bounds.Min)))
	st.NextRunAt = &next
	run.OK, run.HTTPStatus, run.Error = false, status, st.LastError
	run.DurationMS = int(now.Sub(started).Milliseconds())
	s.save(ctx, src, st, run)
	s.log.Warn("source fetch failed", "source", src.Name, "failures", st.ConsecutiveFailures, "err", st.LastError)
}

func (s *Scheduler) save(ctx context.Context, src sourcedef.Source, st sourcedef.State, run sourcedef.Run) {
	if err := s.sources.SaveState(ctx, src.ID, st); err != nil {
		s.log.Error("save source state", "source", src.Name, "err", err)
	}
	if err := s.runs.Add(ctx, run); err != nil {
		s.log.Error("record source run", "source", src.Name, "err", err)
	}
}

func (s *Scheduler) jitter(d time.Duration) time.Duration {
	if s.cfg.jitterDisable || d <= 0 {
		return d
	}
	return d + time.Duration((rand.Float64()*0.2-0.1)*float64(d)) // ±10% so sources do not fire in lockstep
}

// NextInterval stretches the delay for sources that keep returning nothing new
// (x2 after 10 unchanged runs, x4 after 30), never beyond max.
func NextInterval(base time.Duration, unchanged int, max time.Duration) time.Duration {
	d := base
	switch {
	case unchanged >= 30:
		d = base * 4
	case unchanged >= 10:
		d = base * 2
	}
	if d > max {
		d = max
	}
	if d < base {
		d = base
	}
	return d
}

// FailureDelay is the retry delay after the n-th consecutive failure: starts at
// firstRetry, doubles each time, never above the source's own interval and never
// below the global minimum.
func FailureDelay(base time.Duration, failures int, firstRetry, min time.Duration) time.Duration {
	if failures < 1 {
		failures = 1
	}
	shift := failures - 1
	if shift > 16 {
		shift = 16
	}
	d := firstRetry << shift
	if d > base {
		d = base
	}
	if d < min {
		d = min
	}
	return d
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
