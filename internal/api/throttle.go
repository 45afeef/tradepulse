package api

import (
	"context"
	"sync"
	"time"
)

type tEntry struct {
	fails       int
	first       time.Time
	lockedUntil time.Time
}

// throttle locks an account key after too many failures in a window. It is keyed by
// the submitted identifier, whether or not the account exists, so lockouts reveal
// nothing. Trade-off (documented): an attacker can lock a known email out for a
// while; the per-IP limiter and the lock duration bound the damage. In memory:
// per instance, reset on restart.
type throttle struct {
	mu     sync.Mutex
	m      map[string]*tEntry
	max    int
	window time.Duration
	lock   time.Duration
	now    func() time.Time
}

func newThrottle(max int, window, lock time.Duration) *throttle {
	return &throttle{m: map[string]*tEntry{}, max: max, window: window, lock: lock, now: time.Now}
}

// blocked returns the remaining lock time (0 if the key may try).
func (t *throttle) blocked(key string) time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	if e, ok := t.m[key]; ok && t.now().Before(e.lockedUntil) {
		return e.lockedUntil.Sub(t.now())
	}
	return 0
}

// fail records a failure and reports whether the key just became locked.
func (t *throttle) fail(key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	e, ok := t.m[key]
	if !ok {
		if len(t.m) >= 100000 { // bound memory under attack: stop tracking new keys, the IP limiter still applies
			return false
		}
		e = &tEntry{first: now}
		t.m[key] = e
	}
	if now.Sub(e.first) > t.window || now.After(e.lockedUntil) && !e.lockedUntil.IsZero() {
		e.fails, e.first, e.lockedUntil = 0, now, time.Time{}
	}
	e.fails++
	if e.fails >= t.max {
		e.lockedUntil = now.Add(t.lock)
		return true
	}
	return false
}

func (t *throttle) reset(key string) {
	t.mu.Lock()
	delete(t.m, key)
	t.mu.Unlock()
}

func (t *throttle) sweep(ctx context.Context) {
	tick := time.NewTicker(5 * time.Minute)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-tick.C:
			t.mu.Lock()
			for k, e := range t.m {
				if now.Sub(e.first) > t.window && now.After(e.lockedUntil) {
					delete(t.m, k)
				}
			}
			t.mu.Unlock()
		}
	}
}
