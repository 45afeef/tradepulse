package memory

import (
	"context"
	"testing"
	"time"

	"tradepulse/internal/domain/alert"
	"tradepulse/internal/domain/feed"
	"tradepulse/internal/domain/feedback"
	"tradepulse/internal/domain/view"
)

func TestViewRepoScopingDuplicatesAndUpdate(t *testing.T) {
	r := NewViewRepo()
	ctx := context.Background()
	now := time.Now()
	a := view.View{ID: "v1", UserID: "alice", Spec: view.Spec{Name: "Movers"}, CreatedAt: now}
	if err := r.Create(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := r.Create(ctx, view.View{ID: "v2", UserID: "alice", Spec: view.Spec{Name: "movers"}}); err != view.ErrDuplicate {
		t.Fatalf("names are unique per member, case-insensitively: %v", err)
	}
	if err := r.Create(ctx, view.View{ID: "v3", UserID: "bob", Spec: view.Spec{Name: "Movers"}}); err != nil {
		t.Fatalf("another member may reuse the name: %v", err)
	}
	if _, err := r.Get(ctx, "bob", "v1"); err != view.ErrNotFound {
		t.Fatal("scoping")
	}
	if err := r.Update(ctx, view.View{ID: "v1", UserID: "bob", Spec: view.Spec{Name: "x"}}); err != view.ErrNotFound {
		t.Fatal("another member must not update the view")
	}
	if err := r.Delete(ctx, "bob", "v1"); err != view.ErrNotFound {
		t.Fatal("another member must not delete the view")
	}
	a.Spec.Name, a.UpdatedAt = "Renamed", now.Add(time.Minute)
	if err := r.Update(ctx, a); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.Get(ctx, "alice", "v1"); got.Spec.Name != "Renamed" {
		t.Fatalf("update: %+v", got)
	}
	if l, _ := r.List(ctx, "alice"); len(l) != 1 {
		t.Fatalf("list: %d", len(l))
	}
}

func TestFeedbackRepoUpsertSummaryPruneAndLookups(t *testing.T) {
	r := NewFeedbackRepo()
	ctx := context.Background()
	now := time.Now()
	put := func(u, typ, id, rating, grp string, at time.Time) {
		_ = r.Upsert(ctx, feedback.Entry{UserID: u, TargetType: typ, TargetID: id, Rating: rating, Group: grp, Label: grp, CreatedAt: at, UpdatedAt: at})
	}
	put("alice", feedback.TargetAlertFire, "f1", feedback.Useful, "rule:a", now)
	put("alice", feedback.TargetAlertFire, "f2", feedback.NotUseful, "rule:a", now)
	put("bob", feedback.TargetAlertFire, "f3", feedback.Useful, "rule:b", now)
	put("alice", feedback.TargetFeedItem, "i1", feedback.NotUseful, "source:x", now)
	put("alice", feedback.TargetFeedItem, "old", feedback.Useful, "source:x", now.AddDate(-2, 0, 0))
	put("alice", feedback.TargetAlertFire, "f1", feedback.NotUseful, "rule:a", now) // changed her mind
	got, _ := r.Ratings(ctx, "alice", feedback.TargetAlertFire, []string{"f1", "f2", "f3", "nope"})
	if len(got) != 2 || got["f1"] != feedback.NotUseful || got["f2"] != feedback.NotUseful {
		t.Fatalf("ratings are per member and the latest wins: %v", got)
	}
	sum, _ := r.Summary(ctx, now.AddDate(0, 0, -30))
	if sum.AlertFires.Useful != 1 || sum.AlertFires.NotUseful != 2 || sum.FeedItems.NotUseful != 1 || sum.FeedItems.Useful != 0 {
		t.Fatalf("summary window/totals: %+v", sum)
	}
	if len(sum.Groups) != 3 || sum.Groups[0].Group != "rule:a" {
		t.Fatalf("busiest group first: %+v", sum.Groups)
	}
	_ = r.Delete(ctx, "alice", feedback.TargetAlertFire, "f1")
	if got, _ := r.Ratings(ctx, "alice", feedback.TargetAlertFire, []string{"f1"}); len(got) != 0 {
		t.Fatal("delete")
	}
	if n, _ := r.Prune(ctx, now.AddDate(-1, 0, 0)); n != 1 {
		t.Fatalf("pruned %d", n)
	}
}

func TestGetByIDIsScopedToTheOwner(t *testing.T) {
	ctx := context.Background()
	fr, fires := NewFeedRepo(10), NewAlertFireRepo()
	_ = fr.Add(ctx, feed.FeedItem{ID: "i1", UserID: "alice"})
	_ = fires.Add(ctx, alert.Fire{ID: "f1", UserID: "alice"})
	if _, err := fr.Get(ctx, "alice", "i1"); err != nil {
		t.Fatal(err)
	}
	if _, err := fr.Get(ctx, "bob", "i1"); err != feed.ErrNotFound {
		t.Fatalf("another member's item: %v", err)
	}
	if _, err := fires.Get(ctx, "alice", "f1"); err != nil {
		t.Fatal(err)
	}
	if _, err := fires.Get(ctx, "bob", "f1"); err != alert.ErrNotFound {
		t.Fatalf("another member's alert: %v", err)
	}
}
