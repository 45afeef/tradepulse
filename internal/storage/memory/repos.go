// Package memory provides in-memory repositories. They implement the domain
// interfaces; swap for PostgreSQL implementations in production.
package memory

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"tradepulse/internal/domain/audit"
	"tradepulse/internal/domain/feed"
	"tradepulse/internal/domain/invite"
	"tradepulse/internal/domain/user"
)

type UserRepo struct {
	mu      sync.RWMutex
	byID    map[string]user.User
	byEmail map[string]string
}

func NewUserRepo() *UserRepo {
	return &UserRepo{byID: map[string]user.User{}, byEmail: map[string]string{}}
}

func (r *UserRepo) Create(_ context.Context, u user.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	email := strings.ToLower(u.Email)
	if _, ok := r.byEmail[email]; ok {
		return user.ErrEmailTaken
	}
	r.byID[u.ID] = u
	r.byEmail[email] = u.ID
	return nil
}

func (r *UserRepo) GetByID(_ context.Context, id string) (user.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	u, ok := r.byID[id]
	if !ok {
		return user.User{}, user.ErrNotFound
	}
	return u, nil
}

func (r *UserRepo) GetByEmail(_ context.Context, email string) (user.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.byEmail[strings.ToLower(email)]
	if !ok {
		return user.User{}, user.ErrNotFound
	}
	return r.byID[id], nil
}

func (r *UserRepo) UpdateProfile(_ context.Context, id string, p user.Profile) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.byID[id]
	if !ok {
		return user.ErrNotFound
	}
	u.Profile = p
	r.byID[id] = u
	return nil
}

func (r *UserRepo) SetStatus(_ context.Context, id, status string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.byID[id]
	if !ok {
		return user.ErrNotFound
	}
	u.Status = status
	r.byID[id] = u
	return nil
}

func (r *UserRepo) List(_ context.Context) ([]user.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]user.User, 0, len(r.byID))
	for _, u := range r.byID {
		out = append(out, u)
	}
	return out, nil
}

// FeedRepo keeps a bounded history per user.
type FeedRepo struct {
	mu    sync.RWMutex
	max   int
	items map[string][]feed.FeedItem
}

func NewFeedRepo(maxPerUser int) *FeedRepo {
	return &FeedRepo{max: maxPerUser, items: map[string][]feed.FeedItem{}}
}

func (r *FeedRepo) Add(_ context.Context, it feed.FeedItem) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := append(r.items[it.UserID], it)
	if len(s) > r.max {
		s = append([]feed.FeedItem(nil), s[len(s)-r.max:]...)
	}
	r.items[it.UserID] = s
	return nil
}

func (r *FeedRepo) Recent(_ context.Context, userID string, limit int) ([]feed.FeedItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s := r.items[userID]
	if limit > len(s) {
		limit = len(s)
	}
	out := make([]feed.FeedItem, 0, limit)
	for i := len(s) - 1; i >= len(s)-limit; i-- {
		out = append(out, s[i])
	}
	return out, nil
}

// ---- invites ----

type InviteRepo struct {
	mu     sync.Mutex
	byID   map[string]*invite.Invite
	byHash map[string]string
}

func NewInviteRepo() *InviteRepo {
	return &InviteRepo{byID: map[string]*invite.Invite{}, byHash: map[string]string{}}
}

func (r *InviteRepo) Create(_ context.Context, inv invite.Invite) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := inv
	r.byID[inv.ID] = &c
	r.byHash[inv.CodeHash] = inv.ID
	return nil
}

func (r *InviteRepo) List(_ context.Context) ([]invite.Invite, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]invite.Invite, 0, len(r.byID))
	for _, i := range r.byID {
		out = append(out, *i)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].CreatedAt.After(out[b].CreatedAt) })
	return out, nil
}

func (r *InviteRepo) Revoke(_ context.Context, id string, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	i, ok := r.byID[id]
	if !ok {
		return invite.ErrNotFound
	}
	if i.Status(now) != "pending" {
		return invite.ErrInvalid
	}
	t := now
	i.RevokedAt = &t
	return nil
}

func (r *InviteRepo) Redeem(_ context.Context, codeHash, email, userID string, now time.Time) (invite.Invite, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id, ok := r.byHash[codeHash]
	if !ok {
		return invite.Invite{}, invite.ErrInvalid
	}
	i := r.byID[id]
	if i.Status(now) != "pending" || (i.Email != "" && i.Email != strings.ToLower(email)) {
		return invite.Invite{}, invite.ErrInvalid
	}
	t := now
	i.UsedAt, i.UsedBy = &t, userID
	return *i, nil
}

func (r *InviteRepo) Release(_ context.Context, id, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if i, ok := r.byID[id]; ok && i.UsedBy == userID {
		i.UsedAt, i.UsedBy = nil, ""
	}
	return nil
}

// ---- audit ----

type AuditRepo struct {
	mu      sync.Mutex
	max     int
	entries []audit.Entry
}

func NewAuditRepo(max int) *AuditRepo { return &AuditRepo{max: max} }

func (r *AuditRepo) Append(_ context.Context, e audit.Entry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, e)
	if len(r.entries) > r.max {
		r.entries = append([]audit.Entry(nil), r.entries[len(r.entries)-r.max:]...)
	}
	return nil
}

func (r *AuditRepo) Recent(_ context.Context, limit int) ([]audit.Entry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if limit > len(r.entries) {
		limit = len(r.entries)
	}
	out := make([]audit.Entry, 0, limit)
	for i := len(r.entries) - 1; i >= len(r.entries)-limit; i-- {
		out = append(out, r.entries[i])
	}
	return out, nil
}
