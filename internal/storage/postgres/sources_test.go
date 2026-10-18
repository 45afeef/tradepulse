package postgres

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"tradepulse/internal/domain/sourcedef"
	"tradepulse/internal/idgen"
)

func mkSource(t *testing.T, r *SourceRepo, name string, web *sourcedef.WebConfig) sourcedef.Source {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Microsecond)
	typ := sourcedef.TypeRSS
	if web != nil {
		typ = sourcedef.TypeWebMonitor
	}
	s := sourcedef.Source{ID: idgen.New(), Name: name, Type: typ, URL: "https://example.com/" + name, Enabled: true, IntervalSeconds: 300,
		Entities: []string{"markets", "india"}, Web: web, CreatedAt: now, UpdatedAt: now}
	if err := r.Create(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSourceRepoRoundTripAndIndependentState(t *testing.T) {
	pool := testPool(t)
	r := NewSourceRepo(pool)
	ctx := context.Background()
	rss := mkSource(t, r, "cnbc", nil)
	web := mkSource(t, r, "scanx", &sourcedef.WebConfig{ItemSelector: "div.item", TitleSelector: "h3", MaxItems: 20, IgnoreRobots: true, IgnoreRobotsReason: "written permission"})

	if err := r.Create(ctx, sourcedef.Source{ID: idgen.New(), Name: "cnbc", Type: sourcedef.TypeRSS, CreatedAt: time.Now(), UpdatedAt: time.Now()}); err != sourcedef.ErrNameTaken {
		t.Fatalf("duplicate name: %v", err)
	}
	got, err := r.GetByName(ctx, "scanx")
	if err != nil || got.ID != web.ID || got.Web == nil || got.Web.ItemSelector != "div.item" || !got.Web.IgnoreRobots || got.Web.MaxItems != 20 {
		t.Fatalf("web config did not round-trip: %+v %v", got.Web, err)
	}
	if g, _ := r.Get(ctx, rss.ID); g.Web != nil || len(g.Entities) != 2 {
		t.Fatalf("rss source: %+v", g)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	st := sourcedef.State{LastRunAt: &now, LastSuccessAt: &now, NextRunAt: &now, LastItemAt: &now, LastError: "boom",
		ConsecutiveFailures: 2, UnchangedStreak: 7, ETag: `"e"`, LastModified: "Mon", ItemsTotal: 42}
	if err := r.SaveState(ctx, rss.ID, st); err != nil {
		t.Fatal(err)
	}
	upd := rss
	upd.URL, upd.Enabled, upd.IntervalSeconds, upd.Entities, upd.UpdatedAt = "https://example.com/new", false, 900, []string{"x"}, now
	if err := r.Update(ctx, upd); err != nil {
		t.Fatal(err)
	}
	got, _ = r.Get(ctx, rss.ID)
	if got.URL != "https://example.com/new" || got.Enabled || got.IntervalSeconds != 900 || len(got.Entities) != 1 {
		t.Fatalf("update not applied: %+v", got)
	}
	if got.State.ItemsTotal != 42 || got.State.ETag != `"e"` || got.State.ConsecutiveFailures != 2 || got.State.LastError != "boom" || got.State.NextRunAt == nil {
		t.Fatalf("Update must not touch runtime state: %+v", got.State)
	}
	if err := r.SaveState(ctx, rss.ID, sourcedef.State{ItemsTotal: 43}); err != nil {
		t.Fatal(err)
	}
	if got, _ = r.Get(ctx, rss.ID); got.URL != "https://example.com/new" || got.State.ItemsTotal != 43 || got.State.NextRunAt != nil {
		t.Fatalf("SaveState must not touch config: %+v", got)
	}
	for _, f := range []func() error{
		func() error { return r.Update(ctx, sourcedef.Source{ID: "nope"}) },
		func() error { return r.SaveState(ctx, "nope", sourcedef.State{}) },
		func() error { return r.Delete(ctx, "nope") },
	} {
		if err := f(); err != sourcedef.ErrNotFound {
			t.Fatalf("unknown id: %v", err)
		}
	}
	if _, err := r.GetByName(ctx, "nope"); err != sourcedef.ErrNotFound {
		t.Fatal("GetByName unknown")
	}
	if list, _ := r.List(ctx); len(list) != 2 || list[0].Name != "cnbc" {
		t.Fatalf("list order: %+v", list)
	}
	if _, err := pool.Exec(ctx, `UPDATE sources SET type = 'ftp' WHERE id = $1`, rss.ID); err == nil {
		t.Fatal("CHECK constraint must reject unknown source types")
	}
}

func TestItemStoreAtomicDedupeUnderConcurrency(t *testing.T) {
	pool := testPool(t)
	src := mkSource(t, NewSourceRepo(pool), "dedupe", nil)
	items := NewItemStore(pool)
	ctx := context.Background()
	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, err := items.Insert(ctx, src.ID, "same-key", []byte(`{"a":1}`), time.Now().UTC()); err == nil && ok {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("key inserted as new %d times against Postgres, want exactly 1", wins.Load())
	}
	if ok, _ := items.Insert(ctx, src.ID, "other", nil, time.Now().UTC()); !ok {
		t.Fatal("empty raw payload must be accepted")
	}
	old := time.Now().UTC().Add(-45 * 24 * time.Hour)
	_, _ = items.Insert(ctx, src.ID, "ancient", []byte(`{}`), old)
	if n, err := items.Prune(ctx, time.Now().UTC().Add(-30*24*time.Hour)); err != nil || n != 1 {
		t.Fatalf("prune: %d %v", n, err)
	}
	if err := fkViolation(items, "no-such-source"); err == nil {
		t.Fatal("foreign key must reject items for unknown sources")
	}
}

func fkViolation(items *ItemStore, sourceID string) error {
	_, err := items.Insert(context.Background(), sourceID, "k", []byte(`{}`), time.Now().UTC())
	return err
}

func TestRunRepoRecentPruneAndCascadeDelete(t *testing.T) {
	pool := testPool(t)
	srcs, runs, items := NewSourceRepo(pool), NewRunRepo(pool), NewItemStore(pool)
	ctx := context.Background()
	src := mkSource(t, srcs, "runs", nil)
	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		err := runs.Add(ctx, sourcedef.Run{ID: idgen.New(), SourceID: src.ID, StartedAt: now.Add(time.Duration(i) * time.Second),
			DurationMS: 10, OK: i%2 == 0, HTTPStatus: 200, ItemsSeen: i, ItemsNew: 1, ItemsPublished: 1, Error: fmt.Sprint("e", i)})
		if err != nil {
			t.Fatal(err)
		}
	}
	_ = runs.Add(ctx, sourcedef.Run{ID: idgen.New(), SourceID: src.ID, StartedAt: now.Add(-40 * 24 * time.Hour), OK: true})
	got, err := runs.Recent(ctx, src.ID, 3)
	if err != nil || len(got) != 3 || got[0].ItemsSeen != 4 || got[2].ItemsSeen != 2 {
		t.Fatalf("recent: %+v %v", got, err)
	}
	if n, _ := runs.Prune(ctx, now.Add(-30*24*time.Hour)); n != 1 {
		t.Fatalf("pruned %d, want 1", n)
	}
	_, _ = items.Insert(ctx, src.ID, "k", []byte(`{}`), now)
	if err := srcs.Delete(ctx, src.ID); err != nil {
		t.Fatal(err)
	}
	var left int
	_ = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM source_runs) + (SELECT count(*) FROM source_items)`).Scan(&left)
	if left != 0 {
		t.Fatalf("%d rows survived deleting their source", left)
	}
}

func TestWebhookXConfigCursorAndSecretVersion(t *testing.T) {
	pool := testPool(t)
	r := NewSourceRepo(pool)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	hook := sourcedef.Source{ID: idgen.New(), Name: "hook", Type: sourcedef.TypeWebhook, Enabled: true, Webhook: &sourcedef.WebhookConfig{Auth: "token"}, CreatedAt: now, UpdatedAt: now}
	xs := sourcedef.Source{ID: idgen.New(), Name: "modi", Type: sourcedef.TypeX, Enabled: true, IntervalSeconds: 300, X: &sourcedef.XConfig{Handle: "narendramodi", UserID: "55"}, CreatedAt: now, UpdatedAt: now}
	for _, s := range []sourcedef.Source{hook, xs} {
		if err := r.Create(ctx, s); err != nil {
			t.Fatalf("create %s: %v", s.Type, err)
		}
	}
	got, _ := r.Get(ctx, hook.ID)
	if got.Webhook == nil || got.Webhook.Auth != "token" || got.SecretVersion != 1 {
		t.Fatalf("webhook config: %+v", got)
	}
	v, err := r.BumpSecretVersion(ctx, hook.ID)
	if err != nil || v != 2 {
		t.Fatalf("bump: %d %v", v, err)
	}
	if _, err := r.BumpSecretVersion(ctx, "nope"); err != sourcedef.ErrNotFound {
		t.Fatalf("unknown id: %v", err)
	}
	if err := r.SaveState(ctx, xs.ID, sourcedef.State{Cursor: "1846987139428637184", ItemsTotal: 3}); err != nil {
		t.Fatal(err)
	}
	gx, _ := r.Get(ctx, xs.ID)
	if gx.X == nil || gx.X.UserID != "55" || gx.State.Cursor != "1846987139428637184" {
		t.Fatalf("x config/cursor: %+v", gx)
	}
	upd := gx
	upd.X = &sourcedef.XConfig{Handle: "elonmusk", UserID: "44"}
	upd.UpdatedAt = now
	if err := r.Update(ctx, upd); err != nil {
		t.Fatal(err)
	}
	if g, _ := r.Get(ctx, xs.ID); g.X.Handle != "elonmusk" || g.State.Cursor == "" {
		t.Fatalf("update must change config but keep the cursor: %+v", g)
	}
	if _, err := pool.Exec(ctx, `UPDATE sources SET type = 'ftp' WHERE id = $1`, xs.ID); err == nil {
		t.Fatal("CHECK constraint must still reject unknown types")
	}
}

func TestUsageLedgerIsAtomicAndMonthly(t *testing.T) {
	pool := testPool(t)
	u := NewUsageRepo(pool)
	ctx := context.Background()
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := u.Add(ctx, "x", 1000, now); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if got, err := u.Month(ctx, "x", now); err != nil || got != 50000 {
		t.Fatalf("concurrent upserts lost updates: %d %v", got, err)
	}
	if got, _ := u.Month(ctx, "x", now.AddDate(0, 1, 0)); got != 0 {
		t.Fatalf("next month must be empty: %d", got)
	}
	if total, _ := u.Add(ctx, "x", 500, now); total != 50500 {
		t.Fatalf("Add must return the new total: %d", total)
	}
}
