package api

import (
	"context"
	"errors"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"tradepulse/internal/domain/alert"
	"tradepulse/internal/domain/feed"
	"tradepulse/internal/domain/feedback"
	"tradepulse/internal/domain/view"
	"tradepulse/internal/idgen"
	"tradepulse/internal/page"
)

// ---------------- saved views ----------------

type viewView struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	Filter    view.Filter `json:"filter"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
}

func toViewView(v view.View) viewView {
	return viewView{ID: v.ID, Name: v.Spec.Name, Filter: v.Spec.Filter, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt}
}

type viewIn struct{ Body view.Spec }
type viewUpdateIn struct {
	ID   string `path:"id" maxLength:"64"`
	Body view.Spec
}
type viewOut struct{ Body viewView }
type viewListBody struct {
	Views []viewView `json:"views"`
}
type viewListOut struct{ Body viewListBody }

func (s *Server) listViews(ctx context.Context, _ *struct{}) (*viewListOut, error) {
	vs, err := s.Views.List(ctx, userID(ctx))
	if err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	out := make([]viewView, 0, len(vs))
	for _, v := range vs {
		out = append(out, toViewView(v))
	}
	return &viewListOut{Body: viewListBody{Views: out}}, nil
}

func (s *Server) createView(ctx context.Context, in *viewIn) (*viewOut, error) {
	uid := userID(ctx)
	spec, err := in.Body.Normalize(s.Registry)
	if err != nil {
		return nil, huma.Error422UnprocessableEntity(err.Error())
	}
	existing, err := s.Views.List(ctx, uid)
	if err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	if len(existing) >= view.MaxPerUser {
		return nil, huma.Error409Conflict("view limit reached: delete a view before creating another")
	}
	now := time.Now().UTC()
	v := view.View{ID: idgen.New(), UserID: uid, Spec: spec, CreatedAt: now, UpdatedAt: now}
	if err := s.Views.Create(ctx, v); errors.Is(err, view.ErrDuplicate) {
		return nil, huma.Error409Conflict("you already have a view with this name")
	} else if err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	return &viewOut{Body: toViewView(v)}, nil
}

func (s *Server) updateView(ctx context.Context, in *viewUpdateIn) (*viewOut, error) {
	uid := userID(ctx)
	cur, err := s.Views.Get(ctx, uid, in.ID)
	if errors.Is(err, view.ErrNotFound) {
		return nil, huma.Error404NotFound("view not found")
	} else if err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	spec, err := in.Body.Normalize(s.Registry)
	if err != nil {
		return nil, huma.Error422UnprocessableEntity(err.Error())
	}
	cur.Spec, cur.UpdatedAt = spec, time.Now().UTC()
	if err := s.Views.Update(ctx, cur); errors.Is(err, view.ErrDuplicate) {
		return nil, huma.Error409Conflict("you already have a view with this name")
	} else if errors.Is(err, view.ErrNotFound) {
		return nil, huma.Error404NotFound("view not found")
	} else if err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	return &viewOut{Body: toViewView(cur)}, nil
}

func (s *Server) deleteView(ctx context.Context, in *idIn) (*struct{}, error) {
	if err := s.Views.Delete(ctx, userID(ctx), in.ID); errors.Is(err, view.ErrNotFound) {
		return nil, huma.Error404NotFound("view not found")
	} else if err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	return &struct{}{}, nil
}

// ---------------- feed (optionally through a saved view) ----------------

type feedIn struct {
	Limit  int    `query:"limit" default:"50" minimum:"1" maximum:"200" doc:"Page size"`
	Before string `query:"before" maxLength:"64" doc:"Cursor: items OLDER than this position, newest first (scroll back)"`
	After  string `query:"after" maxLength:"64" doc:"Cursor: items NEWER than this position, OLDEST first (catch-up after a reconnect); repeat with next_cursor until it is empty"`
	View   string `query:"view" maxLength:"64" doc:"Id of a saved view (GET /me/views): only items matching it. Paging works the same."`
}

// filteredFeed pages through the member's feed (at most the retained 1,000 items) and keeps
// the items the view accepts, so cursors and page sizes behave exactly like the plain feed.
func (s *Server) filteredFeed(ctx context.Context, uid string, q page.Query, f view.Filter) ([]feed.FeedItem, error) {
	out := []feed.FeedItem{}
	now := time.Now()
	cur := page.Query{Limit: 200, Before: q.Before, After: q.After}
	for scanned := 0; scanned < 1200 && len(out) < q.Limit; scanned += cur.Limit {
		batch, err := s.Feeds.Page(ctx, uid, cur)
		if err != nil {
			return nil, err
		}
		for _, it := range batch {
			if f.Match(it, now) {
				out = append(out, it)
				if len(out) == q.Limit {
					break
				}
			}
		}
		if len(batch) < cur.Limit {
			break
		}
		last := batch[len(batch)-1].Seq
		if q.Asc() {
			cur.After = last
		} else {
			cur.Before = last
		}
	}
	return out, nil
}

func (s *Server) rateFeedItems(ctx context.Context, uid string, items []feed.FeedItem) {
	if s.Feedback == nil || len(items) == 0 {
		return
	}
	ids := make([]string, len(items))
	for i, it := range items {
		ids[i] = it.ID
	}
	m, err := s.Feedback.Ratings(ctx, uid, feedback.TargetFeedItem, ids)
	if err != nil {
		s.Log.Warn("load feedback", "err", err)
		return
	}
	for i := range items {
		items[i].Feedback = m[items[i].ID]
	}
}

func (s *Server) rateFires(ctx context.Context, uid string, fires []alert.Fire) {
	if s.Feedback == nil || len(fires) == 0 {
		return
	}
	ids := make([]string, len(fires))
	for i, f := range fires {
		ids[i] = f.ID
	}
	m, err := s.Feedback.Ratings(ctx, uid, feedback.TargetAlertFire, ids)
	if err != nil {
		s.Log.Warn("load feedback", "err", err)
		return
	}
	for i := range fires {
		fires[i].Feedback = m[fires[i].ID]
	}
}

func (s *Server) getFeed(ctx context.Context, in *feedIn) (*feedOut, error) {
	q, err := page.Parse(in.Limit, 200, in.Before, in.After)
	if err != nil {
		return nil, huma.Error422UnprocessableEntity(err.Error())
	}
	uid := userID(ctx)
	var items []feed.FeedItem
	if in.View != "" {
		v, verr := s.Views.Get(ctx, uid, in.View)
		if errors.Is(verr, view.ErrNotFound) {
			return nil, huma.Error404NotFound("view not found")
		} else if verr != nil {
			return nil, huma.Error500InternalServerError("internal server error")
		}
		items, err = s.filteredFeed(ctx, uid, q, v.Spec.Filter)
	} else {
		items, err = s.Feeds.Page(ctx, uid, q)
	}
	if err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	s.rateFeedItems(ctx, uid, items)
	next := ""
	if n := len(items); n > 0 {
		next = page.NextCursor(q, n, items[n-1].Seq)
	}
	return &feedOut{Body: feedBody{Items: items, NextCursor: next, Order: orderOf(q)}}, nil
}

// ---------------- feedback ----------------

type feedbackBody struct {
	Rating string `json:"rating" enum:"useful,not_useful"`
	Note   string `json:"note,omitempty" maxLength:"280" doc:"Optional comment (why it was not useful, for instance)"`
}
type feedbackIn struct {
	ID   string `path:"id" maxLength:"64"`
	Body feedbackBody
}
type feedbackView struct {
	TargetID  string    `json:"target_id"`
	Rating    string    `json:"rating"`
	Note      string    `json:"note,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}
type feedbackOut struct{ Body feedbackView }

func (s *Server) saveFeedback(ctx context.Context, uid, targetType, targetID, group, label string, b feedbackBody) (*feedbackOut, error) {
	if !feedback.ValidRating(b.Rating) {
		return nil, huma.Error422UnprocessableEntity("rating must be useful or not_useful")
	}
	note, err := feedback.CleanNote(b.Note)
	if err != nil {
		return nil, huma.Error422UnprocessableEntity(err.Error())
	}
	now := time.Now().UTC()
	e := feedback.Entry{UserID: uid, TargetType: targetType, TargetID: targetID, Rating: b.Rating, Note: note, Group: group, Label: label, CreatedAt: now, UpdatedAt: now}
	if err := s.Feedback.Upsert(ctx, e); err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	s.Metrics.Inc("feedback_total", "target", targetType, "rating", b.Rating)
	return &feedbackOut{Body: feedbackView{TargetID: targetID, Rating: b.Rating, Note: note, UpdatedAt: now}}, nil
}

// Ratings are only possible on the member's own items; anything else is a 404 (no existence leak).
func (s *Server) rateFeedItem(ctx context.Context, in *feedbackIn) (*feedbackOut, error) {
	uid := userID(ctx)
	it, err := s.Feeds.Get(ctx, uid, in.ID)
	if errors.Is(err, feed.ErrNotFound) {
		return nil, huma.Error404NotFound("feed item not found")
	} else if err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	return s.saveFeedback(ctx, uid, feedback.TargetFeedItem, it.ID, "source:"+it.Source, it.Source, in.Body)
}

func (s *Server) clearFeedItemFeedback(ctx context.Context, in *idIn) (*struct{}, error) {
	if err := s.Feedback.Delete(ctx, userID(ctx), feedback.TargetFeedItem, in.ID); err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	return &struct{}{}, nil
}

func (s *Server) rateAlertFire(ctx context.Context, in *feedbackIn) (*feedbackOut, error) {
	uid := userID(ctx)
	f, err := s.AlertFires.Get(ctx, uid, in.ID)
	if errors.Is(err, alert.ErrNotFound) {
		return nil, huma.Error404NotFound("alert not found")
	} else if err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	return s.saveFeedback(ctx, uid, feedback.TargetAlertFire, f.ID, "rule:"+f.RuleID, f.RuleName, in.Body)
}

func (s *Server) clearAlertFireFeedback(ctx context.Context, in *idIn) (*struct{}, error) {
	if err := s.Feedback.Delete(ctx, userID(ctx), feedback.TargetAlertFire, in.ID); err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	return &struct{}{}, nil
}

type summaryIn struct {
	Days int `query:"days" default:"30" minimum:"1" maximum:"365" doc:"Look-back window in days"`
}
type totalsView struct {
	Useful    int      `json:"useful"`
	NotUseful int      `json:"not_useful"`
	Precision *float64 `json:"precision,omitempty" doc:"useful / (useful + not_useful); absent when nothing was rated"`
}
type groupView struct {
	TargetType string `json:"target_type" enum:"feed_item,alert_fire"`
	Group      string `json:"group" doc:"source:<name> for feed items, rule:<id> for alerts"`
	Label      string `json:"label" doc:"Source name or alert rule name"`
	Useful     int    `json:"useful"`
	NotUseful  int    `json:"not_useful"`
	Precision  *float64 `json:"precision,omitempty" doc:"useful / (useful + not_useful)"`
}
type summaryBody struct {
	Days       int         `json:"days"`
	FeedItems  totalsView  `json:"feed_items"`
	AlertFires totalsView  `json:"alert_fires" doc:"alert_fires.precision is the alert-precision metric (target above 0.7)"`
	Groups     []groupView `json:"groups" doc:"The 20 most-rated sources and rules"`
}
type summaryOut struct{ Body summaryBody }

func tv(t feedback.Totals) totalsView {
	return totalsView{Useful: t.Useful, NotUseful: t.NotUseful, Precision: t.Precision()}
}

func (s *Server) feedbackSummary(ctx context.Context, in *summaryIn) (*summaryOut, error) {
	sum, err := s.Feedback.Summary(ctx, time.Now().UTC().AddDate(0, 0, -in.Days))
	if err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	groups := make([]groupView, 0, len(sum.Groups))
	for _, g := range sum.Groups {
		groups = append(groups, groupView{TargetType: g.TargetType, Group: g.Group, Label: g.Label, Useful: g.Useful, NotUseful: g.NotUseful, Precision: g.Precision()})
	}
	return &summaryOut{Body: summaryBody{Days: in.Days, FeedItems: tv(sum.FeedItems), AlertFires: tv(sum.AlertFires), Groups: groups}}, nil
}
