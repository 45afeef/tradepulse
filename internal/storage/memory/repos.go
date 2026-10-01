// Package memory provides in-memory repositories. They implement the domain
// interfaces; swap for PostgreSQL implementations in production.
package memory

import (
	"context"
	"strings"
	"sync"

	"tradepulse/internal/domain/feed"
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
