package postgres

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"tradepulse/internal/domain/alert"
	"tradepulse/internal/domain/rule"
	"tradepulse/internal/idgen"
)

func TestAlertRuleRepoRoundTripScopingAndPause(t *testing.T) {
	pool := testPool(t)
	users, rules := NewUserRepo(pool), NewAlertRuleRepo(pool)
	ctx := context.Background()
	alice, bob := mkUser(t, users, "alice@example.com"), mkUser(t, users, "bob@example.com")
	now := time.Now().UTC().Truncate(time.Microsecond)
	r := alert.Rule{ID: idgen.New(), UserID: alice.ID, CreatedAt: now, UpdatedAt: now, Spec: alert.Spec{
		Name: "Big movers", Scope: alert.Scope{Kind: "entity", Values: []string{"tcs", "infy"}}, Types: []string{"price_changed"},
		Conditions:      []rule.Condition{{Field: "abs_change_pct", Op: rule.Gte, Value: 2.5}, {Field: "status", Op: rule.In, Value: []any{"down", "degraded"}}},
		CooldownSeconds: 300, Severity: "high", Channels: []string{alert.InApp, "chan-1"}, Enabled: true}}
	if err := rules.Create(ctx, r); err != nil {
		t.Fatal(err)
	}
	got, err := rules.Get(ctx, alice.ID, r.ID)
	if err != nil || got.Spec.Name != "Big movers" || len(got.Spec.Scope.Values) != 2 || got.Spec.Conditions[0].Value != 2.5 ||
		len(got.Spec.Conditions[1].Value.([]any)) != 2 || len(got.Spec.Channels) != 2 || got.Spec.Severity != "high" {
		t.Fatalf("round trip: %+v %v", got.Spec, err)
	}
	if _, err := rules.Get(ctx, bob.ID, r.ID); err != alert.ErrNotFound {
		t.Fatal("another user must not read the rule")
	}
	other := r
	other.UserID = bob.ID
	if err := rules.Update(ctx, other); err != alert.ErrNotFound {
		t.Fatal("another user must not update the rule")
	}
	if err := rules.Delete(ctx, bob.ID, r.ID); err != alert.ErrNotFound {
		t.Fatal("another user must not delete the rule")
	}
	r.Spec.Name, r.UpdatedAt = "Renamed", now.Add(time.Second)
	if err := rules.Update(ctx, r); err != nil {
		t.Fatal(err)
	}
	if err := rules.Pause(ctx, r.ID, "too noisy"); err != nil {
		t.Fatal(err)
	}
	got, _ = rules.Get(ctx, alice.ID, r.ID)
	if got.Spec.Name != "Renamed" || got.Spec.Enabled || got.PausedReason != "too noisy" {
		t.Fatalf("update/pause: %+v", got)
	}
	if en, _ := rules.ListEnabled(ctx); len(en) != 0 {
		t.Fatal("paused rules must not be listed as enabled")
	}
	if err := rules.MarkFired(ctx, r.ID, now); err != nil {
		t.Fatal(err)
	}
	if got, _ = rules.Get(ctx, alice.ID, r.ID); got.LastFiredAt == nil {
		t.Fatal("last_fired_at lost")
	}
	if list, _ := rules.List(ctx, bob.ID); len(list) != 0 {
		t.Fatal("list leaks other users' rules")
	}
}

func TestFiresPruneAndUserDeletionCascadesEverything(t *testing.T) {
	pool := testPool(t)
	users, rules, fires, chans, dels := NewUserRepo(pool), NewAlertRuleRepo(pool), NewAlertFireRepo(pool), NewChannelRepo(pool), NewDeliveryRepo(pool)
	ctx := context.Background()
	u := mkUser(t, users, "cascade@example.com")
	now := time.Now().UTC()
	_ = rules.Create(ctx, alert.Rule{ID: "r1", UserID: u.ID, CreatedAt: now, UpdatedAt: now, Spec: alert.Spec{Name: "r", Scope: alert.Scope{Kind: "entity", Values: []string{"x"}}, CooldownSeconds: 60, Severity: "info", Enabled: true}})
	_ = chans.Create(ctx, alert.Channel{ID: "c1", UserID: u.ID, Type: "webhook", Name: "n", URL: "https://x.example/h", Enabled: true, CreatedAt: now})
	for i := 0; i < 3; i++ {
		if err := fires.Add(ctx, alert.Fire{ID: idgen.New(), RuleID: "r1", UserID: u.ID, EventID: "e", RuleName: "r", Severity: "info", Title: fmt.Sprint("t", i), Body: "b",
			Source: "s", Entities: nil, Reason: "why", FiredAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	_ = fires.Add(ctx, alert.Fire{ID: "old", RuleID: "r1", UserID: u.ID, EventID: "e", RuleName: "r", Severity: "info", Title: "old", Body: "b", Source: "s", Reason: "why", FiredAt: now.Add(-40 * 24 * time.Hour)})
	_ = dels.Add(ctx, alert.Delivery{ID: "d1", ChannelID: "c1", UserID: u.ID, FireID: "f", At: now, OK: true, Status: 200, Attempts: 1})
	got, err := fires.Recent(ctx, u.ID, 2)
	if err != nil || len(got) != 2 || got[0].Title != "old" && got[0].Title != "t2" || got[0].Entities == nil {
		t.Fatalf("recent (newest insert first, entities never null): %+v %v", got, err)
	}
	if n, _ := fires.Prune(ctx, now.Add(-30*24*time.Hour)); n != 1 {
		t.Fatalf("pruned %d, want 1", n)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, u.ID); err != nil {
		t.Fatal(err)
	}
	var left int
	_ = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM alert_rules)+(SELECT count(*) FROM alert_fires)+(SELECT count(*) FROM notification_channels)+(SELECT count(*) FROM channel_deliveries)`).Scan(&left)
	if left != 0 {
		t.Fatalf("%d rows survived deleting their user", left)
	}
}

func TestChannelRepoAtomicFailureCounterAndScoping(t *testing.T) {
	pool := testPool(t)
	users, chans := NewUserRepo(pool), NewChannelRepo(pool)
	ctx := context.Background()
	alice, bob := mkUser(t, users, "a@example.com"), mkUser(t, users, "b@example.com")
	c := alert.Channel{ID: "c1", UserID: alice.ID, Type: "webhook", Name: "n", URL: "https://x.example/h", Enabled: true, CreatedAt: time.Now().UTC()}
	if err := chans.Create(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := chans.Create(ctx, alert.Channel{ID: "bad", UserID: alice.ID, Type: "sms", Name: "n", URL: "x", CreatedAt: time.Now()}); err == nil {
		t.Fatal("CHECK constraint must reject unknown channel types")
	}
	if _, err := chans.Get(ctx, bob.ID, "c1"); err != alert.ErrNotFound {
		t.Fatal("scoping")
	}
	if _, err := chans.BumpSecretVersion(ctx, bob.ID, "c1"); err != alert.ErrNotFound {
		t.Fatal("another user must not rotate the secret")
	}
	if v, _ := chans.BumpSecretVersion(ctx, alice.ID, "c1"); v != 2 {
		t.Fatalf("version %d", v)
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = chans.RecordResult(ctx, "c1", false, time.Now().UTC()) }()
	}
	wg.Wait()
	if got, _ := chans.ByID(ctx, "c1"); got.ConsecutiveFailures != 20 || got.LastDeliveryAt == nil {
		t.Fatalf("concurrent failures lost updates: %+v", got)
	}
	if n, _ := chans.RecordResult(ctx, "c1", true, time.Now().UTC()); n != 0 {
		t.Fatal("success must reset")
	}
	_, _ = chans.RecordResult(ctx, "c1", false, time.Now().UTC())
	_ = chans.SetEnabled(ctx, alice.ID, "c1", false, "broken")
	if got, _ := chans.ByID(ctx, "c1"); got.Enabled || got.DisabledReason != "broken" || got.ConsecutiveFailures != 1 {
		t.Fatalf("disable: %+v", got)
	}
	_ = chans.SetEnabled(ctx, alice.ID, "c1", true, "")
	if got, _ := chans.ByID(ctx, "c1"); !got.Enabled || got.DisabledReason != "" || got.ConsecutiveFailures != 0 {
		t.Fatalf("re-enable must reset: %+v", got)
	}
	if err := chans.SetEnabled(ctx, bob.ID, "c1", false, "x"); err != alert.ErrNotFound {
		t.Fatal("another user must not disable the channel")
	}
	if err := chans.Delete(ctx, bob.ID, "c1"); err != alert.ErrNotFound {
		t.Fatal("another user must not delete the channel")
	}
	if list, _ := chans.List(ctx, alice.ID); len(list) != 1 {
		t.Fatalf("list: %d", len(list))
	}
}

func TestDeliveryHistoryOrderScopeAndPrune(t *testing.T) {
	pool := testPool(t)
	users, dels := NewUserRepo(pool), NewDeliveryRepo(pool)
	ctx := context.Background()
	alice, bob := mkUser(t, users, "a@example.com"), mkUser(t, users, "b@example.com")
	now := time.Now().UTC()
	_ = dels.Add(ctx, alert.Delivery{ID: "old", ChannelID: "c", UserID: alice.ID, FireID: "f", At: now.Add(-40 * 24 * time.Hour), Attempts: 1})
	_ = dels.Add(ctx, alert.Delivery{ID: "new", ChannelID: "c", UserID: alice.ID, FireID: "f", At: now, OK: true, Status: 200, Attempts: 1})
	_ = dels.Add(ctx, alert.Delivery{ID: "bobs", ChannelID: "c", UserID: bob.ID, FireID: "f", At: now, Attempts: 4, Error: "boom"})
	got, _ := dels.Recent(ctx, alice.ID, 10)
	if len(got) != 2 || got[0].ID != "new" {
		t.Fatalf("order/scope: %+v", got)
	}
	if n, _ := dels.Prune(ctx, now.Add(-30*24*time.Hour)); n != 1 {
		t.Fatalf("pruned %d", n)
	}
}
