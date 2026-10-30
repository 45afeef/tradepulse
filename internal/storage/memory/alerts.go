package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"tradepulse/internal/domain/alert"
	"tradepulse/internal/domain/session"
	"tradepulse/internal/page"
)

// ---- rules ----

type AlertRuleRepo struct {
	mu sync.Mutex
	m  map[string]alert.Rule
}

func NewAlertRuleRepo() *AlertRuleRepo { return &AlertRuleRepo{m: map[string]alert.Rule{}} }

func (r *AlertRuleRepo) Create(_ context.Context, x alert.Rule) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[x.ID] = x
	return nil
}

func (r *AlertRuleRepo) Get(_ context.Context, userID, id string) (alert.Rule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	x, ok := r.m[id]
	if !ok || x.UserID != userID {
		return alert.Rule{}, alert.ErrNotFound
	}
	return x, nil
}

func (r *AlertRuleRepo) List(_ context.Context, userID string) ([]alert.Rule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []alert.Rule{}
	for _, x := range r.m {
		if x.UserID == userID {
			out = append(out, x)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (r *AlertRuleRepo) ListEnabled(_ context.Context) ([]alert.Rule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []alert.Rule{}
	for _, x := range r.m {
		if x.Spec.Enabled {
			out = append(out, x)
		}
	}
	return out, nil
}

func (r *AlertRuleRepo) Update(_ context.Context, x alert.Rule) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cur, ok := r.m[x.ID]
	if !ok || cur.UserID != x.UserID {
		return alert.ErrNotFound
	}
	cur.Spec, cur.PausedReason, cur.UpdatedAt = x.Spec, x.PausedReason, x.UpdatedAt
	r.m[x.ID] = cur
	return nil
}

func (r *AlertRuleRepo) Delete(_ context.Context, userID, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if x, ok := r.m[id]; !ok || x.UserID != userID {
		return alert.ErrNotFound
	}
	delete(r.m, id)
	return nil
}

func (r *AlertRuleRepo) MarkFired(_ context.Context, id string, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	x, ok := r.m[id]
	if !ok {
		return alert.ErrNotFound
	}
	x.LastFiredAt = &at
	r.m[id] = x
	return nil
}

func (r *AlertRuleRepo) Pause(_ context.Context, id, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	x, ok := r.m[id]
	if !ok {
		return alert.ErrNotFound
	}
	x.Spec.Enabled, x.PausedReason = false, reason
	r.m[id] = x
	return nil
}

// ---- fires ----

type AlertFireRepo struct {
	mu    sync.Mutex
	seq   int64
	fires []alert.Fire
}

func NewAlertFireRepo() *AlertFireRepo { return &AlertFireRepo{} }

func (r *AlertFireRepo) Add(_ context.Context, f alert.Fire) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	f.Seq, f.Cursor = r.seq, page.Cursor(r.seq)
	r.fires = append(r.fires, f)
	if len(r.fires) > 20000 {
		r.fires = append([]alert.Fire(nil), r.fires[len(r.fires)-20000:]...)
	}
	return nil
}

func (r *AlertFireRepo) Recent(_ context.Context, userID string, limit int) ([]alert.Fire, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []alert.Fire{}
	for i := len(r.fires) - 1; i >= 0 && len(out) < limit; i-- {
		if r.fires[i].UserID == userID {
			out = append(out, r.fires[i])
		}
	}
	return out, nil
}

func (r *AlertFireRepo) Prune(_ context.Context, olderThan time.Time) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	kept := r.fires[:0:0]
	for _, f := range r.fires {
		if !f.FiredAt.Before(olderThan) {
			kept = append(kept, f)
		}
	}
	n := int64(len(r.fires) - len(kept))
	r.fires = kept
	return n, nil
}

// ---- channels ----

type ChannelRepo struct {
	mu sync.Mutex
	m  map[string]alert.Channel
}

func NewChannelRepo() *ChannelRepo { return &ChannelRepo{m: map[string]alert.Channel{}} }

func (r *ChannelRepo) Create(_ context.Context, c alert.Channel) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c.SecretVersion == 0 {
		c.SecretVersion = 1
	}
	r.m[c.ID] = c
	return nil
}

func (r *ChannelRepo) Get(_ context.Context, userID, id string) (alert.Channel, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.m[id]
	if !ok || c.UserID != userID {
		return alert.Channel{}, alert.ErrNotFound
	}
	return c, nil
}

func (r *ChannelRepo) ByID(_ context.Context, id string) (alert.Channel, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.m[id]
	if !ok {
		return alert.Channel{}, alert.ErrNotFound
	}
	return c, nil
}

func (r *ChannelRepo) List(_ context.Context, userID string) ([]alert.Channel, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []alert.Channel{}
	for _, c := range r.m {
		if c.UserID == userID {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (r *ChannelRepo) Delete(_ context.Context, userID, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.m[id]; !ok || c.UserID != userID {
		return alert.ErrNotFound
	}
	delete(r.m, id)
	return nil
}

func (r *ChannelRepo) SetEnabled(_ context.Context, userID, id string, enabled bool, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.m[id]
	if !ok || c.UserID != userID {
		return alert.ErrNotFound
	}
	c.Enabled, c.DisabledReason = enabled, reason
	if enabled {
		c.ConsecutiveFailures, c.DisabledReason = 0, ""
	}
	r.m[id] = c
	return nil
}

func (r *ChannelRepo) BumpSecretVersion(_ context.Context, userID, id string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.m[id]
	if !ok || c.UserID != userID {
		return 0, alert.ErrNotFound
	}
	c.SecretVersion++
	r.m[id] = c
	return c.SecretVersion, nil
}

func (r *ChannelRepo) RecordResult(_ context.Context, id string, ok bool, at time.Time) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, found := r.m[id]
	if !found {
		return 0, alert.ErrNotFound
	}
	c.LastDeliveryAt = &at
	if ok {
		c.ConsecutiveFailures = 0
	} else {
		c.ConsecutiveFailures++
	}
	r.m[id] = c
	return c.ConsecutiveFailures, nil
}

// ---- deliveries ----

type DeliveryRepo struct {
	mu  sync.Mutex
	seq int64
	ds  []alert.Delivery
}

func NewDeliveryRepo() *DeliveryRepo { return &DeliveryRepo{} }

func (r *DeliveryRepo) Add(_ context.Context, d alert.Delivery) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	d.Seq, d.Cursor = r.seq, page.Cursor(r.seq)
	r.ds = append(r.ds, d)
	if len(r.ds) > 20000 {
		r.ds = append([]alert.Delivery(nil), r.ds[len(r.ds)-20000:]...)
	}
	return nil
}

func (r *DeliveryRepo) Recent(_ context.Context, userID string, limit int) ([]alert.Delivery, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []alert.Delivery{}
	for i := len(r.ds) - 1; i >= 0 && len(out) < limit; i-- {
		if r.ds[i].UserID == userID {
			out = append(out, r.ds[i])
		}
	}
	return out, nil
}

func (r *DeliveryRepo) Prune(_ context.Context, olderThan time.Time) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	kept := r.ds[:0:0]
	for _, d := range r.ds {
		if !d.At.Before(olderThan) {
			kept = append(kept, d)
		}
	}
	n := int64(len(r.ds) - len(kept))
	r.ds = kept
	return n, nil
}

// ---- refresh tokens ----

type SessionRepo struct {
	mu sync.Mutex
	m  map[string]session.Token // by hash
}

func NewSessionRepo() *SessionRepo { return &SessionRepo{m: map[string]session.Token{}} }

func (r *SessionRepo) Create(_ context.Context, t session.Token) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[t.Hash] = t
	return nil
}

func (r *SessionRepo) Rotate(_ context.Context, hash string, now time.Time) (session.Token, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.m[hash]
	if !ok {
		return session.Token{}, session.ErrInvalid
	}
	switch {
	case t.UsedAt != nil:
		return t, session.ErrReused
	case t.RevokedAt != nil || !now.Before(t.ExpiresAt):
		return session.Token{}, session.ErrInvalid
	}
	t.UsedAt = &now
	r.m[hash] = t
	return t, nil
}

func (r *SessionRepo) FamilyOf(_ context.Context, hash string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.m[hash]
	if !ok {
		return "", session.ErrInvalid
	}
	return t.FamilyID, nil
}

func (r *SessionRepo) revoke(match func(session.Token) bool, now time.Time) {
	for h, t := range r.m {
		if match(t) && t.RevokedAt == nil {
			t.RevokedAt = &now
			r.m[h] = t
		}
	}
}

func (r *SessionRepo) RevokeFamily(_ context.Context, familyID string, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.revoke(func(t session.Token) bool { return t.FamilyID == familyID }, now)
	return nil
}

func (r *SessionRepo) RevokeUser(_ context.Context, userID string, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.revoke(func(t session.Token) bool { return t.UserID == userID }, now)
	return nil
}

func (r *SessionRepo) Prune(_ context.Context, before time.Time) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var n int64
	for h, t := range r.m {
		if t.ExpiresAt.Before(before) {
			delete(r.m, h)
			n++
		}
	}
	return n, nil
}

func (r *AlertFireRepo) Page(_ context.Context, userID string, q page.Query) ([]alert.Fire, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return page.Select(r.fires, func(f alert.Fire) int64 { return f.Seq }, func(f alert.Fire) bool { return f.UserID == userID }, q), nil
}

func (r *DeliveryRepo) Page(_ context.Context, userID string, q page.Query) ([]alert.Delivery, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return page.Select(r.ds, func(d alert.Delivery) int64 { return d.Seq }, func(d alert.Delivery) bool { return d.UserID == userID }, q), nil
}

func (r *AlertFireRepo) Get(_ context.Context, userID, id string) (alert.Fire, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := len(r.fires) - 1; i >= 0; i-- {
		if r.fires[i].ID == id && r.fires[i].UserID == userID {
			return r.fires[i], nil
		}
	}
	return alert.Fire{}, alert.ErrNotFound
}
