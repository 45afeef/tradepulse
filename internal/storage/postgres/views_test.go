package postgres

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"tradepulse/internal/domain/alert"
	"tradepulse/internal/domain/feed"
	"tradepulse/internal/domain/feedback"
	"tradepulse/internal/domain/view"
	"tradepulse/internal/idgen"
)

func TestViewRepositoryOnPostgres(t *testing.T) {
	pool := testPool(t)
	users := NewUserRepo(pool)
	alice, bob := mkUser(t, users, "a@example.com"), mkUser(t, users, "b@example.com")
	r := NewViewRepo(pool)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	v := view.View{ID: idgen.New(), UserID: alice.ID, CreatedAt: now, UpdatedAt: now,
		Spec: view.Spec{Name: "Movers", Filter: view.Filter{Sources: []string{"cnbc"}, Keywords: []string{"oil"}, WithinHours: 24}}}
	if err := r.Create(ctx, v); err != nil {
		t.Fatal(err)
	}
	if err := r.Create(ctx, view.View{ID: idgen.New(), UserID: alice.ID, CreatedAt: now, UpdatedAt: now, Spec: view.Spec{Name: "MOVERS", Filter: view.Filter{Sources: []string{"x"}}}}); err != view.ErrDuplicate {
		t.Fatalf("unique per member, case-insensitive: %v", err)
	}
	if err := r.Create(ctx, view.View{ID: idgen.New(), UserID: bob.ID, CreatedAt: now, UpdatedAt: now, Spec: view.Spec{Name: "Movers", Filter: view.Filter{Sources: []string{"x"}}}}); err != nil {
		t.Fatalf("another member may reuse the name: %v", err)
	}
	got, err := r.Get(ctx, alice.ID, v.ID)
	if err != nil || got.Spec.Name != "Movers" || got.Spec.Filter.Sources[0] != "cnbc" || got.Spec.Filter.WithinHours != 24 || got.Spec.Filter.Keywords[0] != "oil" {
		t.Fatalf("round trip: %+v %v", got, err)
	}
	if _, err := r.Get(ctx, bob.ID, v.ID); err != view.ErrNotFound {
		t.Fatal("scoping")
	}
	if err := r.Update(ctx, view.View{ID: v.ID, UserID: bob.ID, UpdatedAt: now, Spec: v.Spec}); err != view.ErrNotFound {
		t.Fatal("another member must not update")
	}
	v.Spec.Name, v.UpdatedAt = "Renamed", now.Add(time.Second)
	if err := r.Update(ctx, v); err != nil {
		t.Fatal(err)
	}
	if err := r.Delete(ctx, bob.ID, v.ID); err != view.ErrNotFound {
		t.Fatal("another member must not delete")
	}
	if err := r.Delete(ctx, alice.ID, v.ID); err != nil {
		t.Fatal(err)
	}
}

func TestFeedbackRepositoryOnPostgres(t *testing.T) {
	pool := testPool(t)
	users := NewUserRepo(pool)
	alice, bob := mkUser(t, users, "a@example.com"), mkUser(t, users, "b@example.com")
	r := NewFeedbackRepo(pool)
	ctx := context.Background()
	now := time.Now().UTC()
	put := func(u, typ, id, rating, grp string, at time.Time) {
		if err := r.Upsert(ctx, feedback.Entry{UserID: u, TargetType: typ, TargetID: id, Rating: rating, Note: "n", Group: grp, Label: grp + "-label", CreatedAt: at, UpdatedAt: at}); err != nil {
			t.Fatal(err)
		}
	}
	put(alice.ID, feedback.TargetAlertFire, "f1", feedback.Useful, "rule:a", now)
	put(alice.ID, feedback.TargetAlertFire, "f2", feedback.NotUseful, "rule:a", now)
	put(bob.ID, feedback.TargetAlertFire, "f3", feedback.Useful, "rule:b", now)
	put(alice.ID, feedback.TargetFeedItem, "i1", feedback.NotUseful, "source:x", now)
	put(alice.ID, feedback.TargetFeedItem, "old", feedback.Useful, "source:x", now.AddDate(-2, 0, 0))
	put(alice.ID, feedback.TargetAlertFire, "f1", feedback.NotUseful, "rule:a", now) // upsert: changed her mind
	got, err := r.Ratings(ctx, alice.ID, feedback.TargetAlertFire, []string{"f1", "f2", "f3", "nope"})
	if err != nil || len(got) != 2 || got["f1"] != feedback.NotUseful {
		t.Fatalf("ratings: %v %v", got, err)
	}
	if empty, _ := r.Ratings(ctx, alice.ID, feedback.TargetFeedItem, nil); len(empty) != 0 {
		t.Fatal("no ids, no ratings")
	}
	sum, err := r.Summary(ctx, now.AddDate(0, 0, -30))
	if err != nil || sum.AlertFires.Useful != 1 || sum.AlertFires.NotUseful != 2 || sum.FeedItems.NotUseful != 1 || sum.FeedItems.Useful != 0 {
		t.Fatalf("summary: %+v %v", sum, err)
	}
	if len(sum.Groups) != 3 || sum.Groups[0].Group != "rule:a" || sum.Groups[0].Label != "rule:a-label" || sum.Groups[0].NotUseful != 2 {
		t.Fatalf("groups: %+v", sum.Groups)
	}
	if err := r.Upsert(ctx, feedback.Entry{UserID: alice.ID, TargetType: "sms", TargetID: "x", Rating: feedback.Useful, CreatedAt: now, UpdatedAt: now}); err == nil {
		t.Fatal("CHECK must reject unknown target types")
	}
	if err := r.Upsert(ctx, feedback.Entry{UserID: alice.ID, TargetType: feedback.TargetFeedItem, TargetID: "x", Rating: "meh", CreatedAt: now, UpdatedAt: now}); err == nil {
		t.Fatal("CHECK must reject unknown ratings")
	}
	_ = r.Delete(ctx, alice.ID, feedback.TargetAlertFire, "f1")
	if got, _ := r.Ratings(ctx, alice.ID, feedback.TargetAlertFire, []string{"f1"}); len(got) != 0 {
		t.Fatal("delete")
	}
	if n, _ := r.Prune(ctx, now.AddDate(-1, 0, 0)); n != 1 {
		t.Fatalf("pruned %d", n)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, alice.ID); err != nil {
		t.Fatal(err)
	}
	var left int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM feedback WHERE user_id = $1`, alice.ID).Scan(&left)
	if left != 0 {
		t.Fatal("feedback must be deleted with the member")
	}
}

func TestGetByIDIsScopedToTheOwnerOnPostgres(t *testing.T) {
	pool := testPool(t)
	users := NewUserRepo(pool)
	alice, bob := mkUser(t, users, "a@example.com"), mkUser(t, users, "b@example.com")
	feeds, fires := NewFeedRepo(pool), NewAlertFireRepo(pool)
	ctx := context.Background()
	now := time.Now().UTC()
	it, err := feeds.AddItem(ctx, feed.FeedItem{ID: idgen.New(), UserID: alice.ID, EventID: "e", EventType: "t", Source: "s", Entities: []string{"x"}, Title: "t", Body: "b", Severity: feed.Info, Reason: "r", CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	got, err := feeds.Get(ctx, alice.ID, it.ID)
	if err != nil || got.Title != "t" || got.Cursor == "" || got.Entities[0] != "x" {
		t.Fatalf("feed get: %+v %v", got, err)
	}
	if _, err := feeds.Get(ctx, bob.ID, it.ID); err != feed.ErrNotFound {
		t.Fatalf("another member's item: %v", err)
	}
	f := alert.Fire{ID: idgen.New(), RuleID: "r", UserID: alice.ID, EventID: "e", RuleName: "n", Severity: "info", Title: "t", Body: "b", Source: "s", Reason: "r", FiredAt: now}
	if err := fires.Add(ctx, f); err != nil {
		t.Fatal(err)
	}
	if g, err := fires.Get(ctx, alice.ID, f.ID); err != nil || g.RuleName != "n" || g.Cursor == "" {
		t.Fatalf("fire get: %+v %v", g, err)
	}
	if _, err := fires.Get(ctx, bob.ID, f.ID); err != alert.ErrNotFound {
		t.Fatalf("another member's alert: %v", err)
	}
	var wg sync.WaitGroup // concurrent upserts of one rating must not fail or duplicate
	fb := NewFeedbackRepo(pool)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rating := feedback.Useful
			if i%2 == 0 {
				rating = feedback.NotUseful
			}
			if err := fb.Upsert(ctx, feedback.Entry{UserID: alice.ID, TargetType: feedback.TargetFeedItem, TargetID: it.ID, Rating: rating, CreatedAt: now, UpdatedAt: now}); err != nil {
				t.Error(fmt.Sprint("upsert: ", err))
			}
		}(i)
	}
	wg.Wait()
	var n int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM feedback WHERE user_id = $1`, alice.ID).Scan(&n)
	if n != 1 {
		t.Fatalf("one rating per member and target, got %d", n)
	}
}
