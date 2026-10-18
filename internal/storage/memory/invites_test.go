package memory

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"tradepulse/internal/domain/audit"
	"tradepulse/internal/domain/invite"
	"tradepulse/internal/domain/sourcedef"
)

func mkInvite(t *testing.T, r *InviteRepo, email string, ttl time.Duration) string {
	t.Helper()
	code, hash, err := invite.NewCode()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := r.Create(context.Background(), invite.Invite{ID: hash[:8], CodeHash: hash, Email: email, CreatedAt: now, ExpiresAt: now.Add(ttl)}); err != nil {
		t.Fatal(err)
	}
	return code
}

func TestRedeemExactlyOnceUnderConcurrency(t *testing.T) {
	r := NewInviteRepo()
	code := mkInvite(t, r, "", time.Hour)
	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := r.Redeem(context.Background(), invite.HashCode(code), "a@example.com", fmt.Sprint(i), time.Now()); err == nil {
				wins.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("invite redeemed %d times, want exactly 1 (race)", wins.Load())
	}
}

func TestRedeemRejections(t *testing.T) {
	r := NewInviteRepo()
	ctx := context.Background()
	expired := mkInvite(t, r, "", -time.Minute)
	bound := mkInvite(t, r, "asha@example.com", time.Hour)
	if _, err := r.Redeem(ctx, invite.HashCode(expired), "a@example.com", "u", time.Now()); !errors.Is(err, invite.ErrInvalid) {
		t.Error("expired invite accepted")
	}
	if _, err := r.Redeem(ctx, invite.HashCode("tp_inv_unknown"), "a@example.com", "u", time.Now()); !errors.Is(err, invite.ErrInvalid) {
		t.Error("unknown code accepted")
	}
	if _, err := r.Redeem(ctx, invite.HashCode(bound), "mallory@example.com", "u", time.Now()); !errors.Is(err, invite.ErrInvalid) {
		t.Error("email-bound invite accepted for another email")
	}
	if _, err := r.Redeem(ctx, invite.HashCode(bound), "ASHA@Example.com", "u", time.Now()); err != nil {
		t.Errorf("bound invite must accept the right email case-insensitively: %v", err)
	}
}

func TestReleaseRestoresOnlyForSameUser(t *testing.T) {
	r := NewInviteRepo()
	ctx := context.Background()
	code := mkInvite(t, r, "", time.Hour)
	inv, err := r.Redeem(ctx, invite.HashCode(code), "a@example.com", "alice", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	_ = r.Release(ctx, inv.ID, "mallory") // someone else cannot release it
	if _, err := r.Redeem(ctx, invite.HashCode(code), "b@example.com", "bob", time.Now()); err == nil {
		t.Fatal("invite released by the wrong user")
	}
	_ = r.Release(ctx, inv.ID, "alice")
	if _, err := r.Redeem(ctx, invite.HashCode(code), "b@example.com", "bob", time.Now()); err != nil {
		t.Fatalf("released invite should be usable again: %v", err)
	}
}

func TestRevokeOnlyPending(t *testing.T) {
	r := NewInviteRepo()
	ctx := context.Background()
	code := mkInvite(t, r, "", time.Hour)
	list, _ := r.List(ctx)
	id := list[0].ID
	if err := r.Revoke(ctx, "nope", time.Now()); !errors.Is(err, invite.ErrNotFound) {
		t.Error("unknown id must be ErrNotFound")
	}
	if err := r.Revoke(ctx, id, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := r.Revoke(ctx, id, time.Now()); !errors.Is(err, invite.ErrInvalid) {
		t.Error("double revoke must fail")
	}
	if _, err := r.Redeem(ctx, invite.HashCode(code), "a@example.com", "u", time.Now()); err == nil {
		t.Error("revoked invite redeemed")
	}
}

func TestInviteStoredOnlyAsHash(t *testing.T) {
	r := NewInviteRepo()
	code := mkInvite(t, r, "", time.Hour)
	list, _ := r.List(context.Background())
	if strings.Contains(fmt.Sprintf("%+v", list), code) {
		t.Fatal("plaintext invite code found in storage")
	}
}

func TestAuditRepoOrderAndCap(t *testing.T) {
	r := NewAuditRepo(3)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		_ = r.Append(ctx, audit.Entry{ID: fmt.Sprint(i)})
	}
	got, _ := r.Recent(ctx, 10)
	if len(got) != 3 || got[0].ID != "4" || got[2].ID != "2" {
		t.Fatalf("unexpected: %+v", got)
	}
}

// ---- sources ----

func TestSourceRepoCRUDAndIndependentState(t *testing.T) {
	r := NewSourceRepo()
	ctx := context.Background()
	now := time.Now().UTC()
	src := sourcedef.Source{ID: "s1", Name: "cnbc", Type: sourcedef.TypeRSS, URL: "https://a.example/f", IntervalSeconds: 300, CreatedAt: now, UpdatedAt: now}
	if err := r.Create(ctx, src); err != nil {
		t.Fatal(err)
	}
	if err := r.Create(ctx, sourcedef.Source{ID: "s2", Name: "cnbc"}); !errors.Is(err, sourcedef.ErrNameTaken) {
		t.Fatalf("duplicate name: %v", err)
	}
	st := sourcedef.State{ConsecutiveFailures: 2, ETag: "e"}
	if err := r.SaveState(ctx, "s1", st); err != nil {
		t.Fatal(err)
	}
	upd := src
	upd.URL, upd.Enabled, upd.IntervalSeconds = "https://b.example/f", true, 600
	if err := r.Update(ctx, upd); err != nil {
		t.Fatal(err)
	}
	got, _ := r.Get(ctx, "s1")
	if got.URL != "https://b.example/f" || !got.Enabled || got.State.ConsecutiveFailures != 2 {
		t.Fatalf("Update must not touch state and vice versa: %+v", got)
	}
	if byName, err := r.GetByName(ctx, "cnbc"); err != nil || byName.ID != "s1" {
		t.Fatalf("GetByName: %v", err)
	}
	if err := r.Delete(ctx, "s1"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Get(ctx, "s1"); !errors.Is(err, sourcedef.ErrNotFound) {
		t.Fatal("deleted source still present")
	}
	if err := r.Create(ctx, sourcedef.Source{ID: "s3", Name: "cnbc"}); err != nil {
		t.Fatalf("name must be reusable after delete: %v", err)
	}
}

func TestItemStoreDedupesAndPrunes(t *testing.T) {
	s := NewItemStore()
	ctx := context.Background()
	now := time.Now()
	if ok, _ := s.Insert(ctx, "a", "k", nil, now); !ok {
		t.Fatal("first insert must be new")
	}
	if ok, _ := s.Insert(ctx, "a", "k", nil, now); ok {
		t.Fatal("duplicate must be rejected")
	}
	if ok, _ := s.Insert(ctx, "b", "k", nil, now); !ok {
		t.Fatal("same key in another source is a different item")
	}
	var wg sync.WaitGroup
	var wins atomic.Int32
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, _ := s.Insert(ctx, "c", "race", nil, now); ok {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("concurrent inserts won %d times, want exactly 1", wins.Load())
	}
	if n, _ := s.Prune(ctx, now.Add(time.Hour)); n != 4 {
		t.Fatalf("pruned %d, want 4", n)
	}
}

func TestSecretVersionAndUsageLedger(t *testing.T) {
	r := NewSourceRepo()
	ctx := context.Background()
	_ = r.Create(ctx, sourcedef.Source{ID: "h", Name: "hook", Type: sourcedef.TypeWebhook})
	if s, _ := r.Get(ctx, "h"); s.SecretVersion != 1 {
		t.Fatalf("new sources start at version 1, got %d", s.SecretVersion)
	}
	v1, err := r.BumpSecretVersion(ctx, "h")
	v2, _ := r.BumpSecretVersion(ctx, "h")
	if err != nil || v1 != 2 || v2 != 3 {
		t.Fatalf("bump: %d %d %v", v1, v2, err)
	}
	if _, err := r.BumpSecretVersion(ctx, "nope"); !errors.Is(err, sourcedef.ErrNotFound) {
		t.Fatal("unknown id")
	}
	u := NewUsageRepo()
	now := time.Date(2026, 10, 31, 23, 0, 0, 0, time.UTC)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = u.Add(ctx, "x", 1000, now) }()
	}
	wg.Wait()
	if got, _ := u.Month(ctx, "x", now); got != 50000 {
		t.Fatalf("concurrent adds lost updates: %d", got)
	}
	if got, _ := u.Month(ctx, "x", now.Add(2*time.Hour)); got != 0 {
		t.Fatalf("a new month must start at zero, got %d", got)
	}
	if got, _ := u.Month(ctx, "other", now); got != 0 {
		t.Fatal("keys are separate")
	}
}
