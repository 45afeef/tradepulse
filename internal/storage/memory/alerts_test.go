package memory

import (
	"context"
	"fmt"
	"testing"
	"time"

	"tradepulse/internal/domain/alert"
)

func TestAlertRuleRepoIsScopedToTheOwner(t *testing.T) {
	r := NewAlertRuleRepo()
	ctx := context.Background()
	now := time.Now()
	rule := alert.Rule{ID: "r1", UserID: "alice", Spec: alert.Spec{Name: "a", Enabled: true}, CreatedAt: now}
	_ = r.Create(ctx, rule)
	if _, err := r.Get(ctx, "bob", "r1"); err != alert.ErrNotFound {
		t.Fatalf("another user must not read the rule: %v", err)
	}
	bobEdit := rule
	bobEdit.UserID = "bob"
	if err := r.Update(ctx, bobEdit); err != alert.ErrNotFound {
		t.Fatalf("another user must not update the rule: %v", err)
	}
	if err := r.Delete(ctx, "bob", "r1"); err != alert.ErrNotFound {
		t.Fatalf("another user must not delete the rule: %v", err)
	}
	if list, _ := r.List(ctx, "bob"); len(list) != 0 {
		t.Fatal("list leaks other users' rules")
	}
	if err := r.Pause(ctx, "r1", "too noisy"); err != nil {
		t.Fatal(err)
	}
	got, _ := r.Get(ctx, "alice", "r1")
	if got.Spec.Enabled || got.PausedReason != "too noisy" {
		t.Fatalf("pause: %+v", got)
	}
	if en, _ := r.ListEnabled(ctx); len(en) != 0 {
		t.Fatal("paused rules must not be listed as enabled")
	}
	at := now.Add(time.Minute)
	_ = r.MarkFired(ctx, "r1", at)
	if got, _ = r.Get(ctx, "alice", "r1"); got.LastFiredAt == nil || !got.LastFiredAt.Equal(at) {
		t.Fatal("MarkFired not stored")
	}
	if err := r.Delete(ctx, "alice", "r1"); err != nil {
		t.Fatal(err)
	}
}

func TestChannelRepoCountersScopingAndSecrets(t *testing.T) {
	r := NewChannelRepo()
	ctx := context.Background()
	_ = r.Create(ctx, alert.Channel{ID: "c1", UserID: "alice", Type: "webhook", Enabled: true, CreatedAt: time.Now()})
	if _, err := r.Get(ctx, "bob", "c1"); err != alert.ErrNotFound {
		t.Fatal("scoping")
	}
	if _, err := r.BumpSecretVersion(ctx, "bob", "c1"); err != alert.ErrNotFound {
		t.Fatal("another user must not rotate the secret")
	}
	if v, _ := r.BumpSecretVersion(ctx, "alice", "c1"); v != 2 {
		t.Fatalf("version after bump: %d", v)
	}
	for i := 1; i <= 3; i++ {
		if n, _ := r.RecordResult(ctx, "c1", false, time.Now()); n != i {
			t.Fatalf("failure %d counted as %d", i, n)
		}
	}
	if n, _ := r.RecordResult(ctx, "c1", true, time.Now()); n != 0 {
		t.Fatal("success must reset the counter")
	}
	_, _ = r.RecordResult(ctx, "c1", false, time.Now())
	_ = r.SetEnabled(ctx, "alice", "c1", false, "broken")
	if c, _ := r.ByID(ctx, "c1"); c.Enabled || c.DisabledReason != "broken" {
		t.Fatalf("disable: %+v", c)
	}
	_ = r.SetEnabled(ctx, "alice", "c1", true, "")
	if c, _ := r.ByID(ctx, "c1"); !c.Enabled || c.ConsecutiveFailures != 0 || c.DisabledReason != "" {
		t.Fatalf("re-enable must reset: %+v", c)
	}
	if err := r.SetEnabled(ctx, "bob", "c1", false, "x"); err != alert.ErrNotFound {
		t.Fatal("another user must not disable the channel")
	}
}

func TestFireAndDeliveryHistoryOrderScopeAndPrune(t *testing.T) {
	fr, dr := NewAlertFireRepo(), NewDeliveryRepo()
	ctx := context.Background()
	old := time.Now().Add(-40 * 24 * time.Hour)
	_ = fr.Add(ctx, alert.Fire{ID: "old", UserID: "alice", FiredAt: old})
	for i := 0; i < 3; i++ {
		_ = fr.Add(ctx, alert.Fire{ID: fmt.Sprint("f", i), UserID: "alice", FiredAt: time.Now()})
	}
	_ = fr.Add(ctx, alert.Fire{ID: "b", UserID: "bob", FiredAt: time.Now()})
	got, _ := fr.Recent(ctx, "alice", 2)
	if len(got) != 2 || got[0].ID != "f2" || got[1].ID != "f1" {
		t.Fatalf("recent: %+v", got)
	}
	if n, _ := fr.Prune(ctx, time.Now().Add(-30*24*time.Hour)); n != 1 {
		t.Fatalf("pruned %d", n)
	}
	_ = dr.Add(ctx, alert.Delivery{ID: "d1", UserID: "alice", At: old})
	_ = dr.Add(ctx, alert.Delivery{ID: "d2", UserID: "alice", At: time.Now()})
	_ = dr.Add(ctx, alert.Delivery{ID: "d3", UserID: "bob", At: time.Now()})
	if got, _ := dr.Recent(ctx, "alice", 10); len(got) != 2 || got[0].ID != "d2" {
		t.Fatalf("deliveries: %+v", got)
	}
	if n, _ := dr.Prune(ctx, time.Now().Add(-30*24*time.Hour)); n != 1 {
		t.Fatalf("pruned %d", n)
	}
}
