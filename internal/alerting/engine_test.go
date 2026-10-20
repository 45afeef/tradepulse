package alerting

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"tradepulse/internal/domain/alert"
	"tradepulse/internal/domain/event"
	"tradepulse/internal/domain/rule"
	"tradepulse/internal/storage/memory"
)

type note struct {
	userID, kind string
	data         any
}
type fakeNotifier struct {
	mu    sync.Mutex
	notes []note
}

func (f *fakeNotifier) Notify(_ context.Context, userID, kind string, data any) {
	f.mu.Lock()
	f.notes = append(f.notes, note{userID, kind, data})
	f.mu.Unlock()
}
func (f *fakeNotifier) of(userID, kind string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, x := range f.notes {
		if x.userID == userID && x.kind == kind {
			n++
		}
	}
	return n
}

type enq struct{ userID, channelID string }
type fakeDispatcher struct {
	mu  sync.Mutex
	got []enq
}

func (f *fakeDispatcher) Enqueue(userID, channelID string, _ alert.Fire) {
	f.mu.Lock()
	f.got = append(f.got, enq{userID, channelID})
	f.mu.Unlock()
}

type fx struct {
	t     *testing.T
	e     *Engine
	rules *memory.AlertRuleRepo
	fires *memory.AlertFireRepo
	n     *fakeNotifier
	d     *fakeDispatcher
	now   time.Time
	reg   *event.Registry
}

func newFx(t *testing.T, maxPerHour int) *fx {
	t.Helper()
	reg := event.NewRegistry()
	if err := reg.Register(event.TypeDef{ID: "metric", Fields: []event.FieldDef{{Name: "value", Kind: event.KindNumber}}, Title: "{entity} at {value:%.0f}"}); err != nil {
		t.Fatal(err)
	}
	f := &fx{t: t, rules: memory.NewAlertRuleRepo(), fires: memory.NewAlertFireRepo(), n: &fakeNotifier{}, d: &fakeDispatcher{}, reg: reg,
		now: time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)}
	f.e = NewEngine(slog.New(slog.NewTextHandler(io.Discard, nil)), f.rules, f.fires, reg, f.n, f.d, Config{MaxFiresPerHour: maxPerHour, Now: func() time.Time { return f.now }})
	return f
}

func (f *fx) add(id, user string, sp alert.Spec) alert.Rule {
	f.t.Helper()
	sp.Name = id
	if sp.CooldownSeconds == 0 {
		sp.CooldownSeconds = 60
	}
	if sp.Severity == "" {
		sp.Severity = "notable"
	}
	if len(sp.Channels) == 0 {
		sp.Channels = []string{alert.InApp}
	}
	sp.Enabled = true
	n, err := sp.Normalize(f.reg, func(string) bool { return true })
	if err != nil {
		f.t.Fatal(err)
	}
	r := alert.Rule{ID: id, UserID: user, Spec: n, CreatedAt: f.now, UpdatedAt: f.now}
	if err := f.rules.Create(context.Background(), r); err != nil {
		f.t.Fatal(err)
	}
	f.e.UpsertRule(r)
	return r
}

func (f *fx) ev(typ, source string, ents []string, title string, payload map[string]any) event.Event {
	e := event.New(typ, source, ents, f.now, payload)
	e.Title = title
	return e
}

func (f *fx) fireCount(user string) int {
	l, _ := f.fires.Recent(context.Background(), user, 1000)
	return len(l)
}

func TestEntityKeywordAndSourceRulesFire(t *testing.T) {
	f := newFx(t, 30)
	ctx := context.Background()
	f.add("ent", "alice", alert.Spec{Scope: alert.Scope{Kind: "entity", Values: []string{"TCS"}}, Channels: []string{alert.InApp, "chan-1"}})
	f.add("kw", "alice", alert.Spec{Scope: alert.Scope{Kind: "keyword", Values: []string{"repo rate"}}})
	f.add("src", "alice", alert.Spec{Scope: alert.Scope{Kind: "source", Values: []string{"CNBC"}}})

	f.e.Handle(ctx, f.ev("metric", "nse", []string{"tcs"}, "", map[string]any{"value": 3500.0}))
	f.e.Handle(ctx, f.ev("content_item", "rss", []string{"rbi"}, "RBI raises repo rate by 25 bps", nil))
	f.e.Handle(ctx, f.ev("content_item", "cnbc", nil, "Anything at all", nil))
	f.e.Handle(ctx, f.ev("content_item", "other", []string{"x"}, "Spoiled repository rated badly", nil)) // no whole-phrase/word match

	if got := f.fireCount("alice"); got != 3 {
		t.Fatalf("expected 3 fires, got %d", got)
	}
	if f.n.of("alice", "alert") != 3 {
		t.Fatalf("in-app notifications: %d", f.n.of("alice", "alert"))
	}
	if len(f.d.got) != 1 || f.d.got[0] != (enq{"alice", "chan-1"}) {
		t.Fatalf("webhook dispatch: %+v", f.d.got)
	}
	fires, _ := f.fires.Recent(ctx, "alice", 10)
	for _, x := range fires {
		if x.Reason == "" || x.RuleName == "" || x.EventID == "" {
			t.Fatalf("every fire must be explainable: %+v", x)
		}
	}
	if r, _ := f.rules.Get(ctx, "alice", "ent"); r.LastFiredAt == nil {
		t.Fatal("last_fired_at must be stored")
	}
}

func TestTypesAndConditionsFilter(t *testing.T) {
	f := newFx(t, 30)
	f.add("big", "alice", alert.Spec{Scope: alert.Scope{Kind: "entity", Values: []string{"tcs"}}, Types: []string{"metric"},
		Conditions: []rule.Condition{{Field: "value", Op: rule.Gte, Value: 100.0}}})
	ctx := context.Background()
	f.e.Handle(ctx, f.ev("metric", "s", []string{"tcs"}, "", map[string]any{"value": 50.0}))
	f.e.Handle(ctx, f.ev("content_item", "s", []string{"tcs"}, "x", nil))
	if f.fireCount("alice") != 0 {
		t.Fatal("below threshold / wrong type must not fire")
	}
	f.e.Handle(ctx, f.ev("metric", "s", []string{"tcs"}, "", map[string]any{"value": 150.0}))
	if f.fireCount("alice") != 1 {
		t.Fatal("matching event must fire")
	}
}

func TestCooldownSuppressesRepeats(t *testing.T) {
	f := newFx(t, 30)
	f.add("r", "alice", alert.Spec{Scope: alert.Scope{Kind: "entity", Values: []string{"tcs"}}, CooldownSeconds: 60})
	ctx := context.Background()
	e := f.ev("content_item", "s", []string{"tcs"}, "news", nil)
	f.e.Handle(ctx, e)
	f.now = f.now.Add(30 * time.Second)
	f.e.Handle(ctx, e)
	if f.fireCount("alice") != 1 {
		t.Fatal("second event inside the cooldown must be suppressed")
	}
	f.now = f.now.Add(31 * time.Second)
	f.e.Handle(ctx, e)
	if f.fireCount("alice") != 2 {
		t.Fatal("after the cooldown the rule fires again")
	}
}

func TestHourlyCapAutoPausesAndReEnableResetsTheCount(t *testing.T) {
	f := newFx(t, 3)
	ctx := context.Background()
	r := f.add("noisy", "alice", alert.Spec{Scope: alert.Scope{Kind: "entity", Values: []string{"tcs"}}, CooldownSeconds: 10})
	e := f.ev("content_item", "s", []string{"tcs"}, "news", nil)
	for i := 0; i < 5; i++ {
		f.now = f.now.Add(11 * time.Second)
		f.e.Handle(ctx, e)
	}
	if got := f.fireCount("alice"); got != 3 {
		t.Fatalf("exactly the cap (3) may fire before pausing, got %d", got)
	}
	stored, _ := f.rules.Get(ctx, "alice", "noisy")
	if stored.Spec.Enabled || stored.PausedReason == "" {
		t.Fatalf("rule must be disabled with a reason: %+v", stored)
	}
	if f.n.of("alice", "alert_paused") != 1 {
		t.Fatalf("user must be told once, got %d", f.n.of("alice", "alert_paused"))
	}
	f.now = f.now.Add(11 * time.Second)
	f.e.Handle(ctx, e)
	if f.fireCount("alice") != 3 {
		t.Fatal("a paused rule must not fire")
	}
	r.Spec.Enabled = true // user re-enables via the API
	f.e.UpsertRule(r)
	f.now = f.now.Add(11 * time.Second)
	f.e.Handle(ctx, e)
	if f.fireCount("alice") != 4 {
		t.Fatal("a re-enabled rule starts a fresh hourly count")
	}
}

func TestDisabledRulesSuspensionAndReload(t *testing.T) {
	f := newFx(t, 30)
	ctx := context.Background()
	r := f.add("r", "alice", alert.Spec{Scope: alert.Scope{Kind: "entity", Values: []string{"tcs"}}, CooldownSeconds: 10})
	e := f.ev("content_item", "s", []string{"tcs"}, "news", nil)

	r.Spec.Enabled = false
	f.e.UpsertRule(r)
	f.e.Handle(ctx, e)
	if f.fireCount("alice") != 0 {
		t.Fatal("disabled rule fired")
	}
	r.Spec.Enabled = true
	_ = f.rules.Update(ctx, r)
	f.e.UpsertRule(r)
	f.e.RemoveUser("alice") // suspension
	f.e.Handle(ctx, e)
	if f.fireCount("alice") != 0 {
		t.Fatal("suspended user's rule fired")
	}
	if err := f.e.LoadUser(ctx, "alice"); err != nil {
		t.Fatal(err)
	}
	f.e.Handle(ctx, e)
	if f.fireCount("alice") != 1 {
		t.Fatal("reactivated user's rule must fire again")
	}
	f.e.RemoveRule("r")
	f.now = f.now.Add(time.Minute)
	f.e.Handle(ctx, e)
	if f.fireCount("alice") != 1 {
		t.Fatal("removed rule fired")
	}
}

func TestUsersAreIsolated(t *testing.T) {
	f := newFx(t, 30)
	f.add("a", "alice", alert.Spec{Scope: alert.Scope{Kind: "entity", Values: []string{"tcs"}}})
	f.add("b", "bob", alert.Spec{Scope: alert.Scope{Kind: "entity", Values: []string{"infy"}}})
	f.e.Handle(context.Background(), f.ev("content_item", "s", []string{"tcs"}, "news", nil))
	if f.fireCount("alice") != 1 || f.fireCount("bob") != 0 || f.n.of("alice", "alert") != 1 || f.n.of("bob", "alert") != 0 {
		t.Fatal("alerts must reach only the rule's owner")
	}
}

func TestLoadSkipsInactiveUsersAndRestoresCooldown(t *testing.T) {
	f := newFx(t, 30)
	ctx := context.Background()
	fired := f.now.Add(-5 * time.Second)
	for _, u := range []string{"alice", "bob"} {
		sp, _ := alert.Spec{Name: u, Scope: alert.Scope{Kind: "entity", Values: []string{"tcs"}}, Enabled: true, CooldownSeconds: 60}.Normalize(f.reg, nil)
		_ = f.rules.Create(ctx, alert.Rule{ID: u, UserID: u, Spec: sp, CreatedAt: f.now, LastFiredAt: &fired})
	}
	e2 := NewEngine(slog.New(slog.NewTextHandler(io.Discard, nil)), f.rules, f.fires, f.reg, f.n, f.d, Config{Now: func() time.Time { return f.now }})
	if err := e2.Load(ctx, func(id string) bool { return id == "alice" }); err != nil {
		t.Fatal(err)
	}
	e2.Handle(ctx, f.ev("content_item", "s", []string{"tcs"}, "news", nil))
	if f.fireCount("bob") != 0 {
		t.Fatal("inactive user's rule must not be loaded")
	}
	if f.fireCount("alice") != 0 {
		t.Fatal("cooldown must survive a restart (last_fired_at restored)")
	}
	f.now = f.now.Add(time.Minute)
	e2.Handle(ctx, f.ev("content_item", "s", []string{"tcs"}, "news", nil))
	if f.fireCount("alice") != 1 {
		t.Fatal("after the cooldown alice's rule fires")
	}
}

func TestPreviewReplaysRecentEventsWithoutSideEffects(t *testing.T) {
	f := newFx(t, 3)
	ctx := context.Background()
	// events flow through the engine (and its ring) with no rules defined
	for i := 0; i < 6; i++ {
		f.e.Handle(ctx, f.ev("content_item", "s", []string{"tcs"}, "tcs news", nil))
		f.e.Handle(ctx, f.ev("content_item", "s", []string{"infy"}, "infy news", nil))
		f.now = f.now.Add(2 * time.Minute)
	}
	old := f.ev("content_item", "s", []string{"tcs"}, "ancient", nil)
	old.Timestamp = f.now.Add(-48 * time.Hour)
	f.e.Handle(ctx, old)

	draft := alert.Rule{ID: "preview", UserID: "alice", Spec: alert.Spec{Name: "d", Scope: alert.Scope{Kind: "entity", Values: []string{"tcs"}}, CooldownSeconds: 60, Severity: "notable", Enabled: true}}
	p := f.e.Preview(draft)
	if p.Examined != 12 || p.WouldMatch != 6 || p.WouldFire != 3 || !p.WouldPause || len(p.Examples) != 3 {
		t.Fatalf("preview: %+v", p)
	}
	draft.Spec.Scope.Values = []string{"infy"}
	draft.Spec.CooldownSeconds = 3600
	if p := f.e.Preview(draft); p.WouldFire != 1 || p.WouldMatch != 6 || p.WouldPause {
		t.Fatalf("cooldown must be simulated: %+v", p)
	}
	if f.fireCount("alice") != 0 || len(f.d.got) != 0 || f.n.of("alice", "alert") != 0 {
		t.Fatal("preview must not store or send anything")
	}
	empty := f.e.Preview(alert.Rule{Spec: alert.Spec{Scope: alert.Scope{Kind: "entity", Values: []string{"nobody"}}, CooldownSeconds: 60}})
	if empty.WouldFire != 0 || empty.Examples == nil {
		t.Fatalf("empty preview: %+v", empty)
	}
}
