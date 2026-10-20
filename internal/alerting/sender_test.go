package alerting

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"tradepulse/internal/domain/alert"
	"tradepulse/internal/safefetch"
	"tradepulse/internal/storage/memory"
	"tradepulse/internal/webhook"
)

type result struct {
	status int
	err    error
}
type call struct {
	url  string
	body []byte
	hdr  http.Header
}
type fakePoster struct {
	mu     sync.Mutex
	calls  []call
	script []result
	def    result
}

func (p *fakePoster) Post(_ context.Context, u string, body []byte, hdr http.Header) (*safefetch.Response, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, call{u, body, hdr.Clone()})
	r := p.def
	if len(p.script) > 0 {
		r, p.script = p.script[0], p.script[1:]
	}
	if r.err != nil {
		return nil, r.err
	}
	return &safefetch.Response{Status: r.status}, nil
}
func (p *fakePoster) n() int { p.mu.Lock(); defer p.mu.Unlock(); return len(p.calls) }

var sKey = []byte(strings.Repeat("k", 40))

type sfx struct {
	s    *Sender
	p    *fakePoster
	ch   *memory.ChannelRepo
	ds   *memory.DeliveryRepo
	note *fakeNotifier
}

func newSfx(t *testing.T, cfg SenderConfig) *sfx {
	t.Helper()
	if cfg.Delays == nil {
		cfg.Delays = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
	}
	f := &sfx{p: &fakePoster{def: result{status: 200}}, ch: memory.NewChannelRepo(), ds: memory.NewDeliveryRepo(), note: &fakeNotifier{}}
	_ = f.ch.Create(context.Background(), alert.Channel{ID: "c1", UserID: "alice", Type: "webhook", Name: "n8n", URL: "https://hook.example.com/x", Enabled: true, CreatedAt: time.Now()})
	f.s = NewSender(slog.New(slog.NewTextHandler(io.Discard, nil)), f.ch, f.ds, f.p, f.note, sKey, cfg)
	return f
}

func fire() alert.Fire {
	return alert.Fire{ID: "fire-1", RuleID: "r1", RuleName: "Big movers", Severity: "high", Title: "TCS up 3%", Body: "Last price 3500", Source: "nse",
		Entities: []string{"tcs"}, Reason: "Matched your alert", FiredAt: time.Now().UTC()}
}

func (f *sfx) deliver(userID, chID string) {
	f.s.deliver(context.Background(), job{userID: userID, channelID: chID, fire: fire()})
}

func (f *sfx) last() alert.Delivery {
	d, _ := f.ds.Recent(context.Background(), "alice", 1)
	if len(d) == 0 {
		return alert.Delivery{}
	}
	return d[0]
}

func TestDeliveryIsSignedAndRecorded(t *testing.T) {
	f := newSfx(t, SenderConfig{})
	f.deliver("alice", "c1")
	if f.p.n() != 1 {
		t.Fatalf("calls: %d", f.p.n())
	}
	c := f.p.calls[0]
	if c.url != "https://hook.example.com/x" || c.hdr.Get("X-TP-Event") != "alert" || c.hdr.Get("X-TP-Delivery") == "" {
		t.Fatalf("request: %s %v", c.url, c.hdr)
	}
	secret := OutboundSecret(sKey, "c1", 1)
	if err := webhook.VerifySignature(secret, c.hdr.Get("X-TP-Timestamp"), c.body, c.hdr.Get("X-TP-Signature"), time.Now(), time.Minute); err != nil {
		t.Fatalf("receiver could not verify the signature: %v", err)
	}
	for _, want := range []string{`"type":"alert"`, `"title":"TCS up 3%"`, `"rule_name":"Big movers"`, `"reason":"Matched your alert"`, `"entities":["tcs"]`} {
		if !strings.Contains(string(c.body), want) {
			t.Errorf("payload missing %s: %s", want, c.body)
		}
	}
	if strings.Contains(string(c.body), `"test"`) {
		t.Error("real deliveries must not carry the test flag")
	}
	if d := f.last(); !d.OK || d.Attempts != 1 || d.Status != 200 || d.FireID != "fire-1" {
		t.Fatalf("delivery record: %+v", d)
	}
	if ch, _ := f.ch.ByID(context.Background(), "c1"); ch.LastDeliveryAt == nil {
		t.Fatal("last delivery time not stored")
	}
}

func TestTransientFailuresAreRetriedPermanentOnesAreNot(t *testing.T) {
	f := newSfx(t, SenderConfig{})
	f.p.script = []result{{status: 500}, {status: 429}, {status: 200}}
	f.deliver("alice", "c1")
	if d := f.last(); !d.OK || d.Attempts != 3 || f.p.n() != 3 {
		t.Fatalf("5xx/429 must be retried: %+v calls=%d", d, f.p.n())
	}
	for _, st := range []int{400, 401, 404, 302} {
		g := newSfx(t, SenderConfig{})
		g.p.def = result{status: st}
		g.deliver("alice", "c1")
		if d := g.last(); d.OK || d.Attempts != 1 || g.p.n() != 1 || d.Status != st {
			t.Errorf("status %d must not be retried: %+v calls=%d", st, d, g.p.n())
		}
	}
}

func TestNetworkErrorsRetryThenRecordAFailure(t *testing.T) {
	f := newSfx(t, SenderConfig{})
	f.p.def = result{err: errors.New("connection refused")}
	f.deliver("alice", "c1")
	if d := f.last(); d.OK || d.Attempts != 4 || f.p.n() != 4 || !strings.Contains(d.Error, "refused") {
		t.Fatalf("%+v calls=%d", d, f.p.n())
	}
	ch, _ := f.ch.ByID(context.Background(), "c1")
	if ch.ConsecutiveFailures != 1 {
		t.Fatalf("one failed delivery (after all retries) counts once, got %d", ch.ConsecutiveFailures)
	}
}

func TestChannelIsDisabledAfterConsecutiveFailuresAndOwnerIsNotified(t *testing.T) {
	f := newSfx(t, SenderConfig{DisableAfter: 2})
	f.p.def = result{status: 400}
	f.deliver("alice", "c1")
	if ch, _ := f.ch.ByID(context.Background(), "c1"); !ch.Enabled {
		t.Fatal("one failure must not disable the channel")
	}
	f.deliver("alice", "c1")
	ch, _ := f.ch.ByID(context.Background(), "c1")
	if ch.Enabled || ch.DisabledReason == "" || f.note.of("alice", "channel_disabled") != 1 {
		t.Fatalf("channel must be disabled and the owner told once: %+v notes=%d", ch, f.note.of("alice", "channel_disabled"))
	}
	before := f.p.n()
	f.deliver("alice", "c1")
	if f.p.n() != before {
		t.Fatal("a disabled channel must never be called")
	}
}

func TestSuccessResetsTheFailureCounter(t *testing.T) {
	f := newSfx(t, SenderConfig{DisableAfter: 2})
	f.p.script = []result{{status: 400}, {status: 200}, {status: 400}}
	f.deliver("alice", "c1")
	f.deliver("alice", "c1")
	f.deliver("alice", "c1")
	if ch, _ := f.ch.ByID(context.Background(), "c1"); !ch.Enabled || ch.ConsecutiveFailures != 1 {
		t.Fatalf("failures must be consecutive: %+v", ch)
	}
}

func TestNeverSendsForDeletedForeignOrDisabledChannels(t *testing.T) {
	f := newSfx(t, SenderConfig{})
	f.deliver("alice", "no-such-channel")
	f.deliver("mallory", "c1") // someone else's channel id
	_ = f.ch.SetEnabled(context.Background(), "alice", "c1", false, "off")
	f.deliver("alice", "c1")
	if f.p.n() != 0 {
		t.Fatalf("%d calls were made", f.p.n())
	}
}

func TestQueueOverflowIsRecordedNotSilent(t *testing.T) {
	f := newSfx(t, SenderConfig{QueueSize: 1})
	f.s.Enqueue("alice", "c1", fire())
	f.s.Enqueue("alice", "c1", fire()) // no worker is draining: the second one overflows
	d, _ := f.ds.Recent(context.Background(), "alice", 5)
	if len(d) != 1 || !strings.Contains(d[0].Error, "queue full") {
		t.Fatalf("overflow must leave a record: %+v", d)
	}
}

func TestTestDeliveriesAreClearlyMarked(t *testing.T) {
	f := newSfx(t, SenderConfig{})
	f.s.EnqueueTest("alice", "c1")
	j := <-f.s.q
	f.s.deliver(context.Background(), j)
	if f.p.n() != 1 || !strings.Contains(string(f.p.calls[0].body), `"test":true`) {
		t.Fatalf("test flag missing: %v", f.p.calls)
	}
}

func TestOutboundSecretsAreSeparateFromInboundSecrets(t *testing.T) {
	if OutboundSecret(sKey, "c1", 1) == webhook.Secret(sKey, "c1", 1) {
		t.Fatal("a channel secret must not double as an inbound source secret")
	}
	if OutboundSecret(sKey, "c1", 1) == OutboundSecret(sKey, "c1", 2) || OutboundSecret(sKey, "c1", 1) == OutboundSecret(sKey, "c2", 1) {
		t.Fatal("secrets must differ per channel and version")
	}
}

func TestRunStopsOnCancelAndDeliversQueuedJobs(t *testing.T) {
	f := newSfx(t, SenderConfig{Workers: 2})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { f.s.Run(ctx); close(done) }()
	f.s.Enqueue("alice", "c1", fire())
	deadline := time.Now().Add(2 * time.Second)
	for f.p.n() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if f.p.n() != 1 {
		t.Fatal("queued job not delivered")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop")
	}
}
