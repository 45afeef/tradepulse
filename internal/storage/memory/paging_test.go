package memory

import (
	"context"
	"fmt"
	"testing"
	"time"

	"tradepulse/internal/domain/alert"
	"tradepulse/internal/domain/audit"
	"tradepulse/internal/domain/feed"
	"tradepulse/internal/page"
)

// walk pages newest-first through a list (Before = last seq seen) and returns everything in order.
func walk[T any](limit int, _ bool, fetch func(page.Query) []T, seq func(T) int64) []T {
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

func TestFeedPagingCoversEverythingOnceAndIsolatesUsers(t *testing.T) {
	r := NewFeedRepo(100)
	ctx := context.Background()
	for i := 1; i <= 7; i++ {
		_ = r.Add(ctx, feed.FeedItem{ID: fmt.Sprint("a", i), UserID: "alice", Title: fmt.Sprint(i)})
		_ = r.Add(ctx, feed.FeedItem{ID: fmt.Sprint("b", i), UserID: "bob"})
	}
	fetch := func(q page.Query) []feed.FeedItem { it, _ := r.Page(ctx, "alice", q); return it }
	seq := func(f feed.FeedItem) int64 { return f.Seq }
	desc := walk(3, true, fetch, seq)
	if len(desc) != 7 || desc[0].ID != "a7" || desc[6].ID != "a1" {
		t.Fatalf("desc walk: %d items", len(desc))
	}
	for _, it := range desc {
		if it.UserID != "alice" || it.Cursor != page.Cursor(it.Seq) || it.Cursor == "" {
			t.Fatalf("isolation/cursor: %+v", it)
		}
	}
	// oldest-first catch-up from the cursor of the 3rd oldest item
	third := desc[4]
	asc, _ := r.Page(ctx, "alice", page.Query{Limit: 10, After: third.Seq})
	if len(asc) != 4 || asc[0].ID != "a4" || asc[3].ID != "a7" {
		t.Fatalf("catch-up: %+v", asc)
	}
	if none, _ := r.Page(ctx, "carol", page.Query{Limit: 5}); len(none) != 0 {
		t.Fatal("unknown users have an empty feed")
	}
	// AddItem returns the assigned position and Recent keeps newest-first with cursors
	got, _ := r.AddItem(ctx, feed.FeedItem{ID: "a8", UserID: "alice"})
	if got.Seq == 0 || got.Cursor == "" {
		t.Fatalf("AddItem must return Seq/Cursor: %+v", got)
	}
	if rec, _ := r.Recent(ctx, "alice", 1); len(rec) != 1 || rec[0].ID != "a8" || rec[0].Cursor != got.Cursor {
		t.Fatalf("recent: %+v", rec)
	}
}

func TestFeedCapKeepsCursorsValid(t *testing.T) {
	r := NewFeedRepo(3)
	ctx := context.Background()
	for i := 1; i <= 6; i++ {
		_ = r.Add(ctx, feed.FeedItem{ID: fmt.Sprint(i), UserID: "u"})
	}
	items, _ := r.Page(ctx, "u", page.Query{Limit: 10})
	if len(items) != 3 || items[0].ID != "6" || items[2].ID != "4" {
		t.Fatalf("cap: %+v", items)
	}
	old, _ := r.Page(ctx, "u", page.Query{Limit: 10, After: 1}) // a cursor from before the cap still works
	if len(old) != 3 || old[0].ID != "4" {
		t.Fatalf("stale cursor must resume at the oldest retained item: %+v", old)
	}
}

func TestFiresDeliveriesAndAuditPaging(t *testing.T) {
	ctx := context.Background()
	fr, dr, ar := NewAlertFireRepo(), NewDeliveryRepo(), NewAuditRepo(100)
	for i := 1; i <= 5; i++ {
		_ = fr.Add(ctx, alert.Fire{ID: fmt.Sprint("f", i), UserID: "alice", FiredAt: time.Now()})
		_ = fr.Add(ctx, alert.Fire{ID: "bob", UserID: "bob", FiredAt: time.Now()})
		_ = dr.Add(ctx, alert.Delivery{ID: fmt.Sprint("d", i), UserID: "alice", At: time.Now()})
		_ = ar.Append(ctx, audit.Entry{ID: fmt.Sprint("e", i), At: time.Now()})
	}
	fires := walk(2, true, func(q page.Query) []alert.Fire { x, _ := fr.Page(ctx, "alice", q); return x }, func(f alert.Fire) int64 { return f.Seq })
	dels := walk(2, true, func(q page.Query) []alert.Delivery { x, _ := dr.Page(ctx, "alice", q); return x }, func(d alert.Delivery) int64 { return d.Seq })
	aud := walk(2, true, func(q page.Query) []audit.Entry { x, _ := ar.Page(ctx, q); return x }, func(e audit.Entry) int64 { return e.Seq })
	if len(fires) != 5 || fires[0].ID != "f5" || len(dels) != 5 || dels[0].ID != "d5" || len(aud) != 5 || aud[0].ID != "e5" {
		t.Fatalf("paging: %d %d %d", len(fires), len(dels), len(aud))
	}
	for _, f := range fires {
		if f.UserID != "alice" || f.Cursor == "" {
			t.Fatalf("fire isolation/cursor: %+v", f)
		}
	}
}
