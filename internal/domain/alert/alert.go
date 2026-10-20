// Package alert models user-defined alert rules, their fires (explainable history),
// notification channels and delivery attempts. Rules are structured data validated
// against a whitelist, never code.
package alert

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"tradepulse/internal/domain/event"
	"tradepulse/internal/domain/rule"
)

const (
	InApp              = "in_app" // built-in channel: live WebSocket message
	MaxRulesPerUser    = 50
	MaxChannelsPerUser = 5
	DefaultCooldown    = 300
	MinCooldown        = 10
	MaxCooldown        = 86400
)

var (
	ErrNotFound = errors.New("not found")
	ErrLimit    = errors.New("limit reached")
)

type Scope struct {
	Kind   string   `json:"kind" enum:"entity,keyword,source" doc:"entity: item lists the entity; keyword: whole phrase in title/summary; source: item comes from the source"`
	Values []string `json:"values" minItems:"1" maxItems:"20" doc:"Any of these matches"`
}

// Spec is what a user submits (create/update/preview).
type Spec struct {
	Name            string           `json:"name" minLength:"1" maxLength:"80" example:"Big movers"`
	Scope           Scope            `json:"scope"`
	Types           []string         `json:"types,omitempty" maxItems:"20" doc:"Only these event types (empty = all)"`
	Conditions      []rule.Condition `json:"conditions,omitempty" maxItems:"10" doc:"All must hold"`
	CooldownSeconds int              `json:"cooldown_seconds,omitempty" minimum:"0" maximum:"86400" doc:"Minimum seconds between two fires of this rule (default 300, minimum 10)"`
	Severity        string           `json:"severity,omitempty" enum:"info,notable,high" doc:"Default notable"`
	Channels        []string         `json:"channels,omitempty" maxItems:"10" doc:"'in_app' and/or ids of your channels (default: in_app)"`
	Enabled         bool             `json:"enabled" doc:"Disabled rules never fire"`
}

type Rule struct {
	ID           string
	UserID       string
	Spec         Spec
	PausedReason string // set when the system auto-paused the rule
	CreatedAt    time.Time
	UpdatedAt    time.Time
	LastFiredAt  *time.Time
}

type Fire struct {
	ID       string    `json:"id"`
	RuleID   string    `json:"rule_id"`
	UserID   string    `json:"-"`
	EventID  string    `json:"event_id"`
	RuleName string    `json:"rule_name"`
	Severity string    `json:"severity"`
	Title    string    `json:"title"`
	Body     string    `json:"body"`
	URL      string    `json:"url,omitempty"`
	Source   string    `json:"source"`
	Entities []string  `json:"entities"`
	Reason   string    `json:"reason" doc:"Why this alert fired (explainability)"`
	FiredAt  time.Time `json:"fired_at"`
}

type Channel struct {
	ID                  string
	UserID              string
	Type                string // "webhook"
	Name                string
	URL                 string
	Enabled             bool
	SecretVersion       int
	ConsecutiveFailures int
	DisabledReason      string
	LastDeliveryAt      *time.Time
	CreatedAt           time.Time
}

type Delivery struct {
	ID        string    `json:"id"`
	ChannelID string    `json:"channel_id"`
	FireID    string    `json:"fire_id"`
	UserID    string    `json:"-"`
	At        time.Time `json:"at"`
	OK        bool      `json:"ok"`
	Status    int       `json:"status,omitempty" doc:"HTTP status of the last attempt"`
	Attempts  int       `json:"attempts"`
	Error     string    `json:"error,omitempty"`
}

type RuleRepository interface {
	Create(ctx context.Context, r Rule) error
	Get(ctx context.Context, userID, id string) (Rule, error)
	List(ctx context.Context, userID string) ([]Rule, error)
	ListEnabled(ctx context.Context) ([]Rule, error)
	Update(ctx context.Context, r Rule) error // spec + paused reason + updated_at
	Delete(ctx context.Context, userID, id string) error
	MarkFired(ctx context.Context, id string, at time.Time) error
	Pause(ctx context.Context, id, reason string) error // disables and records why
}

type FireRepository interface {
	Add(ctx context.Context, f Fire) error
	Recent(ctx context.Context, userID string, limit int) ([]Fire, error)
	Prune(ctx context.Context, olderThan time.Time) (int64, error)
}

type ChannelRepository interface {
	Create(ctx context.Context, c Channel) error
	Get(ctx context.Context, userID, id string) (Channel, error)
	ByID(ctx context.Context, id string) (Channel, error)
	List(ctx context.Context, userID string) ([]Channel, error)
	Delete(ctx context.Context, userID, id string) error
	// SetEnabled(true) also resets the failure counter.
	SetEnabled(ctx context.Context, userID, id string, enabled bool, reason string) error
	BumpSecretVersion(ctx context.Context, userID, id string) (int, error)
	// RecordResult updates the failure counter (reset on success) and returns it.
	RecordResult(ctx context.Context, id string, ok bool, at time.Time) (int, error)
}

type DeliveryRepository interface {
	Add(ctx context.Context, d Delivery) error
	Recent(ctx context.Context, userID string, limit int) ([]Delivery, error)
	Prune(ctx context.Context, olderThan time.Time) (int64, error)
}

func printable(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// Normalize validates and canonicalizes a spec. validChannel reports whether an id
// is one of the caller's own channels.
func (s Spec) Normalize(reg *event.Registry, validChannel func(id string) bool) (Spec, error) {
	s.Name = strings.Join(strings.Fields(s.Name), " ")
	if s.Name == "" || len([]rune(s.Name)) > 80 || !printable(s.Name) {
		return s, errors.New("name must be 1-80 printable characters")
	}
	s.Scope.Kind = strings.ToLower(strings.TrimSpace(s.Scope.Kind))
	if s.Scope.Kind != "entity" && s.Scope.Kind != "keyword" && s.Scope.Kind != "source" {
		return s, errors.New("scope.kind must be entity, keyword or source")
	}
	if len(s.Scope.Values) < 1 || len(s.Scope.Values) > 20 {
		return s, errors.New("scope.values must have 1-20 items")
	}
	seen := map[string]bool{}
	vals := make([]string, 0, len(s.Scope.Values))
	for _, v := range s.Scope.Values {
		if !printable(v) {
			return s, errors.New("scope.values: control characters are not allowed")
		}
		n := event.NormEntity(v)
		if s.Scope.Kind == "keyword" {
			n = event.Phrase(v)
		}
		if n == "" || len(n) > 100 {
			return s, errors.New("scope.values: each value must be 1-100 characters")
		}
		if !seen[n] {
			seen[n] = true
			vals = append(vals, n)
		}
	}
	s.Scope.Values = vals

	types := make([]string, 0, len(s.Types))
	tseen := map[string]bool{}
	for _, t := range s.Types {
		t = strings.ToLower(strings.TrimSpace(t))
		if _, ok := reg.Get(t); !ok {
			return s, fmt.Errorf("unknown event type %q", t)
		}
		if !tseen[t] {
			tseen[t] = true
			types = append(types, t)
		}
	}
	s.Types = types
	conds, err := rule.Normalize(s.Conditions, reg.KindOf)
	if err != nil {
		return s, err
	}
	s.Conditions = conds

	switch {
	case s.CooldownSeconds == 0:
		s.CooldownSeconds = DefaultCooldown
	case s.CooldownSeconds < MinCooldown || s.CooldownSeconds > MaxCooldown:
		return s, fmt.Errorf("cooldown_seconds must be between %d and %d", MinCooldown, MaxCooldown)
	}
	s.Severity = strings.ToLower(strings.TrimSpace(s.Severity))
	switch s.Severity {
	case "":
		s.Severity = "notable"
	case "info", "notable", "high":
	default:
		return s, errors.New("severity must be info, notable or high")
	}
	if len(s.Channels) == 0 {
		s.Channels = []string{InApp}
	}
	if len(s.Channels) > 10 {
		return s, errors.New("at most 10 channels")
	}
	cseen := map[string]bool{}
	chans := make([]string, 0, len(s.Channels))
	for _, c := range s.Channels {
		c = strings.TrimSpace(c)
		if c != InApp && (validChannel == nil || !validChannel(c)) {
			return s, errors.New("unknown channel: use 'in_app' or the id of one of your channels")
		}
		if !cseen[c] {
			cseen[c] = true
			chans = append(chans, c)
		}
	}
	s.Channels = chans
	return s, nil
}

// ---- templates ----

// Template is a shipped, pre-filled rule. Users supply the scope values (and may tweak the rest).
type Template struct {
	ID              string           `json:"id" example:"keyword-mention"`
	Name            string           `json:"name"`
	Description     string           `json:"description"`
	ScopeKind       string           `json:"scope_kind" enum:"entity,keyword,source"`
	ValuesHint      string           `json:"values_hint" doc:"What to put in scope.values"`
	Types           []string         `json:"types,omitempty"`
	Conditions      []rule.Condition `json:"conditions,omitempty"`
	CooldownSeconds int              `json:"cooldown_seconds"`
	Severity        string           `json:"severity"`
}

type Templates struct {
	mu   sync.RWMutex
	list map[string]Template
}

// NewTemplates returns the domain-agnostic default templates.
func NewTemplates() *Templates {
	t := &Templates{list: map[string]Template{}}
	for _, x := range []Template{
		{ID: "keyword-mention", Name: "Keyword mention", Description: "Alert when a word or phrase appears in any new item.", ScopeKind: "keyword", ValuesHint: "words or phrases to watch", CooldownSeconds: 300, Severity: "notable"},
		{ID: "topic-updates", Name: "Topic updates", Description: "Alert on new items about a topic or entity.", ScopeKind: "entity", ValuesHint: "topics or entities", CooldownSeconds: 600, Severity: "info"},
		{ID: "source-new-items", Name: "New items from a source", Description: "Alert on every new item from a source (see GET /sources for names).", ScopeKind: "source", ValuesHint: "source names", CooldownSeconds: 300, Severity: "info"},
	} {
		t.list[x.ID] = x
	}
	return t
}

func (t *Templates) Register(x Template) error {
	if x.ID == "" || x.Name == "" || (x.ScopeKind != "entity" && x.ScopeKind != "keyword" && x.ScopeKind != "source") {
		return errors.New("invalid template")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, dup := t.list[x.ID]; dup {
		return fmt.Errorf("template %q already registered", x.ID)
	}
	t.list[x.ID] = x
	return nil
}

func (t *Templates) Get(id string) (Template, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	x, ok := t.list[id]
	return x, ok
}

func (t *Templates) List() []Template {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]Template, 0, len(t.list))
	for _, x := range t.list {
		out = append(out, x)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Spec builds a rule spec from the template for the given scope values.
func (x Template) Spec(values []string) Spec {
	return Spec{Name: x.Name, Scope: Scope{Kind: x.ScopeKind, Values: values}, Types: x.Types, Conditions: x.Conditions,
		CooldownSeconds: x.CooldownSeconds, Severity: x.Severity, Enabled: true}
}

type PreviewExample struct {
	At    time.Time `json:"at"`
	Title string    `json:"title"`
}

// Preview reports how a rule would have behaved on recently seen events.
type Preview struct {
	Examined   int              `json:"examined" doc:"Events replayed (seen since the server started, last 24 hours, up to 1000)"`
	WouldFire  int              `json:"would_fire" doc:"Alerts after applying cooldown"`
	WouldMatch int              `json:"would_match" doc:"Events matching before cooldown"`
	WouldPause bool             `json:"would_pause" doc:"True if the hourly cap would auto-pause this rule"`
	Examples   []PreviewExample `json:"examples"`
}
