package alerting

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"tradepulse/internal/domain/alert"
	"tradepulse/internal/idgen"
	"tradepulse/internal/safefetch"
	"tradepulse/internal/webhook"
)

// Poster is the outbound HTTP layer (*safefetch.Fetcher).
type Poster interface {
	Post(ctx context.Context, rawURL string, body []byte, hdr http.Header) (*safefetch.Response, error)
}

type SenderConfig struct {
	Workers      int             // default 4
	QueueSize    int             // default 1000
	Delays       []time.Duration // waits before retries (default 2s, 10s, 60s)
	DisableAfter int             // consecutive failed deliveries before a channel is disabled (default 10)
	Now          func() time.Time
}

type job struct {
	userID, channelID string
	fire              alert.Fire
	test              bool
}

// OutboundSecret derives a channel's signing secret (nothing secret is stored).
func OutboundSecret(key []byte, channelID string, version int) string {
	return webhook.Secret(key, "out:"+channelID, version)
}

// Sender delivers fires to user webhooks: signed, SSRF-safe, retried with back-off.
// Delivery is at-least-once and best-effort (the queue is in memory).
type Sender struct {
	log        *slog.Logger
	channels   alert.ChannelRepository
	deliveries alert.DeliveryRepository
	poster     Poster
	notify     Notifier
	key        []byte
	cfg        SenderConfig
	q          chan job
	wg         sync.WaitGroup
}

func NewSender(log *slog.Logger, ch alert.ChannelRepository, d alert.DeliveryRepository, p Poster, n Notifier, signingKey []byte, cfg SenderConfig) *Sender {
	if cfg.Workers <= 0 {
		cfg.Workers = 4
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 1000
	}
	if cfg.Delays == nil {
		cfg.Delays = []time.Duration{2 * time.Second, 10 * time.Second, 60 * time.Second}
	}
	if cfg.DisableAfter <= 0 {
		cfg.DisableAfter = 10
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Sender{log: log, channels: ch, deliveries: d, poster: p, notify: n, key: signingKey, cfg: cfg, q: make(chan job, cfg.QueueSize)}
}

func (s *Sender) Enqueue(userID, channelID string, f alert.Fire) { s.enqueue(job{userID: userID, channelID: channelID, fire: f}) }

// EnqueueTest queues a synthetic delivery so users can verify an endpoint.
func (s *Sender) EnqueueTest(userID, channelID string) {
	f := alert.Fire{ID: idgen.New(), RuleName: "Test delivery", Severity: "info", Title: "Test alert from TradePulse",
		Body: "If you can read this, your channel works.", Source: "test", Entities: []string{}, Reason: "You requested a test delivery.", FiredAt: s.cfg.Now().UTC()}
	s.enqueue(job{userID: userID, channelID: channelID, fire: f, test: true})
}

func (s *Sender) enqueue(j job) {
	select {
	case s.q <- j:
	default:
		s.log.Warn("outbound queue full: delivery dropped", "channel", j.channelID)
		_ = s.deliveries.Add(context.Background(), alert.Delivery{ID: idgen.New(), ChannelID: j.channelID, FireID: j.fire.ID, UserID: j.userID,
			At: s.cfg.Now().UTC(), Error: "delivery queue full"})
	}
}

// Run starts the workers and returns when ctx is cancelled and workers have stopped.
func (s *Sender) Run(ctx context.Context) {
	for i := 0; i < s.cfg.Workers; i++ {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case j := <-s.q:
					s.deliver(ctx, j)
				}
			}
		}()
	}
	s.wg.Wait()
}

type payload struct {
	Type     string   `json:"type"`
	Test     bool     `json:"test,omitempty"`
	ID       string   `json:"id"`
	RuleID   string   `json:"rule_id,omitempty"`
	RuleName string   `json:"rule_name"`
	Severity string   `json:"severity"`
	Title    string   `json:"title"`
	Body     string   `json:"body"`
	URL      string   `json:"url,omitempty"`
	Source   string   `json:"source"`
	Entities []string `json:"entities"`
	Reason   string   `json:"reason"`
	FiredAt  string   `json:"fired_at"`
}

func buildPayload(j job) []byte {
	f := j.fire
	p := payload{Type: "alert", Test: j.test, ID: f.ID, RuleID: f.RuleID, RuleName: f.RuleName, Severity: f.Severity, Title: f.Title,
		Body: f.Body, URL: f.URL, Source: f.Source, Entities: f.Entities, Reason: f.Reason, FiredAt: f.FiredAt.Format(time.RFC3339)}
	if p.Entities == nil {
		p.Entities = []string{}
	}
	b, _ := json.Marshal(p)
	return b
}

func (s *Sender) deliver(ctx context.Context, j job) {
	ch, err := s.channels.ByID(ctx, j.channelID)
	if err != nil || ch.UserID != j.userID || !ch.Enabled || ch.Type != "webhook" {
		return // deleted, disabled, or not the owner's: never send
	}
	body := buildPayload(j)
	d := alert.Delivery{ID: idgen.New(), ChannelID: ch.ID, FireID: j.fire.ID, UserID: ch.UserID}
	for attempt := 0; attempt <= len(s.cfg.Delays); attempt++ {
		if attempt > 0 {
			t := time.NewTimer(s.cfg.Delays[attempt-1])
			select {
			case <-t.C:
			case <-ctx.Done():
				t.Stop()
				return
			}
		}
		d.Attempts = attempt + 1
		ts := strconv.FormatInt(s.cfg.Now().Unix(), 10)
		h := http.Header{}
		h.Set("X-TP-Timestamp", ts)
		h.Set("X-TP-Signature", webhook.Sign(OutboundSecret(s.key, ch.ID, ch.SecretVersion), ts, body))
		h.Set("X-TP-Event", "alert")
		h.Set("X-TP-Delivery", d.ID)
		resp, err := s.poster.Post(ctx, ch.URL, body, h)
		retry := true
		switch {
		case err != nil:
			d.Error, d.Status = clipErr(err), 0
		case resp.Status >= 200 && resp.Status < 300:
			d.OK, d.Status, d.Error = true, resp.Status, ""
		default:
			d.Status, d.Error = resp.Status, fmt.Sprintf("receiver answered HTTP %d", resp.Status)
			if resp.Status >= 300 && resp.Status < 500 && resp.Status != 408 && resp.Status != 429 {
				retry = false // redirects and client errors will not fix themselves
			}
		}
		if d.OK || !retry {
			break
		}
	}
	d.At = s.cfg.Now().UTC()
	_ = s.deliveries.Add(ctx, d)
	fails, err := s.channels.RecordResult(ctx, ch.ID, d.OK, d.At)
	if err != nil {
		return
	}
	if !d.OK && fails >= s.cfg.DisableAfter {
		reason := fmt.Sprintf("disabled after %d consecutive failed deliveries (last: %s)", fails, d.Error)
		if err := s.channels.SetEnabled(ctx, ch.UserID, ch.ID, false, reason); err == nil {
			s.log.Warn("channel disabled", "channel", ch.ID)
			s.notify.Notify(ctx, ch.UserID, "channel_disabled", map[string]string{"channel_id": ch.ID, "name": ch.Name, "reason": reason})
		}
	}
}

func clipErr(err error) string {
	m := err.Error()
	if len(m) > 200 {
		m = m[:200]
	}
	return m
}

// RunRetention prunes fires and delivery history older than maxAge, hourly.
func RunRetention(ctx context.Context, fires alert.FireRepository, ds alert.DeliveryRepository, maxAge time.Duration, log *slog.Logger) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			cutoff := now.Add(-maxAge)
			if _, err := fires.Prune(ctx, cutoff); err != nil {
				log.Error("prune alert fires", "err", err)
			}
			if _, err := ds.Prune(ctx, cutoff); err != nil {
				log.Error("prune deliveries", "err", err)
			}
		}
	}
}
