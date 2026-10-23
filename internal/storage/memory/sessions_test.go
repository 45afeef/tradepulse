package memory

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"tradepulse/internal/domain/session"
)

func newTok(t *testing.T, r *SessionRepo, user, fam string, ttl time.Duration) string {
	t.Helper()
	tok, hash, err := session.New()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := r.Create(context.Background(), session.Token{ID: hash[:8], UserID: user, FamilyID: fam, Hash: hash, CreatedAt: now, ExpiresAt: now.Add(ttl)}); err != nil {
		t.Fatal(err)
	}
	return tok
}

func TestRotationConsumesOnceAndDetectsReuse(t *testing.T) {
	r := NewSessionRepo()
	tok := newTok(t, r, "u", "fam", time.Hour)
	var ok, reused atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			switch _, err := r.Rotate(context.Background(), session.Hash(tok), time.Now()); err {
			case nil:
				ok.Add(1)
			case session.ErrReused:
				reused.Add(1)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 1 || reused.Load() != 49 {
		t.Fatalf("exactly one rotation may win: ok=%d reused=%d", ok.Load(), reused.Load())
	}
	if got, err := r.Rotate(context.Background(), session.Hash(tok), time.Now()); err != session.ErrReused || got.FamilyID != "fam" {
		t.Fatalf("reuse must report the family: %+v %v", got, err)
	}
}

func TestRotateRejectsUnknownExpiredAndRevoked(t *testing.T) {
	r := NewSessionRepo()
	ctx := context.Background()
	if _, err := r.Rotate(ctx, session.Hash("nope"), time.Now()); err != session.ErrInvalid {
		t.Fatalf("unknown: %v", err)
	}
	expired := newTok(t, r, "u", "f1", -time.Minute)
	if _, err := r.Rotate(ctx, session.Hash(expired), time.Now()); err != session.ErrInvalid {
		t.Fatalf("expired: %v", err)
	}
	revoked := newTok(t, r, "u", "f2", time.Hour)
	_ = r.RevokeFamily(ctx, "f2", time.Now())
	if _, err := r.Rotate(ctx, session.Hash(revoked), time.Now()); err != session.ErrInvalid {
		t.Fatalf("revoked: %v", err)
	}
	if _, err := r.FamilyOf(ctx, session.Hash("nope")); err != session.ErrInvalid {
		t.Fatal("FamilyOf unknown")
	}
}

func TestRevokeByFamilyAndUserAndPrune(t *testing.T) {
	r := NewSessionRepo()
	ctx := context.Background()
	a1 := newTok(t, r, "alice", "fa", time.Hour)
	a2 := newTok(t, r, "alice", "fb", time.Hour)
	b1 := newTok(t, r, "bob", "fc", time.Hour)
	_ = r.RevokeFamily(ctx, "fa", time.Now())
	if _, err := r.Rotate(ctx, session.Hash(a1), time.Now()); err != session.ErrInvalid {
		t.Fatal("family revoke")
	}
	if _, err := r.Rotate(ctx, session.Hash(a2), time.Now()); err != nil {
		t.Fatal("other families must survive")
	}
	_ = r.RevokeUser(ctx, "bob", time.Now())
	if _, err := r.Rotate(ctx, session.Hash(b1), time.Now()); err != session.ErrInvalid {
		t.Fatal("user revoke")
	}
	_ = newTok(t, r, "carol", "fd", -48*time.Hour)
	if n, _ := r.Prune(ctx, time.Now().Add(-24*time.Hour)); n != 1 {
		t.Fatalf("pruned %d", n)
	}
}

func TestNewTokenIsRandomPrefixedAndHashStable(t *testing.T) {
	a, ha, _ := session.New()
	b, hb, _ := session.New()
	if a == b || ha == hb || len(a) < len(session.TokenPrefix)+40 || session.Hash(a) != ha || ha == a {
		t.Fatal("tokens must be unique, long, and only their hash is derived")
	}
}
