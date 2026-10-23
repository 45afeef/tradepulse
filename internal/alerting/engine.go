// Package alerting evaluates user alert rules against the event stream and delivers
// fires to channels (in-app and signed outbound webhooks).
package alerting

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"tradepulse/internal/domain/alert"
	"tradepulse/internal/domain/event"
	"tradepulse/internal/domain/rule"
	"tradepulse/internal/idgen"
	"tradepulse/internal/metrics"
)

// Notifier pushes a live message to a user's connections (delivery.Hub).
type Notifier interface {
	Notify(ctx context.Context, userID, kind string, data any)
}

// Dispatcher queues a fire for an outbound channel (Sender).
type Dispatcher interface {
	Enqueue(userID, channelID string, f alert.Fire)
}

type compiled struct {
	r      alert.Rule
	scope  map[string]struct{}
	types  map[string]struct{}
	tokens []string // keyword: first token of each phrase, for indexing
}

type Config struct {
	MaxFiresPerHour int // auto-pause threshold (default 30)
	Now             func() time.Time
	RingSize        int // recent events kept for previews (default 1000)
	Metrics         *metrics.Registry
}

type Engine struct {
	log   *slog.Logger
	rules alert.RuleRepository
	fires alert.FireRepository
	reg   *event.Registry
	out   Notifier
	disp  Dispatcher
	cfg   Config

	mu       sync.RWMutex
	byID     map[string]*compiled
	byUser   map[string]map[string]struct{}
	byEntity map[string][]*compiled
	bySource map[string][]*compiled
	byToken  map[string][]*compiled

	stMu      sync.Mutex
	lastFired map[string]time.Time
	fireTimes map[string][]time.Time
	ring      []event.Event
}

func NewEngine(log *slog.Logger, rules alert.RuleRepository, fires alert.FireRepository, reg *event.Registry, out Notifier, disp Dispatcher, cfg Config) *Engine {
	if cfg.MaxFiresPerHour <= 0 {
		cfg.MaxFiresPerHour = 30
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.RingSize <= 0 {
		cfg.RingSize = 1000
	}
	return &Engine{
		log: log, rules: rules, fires: fires, reg: reg, out: out, disp: disp, cfg: cfg,
		byID: map[string]*compiled{}, byUser: map[string]map[string]struct{}{},
		byEntity: map[string][]*compiled{}, bySource: map[string][]*compiled{}, byToken: map[string][]*compiled{},
		lastFired: map[string]time.Time{}, fireTimes: map[string][]time.Time{},
	}
}

func compile(r alert.Rule) *compiled {
	c := &compiled{r: r, scope: map[string]struct{}{}}
	for _, v := range r.Spec.Scope.Values {
		c.scope[v] = struct{}{}
		if r.Spec.Scope.Kind == "keyword" {
			c.tokens = append(c.tokens, firstToken(v))
		}
	}
	if len(r.Spec.Types) > 0 {
		c.types = map[string]struct{}{}
		for _, t := range r.Spec.Types {
			c.types[t] = struct{}{}
		}
	}
	return c
}

func firstToken(p string) string {
	if i := strings.IndexByte(p, ' '); i >= 0 {
		return p[:i]
	}
	return p
}

// Load indexes every enabled rule whose owner isActive.
func (e *Engine) Load(ctx context.Context, isActive func(userID string) bool) error {
	rs, err := e.rules.ListEnabled(ctx)
	if err != nil {
		return err
	}
	for _, r := range rs {
		if isActive == nil || isActive(r.UserID) {
			e.UpsertRule(r)
		}
		if r.LastFiredAt != nil {
			e.stMu.Lock()
			e.lastFired[r.ID] = *r.LastFiredAt
			e.stMu.Unlock()
		}
	}
	return nil
}

// UpsertRule (re)indexes an enabled rule; disabled rules are removed.
func (e *Engine) UpsertRule(r alert.Rule) {
	e.stMu.Lock()
	delete(e.fireTimes, r.ID) // saving/re-enabling a rule starts a fresh hourly count
	e.stMu.Unlock()
	e.mu.Lock()
	defer e.mu.Unlock()
	e.removeLocked(r.ID)
	if !r.Spec.Enabled {
		return
	}
	c := compile(r)
	e.byID[r.ID] = c
	if e.byUser[r.UserID] == nil {
		e.byUser[r.UserID] = map[string]struct{}{}
	}
	e.byUser[r.UserID][r.ID] = struct{}{}
	switch r.Spec.Scope.Kind {
	case "entity":
		for v := range c.scope {
			e.byEntity[v] = append(e.byEntity[v], c)
		}
	case "source":
		for v := range c.scope {
			e.bySource[v] = append(e.bySource[v], c)
		}
	case "keyword":
		for _, t := range c.tokens {
			e.byToken[t] = append(e.byToken[t], c)
		}
	}
}

func (e *Engine) RemoveRule(id string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.removeLocked(id)
}

// RemoveUser stops all of a user's rules (suspension) without deleting them.
func (e *Engine) RemoveUser(userID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for id := range e.byUser[userID] {
		e.removeLocked(id)
	}
}

// LoadUser re-indexes a user's enabled rules (reactivation).
func (e *Engine) LoadUser(ctx context.Context, userID string) error {
	rs, err := e.rules.List(ctx, userID)
	if err != nil {
		return err
	}
	for _, r := range rs {
		e.UpsertRule(r)
	}
	return nil
}

func drop(m map[string][]*compiled, id string) {
	for k, list := range m {
		kept := make([]*compiled, 0, len(list))
		for _, c := range list {
			if c.r.ID != id {
				kept = append(kept, c)
			}
		}
		if len(kept) == 0 {
			delete(m, k)
		} else {
			m[k] = kept
		}
	}
}

func (e *Engine) removeLocked(id string) {
	c, ok := e.byID[id]
	if !ok {
		return
	}
	delete(e.byID, id)
	if u := e.byUser[c.r.UserID]; u != nil {
		delete(u, id)
		if len(u) == 0 {
			delete(e.byUser, c.r.UserID)
		}
	}
	drop(e.byEntity, id)
	drop(e.bySource, id)
	drop(e.byToken, id)
}

// matches evaluates scope, types and conditions for one event.
func matches(c *compiled, ev event.Event, ents []string, src, phrase string) bool {
	if c.types != nil {
		if _, ok := c.types[ev.Type]; !ok {
			return false
		}
	}
	ok := false
	switch c.r.Spec.Scope.Kind {
	case "entity":
		for _, x := range ents {
			if _, hit := c.scope[x]; hit {
				ok = true
				break
			}
		}
	case "source":
		_, ok = c.scope[src]
	case "keyword":
		for v := range c.scope {
			if strings.Contains(phrase, " "+v+" ") {
				ok = true
				break
			}
		}
	}
	return ok && rule.Match(c.r.Spec.Conditions, ev)
}

func prepare(ev event.Event, title, body string) (tokens, ents []string, src, phrase string) {
	tokens = event.Tokens(ev.Title + " " + ev.Summary + " " + title + " " + body)
	phrase = " " + strings.Join(tokens, " ") + " "
	for _, x := range ev.Entities {
		ents = append(ents, event.NormEntity(x))
	}
	return tokens, ents, event.NormEntity(ev.Source), phrase
}

// Handle is the event.Handler subscribed to the bus.
func (e *Engine) Handle(ctx context.Context, ev event.Event) {
	title, body, _ := e.reg.Render(ev)
	tokens, ents, src, phrase := prepare(ev, title, body)

	e.stMu.Lock()
	e.ring = append(e.ring, ev)
	if len(e.ring) > e.cfg.RingSize {
		e.ring = append([]event.Event(nil), e.ring[len(e.ring)-e.cfg.RingSize:]...)
	}
	e.stMu.Unlock()

	cands := map[*compiled]struct{}{}
	e.mu.RLock()
	for _, x := range ents {
		for _, c := range e.byEntity[x] {
			cands[c] = struct{}{}
		}
	}
	for _, c := range e.bySource[src] {
		cands[c] = struct{}{}
	}
	seen := map[string]struct{}{}
	for _, t := range tokens {
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		for _, c := range e.byToken[t] {
			cands[c] = struct{}{}
		}
	}
	e.mu.RUnlock()

	for c := range cands {
		if matches(c, ev, ents, src, phrase) {
			e.fire(ctx, c, ev, title, body)
		}
	}
}

func (e *Engine) fire(ctx context.Context, c *compiled, ev event.Event, title, body string) {
	now := e.cfg.Now()
	e.stMu.Lock()
	if last, ok := e.lastFired[c.r.ID]; ok && now.Sub(last) < time.Duration(c.r.Spec.CooldownSeconds)*time.Second {
		e.stMu.Unlock()
		return // cooldown: suppressed, not an error
	}
	recent := e.fireTimes[c.r.ID][:0:0]
	for _, t := range e.fireTimes[c.r.ID] {
		if now.Sub(t) < time.Hour {
			recent = append(recent, t)
		}
	}
	if len(recent) >= e.cfg.MaxFiresPerHour {
		e.fireTimes[c.r.ID] = recent
		e.stMu.Unlock()
		e.pause(ctx, c, fmt.Sprintf("auto-paused: more than %d alerts in one hour (tighten the rule or raise its cooldown, then re-enable)", e.cfg.MaxFiresPerHour))
		return
	}
	e.fireTimes[c.r.ID] = append(recent, now)
	e.lastFired[c.r.ID] = now
	e.stMu.Unlock()

	f := alert.Fire{
		ID: idgen.New(), RuleID: c.r.ID, UserID: c.r.UserID, EventID: ev.ID, RuleName: c.r.Spec.Name, Severity: c.r.Spec.Severity,
		Title: title, Body: body, URL: ev.URL, Source: ev.Source, Entities: ev.Entities, FiredAt: now.UTC(),
		Reason: fmt.Sprintf("Matched your alert %q (%s: %s)%s", c.r.Spec.Name, c.r.Spec.Scope.Kind, strings.Join(c.r.Spec.Scope.Values, ", "), condNote(c.r.Spec.Conditions)),
	}
	e.cfg.Metrics.Inc("alert_fires_total", "severity", c.r.Spec.Severity)
	if err := e.fires.Add(ctx, f); err != nil {
		e.log.Error("persist alert fire", "err", err)
	}
	if err := e.rules.MarkFired(ctx, c.r.ID, now); err != nil {
		e.log.Warn("mark fired", "err", err)
	}
	for _, ch := range c.r.Spec.Channels {
		if ch == alert.InApp {
			e.out.Notify(ctx, c.r.UserID, "alert", f)
		} else if e.disp != nil {
			e.disp.Enqueue(c.r.UserID, ch, f)
		}
	}
}

func condNote(cs []rule.Condition) string {
	if len(cs) == 0 {
		return ""
	}
	return fmt.Sprintf(" with %d condition(s)", len(cs))
}

func (e *Engine) pause(ctx context.Context, c *compiled, reason string) {
	if err := e.rules.Pause(ctx, c.r.ID, reason); err != nil {
		e.log.Error("pause rule", "err", err)
	}
	e.RemoveRule(c.r.ID)
	e.cfg.Metrics.Inc("alert_rules_auto_paused_total")
	e.log.Warn("alert rule auto-paused", "rule", c.r.ID)
	e.out.Notify(ctx, c.r.UserID, "alert_paused", map[string]string{"rule_id": c.r.ID, "name": c.r.Spec.Name, "reason": reason})
}

// Preview replays recently seen events against a draft rule (nothing is stored or sent).
func (e *Engine) Preview(r alert.Rule) alert.Preview {
	c := compile(r)
	now := e.cfg.Now()
	e.stMu.Lock()
	ring := append([]event.Event(nil), e.ring...)
	e.stMu.Unlock()

	p := alert.Preview{Examples: []alert.PreviewExample{}}
	var last time.Time
	var times []time.Time
	for _, ev := range ring {
		if now.Sub(ev.Timestamp) > 24*time.Hour {
			continue
		}
		p.Examined++
		title, body, _ := e.reg.Render(ev)
		_, ents, src, phrase := prepare(ev, title, body)
		if !matches(c, ev, ents, src, phrase) {
			continue
		}
		p.WouldMatch++
		at := ev.Timestamp
		if !last.IsZero() && at.Sub(last) < time.Duration(r.Spec.CooldownSeconds)*time.Second {
			continue
		}
		recent := times[:0:0]
		for _, t := range times {
			if at.Sub(t) < time.Hour {
				recent = append(recent, t)
			}
		}
		if len(recent) >= e.cfg.MaxFiresPerHour {
			p.WouldPause = true // the rule would be paused here; keep counting matches for the report
			continue
		}
		times = append(recent, at)
		last = at
		p.WouldFire++
		if len(p.Examples) < 5 {
			p.Examples = append(p.Examples, alert.PreviewExample{At: at, Title: title})
		}
	}
	return p
}
