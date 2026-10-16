package personalization

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"tradepulse/internal/domain/event"
	"tradepulse/internal/domain/feed"
	"tradepulse/internal/domain/rule"
	"tradepulse/internal/domain/user"
	"tradepulse/internal/storage/memory"
)

type capture struct {
	mu    sync.Mutex
	items []feed.FeedItem
}

func (c *capture) Deliver(_ context.Context, it feed.FeedItem) {
	c.mu.Lock()
	c.items = append(c.items, it)
	c.mu.Unlock()
}

func (c *capture) forUser(id string) []feed.FeedItem {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []feed.FeedItem
	for _, it := range c.items {
		if it.UserID == id {
			out = append(out, it)
		}
	}
	return out
}

func setup(t *testing.T) (*Engine, *capture, *event.Registry, *memory.FeedRepo) {
	t.Helper()
	reg := event.NewRegistry()
	if err := reg.Register(event.TypeDef{
		ID: "service_status", Severity: "notable",
		Fields: []event.FieldDef{{Name: "status", Kind: event.KindString}, {Name: "latency_ms", Kind: event.KindNumber}},
		Title:  "{entity} is {status}", Body: "Latency {latency_ms:%.0f} ms",
	}); err != nil {
		t.Fatal(err)
	}
	c := &capture{}
	feeds := memory.NewFeedRepo(50)
	return NewEngine(slog.New(slog.NewTextHandler(io.Discard, nil)), feeds, c, reg), c, reg, feeds
}

func profile(t *testing.T, reg *event.Registry, muted []string, subs ...user.Subscription) user.Profile {
	t.Helper()
	p, err := user.Profile{Subscriptions: subs, Muted: muted}.Normalize(reg)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func statusEvent(entity, st string, latency float64) event.Event {
	return event.New("service_status", "STATUS_SIM", []string{entity}, time.Now(),
		map[string]any{"status": st, "latency_ms": latency})
}

func TestKeywordSubscription(t *testing.T) {
	eng, c, reg, _ := setup(t)
	eng.UpsertUser("u1", profile(t, reg, nil, user.Subscription{Kind: "keyword", Value: "repo rate"}))
	eng.UpsertUser("u2", profile(t, reg, nil, user.Subscription{Kind: "keyword", Value: "oil"}))
	ev := event.New("content_item", "RSS", []string{"rbi"}, time.Now(), nil)
	ev.Title = "RBI raises repo rate by 25 bps"
	eng.Handle(context.Background(), ev)

	if got := c.forUser("u1"); len(got) != 1 || got[0].Reason == "" || got[0].Title != ev.Title {
		t.Fatalf("u1 expected one explained item, got %+v", got)
	}
	if got := c.forUser("u2"); len(got) != 0 {
		t.Fatalf("u2 must receive nothing, got %+v", got)
	}
}

func TestKeywordIsWholeWord(t *testing.T) {
	eng, c, reg, _ := setup(t)
	eng.UpsertUser("u1", profile(t, reg, nil, user.Subscription{Kind: "keyword", Value: "oil"}))
	ev := event.New("content_item", "RSS", nil, time.Now(), nil)
	ev.Title = "Spoiled milk recall" // contains "oil" only inside a word
	eng.Handle(context.Background(), ev)
	if len(c.forUser("u1")) != 0 {
		t.Fatal("substring must not match a whole-word keyword")
	}
}

func TestEntityWithConditions(t *testing.T) {
	eng, c, reg, feeds := setup(t)
	eng.UpsertUser("u1", profile(t, reg, nil, user.Subscription{
		Kind: "entity", Value: "Checkout",
		Conditions: []rule.Condition{{Field: "status", Op: rule.Neq, Value: "up"}},
	}))
	eng.Handle(context.Background(), statusEvent("checkout", "up", 50))
	if len(c.forUser("u1")) != 0 {
		t.Fatal("condition status != up must filter out 'up'")
	}
	eng.Handle(context.Background(), statusEvent("checkout", "down", 950))
	got := c.forUser("u1")
	if len(got) != 1 || got[0].Title != "checkout is down" || got[0].Body != "Latency 950 ms" || got[0].Severity != feed.Notable {
		t.Fatalf("unexpected item: %+v", got)
	}
	if rec, _ := feeds.Recent(context.Background(), "u1", 10); len(rec) != 1 {
		t.Fatal("item must be persisted")
	}
}

func TestTypeRestrictionAndMute(t *testing.T) {
	eng, c, reg, _ := setup(t)
	eng.UpsertUser("typed", profile(t, reg, nil, user.Subscription{Kind: "entity", Value: "checkout", Types: []string{"content_item"}}))
	eng.UpsertUser("muted", profile(t, reg, []string{"Checkout"}, user.Subscription{Kind: "source", Value: "status_sim"}))
	eng.Handle(context.Background(), statusEvent("checkout", "down", 1))
	if len(c.forUser("typed")) != 0 {
		t.Fatal("type restriction ignored")
	}
	if len(c.forUser("muted")) != 0 {
		t.Fatal("muted entity must suppress item")
	}
}

func TestOneItemPerUserPerEvent(t *testing.T) {
	eng, c, reg, _ := setup(t)
	eng.UpsertUser("u1", profile(t, reg, nil,
		user.Subscription{Kind: "entity", Value: "checkout"},
		user.Subscription{Kind: "source", Value: "status_sim"},
		user.Subscription{Kind: "keyword", Value: "checkout"},
	))
	eng.Handle(context.Background(), statusEvent("checkout", "down", 1))
	if n := len(c.forUser("u1")); n != 1 {
		t.Fatalf("expected exactly 1 item, got %d", n)
	}
}

func TestUpsertAndRemoveUpdateIndex(t *testing.T) {
	eng, c, reg, _ := setup(t)
	eng.UpsertUser("u1", profile(t, reg, nil, user.Subscription{Kind: "entity", Value: "checkout"}))
	eng.UpsertUser("u1", profile(t, reg, nil, user.Subscription{Kind: "entity", Value: "search"})) // replaces
	eng.Handle(context.Background(), statusEvent("checkout", "down", 1))
	if len(c.forUser("u1")) != 0 {
		t.Fatal("old subscription still indexed")
	}
	eng.Handle(context.Background(), statusEvent("search", "down", 1))
	if len(c.forUser("u1")) != 1 {
		t.Fatal("new subscription not indexed")
	}
	eng.RemoveUser("u1")
	eng.Handle(context.Background(), statusEvent("search", "down", 1))
	if len(c.forUser("u1")) != 1 {
		t.Fatal("removed user must receive nothing further")
	}
}

func TestUsersAreIsolated(t *testing.T) {
	eng, c, reg, _ := setup(t)
	eng.UpsertUser("alice", profile(t, reg, nil, user.Subscription{Kind: "entity", Value: "payments"}))
	eng.UpsertUser("bob", profile(t, reg, nil, user.Subscription{Kind: "entity", Value: "auth"}))
	eng.Handle(context.Background(), statusEvent("payments", "down", 1))
	if len(c.forUser("alice")) != 1 || len(c.forUser("bob")) != 0 {
		t.Fatal("items must only reach the subscribed user")
	}
}
