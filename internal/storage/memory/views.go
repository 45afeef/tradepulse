package memory

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"tradepulse/internal/domain/feedback"
	"tradepulse/internal/domain/view"
)

type ViewRepo struct {
	mu sync.Mutex
	m  map[string]view.View
}

func NewViewRepo() *ViewRepo { return &ViewRepo{m: map[string]view.View{}} }

func (r *ViewRepo) clash(userID, name, exceptID string) bool {
	for _, v := range r.m {
		if v.UserID == userID && v.ID != exceptID && strings.EqualFold(v.Spec.Name, name) {
			return true
		}
	}
	return false
}

func (r *ViewRepo) Create(_ context.Context, v view.View) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.clash(v.UserID, v.Spec.Name, "") {
		return view.ErrDuplicate
	}
	r.m[v.ID] = v
	return nil
}

func (r *ViewRepo) List(_ context.Context, userID string) ([]view.View, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []view.View{}
	for _, v := range r.m {
		if v.UserID == userID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (r *ViewRepo) Get(_ context.Context, userID, id string) (view.View, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.m[id]
	if !ok || v.UserID != userID {
		return view.View{}, view.ErrNotFound
	}
	return v, nil
}

func (r *ViewRepo) Update(_ context.Context, v view.View) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cur, ok := r.m[v.ID]
	if !ok || cur.UserID != v.UserID {
		return view.ErrNotFound
	}
	if r.clash(v.UserID, v.Spec.Name, v.ID) {
		return view.ErrDuplicate
	}
	cur.Spec, cur.UpdatedAt = v.Spec, v.UpdatedAt
	r.m[v.ID] = cur
	return nil
}

func (r *ViewRepo) Delete(_ context.Context, userID, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if v, ok := r.m[id]; !ok || v.UserID != userID {
		return view.ErrNotFound
	}
	delete(r.m, id)
	return nil
}

type FeedbackRepo struct {
	mu sync.Mutex
	m  map[string]feedback.Entry // userID|type|target
}

func NewFeedbackRepo() *FeedbackRepo { return &FeedbackRepo{m: map[string]feedback.Entry{}} }

func fbKey(u, t, id string) string { return u + "|" + t + "|" + id }

func (r *FeedbackRepo) Upsert(_ context.Context, e feedback.Entry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := fbKey(e.UserID, e.TargetType, e.TargetID)
	if old, ok := r.m[k]; ok {
		e.CreatedAt = old.CreatedAt
	}
	r.m[k] = e
	return nil
}

func (r *FeedbackRepo) Delete(_ context.Context, userID, targetType, targetID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.m, fbKey(userID, targetType, targetID))
	return nil
}

func (r *FeedbackRepo) Ratings(_ context.Context, userID, targetType string, ids []string) (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]string{}
	for _, id := range ids {
		if e, ok := r.m[fbKey(userID, targetType, id)]; ok {
			out[id] = e.Rating
		}
	}
	return out, nil
}

func (r *FeedbackRepo) Summary(_ context.Context, since time.Time) (feedback.Summary, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var s feedback.Summary
	groups := map[string]*feedback.GroupStat{}
	for _, e := range r.m {
		if e.UpdatedAt.Before(since) {
			continue
		}
		t := &s.FeedItems
		if e.TargetType == feedback.TargetAlertFire {
			t = &s.AlertFires
		}
		g := groups[e.TargetType+"|"+e.Group]
		if g == nil {
			g = &feedback.GroupStat{TargetType: e.TargetType, Group: e.Group, Label: e.Label}
			groups[e.TargetType+"|"+e.Group] = g
		}
		if e.Rating == feedback.Useful {
			t.Useful++
			g.Useful++
		} else {
			t.NotUseful++
			g.NotUseful++
		}
	}
	for _, g := range groups {
		s.Groups = append(s.Groups, *g)
	}
	sort.Slice(s.Groups, func(i, j int) bool {
		a, b := s.Groups[i], s.Groups[j]
		if a.Useful+a.NotUseful != b.Useful+b.NotUseful {
			return a.Useful+a.NotUseful > b.Useful+b.NotUseful
		}
		return a.Group < b.Group
	})
	if len(s.Groups) > 20 {
		s.Groups = s.Groups[:20]
	}
	return s, nil
}

func (r *FeedbackRepo) Prune(_ context.Context, before time.Time) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var n int64
	for k, e := range r.m {
		if e.UpdatedAt.Before(before) {
			delete(r.m, k)
			n++
		}
	}
	return n, nil
}
