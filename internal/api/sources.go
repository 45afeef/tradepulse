package api

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"tradepulse/internal/connector"
	"tradepulse/internal/domain/event"
	"tradepulse/internal/domain/sourcedef"
	"tradepulse/internal/idgen"
	"tradepulse/internal/safefetch"
)

type sourceBody struct {
	Name            string               `json:"name" minLength:"2" maxLength:"40" pattern:"^[a-z0-9][a-z0-9_-]{1,39}$" doc:"Permanent slug members subscribe to" example:"cnbc-top-news"`
	Type            string               `json:"type" enum:"rss,webmonitor,manual"`
	URL             string               `json:"url,omitempty" maxLength:"2000" doc:"Public http(s) URL (rss and webmonitor)" example:"https://example.com/feed.xml"`
	Enabled         bool                 `json:"enabled,omitempty" doc:"Default false: test first, then enable."`
	IntervalSeconds int                  `json:"interval_seconds,omitempty" minimum:"0" maximum:"604800" doc:"Polling interval for this source (rss and webmonitor)" example:"300"`
	Entities        []string             `json:"entities,omitempty" maxItems:"10" doc:"Topics attached to every item (members can subscribe to them)"`
	Web             *sourcedef.WebConfig `json:"web,omitempty" doc:"Required for webmonitor"`
}
type sourceCreateIn struct{ Body sourceBody }

type sourceUpdateBody struct {
	URL             string               `json:"url,omitempty" maxLength:"2000"`
	Enabled         bool                 `json:"enabled"`
	IntervalSeconds int                  `json:"interval_seconds,omitempty" minimum:"0" maximum:"604800"`
	Entities        []string             `json:"entities,omitempty" maxItems:"10"`
	Web             *sourcedef.WebConfig `json:"web,omitempty"`
}
type sourceUpdateIn struct {
	ID   string `path:"id" maxLength:"64"`
	Body sourceUpdateBody
}

type sourceView struct {
	ID                  string               `json:"id"`
	Name                string               `json:"name"`
	Type                string               `json:"type" enum:"rss,webmonitor,manual"`
	URL                 string               `json:"url,omitempty"`
	Enabled             bool                 `json:"enabled"`
	IntervalSeconds     int                  `json:"interval_seconds"`
	Entities            []string             `json:"entities"`
	Web                 *sourcedef.WebConfig `json:"web,omitempty"`
	Health              string               `json:"health" enum:"ok,pending,degraded,failing,overdue,disabled"`
	LastRunAt           *time.Time           `json:"last_run_at,omitempty"`
	LastSuccessAt       *time.Time           `json:"last_success_at,omitempty"`
	NextRunAt           *time.Time           `json:"next_run_at,omitempty"`
	LastItemAt          *time.Time           `json:"last_item_at,omitempty"`
	LastError           string               `json:"last_error,omitempty"`
	ConsecutiveFailures int                  `json:"consecutive_failures"`
	ItemsTotal          int64                `json:"items_total"`
	CreatedAt           time.Time            `json:"created_at"`
}
type sourceOut struct{ Body sourceView }

type sourceListBody struct {
	Sources []sourceView `json:"sources"`
}
type sourceListOut struct{ Body sourceListBody }

type runView struct {
	StartedAt      time.Time `json:"started_at"`
	DurationMS     int       `json:"duration_ms"`
	OK             bool      `json:"ok"`
	HTTPStatus     int       `json:"http_status,omitempty"`
	ItemsSeen      int       `json:"items_seen"`
	ItemsNew       int       `json:"items_new"`
	ItemsPublished int       `json:"items_published"`
	Error          string    `json:"error,omitempty"`
}
type sourceDetailBody struct {
	Source     sourceView `json:"source"`
	RecentRuns []runView  `json:"recent_runs"`
}
type sourceDetailOut struct{ Body sourceDetailBody }

type previewItem struct {
	Title       string     `json:"title"`
	URL         string     `json:"url,omitempty"`
	Summary     string     `json:"summary,omitempty"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
}
type testBody struct {
	OK      bool          `json:"ok"`
	Error   string        `json:"error,omitempty"`
	Count   int           `json:"count" doc:"Items found (up to 20 are shown)"`
	Preview []previewItem `json:"preview"`
}
type testOut struct{ Body testBody }
type testIn struct{ Body sourceBody }

type catalogItem struct {
	Name     string   `json:"name" doc:"Use as the value of a 'source' subscription"`
	Type     string   `json:"type"`
	Entities []string `json:"entities" doc:"Topics usable in 'entity' subscriptions"`
}
type catalogBody struct {
	Sources []catalogItem `json:"sources"`
}
type catalogOut struct{ Body catalogBody }

type manualBody struct {
	Source   string   `json:"source" maxLength:"40" doc:"Name of an enabled manual source" example:"admin-posts"`
	Title    string   `json:"title" minLength:"1" maxLength:"300"`
	Summary  string   `json:"summary,omitempty" maxLength:"1000"`
	URL      string   `json:"url,omitempty" maxLength:"2000" doc:"Optional link (http/https)"`
	Entities []string `json:"entities,omitempty" maxItems:"10" doc:"Extra topics for this item"`
}
type manualIn struct{ Body manualBody }
type manualOutBody struct {
	EventID string `json:"event_id"`
}
type manualOut struct{ Body manualOutBody }

func (s *Server) bounds() sourcedef.Bounds {
	return sourcedef.Bounds{Min: s.Cfg.SourceMinInterval, Max: s.Cfg.SourceMaxInterval}
}

func toView(src sourcedef.Source, now time.Time) sourceView {
	ents := src.Entities
	if ents == nil {
		ents = []string{}
	}
	st := src.State
	return sourceView{
		ID: src.ID, Name: src.Name, Type: string(src.Type), URL: src.URL, Enabled: src.Enabled,
		IntervalSeconds: src.IntervalSeconds, Entities: ents, Web: src.Web, Health: src.Health(now),
		LastRunAt: st.LastRunAt, LastSuccessAt: st.LastSuccessAt, NextRunAt: st.NextRunAt, LastItemAt: st.LastItemAt,
		LastError: st.LastError, ConsecutiveFailures: st.ConsecutiveFailures, ItemsTotal: st.ItemsTotal, CreatedAt: src.CreatedAt,
	}
}

// validateSource applies domain rules, the SSRF static checks and connector-specific rules.
func (s *Server) validateSource(src sourcedef.Source) (sourcedef.Source, error) {
	n, err := src.Normalize(s.bounds())
	if err != nil {
		return n, huma.Error422UnprocessableEntity(err.Error())
	}
	if n.URL != "" {
		if err := safefetch.ValidateURL(n.URL); err != nil {
			return n, huma.Error422UnprocessableEntity("the url is not allowed: it must be a public http(s) address on port 80 or 443")
		}
	}
	if s.Connectors != nil {
		if err := s.Connectors.Validate(n); err != nil {
			return n, huma.Error422UnprocessableEntity(err.Error())
		}
	}
	return n, nil
}

func fromBody(b sourceBody) sourcedef.Source {
	return sourcedef.Source{Name: b.Name, Type: sourcedef.Type(b.Type), URL: b.URL, Enabled: b.Enabled,
		IntervalSeconds: b.IntervalSeconds, Entities: b.Entities, Web: b.Web}
}

func (s *Server) createSource(ctx context.Context, in *sourceCreateIn) (*sourceOut, error) {
	src, err := s.validateSource(fromBody(in.Body))
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	src.ID, src.CreatedAt, src.UpdatedAt = idgen.New(), now, now
	if err := s.Sources.Create(ctx, src); err != nil {
		if errors.Is(err, sourcedef.ErrNameTaken) {
			return nil, huma.Error409Conflict("a source with this name already exists")
		}
		s.Log.Error("create source", "err", err)
		return nil, huma.Error500InternalServerError("internal server error")
	}
	s.record(ctx, userID(ctx), "source.create", "source", src.ID, src.Name)
	return &sourceOut{Body: toView(src, now)}, nil
}

func (s *Server) listSources(ctx context.Context, _ *struct{}) (*sourceListOut, error) {
	list, err := s.Sources.List(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	now := time.Now().UTC()
	out := make([]sourceView, 0, len(list))
	for _, src := range list {
		out = append(out, toView(src, now))
	}
	return &sourceListOut{Body: sourceListBody{Sources: out}}, nil
}

func (s *Server) loadSource(ctx context.Context, id string) (sourcedef.Source, error) {
	src, err := s.Sources.Get(ctx, id)
	if errors.Is(err, sourcedef.ErrNotFound) {
		return src, huma.Error404NotFound("source not found")
	} else if err != nil {
		return src, huma.Error500InternalServerError("internal server error")
	}
	return src, nil
}

func (s *Server) getSource(ctx context.Context, in *idIn) (*sourceDetailOut, error) {
	src, err := s.loadSource(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	runs, err := s.Runs.Recent(ctx, src.ID, 20)
	if err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	rv := make([]runView, 0, len(runs))
	for _, r := range runs {
		rv = append(rv, runView{StartedAt: r.StartedAt, DurationMS: r.DurationMS, OK: r.OK, HTTPStatus: r.HTTPStatus,
			ItemsSeen: r.ItemsSeen, ItemsNew: r.ItemsNew, ItemsPublished: r.ItemsPublished, Error: r.Error})
	}
	return &sourceDetailOut{Body: sourceDetailBody{Source: toView(src, time.Now().UTC()), RecentRuns: rv}}, nil
}

func (s *Server) updateSource(ctx context.Context, in *sourceUpdateIn) (*sourceOut, error) {
	cur, err := s.loadSource(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	cur.URL, cur.Enabled, cur.IntervalSeconds, cur.Entities, cur.Web = in.Body.URL, in.Body.Enabled, in.Body.IntervalSeconds, in.Body.Entities, in.Body.Web
	n, err := s.validateSource(cur)
	if err != nil {
		return nil, err
	}
	n.UpdatedAt = time.Now().UTC()
	if err := s.Sources.Update(ctx, n); err != nil {
		if errors.Is(err, sourcedef.ErrNotFound) {
			return nil, huma.Error404NotFound("source not found")
		}
		return nil, huma.Error500InternalServerError("internal server error")
	}
	s.record(ctx, userID(ctx), "source.update", "source", n.ID, n.Name)
	fresh, _ := s.Sources.Get(ctx, n.ID)
	return &sourceOut{Body: toView(fresh, time.Now().UTC())}, nil
}

func (s *Server) deleteSource(ctx context.Context, in *idIn) (*struct{}, error) {
	src, err := s.loadSource(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	if err := s.Sources.Delete(ctx, in.ID); err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	s.record(ctx, userID(ctx), "source.delete", "source", src.ID, src.Name)
	return &struct{}{}, nil
}

func (s *Server) runSourceNow(ctx context.Context, in *idIn) (*sourceOut, error) {
	src, err := s.loadSource(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	if !src.Enabled || src.Type == sourcedef.TypeManual {
		return nil, huma.Error409Conflict("only enabled rss or webmonitor sources can be run")
	}
	now := time.Now().UTC()
	st := src.State
	st.NextRunAt = &now
	if err := s.Sources.SaveState(ctx, src.ID, st); err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	s.record(ctx, userID(ctx), "source.run", "source", src.ID, src.Name)
	src.State = st
	return &sourceOut{Body: toView(src, now)}, nil
}

func (s *Server) testSource(ctx context.Context, in *testIn) (*testOut, error) {
	src, err := s.validateSource(fromBody(in.Body))
	if err != nil {
		return nil, err
	}
	if src.Type == sourcedef.TypeManual {
		return nil, huma.Error422UnprocessableEntity("manual sources have nothing to fetch")
	}
	conn, ok := s.Connectors.Get(src.Type)
	if !ok {
		return nil, huma.Error422UnprocessableEntity("no connector for this type")
	}
	cctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	res, err := conn.Fetch(cctx, src)
	if err != nil {
		msg := err.Error()
		if errors.Is(err, safefetch.ErrBlocked) {
			msg = "destination not allowed"
		}
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return &testOut{Body: testBody{OK: false, Error: msg, Preview: []previewItem{}}}, nil
	}
	pv := make([]previewItem, 0, 20)
	for i, it := range res.Items {
		if i >= 20 {
			break
		}
		p := previewItem{Title: it.Title, URL: it.URL, Summary: it.Summary}
		if !it.PublishedAt.IsZero() {
			t := it.PublishedAt
			p.PublishedAt = &t
		}
		pv = append(pv, p)
	}
	return &testOut{Body: testBody{OK: true, Count: len(res.Items), Preview: pv}}, nil
}

func (s *Server) catalogSources(ctx context.Context, _ *struct{}) (*catalogOut, error) {
	list, err := s.Sources.List(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	out := make([]catalogItem, 0, len(list))
	for _, src := range list {
		if !src.Enabled {
			continue
		}
		ents := src.Entities
		if ents == nil {
			ents = []string{}
		}
		out = append(out, catalogItem{Name: src.Name, Type: string(src.Type), Entities: ents})
	}
	return &catalogOut{Body: catalogBody{Sources: out}}, nil
}

func (s *Server) postItem(ctx context.Context, in *manualIn) (*manualOut, error) {
	src, err := s.Sources.GetByName(ctx, strings.ToLower(strings.TrimSpace(in.Body.Source)))
	if errors.Is(err, sourcedef.ErrNotFound) {
		return nil, huma.Error404NotFound("source not found")
	} else if err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	if src.Type != sourcedef.TypeManual || !src.Enabled {
		return nil, huma.Error409Conflict("the source must be an enabled manual source")
	}
	title := strings.Join(strings.Fields(in.Body.Title), " ")
	if title == "" {
		return nil, huma.Error422UnprocessableEntity("title is required")
	}
	link := strings.TrimSpace(in.Body.URL)
	if link != "" {
		u, err := url.Parse(link)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
			return nil, huma.Error422UnprocessableEntity("url must be a plain http(s) link")
		}
	}
	for _, e := range in.Body.Entities {
		if n := event.NormEntity(e); n != "" && len(n) <= 50 {
			src.Entities = append(src.Entities, n)
		}
	}
	item := connector.Item{Key: "manual:" + idgen.New(), Title: title, Summary: strings.Join(strings.Fields(in.Body.Summary), " "), URL: link}
	ev := connector.ItemEvent(src, item, time.Now().UTC())
	if err := s.Registry.Validate(ev); err != nil {
		return nil, huma.Error422UnprocessableEntity(err.Error())
	}
	if err := s.Bus.Publish(ctx, ev); err != nil {
		return nil, huma.Error503ServiceUnavailable("the service is shutting down")
	}
	s.record(ctx, userID(ctx), "item.manual", "source", src.ID, src.Name)
	return &manualOut{Body: manualOutBody{EventID: ev.ID}}, nil
}
