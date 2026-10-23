package postgres

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"tradepulse/internal/domain/session"
	"tradepulse/internal/idgen"
)

func mkToken(t *testing.T, r *SessionRepo, userID, fam string, ttl time.Duration) string {
	t.Helper()
	tok, hash, _ := session.New()
	now := time.Now().UTC()
	if err := r.Create(context.Background(), session.Token{ID: idgen.New(), UserID: userID, FamilyID: fam, Hash: hash, CreatedAt: now, ExpiresAt: now.Add(ttl)}); err != nil {
		t.Fatal(err)
	}
	return tok
}

func TestRefreshRotationIsAtomicAndDetectsReuse(t *testing.T) {
	pool := testPool(t)
	u := mkUser(t, NewUserRepo(pool), "a@example.com")
	r := NewSessionRepo(pool)
	tok := mkToken(t, r, u.ID, "fam", time.Hour)
	var ok, reused atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			switch _, err := r.Rotate(context.Background(), session.Hash(tok), time.Now().UTC()); err {
			case nil:
				ok.Add(1)
			case session.ErrReused:
				reused.Add(1)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 1 || reused.Load() != 29 {
		t.Fatalf("against real Postgres exactly one rotation may win: ok=%d reused=%d", ok.Load(), reused.Load())
	}
	if got, err := r.Rotate(context.Background(), session.Hash(tok), time.Now().UTC()); err != session.ErrReused || got.FamilyID != "fam" {
		t.Fatalf("reuse: %+v %v", got, err)
	}
}

func TestExpiredRevokedUnknownRevocationAndPrune(t *testing.T) {
	pool := testPool(t)
	users := NewUserRepo(pool)
	alice, bob := mkUser(t, users, "a@example.com"), mkUser(t, users, "b@example.com")
	r := NewSessionRepo(pool)
	ctx := context.Background()
	if _, err := r.Rotate(ctx, session.Hash("nope"), time.Now().UTC()); err != session.ErrInvalid {
		t.Fatalf("unknown: %v", err)
	}
	expired := mkToken(t, r, alice.ID, "f1", -time.Minute)
	if _, err := r.Rotate(ctx, session.Hash(expired), time.Now().UTC()); err != session.ErrInvalid {
		t.Fatalf("expired: %v", err)
	}
	t1 := mkToken(t, r, alice.ID, "f2", time.Hour)
	t2 := mkToken(t, r, alice.ID, "f3", time.Hour)
	t3 := mkToken(t, r, bob.ID, "f4", time.Hour)
	if fam, err := r.FamilyOf(ctx, session.Hash(t1)); err != nil || fam != "f2" {
		t.Fatalf("FamilyOf: %q %v", fam, err)
	}
	_ = r.RevokeFamily(ctx, "f2", time.Now().UTC())
	if _, err := r.Rotate(ctx, session.Hash(t1), time.Now().UTC()); err != session.ErrInvalid {
		t.Fatal("revoked family")
	}
	if _, err := r.Rotate(ctx, session.Hash(t2), time.Now().UTC()); err != nil {
		t.Fatal("other family must survive")
	}
	_ = r.RevokeUser(ctx, bob.ID, time.Now().UTC())
	if _, err := r.Rotate(ctx, session.Hash(t3), time.Now().UTC()); err != session.ErrInvalid {
		t.Fatal("revoked user")
	}
	if n, _ := r.Prune(ctx, time.Now().UTC()); n != 1 {
		t.Fatalf("pruned %d, want 1 (the expired one)", n)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, alice.ID); err != nil {
		t.Fatal(err)
	}
	var left int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM refresh_tokens WHERE user_id = $1`, alice.ID).Scan(&left)
	if left != 0 {
		t.Fatal("refresh tokens must be deleted with their user")
	}
}

func TestSessionsValidAfterAndPasswordRoundTrip(t *testing.T) {
	pool := testPool(t)
	users := NewUserRepo(pool)
	u := mkUser(t, users, "a@example.com")
	ctx := context.Background()
	if got, _ := users.GetByID(ctx, u.ID); !got.SessionsValidAfter.Before(time.Now().Add(-24 * time.Hour * 365)) {
		t.Fatalf("new users start with an epoch marker: %v", got.SessionsValidAfter)
	}
	at := time.Now().UTC().Truncate(time.Microsecond)
	if err := users.RevokeSessions(ctx, u.ID, at); err != nil {
		t.Fatal(err)
	}
	if got, _ := users.GetByID(ctx, u.ID); !got.SessionsValidAfter.Equal(at) {
		t.Fatalf("revocation marker: %v want %v", got.SessionsValidAfter, at)
	}
	later := at.Add(time.Minute)
	if err := users.SetPassword(ctx, u.ID, "new-hash", later); err != nil {
		t.Fatal(err)
	}
	if got, _ := users.GetByID(ctx, u.ID); got.PasswordHash != "new-hash" || !got.SessionsValidAfter.Equal(later) {
		t.Fatalf("password: %+v", got)
	}
	if err := users.SetPassword(ctx, "nope", "x", later); err == nil {
		t.Fatal("unknown user must be an error")
	}
	if err := users.RevokeSessions(ctx, "nope", later); err == nil {
		t.Fatal("unknown user must be an error")
	}
}
