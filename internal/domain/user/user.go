// Package user holds user identity and the personalization profile
// (subscriptions + mutes). It contains no domain-specific concepts.
package user

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"tradepulse/internal/domain/event"
	"tradepulse/internal/domain/rule"
)

var (
	ErrNotFound   = errors.New("user not found")
	ErrEmailTaken = errors.New("email already registered")
)

const (
	RoleAdmin  = "admin"
	RoleMember = "member"

	StatusActive    = "active"
	StatusSuspended = "suspended"
)

const (
	KindEntity  = "entity"
	KindKeyword = "keyword"
	KindSource  = "source"

	MaxSubscriptions = 200
	MaxMuted         = 200
	maxValueLen      = 100
)

type Subscription struct {
	Kind       string           `json:"kind" enum:"entity,keyword,source" doc:"entity: matches event entities; keyword: whole-word phrase in title/summary; source: events from a source id"`
	Value      string           `json:"value" minLength:"1" maxLength:"100"`
	Types      []string         `json:"types,omitempty" maxItems:"20" doc:"Only these event types (empty = all types)"`
	Conditions []rule.Condition `json:"conditions,omitempty" maxItems:"10" doc:"All conditions must hold for an item to be delivered"`
}

type Profile struct {
	Subscriptions []Subscription `json:"subscriptions,omitempty" maxItems:"200"`
	Muted         []string       `json:"muted,omitempty" maxItems:"200" doc:"Entities, sources or phrases that suppress matching items"`
}

func DefaultProfile() Profile {
	return Profile{Subscriptions: []Subscription{}, Muted: []string{}}
}

func printable(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// Normalize validates and canonicalizes a profile against the event registry.
func (p Profile) Normalize(reg *event.Registry) (Profile, error) {
	if len(p.Subscriptions) > MaxSubscriptions {
		return Profile{}, fmt.Errorf("at most %d subscriptions", MaxSubscriptions)
	}
	if len(p.Muted) > MaxMuted {
		return Profile{}, fmt.Errorf("at most %d muted entries", MaxMuted)
	}
	out := Profile{Subscriptions: make([]Subscription, 0, len(p.Subscriptions)), Muted: []string{}}
	seen := map[string]struct{}{}
	for i, s := range p.Subscriptions {
		ns, err := s.normalize(reg)
		if err != nil {
			return Profile{}, fmt.Errorf("subscriptions[%d]: %w", i, err)
		}
		k := ns.Kind + "|" + ns.Value
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		out.Subscriptions = append(out.Subscriptions, ns)
	}
	mseen := map[string]struct{}{}
	for _, m := range p.Muted {
		if !printable(m) {
			return Profile{}, errors.New("muted: control characters are not allowed")
		}
		nm := event.NormEntity(m)
		if nm == "" || len(nm) > maxValueLen {
			return Profile{}, fmt.Errorf("muted: value must be 1-%d characters", maxValueLen)
		}
		if _, dup := mseen[nm]; !dup {
			mseen[nm] = struct{}{}
			out.Muted = append(out.Muted, nm)
		}
	}
	return out, nil
}

func (s Subscription) normalize(reg *event.Registry) (Subscription, error) {
	if !printable(s.Value) {
		return s, errors.New("value: control characters are not allowed")
	}
	s.Kind = strings.ToLower(strings.TrimSpace(s.Kind))
	switch s.Kind {
	case KindEntity, KindSource:
		s.Value = event.NormEntity(s.Value)
	case KindKeyword:
		s.Value = event.Phrase(s.Value)
	default:
		return s, errors.New("kind must be entity, keyword or source")
	}
	if s.Value == "" || len(s.Value) > maxValueLen {
		return s, fmt.Errorf("value must be 1-%d characters", maxValueLen)
	}
	if len(s.Types) > 20 {
		return s, errors.New("at most 20 types")
	}
	types := make([]string, 0, len(s.Types))
	tseen := map[string]struct{}{}
	for _, t := range s.Types {
		t = strings.ToLower(strings.TrimSpace(t))
		if _, ok := reg.Get(t); !ok {
			return s, fmt.Errorf("unknown event type %q", t)
		}
		if _, dup := tseen[t]; !dup {
			tseen[t] = struct{}{}
			types = append(types, t)
		}
	}
	s.Types = types
	conds, err := rule.Normalize(s.Conditions, reg.KindOf)
	if err != nil {
		return s, err
	}
	s.Conditions = conds
	return s, nil
}

type User struct {
	ID           string
	Email        string
	PasswordHash string
	CreatedAt    time.Time
	Profile      Profile
	Role         string // RoleAdmin | RoleMember
	Status       string // StatusActive | StatusSuspended
	// SessionsValidAfter: access tokens issued before this instant are rejected (logout-all, password change/reset).
	SessionsValidAfter time.Time
}

type Repository interface {
	Create(ctx context.Context, u User) error
	GetByID(ctx context.Context, id string) (User, error)
	GetByEmail(ctx context.Context, email string) (User, error)
	UpdateProfile(ctx context.Context, id string, p Profile) error
	SetStatus(ctx context.Context, id, status string) error
	// SetPassword replaces the hash and invalidates tokens issued before validAfter.
	SetPassword(ctx context.Context, id, hash string, validAfter time.Time) error
	// RevokeSessions invalidates access tokens issued before at.
	RevokeSessions(ctx context.Context, id string, at time.Time) error
	List(ctx context.Context) ([]User, error)
}
