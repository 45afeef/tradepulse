// Package personalization maps the global event stream to per-user FeedItems.
package personalization

import (
	"context"
	"log/slog"
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

// Engine keeps an in-memory index entity -> interested users, so each event is
// matched only against users who follow that entity (not every user).
type Engine struct {
	log   *slog.Logger
	feeds feed.Repository
	out   Deliverer
	rules []rule.Rule

	mu       sync.RWMutex
	byEntity map[string]map[string]rule.UserContext
	byUser   map[string][]string
}

func NewEngine(log *slog.Logger, feeds feed.Repository, out Deliverer, rules []rule.Rule) *Engine {
	return &Engine{
		log: log, feeds: feeds, out: out, rules: rules,
		byEntity: map[string]map[string]rule.UserContext{},
		byUser:   map[string][]string{},
	}
}

// Load builds the index from persisted users at startup.
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

// UpsertUser (re)indexes a user after registration or a profile change.
func (e *Engine) UpsertUser(id string, p user.Profile) {
	uc := rule.NewUserContext(id, p)
	e.mu.Lock()
	defer e.mu.Unlock()
	e.removeLocked(id)
	for _, ent := range p.Watchlist {
		m := e.byEntity[ent]
		if m == nil {
			m = map[string]rule.UserContext{}
			e.byEntity[ent] = m
		}
		m[id] = uc
	}
	e.byUser[id] = append([]string(nil), p.Watchlist...)
}

func (e *Engine) removeLocked(id string) {
	for _, ent := range e.byUser[id] {
		if m := e.byEntity[ent]; m != nil {
			delete(m, id)
			if len(m) == 0 {
				delete(e.byEntity, ent)
			}
		}
	}
	delete(e.byUser, id)
}

// Handle is the event.Handler subscribed to the bus.
func (e *Engine) Handle(ctx context.Context, ev event.Event) {
	e.mu.RLock()
	cands := make([]rule.UserContext, 0, len(e.byEntity[ev.Entity]))
	for _, uc := range e.byEntity[ev.Entity] {
		cands = append(cands, uc)
	}
	e.mu.RUnlock()

	for _, uc := range cands {
		if !e.matches(ev, uc) {
			continue
		}
		title, body, sev := render(ev)
		item := feed.FeedItem{
			ID: idgen.New(), UserID: uc.UserID, EventID: ev.ID, EventType: string(ev.Type),
			Entity: ev.Entity, Title: title, Body: body, Severity: sev, CreatedAt: time.Now().UTC(),
		}
		if err := e.feeds.Add(ctx, item); err != nil {
			e.log.Error("persist feed item", "err", err)
		}
		e.out.Deliver(ctx, item)
	}
}

func (e *Engine) matches(ev event.Event, uc rule.UserContext) bool {
	for _, r := range e.rules {
		if !r.Matches(ev, uc) {
			return false
		}
	}
	return true
}
