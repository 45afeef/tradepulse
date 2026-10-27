// Package personalization maps the global event stream to per-user FeedItems.
package personalization

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"tradepulse/internal/domain/event"
	"tradepulse/internal/domain/feed"
	"tradepulse/internal/domain/rule"
	"tradepulse/internal/domain/user"
	"tradepulse/internal/idgen"
)

// Deliverer pushes a FeedItem to a user's live connections.
type Deliverer interface {
	Deliver(ctx context.Context, item feed.FeedItem)
}

type compiledSub struct {
	kind, value string
	types       map[string]struct{}
	conds       []rule.Condition
}

// userEntry is immutable once built (replaced wholesale on update).
type userEntry struct {
	id          string
	subs        []compiledSub
	mutes       []string // normalized entity/source names
	mutePhrases []string // " phrase " forms for whole-word text matching
}

type subRef struct {
	u *userEntry
	i int
}

type indexKey struct{ kind, key string }

// Engine keeps inverted indexes (entity, source, keyword-first-token -> subscribers)
// so each event is evaluated only against users who might care about it.
type Engine struct {
	log   *slog.Logger
	feeds feed.Repository
	out   Deliverer
	reg   *event.Registry

	mu       sync.RWMutex
	users    map[string]*userEntry
	keys     map[string][]indexKey
	byEntity map[string][]subRef
	bySource map[string][]subRef
	byToken  map[string][]subRef
}

func NewEngine(log *slog.Logger, feeds feed.Repository, out Deliverer, reg *event.Registry) *Engine {
	return &Engine{
		log: log, feeds: feeds, out: out, reg: reg,
		users: map[string]*userEntry{}, keys: map[string][]indexKey{},
		byEntity: map[string][]subRef{}, bySource: map[string][]subRef{}, byToken: map[string][]subRef{},
	}
}

// Load builds the indexes from persisted users at startup.
func (e *Engine) Load(ctx context.Context, repo user.Repository) error {
	users, err := repo.List(ctx)
	if err != nil {
		return err
	}
	for _, u := range users {
		e.UpsertUser(u.ID, u.Profile)
	}
	return nil
}

func compile(id string, p user.Profile) *userEntry {
	u := &userEntry{id: id}
	for _, s := range p.Subscriptions {
		cs := compiledSub{kind: s.Kind, value: s.Value, conds: s.Conditions}
		if len(s.Types) > 0 {
			cs.types = make(map[string]struct{}, len(s.Types))
			for _, t := range s.Types {
				cs.types[t] = struct{}{}
			}
		}
		u.subs = append(u.subs, cs)
	}
	for _, m := range p.Muted {
		u.mutes = append(u.mutes, m)
		if ph := event.Phrase(m); ph != "" {
			u.mutePhrases = append(u.mutePhrases, " "+ph+" ")
		}
	}
	return u
}

func firstToken(phrase string) string {
	if i := strings.IndexByte(phrase, ' '); i >= 0 {
		return phrase[:i]
	}
	return phrase
}

// UpsertUser (re)indexes a user after registration or a subscription change.
func (e *Engine) UpsertUser(id string, p user.Profile) {
	entry := compile(id, p)
	e.mu.Lock()
	defer e.mu.Unlock()
	e.removeLocked(id)
	e.users[id] = entry
	var keys []indexKey
	for i, s := range entry.subs {
		ref := subRef{u: entry, i: i}
		switch s.kind {
		case user.KindEntity:
			e.byEntity[s.value] = append(e.byEntity[s.value], ref)
			keys = append(keys, indexKey{user.KindEntity, s.value})
		case user.KindSource:
			e.bySource[s.value] = append(e.bySource[s.value], ref)
			keys = append(keys, indexKey{user.KindSource, s.value})
		case user.KindKeyword:
			t := firstToken(s.value)
			e.byToken[t] = append(e.byToken[t], ref)
			keys = append(keys, indexKey{user.KindKeyword, t})
		}
	}
	e.keys[id] = keys
}

// RemoveUser drops a user from all indexes (e.g. on suspension).
func (e *Engine) RemoveUser(id string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.removeLocked(id)
}

func (e *Engine) removeLocked(id string) {
	for _, k := range e.keys[id] {
		switch k.kind {
		case user.KindEntity:
			dropUser(e.byEntity, k.key, id)
		case user.KindSource:
			dropUser(e.bySource, k.key, id)
		case user.KindKeyword:
			dropUser(e.byToken, k.key, id)
		}
	}
	delete(e.keys, id)
	delete(e.users, id)
}

func dropUser(m map[string][]subRef, key, id string) {
	refs := m[key]
	kept := make([]subRef, 0, len(refs))
	for _, r := range refs {
		if r.u.id != id {
			kept = append(kept, r)
		}
	}
	if len(kept) == 0 {
		delete(m, key)
	} else {
		m[key] = kept
	}
}

// Handle is the event.Handler subscribed to the bus.
func (e *Engine) Handle(ctx context.Context, ev event.Event) {
	title, body, sev := e.reg.Render(ev)
	tokens := event.Tokens(ev.Title + " " + ev.Summary + " " + title + " " + body)
	phrase := " " + strings.Join(tokens, " ") + " "
	ents := make([]string, 0, len(ev.Entities))
	for _, x := range ev.Entities {
		ents = append(ents, event.NormEntity(x))
	}
	src := event.NormEntity(ev.Source)

	cands := map[*userEntry][]int{}
	add := func(r subRef) { cands[r.u] = append(cands[r.u], r.i) }

	e.mu.RLock()
	for _, ent := range ents {
		for _, r := range e.byEntity[ent] {
			add(r)
		}
	}
	for _, r := range e.bySource[src] {
		add(r)
	}
	if len(e.byToken) > 0 {
		seen := map[string]struct{}{}
		for _, t := range tokens {
			if _, dup := seen[t]; dup {
				continue
			}
			seen[t] = struct{}{}
			for _, r := range e.byToken[t] {
				if strings.Contains(phrase, " "+r.u.subs[r.i].value+" ") {
					add(r)
				}
			}
		}
	}
	e.mu.RUnlock()

	for u, idxs := range cands {
		if isMuted(u, ents, src, phrase) {
			continue
		}
		for _, i := range idxs {
			sub := u.subs[i]
			if len(sub.types) > 0 {
				if _, ok := sub.types[ev.Type]; !ok {
					continue
				}
			}
			if !rule.Match(sub.conds, ev) {
				continue
			}
			reason := fmt.Sprintf("Matched your %s subscription %q", sub.kind, sub.value)
			if len(sub.conds) > 0 {
				reason += fmt.Sprintf(" with %d condition(s)", len(sub.conds))
			}
			item := feed.FeedItem{
				ID: idgen.New(), UserID: u.id, EventID: ev.ID, EventType: ev.Type, Source: ev.Source,
				Entities: ev.Entities, Title: title, Body: body, URL: ev.URL, Severity: feed.Severity(sev),
				Reason: reason, CreatedAt: time.Now().UTC(),
			}
			stored, err := e.feeds.AddItem(ctx, item)
			if err != nil {
				e.log.Error("persist feed item", "err", err)
				stored = item
			}
			e.out.Deliver(ctx, stored)
			break
		}
	}
}

func isMuted(u *userEntry, ents []string, src, phrase string) bool {
	for _, m := range u.mutes {
		if m == src {
			return true
		}
		for _, ent := range ents {
			if m == ent {
				return true
			}
		}
	}
	for _, mp := range u.mutePhrases {
		if strings.Contains(phrase, mp) {
			return true
		}
	}
	return false
}
