// Package sourcedef models configured information sources (what to read, how
// often) and their runtime state. It is pure domain: no HTTP, no SQL.
package sourcedef

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"tradepulse/internal/domain/event"
)

var (
	ErrNotFound  = errors.New("source not found")
	ErrNameTaken = errors.New("source name already exists")
)

type Type string

const (
	TypeRSS        Type = "rss"
	TypeWebMonitor Type = "webmonitor"
	TypeManual     Type = "manual"
	TypeWebhook    Type = "webhook" // push: external systems POST items
	TypeX          Type = "x"       // official X API, budget-capped
)

// WebConfig tells the page monitor how to extract items from an HTML page.
type WebConfig struct {
	ItemSelector       string `json:"item_selector" maxLength:"200" doc:"CSS selector matching each article block or link, e.g. 'div.news-item' or 'h3 a'"`
	TitleSelector      string `json:"title_selector,omitempty" maxLength:"200" doc:"Relative to the item. Default: the link text."`
	LinkSelector       string `json:"link_selector,omitempty" maxLength:"200" doc:"Relative to the item. Default: the first link."`
	SummarySelector    string `json:"summary_selector,omitempty" maxLength:"200" doc:"Optional, relative to the item."`
	MaxItems           int    `json:"max_items,omitempty" minimum:"1" maximum:"100" doc:"Default 30."`
	IgnoreRobots       bool   `json:"ignore_robots,omitempty" doc:"Only with a recorded reason; robots.txt is respected by default."`
	IgnoreRobotsReason string `json:"ignore_robots_reason,omitempty" maxLength:"200"`
}

// WebhookConfig configures an inbound webhook source.
type WebhookConfig struct {
	Auth string `json:"auth" enum:"hmac,token" doc:"hmac (default, recommended): signed requests with replay protection. token: static secret in a header, for tools that cannot sign."`
}

// XConfig configures an X (Twitter) account source.
type XConfig struct {
	Handle string `json:"handle" maxLength:"16" doc:"Account handle without @" example:"narendramodi"`
	UserID string `json:"user_id,omitempty" doc:"Resolved automatically when the source is saved"`
}

// State is runtime information maintained by the scheduler.
type State struct {
	LastRunAt           *time.Time
	LastSuccessAt       *time.Time
	NextRunAt           *time.Time
	LastItemAt          *time.Time
	LastError           string
	ConsecutiveFailures int
	UnchangedStreak     int
	ETag                string
	LastModified        string
	ItemsTotal          int64
	Cursor              string // connector cursor, e.g. the X since_id
}

type Source struct {
	ID              string
	Name            string // unique slug, immutable; used as the event "source" (subscribable)
	Type            Type
	URL             string
	Enabled         bool
	IntervalSeconds int
	Entities        []string // attached to every event from this source (subscribable topics)
	Web             *WebConfig
	Webhook         *WebhookConfig
	X               *XConfig
	SecretVersion   int // webhook secret generation; bumping it rotates the secret
	CreatedAt       time.Time
	UpdatedAt       time.Time
	State           State
}

// Bounds limit per-source polling intervals (SOURCE_MIN/MAX_INTERVAL_SECONDS).
type Bounds struct{ Min, Max time.Duration }

var (
	nameRe   = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,39}$`)
	handleRe = regexp.MustCompile(`^[A-Za-z0-9_]{1,15}$`)
	userIDRe = regexp.MustCompile(`^[0-9]{0,25}$`)
)

func hasControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

// Normalize validates and canonicalizes the configurable fields. Connector-specific
// checks (CSS selector syntax) and SSRF host checks are done by the callers.
func (s Source) Normalize(b Bounds) (Source, error) {
	s.Name = strings.ToLower(strings.TrimSpace(s.Name))
	if !nameRe.MatchString(s.Name) {
		return s, errors.New("name must be 2-40 characters: lowercase letters, digits, '-' or '_'")
	}
	switch s.Type {
	case TypeRSS, TypeWebMonitor:
		s.URL = strings.TrimSpace(s.URL)
		u, err := url.Parse(s.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || len(s.URL) > 2000 || hasControl(s.URL) {
			return s, errors.New("url must be a valid http(s) URL up to 2000 characters")
		}
		secs := time.Duration(s.IntervalSeconds) * time.Second
		if secs < b.Min || secs > b.Max {
			return s, fmt.Errorf("interval_seconds must be between %d and %d", int(b.Min.Seconds()), int(b.Max.Seconds()))
		}
	case TypeManual:
		s.URL, s.IntervalSeconds = "", 0
	case TypeWebhook:
		s.URL, s.IntervalSeconds = "", 0
		auth := "hmac"
		if s.Webhook != nil && strings.TrimSpace(s.Webhook.Auth) != "" {
			auth = strings.ToLower(strings.TrimSpace(s.Webhook.Auth))
		}
		if auth != "hmac" && auth != "token" {
			return s, errors.New("webhook.auth must be hmac or token")
		}
		s.Webhook = &WebhookConfig{Auth: auth}
	case TypeX:
		s.URL = ""
		secs := time.Duration(s.IntervalSeconds) * time.Second
		if secs < b.Min || secs > b.Max {
			return s, fmt.Errorf("interval_seconds must be between %d and %d", int(b.Min.Seconds()), int(b.Max.Seconds()))
		}
		if s.X == nil {
			return s, errors.New("x.handle is required for x sources")
		}
		h := strings.TrimPrefix(strings.TrimSpace(s.X.Handle), "@")
		if !handleRe.MatchString(h) {
			return s, errors.New("x.handle must be 1-15 letters, digits or underscores")
		}
		if !userIDRe.MatchString(s.X.UserID) {
			return s, errors.New("x.user_id is invalid")
		}
		s.X = &XConfig{Handle: h, UserID: s.X.UserID}
	default:
		return s, errors.New("type must be rss, webmonitor, manual, webhook or x")
	}
	if s.Type != TypeWebhook {
		s.Webhook = nil
	}
	if s.Type != TypeX {
		s.X = nil
	}

	if len(s.Entities) > 10 {
		return s, errors.New("at most 10 entities")
	}
	seen := map[string]bool{}
	ents := make([]string, 0, len(s.Entities))
	for _, e := range s.Entities {
		n := event.NormEntity(e)
		if n == "" || len(n) > 50 || hasControl(n) {
			return s, errors.New("entities must be 1-50 printable characters")
		}
		if !seen[n] {
			seen[n] = true
			ents = append(ents, n)
		}
	}
	s.Entities = ents

	if s.Type == TypeWebMonitor {
		if s.Web == nil || strings.TrimSpace(s.Web.ItemSelector) == "" {
			return s, errors.New("web.item_selector is required for webmonitor sources")
		}
		w := *s.Web
		for _, sel := range []*string{&w.ItemSelector, &w.TitleSelector, &w.LinkSelector, &w.SummarySelector} {
			*sel = strings.TrimSpace(*sel)
			if len(*sel) > 200 || hasControl(*sel) {
				return s, errors.New("selectors must be printable and at most 200 characters")
			}
		}
		if w.MaxItems == 0 {
			w.MaxItems = 30
		}
		if w.MaxItems < 1 || w.MaxItems > 100 {
			return s, errors.New("web.max_items must be 1-100")
		}
		w.IgnoreRobotsReason = strings.TrimSpace(w.IgnoreRobotsReason)
		if w.IgnoreRobots && len(w.IgnoreRobotsReason) < 10 {
			return s, errors.New("ignoring robots.txt requires web.ignore_robots_reason (at least 10 characters)")
		}
		if !w.IgnoreRobots {
			w.IgnoreRobotsReason = ""
		}
		s.Web = &w
	} else {
		s.Web = nil
	}
	return s, nil
}

// Health summarizes a source for admins: ok | pending | degraded | failing | overdue | disabled.
func (s Source) Health(now time.Time) string {
	switch {
	case !s.Enabled:
		return "disabled"
	case s.Type == TypeManual || s.Type == TypeWebhook: // push sources have no schedule
		return "ok"
	case s.State.LastRunAt == nil:
		return "pending"
	case s.State.ConsecutiveFailures >= 3:
		return "failing"
	case s.State.NextRunAt != nil && now.After(s.State.NextRunAt.Add(3*time.Duration(s.IntervalSeconds)*time.Second+time.Minute)):
		return "overdue" // the scheduler has not run it when it should have (silence watchdog)
	case s.State.ConsecutiveFailures > 0:
		return "degraded"
	}
	return "ok"
}

// Run is one fetch attempt, kept for troubleshooting (retention: 30 days).
type Run struct {
	ID             string
	SourceID       string
	StartedAt      time.Time
	DurationMS     int
	OK             bool
	HTTPStatus     int
	ItemsSeen      int
	ItemsNew       int
	ItemsPublished int
	Error          string
}

type Repository interface {
	Create(ctx context.Context, s Source) error // ErrNameTaken
	Get(ctx context.Context, id string) (Source, error)
	GetByName(ctx context.Context, name string) (Source, error)
	List(ctx context.Context) ([]Source, error) // ordered by name
	// Update replaces the configurable fields (URL, Enabled, IntervalSeconds, Entities, Web); never name/type/state.
	Update(ctx context.Context, s Source) error
	Delete(ctx context.Context, id string) error
	// SaveState replaces only the runtime state, so it never races with admin edits.
	SaveState(ctx context.Context, id string, st State) error
	// BumpSecretVersion atomically rotates the webhook secret generation and returns the new version.
	BumpSecretVersion(ctx context.Context, id string) (int, error)
}

type RunRepository interface {
	Add(ctx context.Context, r Run) error
	Recent(ctx context.Context, sourceID string, limit int) ([]Run, error)
	Prune(ctx context.Context, olderThan time.Time) (int64, error)
}

// ItemStore is the dedupe + raw store. Insert reports whether the key was new.
type ItemStore interface {
	Insert(ctx context.Context, sourceID, key string, raw []byte, now time.Time) (bool, error)
	Prune(ctx context.Context, olderThan time.Time) (int64, error)
}

// UsageStore tracks paid-API spend per month in micro-USD (1 USD = 1,000,000).
type UsageStore interface {
	// Add records spend and returns the month's total including it.
	Add(ctx context.Context, key string, microUSD int64, now time.Time) (int64, error)
	Month(ctx context.Context, key string, now time.Time) (int64, error)
}
