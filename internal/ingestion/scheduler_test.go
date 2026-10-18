package ingestion

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"tradepulse/internal/connector"
	"tradepulse/internal/domain/event"
	"tradepulse/internal/domain/sourcedef"
	"tradepulse/internal/idgen"
	"tradepulse/internal/infrastructure/eventbus"
	"tradepulse/internal/storage/memory"
)

type stubConn struct {
	mu      sync.Mutex
	res     map[string]connector.Result // by source URL
	err     error
	calls   map[string]int
	block   map[string]chan struct{}
	started chan string
}

func (c *stubConn) Type() sourcedef.Type { return sourcedef.TypeRSS }
func (c *stubConn) Fetch(ctx context.Context, src sourcedef.Source) (connector.Result, error) {
	c.mu.Lock()
	if c.calls == nil {
		c.calls = map[string]int{}
	}
	c.calls[src.URL]++
	res, err, ch := c.res[src.URL], c.err, c.block[src.URL]
	c.mu.Unlock()
	if c.started != nil {
		c.started <- src.URL
	}
	if ch != nil {
		select {
		case <-ch:
		case <-ctx.Done():
			return connector.Result{}, ctx.Err()
		}
	}
	return res, err
}
func (c *stubConn) count(u string) int { c.mu.Lock(); defer c.mu.Unlock(); return c.calls[u] }
func (c *stubConn) set(u string, r connector.Result, err error) {
	c.mu.Lock()
	if c.res == nil {
		c.res = map[string]connector.Result{}
	}
	c.res[u], c.err = r, err
	c.mu.Unlock()
}

type fixture struct {
	t       *testing.T
	s       *Scheduler
	conn    *stubConn
	sources *memory.SourceRepo
	runs    *memory.RunRepo
	items   *memory.ItemStore
	clock   *time.Time
	mu      sync.Mutex
	events  []event.Event
	bus     *eventbus.Memory
}

func newFixture(t *testing.T, items sourcedef.ItemStore) *fixture {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	f := &fixture{t: t, conn: &stubConn{}, sources: memory.NewSourceRepo(), runs: memory.NewRunRepo(), items: memory.NewItemStore(), clock: &now}
	f.bus = eventbus.NewMemory(256, log)
	t.Cleanup(f.bus.Close)
	f.bus.Subscribe("test", func(_ context.Context, e event.Event) { f.mu.Lock(); f.events = append(f.events, e); f.mu.Unlock() })
	var store sourcedef.ItemStore = f.items
	if items != nil {
		store = items
	}
	f.s = NewScheduler(log, f.sources, f.runs, store, connector.NewRegistry(f.conn), f.bus, event.NewRegistry(), SchedulerConfig{
		HostGap: -1, jitterDisable: true, FailureRetry: 30 * time.Second,
		Bounds: sourcedef.Bounds{Min: 60 * time.Second, Max: 24 * time.Hour}, Now: func() time.Time { return *f.clock },
	})
	return f
}

func (f *fixture) add(name, url string, interval int) sourcedef.Source {
	f.t.Helper()
	src := sourcedef.Source{ID: idgen.New(), Name: name, Type: sourcedef.TypeRSS, URL: url, Enabled: true, IntervalSeconds: interval,
		Entities: []string{"markets"}, CreatedAt: *f.clock, UpdatedAt: *f.clock}
	if err := f.sources.Create(context.Background(), src); err != nil {
		f.t.Fatal(err)
	}
	return src
}

func (f *fixture) reload(id string) sourcedef.Source {
	s, err := f.sources.Get(context.Background(), id)
	if err != nil {
		f.t.Fatal(err)
	}
	return s
}

func (f *fixture) published() []event.Event {
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]event.Event(nil), f.events...)
}

func item(key, title string, age time.Duration, now time.Time) connector.Item {
	it := connector.Item{Key: key, Title: title, URL: "https://example.com/" + key, Raw: []byte(`{}`)}
	if age >= 0 {
		it.PublishedAt = now.Add(-age)
	}
	return it
}

func TestFirstRunIsBaselineThenOnlyNewItemsPublish(t *testing.T) {
	f := newFixture(t, nil)
	src := f.add("news", "https://a.example/feed", 300)
	ctx := context.Background()
	f.conn.set(src.URL, connector.Result{Items: []connector.Item{item("1", "old one", time.Hour, *f.clock), item("2", "old two", time.Hour, *f.clock)}}, nil)

	f.s.RunOnce(ctx, src)
	if ev := f.published(); len(ev) != 0 {
		t.Fatalf("baseline run must not publish, got %d events", len(ev))
	}
	st := f.reload(src.ID).State
	if st.LastSuccessAt == nil || st.ItemsTotal != 2 || st.NextRunAt == nil || st.ConsecutiveFailures != 0 {
		t.Fatalf("state after baseline: %+v", st)
	}
	if want := f.clock.Add(300 * time.Second); !st.NextRunAt.Equal(want) {
		t.Fatalf("next run %v want %v (own interval)", st.NextRunAt, want)
	}

	*f.clock = f.clock.Add(5 * time.Minute)
	f.conn.set(src.URL, connector.Result{Items: []connector.Item{
		item("1", "old one", time.Hour, *f.clock), item("2", "old two", time.Hour, *f.clock), item("3", "fresh", time.Minute, *f.clock)}}, nil)
	f.s.RunOnce(ctx, f.reload(src.ID))
	ev := f.published()
	if len(ev) != 1 || ev[0].Title != "fresh" || ev[0].Source != "news" || ev[0].Entities[0] != "markets" || ev[0].DedupeKey != "3" {
		t.Fatalf("expected exactly the new item, got %+v", ev)
	}

	f.s.RunOnce(ctx, f.reload(src.ID)) // same content again: nothing new
	if got := len(f.published()); got != 1 {
		t.Fatalf("dedupe failed: %d events", got)
	}
	runs, _ := f.runs.Recent(ctx, src.ID, 10)
	if len(runs) != 3 || runs[1].ItemsPublished != 1 || runs[1].ItemsNew != 1 || runs[0].ItemsNew != 0 {
		t.Fatalf("run history wrong: %+v", runs)
	}
}

func TestStaleItemsAreStoredButNotPublished(t *testing.T) {
	f := newFixture(t, nil)
	src := f.add("news", "https://a.example/feed", 300)
	ctx := context.Background()
	f.conn.set(src.URL, connector.Result{}, nil)
	f.s.RunOnce(ctx, src) // baseline (empty)
	f.conn.set(src.URL, connector.Result{Items: []connector.Item{item("old", "ancient story", 10*24*time.Hour, *f.clock)}}, nil)
	f.s.RunOnce(ctx, f.reload(src.ID))
	if len(f.published()) != 0 {
		t.Fatal("items older than MaxItemAge must not be published")
	}
	if inserted, _ := f.items.Insert(ctx, src.ID, "old", nil, *f.clock); inserted {
		t.Fatal("stale item must still be remembered (dedupe)")
	}
}

func TestFailureBacksOffRecordsRunAndRecovers(t *testing.T) {
	f := newFixture(t, nil)
	src := f.add("flaky", "https://flaky.example/feed", 600)
	ctx := context.Background()
	f.conn.set(src.URL, connector.Result{HTTPStatus: 503}, errors.New("unexpected HTTP status 503"))

	f.s.RunOnce(ctx, src)
	st := f.reload(src.ID).State
	if st.ConsecutiveFailures != 1 || st.LastError == "" || st.LastSuccessAt != nil {
		t.Fatalf("after 1st failure: %+v", st)
	}
	if want := f.clock.Add(60 * time.Second); !st.NextRunAt.Equal(want) { // 30s retry floored to the 60s global minimum
		t.Fatalf("retry at %v want %v", st.NextRunAt, want)
	}
	for i := 0; i < 3; i++ {
		f.s.RunOnce(ctx, f.reload(src.ID))
	}
	st = f.reload(src.ID).State
	if st.ConsecutiveFailures != 4 || f.reload(src.ID).Health(*f.clock) != "failing" {
		t.Fatalf("after 4 failures: %+v health=%s", st, f.reload(src.ID).Health(*f.clock))
	}
	runs, _ := f.runs.Recent(ctx, src.ID, 10)
	if len(runs) != 4 || runs[0].OK || runs[0].HTTPStatus != 503 || runs[0].Error == "" {
		t.Fatalf("failed runs must be recorded: %+v", runs)
	}
	f.conn.set(src.URL, connector.Result{Items: []connector.Item{item("1", "back", time.Minute, *f.clock)}}, nil)
	f.s.RunOnce(ctx, f.reload(src.ID))
	st = f.reload(src.ID).State
	if st.ConsecutiveFailures != 0 || st.LastError != "" || st.LastSuccessAt == nil {
		t.Fatalf("recovery must reset failures: %+v", st)
	}
}

func TestNotModifiedKeepsValidatorsAndStretchesInterval(t *testing.T) {
	f := newFixture(t, nil)
	src := f.add("quiet", "https://quiet.example/feed", 300)
	ctx := context.Background()
	f.conn.set(src.URL, connector.Result{ETag: `"v1"`, LastModified: "Mon", Items: []connector.Item{item("1", "x", time.Minute, *f.clock)}}, nil)
	f.s.RunOnce(ctx, src)
	f.conn.set(src.URL, connector.Result{NotModified: true, HTTPStatus: 304}, nil)
	for i := 0; i < 10; i++ {
		f.s.RunOnce(ctx, f.reload(src.ID))
	}
	cur := f.reload(src.ID)
	if cur.State.ETag != `"v1"` || cur.State.LastModified != "Mon" {
		t.Fatalf("validators lost on 304: %+v", cur.State)
	}
	if cur.State.UnchangedStreak != 10 {
		t.Fatalf("unchanged streak = %d", cur.State.UnchangedStreak)
	}
	if want := f.clock.Add(600 * time.Second); !cur.State.NextRunAt.Equal(want) {
		t.Fatalf("interval must double after 10 unchanged runs: %v want %v", cur.State.NextRunAt, want)
	}
}

func TestTickRespectsScheduleEnabledAndType(t *testing.T) {
	f := newFixture(t, nil)
	ctx := context.Background()
	fast := f.add("fast", "https://fast.example/f", 60)
	slow := f.add("slow", "https://slow.example/f", 3600)
	off := f.add("off", "https://off.example/f", 60)
	off.Enabled = false
	_ = f.sources.Update(ctx, off)
	man := sourcedef.Source{ID: idgen.New(), Name: "manual", Type: sourcedef.TypeManual, Enabled: true, CreatedAt: *f.clock}
	_ = f.sources.Create(ctx, man)
	for _, s := range []sourcedef.Source{fast, slow, off} {
		f.conn.set(s.URL, connector.Result{}, nil)
	}
	f.s.Tick(ctx)
	f.s.Wait()
	if f.conn.count(fast.URL) != 1 || f.conn.count(slow.URL) != 1 || f.conn.count(off.URL) != 0 {
		t.Fatalf("first tick: fast=%d slow=%d off=%d", f.conn.count(fast.URL), f.conn.count(slow.URL), f.conn.count(off.URL))
	}
	*f.clock = f.clock.Add(61 * time.Second)
	f.s.Tick(ctx)
	f.s.Wait()
	if f.conn.count(fast.URL) != 2 || f.conn.count(slow.URL) != 1 {
		t.Fatalf("independent intervals broken: fast=%d slow=%d", f.conn.count(fast.URL), f.conn.count(slow.URL))
	}
}

func TestSlowSourceDoesNotBlockOthersAndIsNotRunTwice(t *testing.T) {
	f := newFixture(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	slow := f.add("slow", "https://slow.example/f", 60)
	fast := f.add("fast", "https://fast.example/f", 60)
	release := make(chan struct{})
	f.conn.block = map[string]chan struct{}{slow.URL: release}
	f.conn.set(slow.URL, connector.Result{}, nil)
	f.conn.set(fast.URL, connector.Result{}, nil)

	f.s.Tick(ctx)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if r, _ := f.runs.Recent(ctx, fast.ID, 1); len(r) == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if r, _ := f.runs.Recent(ctx, fast.ID, 1); len(r) != 1 {
		t.Fatal("a blocked source must not delay other sources")
	}
	f.s.Tick(ctx) // slow is still in flight: must not start a second fetch
	time.Sleep(50 * time.Millisecond)
	if n := f.conn.count(slow.URL); n != 1 {
		t.Fatalf("in-flight source was started %d times", n)
	}
	close(release)
	f.s.Wait()
}

type failingStore struct{ sourcedef.ItemStore }

func (failingStore) Insert(context.Context, string, string, []byte, time.Time) (bool, error) {
	return false, errors.New("db down")
}

func TestStorageFailureDoesNotAdvanceSuccessState(t *testing.T) {
	f := newFixture(t, failingStore{memory.NewItemStore()})
	src := f.add("news", "https://a.example/feed", 300)
	f.conn.set(src.URL, connector.Result{Items: []connector.Item{item("1", "x", time.Minute, *f.clock)}}, nil)
	f.s.RunOnce(context.Background(), src)
	st := f.reload(src.ID).State
	if st.LastSuccessAt != nil || st.ConsecutiveFailures != 1 || st.ItemsTotal != 0 {
		t.Fatalf("a storage failure must count as a failed run: %+v", st)
	}
	if len(f.published()) != 0 {
		t.Fatal("nothing may be published when items cannot be recorded")
	}
}

func TestNextIntervalAndFailureDelay(t *testing.T) {
	base, max := 5*time.Minute, 24*time.Hour
	for _, c := range []struct {
		unchanged int
		want      time.Duration
	}{{0, base}, {9, base}, {10, 2 * base}, {29, 2 * base}, {30, 4 * base}, {500, 4 * base}} {
		if got := NextInterval(base, c.unchanged, max); got != c.want {
			t.Errorf("NextInterval(%d) = %v want %v", c.unchanged, got, c.want)
		}
	}
	if got := NextInterval(20*time.Hour, 100, max); got != max {
		t.Errorf("cap at max: %v", got)
	}
	first, min := 30*time.Second, 60*time.Second
	for _, c := range []struct {
		failures int
		base     time.Duration
		want     time.Duration
	}{{1, time.Hour, 60 * time.Second}, {2, time.Hour, 60 * time.Second}, {3, time.Hour, 2 * time.Minute}, {4, time.Hour, 4 * time.Minute},
		{30, time.Hour, time.Hour}, {1, 2 * time.Minute, 60 * time.Second}, {10, 2 * time.Minute, 2 * time.Minute}, {0, time.Hour, 60 * time.Second}} {
		if got := FailureDelay(c.base, c.failures, first, min); got != c.want {
			t.Errorf("FailureDelay(base=%v, n=%d) = %v want %v", c.base, c.failures, got, c.want)
		}
	}
}

func TestPrunesOldRunsAndItems(t *testing.T) {
	f := newFixture(t, nil)
	ctx := context.Background()
	src := f.add("news", "https://a.example/feed", 300)
	old := f.clock.Add(-40 * 24 * time.Hour)
	_ = f.runs.Add(ctx, sourcedef.Run{ID: "old", SourceID: src.ID, StartedAt: old})
	_ = f.runs.Add(ctx, sourcedef.Run{ID: "new", SourceID: src.ID, StartedAt: *f.clock})
	_, _ = f.items.Insert(ctx, src.ID, "old", nil, old)
	_, _ = f.items.Insert(ctx, src.ID, "new", nil, *f.clock)
	f.s.prune(ctx)
	if r, _ := f.runs.Recent(ctx, src.ID, 10); len(r) != 1 || r[0].ID != "new" {
		t.Fatalf("runs: %+v", r)
	}
	if ins, _ := f.items.Insert(ctx, src.ID, "old", nil, *f.clock); !ins {
		t.Fatal("old item should have been pruned")
	}
	if ins, _ := f.items.Insert(ctx, src.ID, "new", nil, *f.clock); ins {
		t.Fatal("recent item must be kept")
	}
}

func TestCursorIsPersistedAndPushSourcesAreNeverPolled(t *testing.T) {
	f := newFixture(t, nil)
	ctx := context.Background()
	src := f.add("xacct", "https://x.example/f", 300)
	f.conn.set(src.URL, connector.Result{Cursor: "777", Items: []connector.Item{item("1", "post", time.Minute, *f.clock)}}, nil)
	f.s.RunOnce(ctx, src)
	if got := f.reload(src.ID).State.Cursor; got != "777" {
		t.Fatalf("cursor = %q", got)
	}
	f.conn.set(src.URL, connector.Result{NotModified: true}, nil) // no new cursor: keep the old one
	f.s.RunOnce(ctx, f.reload(src.ID))
	if got := f.reload(src.ID).State.Cursor; got != "777" {
		t.Fatalf("cursor lost: %q", got)
	}
	hook := sourcedef.Source{ID: idgen.New(), Name: "hook", Type: sourcedef.TypeWebhook, Enabled: true, CreatedAt: *f.clock}
	_ = f.sources.Create(ctx, hook)
	before := f.conn.count("")
	f.s.Tick(ctx)
	f.s.Wait()
	if f.conn.count("") != before {
		t.Fatal("webhook (push) sources must never be polled")
	}
}
