package postgres

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"tradepulse/internal/domain/audit"
	"tradepulse/internal/domain/feed"
	"tradepulse/internal/domain/invite"
	"tradepulse/internal/domain/rule"
	"tradepulse/internal/domain/user"
	"tradepulse/internal/idgen"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// testPool returns a pool bound to a fresh, isolated schema (dropped afterwards).
// Integration tests are skipped when TEST_DATABASE_URL is not set.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping Postgres integration test")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	schema := "t_" + idgen.New()[:12]
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
		admin.Close()
	})
	if err := Migrate(ctx, pool, quiet); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return pool
}

func mkUser(t *testing.T, r *UserRepo, email string) user.User {
	t.Helper()
	u := user.User{ID: idgen.New(), Email: email, PasswordHash: "hash", CreatedAt: time.Now().UTC(),
		Profile: user.DefaultProfile(), Role: user.RoleMember, Status: user.StatusActive}
	if err := r.Create(context.Background(), u); err != nil {
		t.Fatal(err)
	}
	return u
}

func TestMigrateUpDownUpAndIdempotent(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	has := func() bool {
		var ok bool
		if err := pool.QueryRow(ctx, "SELECT to_regclass('users') IS NOT NULL").Scan(&ok); err != nil {
			t.Fatal(err)
		}
		return ok
	}
	if !has() {
		t.Fatal("users table missing after migrate")
	}
	if err := Migrate(ctx, pool, quiet); err != nil {
		t.Fatalf("second migrate must be a no-op: %v", err)
	}
	if err := MigrateDown(ctx, pool); err != nil {
		t.Fatalf("down: %v", err)
	}
	if has() {
		t.Fatal("users table still present after down")
	}
	if err := Migrate(ctx, pool, quiet); err != nil || !has() {
		t.Fatalf("up after down failed: %v", err)
	}
}

func TestEveryTableHasRowLevelSecurity(t *testing.T) {
	pool := testPool(t)
	rows, err := pool.Query(context.Background(),
		`SELECT c.relname FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		 WHERE n.nspname = current_schema() AND c.relkind = 'r' AND NOT c.relrowsecurity`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var bad []string
	for rows.Next() {
		var n string
		_ = rows.Scan(&n)
		bad = append(bad, n)
	}
	if len(bad) > 0 {
		t.Fatalf("tables without RLS (exposed by provider HTTP APIs): %v", bad)
	}
}

func TestUserRepoRoundTrip(t *testing.T) {
	pool := testPool(t)
	r := NewUserRepo(pool)
	ctx := context.Background()
	u := mkUser(t, r, "Asha@Example.com")

	if err := r.Create(ctx, user.User{ID: idgen.New(), Email: "ASHA@example.COM", PasswordHash: "x", CreatedAt: time.Now()}); err != user.ErrEmailTaken {
		t.Fatalf("duplicate email (case-insensitive): got %v", err)
	}
	got, err := r.GetByEmail(ctx, "asha@EXAMPLE.com")
	if err != nil || got.ID != u.ID || got.Role != user.RoleMember {
		t.Fatalf("GetByEmail: %+v %v", got, err)
	}
	if _, err := r.GetByID(ctx, "nope"); err != user.ErrNotFound {
		t.Fatalf("unknown id: %v", err)
	}

	p := user.Profile{Subscriptions: []user.Subscription{{
		Kind: "entity", Value: "tcs", Types: []string{"price_changed"},
		Conditions: []rule.Condition{{Field: "abs_change_pct", Op: rule.Gte, Value: 1.5}, {Field: "status", Op: rule.In, Value: []any{"down", "degraded"}}},
	}}, Muted: []string{"spam"}}
	if err := r.UpdateProfile(ctx, u.ID, p); err != nil {
		t.Fatal(err)
	}
	got, _ = r.GetByID(ctx, u.ID)
	if len(got.Profile.Subscriptions) != 1 || got.Profile.Subscriptions[0].Conditions[0].Value != 1.5 ||
		len(got.Profile.Subscriptions[0].Conditions[1].Value.([]any)) != 2 || got.Profile.Muted[0] != "spam" {
		t.Fatalf("profile did not round-trip: %+v", got.Profile)
	}
	if err := r.UpdateProfile(ctx, "nope", p); err != user.ErrNotFound {
		t.Fatalf("update unknown: %v", err)
	}

	if err := r.SetStatus(ctx, u.ID, user.StatusSuspended); err != nil {
		t.Fatal(err)
	}
	if got, _ = r.GetByID(ctx, u.ID); got.Status != user.StatusSuspended {
		t.Fatal("status not persisted")
	}
	if err := r.SetStatus(ctx, "nope", user.StatusActive); err != user.ErrNotFound {
		t.Fatalf("status unknown: %v", err)
	}
	if err := r.SetStatus(ctx, u.ID, "banned"); err == nil {
		t.Fatal("CHECK constraint must reject invalid status")
	}
	if list, _ := r.List(ctx); len(list) != 1 {
		t.Fatalf("list: %d", len(list))
	}
}

func TestSQLInjectionStringsAreInert(t *testing.T) {
	pool := testPool(t)
	r := NewUserRepo(pool)
	ctx := context.Background()
	mkUser(t, r, "victim@example.com")
	for _, evil := range []string{
		"' OR '1'='1", "x'; DROP TABLE users; --", "victim@example.com' --", `"; DELETE FROM users; --`, "%", "victim@example.com\x00",
	} {
		if u, err := r.GetByEmail(ctx, evil); err == nil {
			t.Fatalf("injection string matched a user: %q -> %+v", evil, u)
		}
		if _, err := r.GetByID(ctx, evil); err == nil {
			t.Fatalf("injection string matched by id: %q", evil)
		}
	}
	if list, err := r.List(ctx); err != nil || len(list) != 1 {
		t.Fatalf("users table damaged: %v %d", err, len(list))
	}
	weird := mkUser(t, r, "o'brien+test@example.com") // quotes in data must round-trip
	if got, err := r.GetByEmail(ctx, "O'BRIEN+test@example.com"); err != nil || got.ID != weird.ID {
		t.Fatalf("quoted email lookup failed: %v", err)
	}
}

func newInvite(t *testing.T, r *InviteRepo, email string, ttl time.Duration) (code string, id string) {
	t.Helper()
	code, hash, err := invite.NewCode()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	id = idgen.New()
	if err := r.Create(context.Background(), invite.Invite{ID: id, CodeHash: hash, Email: email, Note: "n", CreatedBy: "admin", CreatedAt: now, ExpiresAt: now.Add(ttl)}); err != nil {
		t.Fatal(err)
	}
	return code, id
}

func TestInviteRedeemExactlyOnceConcurrently(t *testing.T) {
	pool := testPool(t)
	r := NewInviteRepo(pool)
	code, _ := newInvite(t, r, "", time.Hour)
	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := r.Redeem(context.Background(), invite.HashCode(code), "a@example.com", fmt.Sprint(i), time.Now().UTC()); err == nil {
				wins.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("invite redeemed %d times against a real database, want exactly 1", wins.Load())
	}
}

func TestInviteRules(t *testing.T) {
	pool := testPool(t)
	r := NewInviteRepo(pool)
	ctx := context.Background()
	now := time.Now().UTC()

	expired, _ := newInvite(t, r, "", -time.Minute)
	if _, err := r.Redeem(ctx, invite.HashCode(expired), "a@example.com", "u", now); err != invite.ErrInvalid {
		t.Errorf("expired accepted: %v", err)
	}
	bound, _ := newInvite(t, r, "asha@example.com", time.Hour)
	if _, err := r.Redeem(ctx, invite.HashCode(bound), "mallory@example.com", "u", now); err != invite.ErrInvalid {
		t.Errorf("wrong email accepted: %v", err)
	}
	if _, err := r.Redeem(ctx, invite.HashCode("tp_inv_unknown"), "a@example.com", "u", now); err != invite.ErrInvalid {
		t.Errorf("unknown code accepted: %v", err)
	}
	inv, err := r.Redeem(ctx, invite.HashCode(bound), "ASHA@Example.com", "alice", now)
	if err != nil || inv.UsedBy != "alice" || inv.UsedAt == nil {
		t.Fatalf("bound redeem failed: %+v %v", inv, err)
	}
	_ = r.Release(ctx, inv.ID, "mallory")
	if _, err := r.Redeem(ctx, invite.HashCode(bound), "asha@example.com", "bob", now); err == nil {
		t.Error("release by a different user must not free the invite")
	}
	_ = r.Release(ctx, inv.ID, "alice")
	if _, err := r.Redeem(ctx, invite.HashCode(bound), "asha@example.com", "bob", now); err != nil {
		t.Errorf("released invite must be usable again: %v", err)
	}

	code, id := newInvite(t, r, "", time.Hour)
	if err := r.Revoke(ctx, "nope", now); err != invite.ErrNotFound {
		t.Errorf("revoke unknown: %v", err)
	}
	if err := r.Revoke(ctx, id, now); err != nil {
		t.Fatal(err)
	}
	if err := r.Revoke(ctx, id, now); err != invite.ErrInvalid {
		t.Errorf("double revoke: %v", err)
	}
	if _, err := r.Redeem(ctx, invite.HashCode(code), "a@example.com", "u", now); err != invite.ErrInvalid {
		t.Errorf("revoked invite redeemed: %v", err)
	}

	list, _ := r.List(ctx)
	if strings.Contains(fmt.Sprintf("%+v", list), code) {
		t.Fatal("plaintext code present in stored data")
	}
}

func TestFeedRepoOrderIsolationAndPrune(t *testing.T) {
	pool := testPool(t)
	users, feeds := NewUserRepo(pool), NewFeedRepo(pool)
	ctx := context.Background()
	alice, bob := mkUser(t, users, "alice@example.com"), mkUser(t, users, "bob@example.com")
	for i := 0; i < 5; i++ {
		err := feeds.Add(ctx, feed.FeedItem{ID: idgen.New(), UserID: alice.ID, EventID: "e", EventType: "content_item", Source: "S",
			Entities: []string{"x"}, Title: fmt.Sprint("a", i), Body: "b", Severity: feed.Info, Reason: "r", CreatedAt: time.Now().UTC()})
		if err != nil {
			t.Fatal(err)
		}
	}
	_ = feeds.Add(ctx, feed.FeedItem{ID: idgen.New(), UserID: bob.ID, EventID: "e", EventType: "t", Source: "S", Title: "b0", Body: "b", Severity: feed.High, Reason: "r", CreatedAt: time.Now().UTC()})

	got, err := feeds.Recent(ctx, alice.ID, 10)
	if err != nil || len(got) != 5 || got[0].Title != "a4" || got[4].Title != "a0" {
		t.Fatalf("order wrong: %+v %v", got, err)
	}
	for _, it := range got {
		if it.UserID != alice.ID {
			t.Fatal("cross-user leak")
		}
	}
	if none, _ := feeds.Recent(ctx, "carol", 10); len(none) != 0 {
		t.Fatal("unknown user must have an empty feed")
	}
	if bobItems, _ := feeds.Recent(ctx, bob.ID, 10); len(bobItems) != 1 || bobItems[0].Entities == nil {
		t.Fatalf("bob: %+v", bobItems)
	}

	fd, _, err := Prune(ctx, pool, 2, 24*time.Hour)
	if err != nil || fd != 3 {
		t.Fatalf("prune deleted %d (want 3): %v", fd, err)
	}
	if after, _ := feeds.Recent(ctx, alice.ID, 10); len(after) != 2 || after[0].Title != "a4" {
		t.Fatalf("prune must keep the newest: %+v", after)
	}
	if bobAfter, _ := feeds.Recent(ctx, bob.ID, 10); len(bobAfter) != 1 {
		t.Fatal("prune must be per user")
	}
}

func TestDeletingUserCascadesFeed(t *testing.T) {
	pool := testPool(t)
	users, feeds := NewUserRepo(pool), NewFeedRepo(pool)
	ctx := context.Background()
	u := mkUser(t, users, "gone@example.com")
	_ = feeds.Add(ctx, feed.FeedItem{ID: idgen.New(), UserID: u.ID, EventID: "e", EventType: "t", Source: "S", Title: "t", Body: "b", Severity: feed.Info, Reason: "r", CreatedAt: time.Now().UTC()})
	if _, err := pool.Exec(ctx, "DELETE FROM users WHERE id = $1", u.ID); err != nil {
		t.Fatal(err)
	}
	if items, _ := feeds.Recent(ctx, u.ID, 10); len(items) != 0 {
		t.Fatal("feed items survived user deletion")
	}
	if err := feeds.Add(ctx, feed.FeedItem{ID: idgen.New(), UserID: "no-such-user", EventID: "e", EventType: "t", Source: "S", Title: "t", Body: "b", Severity: feed.Info, Reason: "r", CreatedAt: time.Now().UTC()}); err == nil {
		t.Fatal("foreign key must reject items for unknown users")
	}
}

func TestAuditRepoAndAgePrune(t *testing.T) {
	pool := testPool(t)
	r := NewAuditRepo(pool)
	ctx := context.Background()
	old := time.Now().UTC().Add(-48 * time.Hour)
	_ = r.Append(ctx, audit.Entry{ID: "1", At: old, ActorID: "a", Action: "old"})
	_ = r.Append(ctx, audit.Entry{ID: "2", At: time.Now().UTC().Add(-time.Minute), ActorID: "a", Action: "mid"})
	_ = r.Append(ctx, audit.Entry{ID: "3", At: time.Now().UTC(), ActorID: "a", Action: "new", TargetType: "user", TargetID: "u", Detail: "d"})
	got, err := r.Recent(ctx, 10)
	if err != nil || len(got) != 3 || got[0].Action != "new" || got[2].Action != "old" || got[0].TargetID != "u" {
		t.Fatalf("recent: %+v %v", got, err)
	}
	if limited, _ := r.Recent(ctx, 1); len(limited) != 1 {
		t.Fatal("limit ignored")
	}
	_, ad, err := Prune(ctx, pool, 1000, 24*time.Hour)
	if err != nil || ad != 1 {
		t.Fatalf("audit prune deleted %d (want 1): %v", ad, err)
	}
}
