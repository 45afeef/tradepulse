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
	"tradepulse/internal/domain/sourcedef"
	"tradepulse/internal/domain/user"
	"tradepulse/internal/page"
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

func (r *UserRepo) SetPassword(_ context.Context, id, hash string, validAfter time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.byID[id]
	if !ok {
		return user.ErrNotFound
	}
	u.PasswordHash, u.SessionsValidAfter = hash, validAfter
	r.byID[id] = u
	return nil
}

func (r *UserRepo) RevokeSessions(_ context.Context, id string, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.byID[id]
	if !ok {
		return user.ErrNotFound
	}
	u.SessionsValidAfter = at
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
	seq   int64
	items map[string][]feed.FeedItem
}

func NewFeedRepo(maxPerUser int) *FeedRepo {
	return &FeedRepo{max: maxPerUser, items: map[string][]feed.FeedItem{}}
}

func (r *FeedRepo) Add(ctx context.Context, it feed.FeedItem) error {
	_, err := r.AddItem(ctx, it)
	return err
}

func (r *FeedRepo) AddItem(_ context.Context, it feed.FeedItem) (feed.FeedItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	it.Seq, it.Cursor = r.seq, page.Cursor(r.seq)
	s := append(r.items[it.UserID], it)
	if len(s) > r.max {
		s = append([]feed.FeedItem(nil), s[len(s)-r.max:]...)
	}
	r.items[it.UserID] = s
	return it, nil
}

func (r *FeedRepo) Page(_ context.Context, userID string, q page.Query) ([]feed.FeedItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return page.Select(r.items[userID], func(f feed.FeedItem) int64 { return f.Seq }, func(feed.FeedItem) bool { return true }, q), nil
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
	seq     int64
	entries []audit.Entry
}

func NewAuditRepo(max int) *AuditRepo { return &AuditRepo{max: max} }

func (r *AuditRepo) Append(_ context.Context, e audit.Entry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	e.Seq, e.Cursor = r.seq, page.Cursor(r.seq)
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

// ---- sources ----

type SourceRepo struct {
	mu     sync.Mutex
	byID   map[string]sourcedef.Source
	byName map[string]string
}

func NewSourceRepo() *SourceRepo {
	return &SourceRepo{byID: map[string]sourcedef.Source{}, byName: map[string]string{}}
}

func (r *SourceRepo) Create(_ context.Context, s sourcedef.Source) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byName[s.Name]; ok {
		return sourcedef.ErrNameTaken
	}
	if s.SecretVersion == 0 {
		s.SecretVersion = 1
	}
	r.byID[s.ID] = s
	r.byName[s.Name] = s.ID
	return nil
}

func (r *SourceRepo) Get(_ context.Context, id string) (sourcedef.Source, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.byID[id]
	if !ok {
		return sourcedef.Source{}, sourcedef.ErrNotFound
	}
	return s, nil
}

func (r *SourceRepo) GetByName(_ context.Context, name string) (sourcedef.Source, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id, ok := r.byName[name]
	if !ok {
		return sourcedef.Source{}, sourcedef.ErrNotFound
	}
	return r.byID[id], nil
}

func (r *SourceRepo) List(_ context.Context) ([]sourcedef.Source, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]sourcedef.Source, 0, len(r.byID))
	for _, s := range r.byID {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (r *SourceRepo) Update(_ context.Context, s sourcedef.Source) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cur, ok := r.byID[s.ID]
	if !ok {
		return sourcedef.ErrNotFound
	}
	cur.URL, cur.Enabled, cur.IntervalSeconds, cur.Entities, cur.Web, cur.UpdatedAt = s.URL, s.Enabled, s.IntervalSeconds, s.Entities, s.Web, s.UpdatedAt
	cur.Webhook, cur.X = s.Webhook, s.X
	r.byID[s.ID] = cur
	return nil
}

func (r *SourceRepo) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.byID[id]
	if !ok {
		return sourcedef.ErrNotFound
	}
	delete(r.byID, id)
	delete(r.byName, s.Name)
	return nil
}

func (r *SourceRepo) SaveState(_ context.Context, id string, st sourcedef.State) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.byID[id]
	if !ok {
		return sourcedef.ErrNotFound
	}
	s.State = st
	r.byID[id] = s
	return nil
}

func (r *SourceRepo) BumpSecretVersion(_ context.Context, id string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.byID[id]
	if !ok {
		return 0, sourcedef.ErrNotFound
	}
	s.SecretVersion++
	r.byID[id] = s
	return s.SecretVersion, nil
}

type RunRepo struct {
	mu   sync.Mutex
	runs []sourcedef.Run
}

func NewRunRepo() *RunRepo { return &RunRepo{} }

func (r *RunRepo) Add(_ context.Context, run sourcedef.Run) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs = append(r.runs, run)
	if len(r.runs) > 5000 {
		r.runs = append([]sourcedef.Run(nil), r.runs[len(r.runs)-5000:]...)
	}
	return nil
}

func (r *RunRepo) Recent(_ context.Context, sourceID string, limit int) ([]sourcedef.Run, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []sourcedef.Run{}
	for i := len(r.runs) - 1; i >= 0 && len(out) < limit; i-- {
		if r.runs[i].SourceID == sourceID {
			out = append(out, r.runs[i])
		}
	}
	return out, nil
}

func (r *RunRepo) Prune(_ context.Context, olderThan time.Time) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	kept := r.runs[:0:0]
	for _, run := range r.runs {
		if !run.StartedAt.Before(olderThan) {
			kept = append(kept, run)
		}
	}
	n := int64(len(r.runs) - len(kept))
	r.runs = kept
	return n, nil
}

type ItemStore struct {
	mu    sync.Mutex
	items map[string]time.Time // sourceID + "\x00" + key -> fetched
}

func NewItemStore() *ItemStore { return &ItemStore{items: map[string]time.Time{}} }

func (s *ItemStore) Insert(_ context.Context, sourceID, key string, _ []byte, now time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := sourceID + "\x00" + key
	if _, ok := s.items[k]; ok {
		return false, nil
	}
	s.items[k] = now
	return true, nil
}

func (s *ItemStore) Prune(_ context.Context, olderThan time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var n int64
	for k, t := range s.items {
		if t.Before(olderThan) {
			delete(s.items, k)
			n++
		}
	}
	return n, nil
}

// UsageRepo tracks monthly paid-API spend (micro-USD).
type UsageRepo struct {
	mu sync.Mutex
	m  map[string]int64
}

func NewUsageRepo() *UsageRepo { return &UsageRepo{m: map[string]int64{}} }

func usageKey(key string, now time.Time) string { return now.UTC().Format("2006-01") + "|" + key }

func (u *UsageRepo) Add(_ context.Context, key string, micro int64, now time.Time) (int64, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.m[usageKey(key, now)] += micro
	return u.m[usageKey(key, now)], nil
}

func (u *UsageRepo) Month(_ context.Context, key string, now time.Time) (int64, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.m[usageKey(key, now)], nil
}

func (r *AuditRepo) Page(_ context.Context, q page.Query) ([]audit.Entry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return page.Select(r.entries, func(e audit.Entry) int64 { return e.Seq }, func(audit.Entry) bool { return true }, q), nil
}
