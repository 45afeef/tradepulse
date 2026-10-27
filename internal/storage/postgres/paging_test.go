package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"tradepulse/internal/domain/alert"
	"tradepulse/internal/domain/audit"
	"tradepulse/internal/domain/feed"
	"tradepulse/internal/idgen"
	"tradepulse/internal/page"
)

func walkDesc[T any](limit int, fetch func(page.Query) []T, seq func(T) int64) []T {
	var all []T
	q := page.Query{Limit: limit}
	for guard := 0; guard < 1000; guard++ {
		items := fetch(q)
		if len(items) == 0 {
			break
		}
		all = append(all, items...)
		q.Before = seq(items[len(items)-1])
		if len(items) < limit {
			break
		}
	}
	return all
}

func TestCursorPaginationAcrossAllFourRepositories(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	users := NewUserRepo(pool)
	alice, bob := mkUser(t, users, "a@example.com"), mkUser(t, users, "b@example.com")
	feeds, fires, dels, auds := NewFeedRepo(pool), NewAlertFireRepo(pool), NewDeliveryRepo(pool), NewAuditRepo(pool)
	now := time.Now().UTC()
	for i := 1; i <= 7; i++ {
		for _, u := range []string{alice.ID, bob.ID} {
			if _, err := feeds.AddItem(ctx, feed.FeedItem{ID: idgen.New(), UserID: u, EventID: "e", EventType: "t", Source: "s", Title: fmt.Sprint(u[:4], i), Body: "b", Severity: feed.Info, Reason: "r", CreatedAt: now}); err != nil {
				t.Fatal(err)
			}
			if err := fires.Add(ctx, alert.Fire{ID: idgen.New(), RuleID: "r", UserID: u, EventID: "e", RuleName: "n", Severity: "info", Title: fmt.Sprint(i), Body: "b", Source: "s", Reason: "r", FiredAt: now}); err != nil {
				t.Fatal(err)
			}
			if err := dels.Add(ctx, alert.Delivery{ID: idgen.New(), ChannelID: "c", UserID: u, FireID: "f", At: now, OK: true, Attempts: 1}); err != nil {
				t.Fatal(err)
			}
		}
		if err := auds.Append(ctx, audit.Entry{ID: idgen.New(), At: now, ActorID: "a", Action: fmt.Sprint("act", i)}); err != nil {
			t.Fatal(err)
		}
	}
	feedAll := walkDesc(3, func(q page.Query) []feed.FeedItem { x, err := feeds.Page(ctx, alice.ID, q); mustNoErr(t, err); return x }, func(f feed.FeedItem) int64 { return f.Seq })
	fireAll := walkDesc(3, func(q page.Query) []alert.Fire { x, err := fires.Page(ctx, alice.ID, q); mustNoErr(t, err); return x }, func(f alert.Fire) int64 { return f.Seq })
	delAll := walkDesc(3, func(q page.Query) []alert.Delivery { x, err := dels.Page(ctx, alice.ID, q); mustNoErr(t, err); return x }, func(d alert.Delivery) int64 { return d.Seq })
	audAll := walkDesc(3, func(q page.Query) []audit.Entry { x, err := auds.Page(ctx, q); mustNoErr(t, err); return x }, func(e audit.Entry) int64 { return e.Seq })
	if len(feedAll) != 7 || len(fireAll) != 7 || len(delAll) != 7 || len(audAll) != 7 {
		t.Fatalf("every list must be fully covered exactly once: %d %d %d %d", len(feedAll), len(fireAll), len(delAll), len(audAll))
	}
	if feedAll[0].Title != alice.ID[:4]+"7" || feedAll[6].Title != alice.ID[:4]+"1" || audAll[0].Action != "act7" || fireAll[0].Title != "7" {
		t.Fatalf("newest first: %q %q %q", feedAll[0].Title, audAll[0].Action, fireAll[0].Title)
	}
	for _, f := range feedAll {
		if f.UserID != alice.ID || f.Cursor != page.Cursor(f.Seq) || f.Seq == 0 {
			t.Fatalf("isolation/cursor: %+v", f)
		}
	}
	// catch-up (oldest first) from the 3rd oldest feed item
	asc, err := feeds.Page(ctx, alice.ID, page.Query{Limit: 10, After: feedAll[4].Seq})
	if err != nil || len(asc) != 4 || asc[0].Title != alice.ID[:4]+"4" || asc[3].Title != alice.ID[:4]+"7" {
		t.Fatalf("catch-up: %+v %v", asc, err)
	}
	fa, _ := fires.Page(ctx, alice.ID, page.Query{Limit: 2, After: fireAll[3].Seq})
	if len(fa) != 2 || fa[0].Title != "4" || fa[1].Title != "5" {
		t.Fatalf("fires catch-up: %+v", fa)
	}
	if rec, _ := feeds.Recent(ctx, alice.ID, 2); len(rec) != 2 || rec[0].Seq <= rec[1].Seq || rec[0].Cursor == "" {
		t.Fatalf("Recent must be newest first with cursors: %+v", rec)
	}
	none, _ := feeds.Page(ctx, "nobody", page.Query{Limit: 5})
	if len(none) != 0 {
		t.Fatal("unknown user must be empty")
	}
}

func mustNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
