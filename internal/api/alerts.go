package api

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"tradepulse/internal/alerting"
	"tradepulse/internal/domain/alert"
	"tradepulse/internal/domain/rule"
	"tradepulse/internal/idgen"
	"tradepulse/internal/safefetch"
)

// AlertEngine keeps the live alert evaluator in sync with stored rules.
type AlertEngine interface {
	UpsertRule(r alert.Rule)
	RemoveRule(id string)
	RemoveUser(userID string)
	LoadUser(ctx context.Context, userID string) error
	Preview(r alert.Rule) alert.Preview
}

// Outbound can send a test delivery to a channel (alerting.Sender).
type Outbound interface {
	EnqueueTest(userID, channelID string)
}

var _ Outbound = (*alerting.Sender)(nil)

type ruleView struct {
	ID              string           `json:"id"`
	Name            string           `json:"name"`
	Scope           alert.Scope      `json:"scope"`
	Types           []string         `json:"types"`
	Conditions      []rule.Condition `json:"conditions"`
	CooldownSeconds int              `json:"cooldown_seconds"`
	Severity        string           `json:"severity"`
	Channels        []string         `json:"channels"`
	Enabled         bool             `json:"enabled"`
	PausedReason    string           `json:"paused_reason,omitempty" doc:"Why the system paused this rule (re-enable it after tightening the rule)"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
	LastFiredAt     *time.Time       `json:"last_fired_at,omitempty"`
}

func toRuleView(r alert.Rule) ruleView {
	sp := r.Spec
	if sp.Types == nil {
		sp.Types = []string{}
	}
	if sp.Conditions == nil {
		sp.Conditions = []rule.Condition{}
	}
	return ruleView{ID: r.ID, Name: sp.Name, Scope: sp.Scope, Types: sp.Types, Conditions: sp.Conditions, CooldownSeconds: sp.CooldownSeconds,
		Severity: sp.Severity, Channels: sp.Channels, Enabled: sp.Enabled, PausedReason: r.PausedReason, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, LastFiredAt: r.LastFiredAt}
}

type ruleIn struct{ Body alert.Spec }
type ruleOut struct{ Body ruleView }
type ruleUpdateIn struct {
	ID   string `path:"id" maxLength:"64"`
	Body alert.Spec
}
type ruleListBody struct {
	Rules []ruleView `json:"rules"`
}
type ruleListOut struct{ Body ruleListBody }
type templatesBody struct {
	Templates []alert.Template `json:"templates"`
}
type templatesOut struct{ Body templatesBody }
type fromTemplateBody struct {
	TemplateID string   `json:"template_id" maxLength:"64" example:"keyword-mention"`
	Values     []string `json:"values" minItems:"1" maxItems:"20" doc:"Scope values (see the template's values_hint)"`
	Name       string   `json:"name,omitempty" maxLength:"80" doc:"Optional rule name"`
	Channels   []string `json:"channels,omitempty" maxItems:"10"`
}
type fromTemplateIn struct{ Body fromTemplateBody }
type previewIn struct{ Body alert.Spec }
type previewOut struct{ Body alert.Preview }
type limitIn struct {
	Limit int `query:"limit" default:"50" minimum:"1" maximum:"200" doc:"Max entries, newest first"`
}
type firesBody struct {
	Fires []alert.Fire `json:"fires"`
}
type firesOut struct{ Body firesBody }

// channelChecker returns a validator accepting only the caller's own channel ids.
func (s *Server) channelChecker(ctx context.Context, uid string) func(string) bool {
	chs, _ := s.Channels.List(ctx, uid)
	own := map[string]bool{}
	for _, c := range chs {
		own[c.ID] = true
	}
	return func(id string) bool { return own[id] }
}

func (s *Server) saveNewRule(ctx context.Context, spec alert.Spec) (*ruleOut, error) {
	uid := userID(ctx)
	sp, err := spec.Normalize(s.Registry, s.channelChecker(ctx, uid))
	if err != nil {
		return nil, huma.Error422UnprocessableEntity(err.Error())
	}
	existing, err := s.AlertRules.List(ctx, uid)
	if err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	if len(existing) >= alert.MaxRulesPerUser {
		return nil, huma.Error409Conflict("rule limit reached: delete a rule before creating another")
	}
	now := time.Now().UTC()
	r := alert.Rule{ID: idgen.New(), UserID: uid, Spec: sp, CreatedAt: now, UpdatedAt: now}
	if err := s.AlertRules.Create(ctx, r); err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	s.Alerts.UpsertRule(r)
	return &ruleOut{Body: toRuleView(r)}, nil
}

func (s *Server) listTemplates(_ context.Context, _ *struct{}) (*templatesOut, error) {
	return &templatesOut{Body: templatesBody{Templates: s.Templates.List()}}, nil
}

func (s *Server) listRules(ctx context.Context, _ *struct{}) (*ruleListOut, error) {
	rs, err := s.AlertRules.List(ctx, userID(ctx))
	if err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	out := make([]ruleView, 0, len(rs))
	for _, r := range rs {
		out = append(out, toRuleView(r))
	}
	return &ruleListOut{Body: ruleListBody{Rules: out}}, nil
}

func (s *Server) createRule(ctx context.Context, in *ruleIn) (*ruleOut, error) { return s.saveNewRule(ctx, in.Body) }

func (s *Server) createFromTemplate(ctx context.Context, in *fromTemplateIn) (*ruleOut, error) {
	t, ok := s.Templates.Get(in.Body.TemplateID)
	if !ok {
		return nil, huma.Error404NotFound("template not found")
	}
	spec := t.Spec(in.Body.Values)
	if n := strings.TrimSpace(in.Body.Name); n != "" {
		spec.Name = n
	}
	spec.Channels = in.Body.Channels
	return s.saveNewRule(ctx, spec)
}

func (s *Server) ownedRule(ctx context.Context, id string) (alert.Rule, error) {
	r, err := s.AlertRules.Get(ctx, userID(ctx), id)
	if errors.Is(err, alert.ErrNotFound) {
		return r, huma.Error404NotFound("rule not found") // also for other users' rules: no existence leak
	} else if err != nil {
		return r, huma.Error500InternalServerError("internal server error")
	}
	return r, nil
}

func (s *Server) getRule(ctx context.Context, in *idIn) (*ruleOut, error) {
	r, err := s.ownedRule(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	return &ruleOut{Body: toRuleView(r)}, nil
}

func (s *Server) updateRule(ctx context.Context, in *ruleUpdateIn) (*ruleOut, error) {
	cur, err := s.ownedRule(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	sp, err := in.Body.Normalize(s.Registry, s.channelChecker(ctx, cur.UserID))
	if err != nil {
		return nil, huma.Error422UnprocessableEntity(err.Error())
	}
	cur.Spec, cur.UpdatedAt = sp, time.Now().UTC()
	if sp.Enabled {
		cur.PausedReason = ""
	}
	if err := s.AlertRules.Update(ctx, cur); err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	s.Alerts.UpsertRule(cur)
	return &ruleOut{Body: toRuleView(cur)}, nil
}

func (s *Server) deleteRule(ctx context.Context, in *idIn) (*struct{}, error) {
	if _, err := s.ownedRule(ctx, in.ID); err != nil {
		return nil, err
	}
	if err := s.AlertRules.Delete(ctx, userID(ctx), in.ID); err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	s.Alerts.RemoveRule(in.ID)
	return &struct{}{}, nil
}

func (s *Server) previewRule(ctx context.Context, in *previewIn) (*previewOut, error) {
	sp, err := in.Body.Normalize(s.Registry, s.channelChecker(ctx, userID(ctx)))
	if err != nil {
		return nil, huma.Error422UnprocessableEntity(err.Error())
	}
	return &previewOut{Body: s.Alerts.Preview(alert.Rule{ID: "preview", UserID: userID(ctx), Spec: sp})}, nil
}

func (s *Server) listFires(ctx context.Context, in *limitIn) (*firesOut, error) {
	fs, err := s.AlertFires.Recent(ctx, userID(ctx), in.Limit)
	if err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	return &firesOut{Body: firesBody{Fires: fs}}, nil
}

// ---------------- channels ----------------

type channelBody struct {
	Type string `json:"type" enum:"webhook"`
	Name string `json:"name" minLength:"1" maxLength:"60" example:"My n8n"`
	URL  string `json:"url" maxLength:"2000" doc:"HTTPS endpoint that receives signed alert deliveries" example:"https://example.com/hooks/tradepulse"`
}
type channelIn struct{ Body channelBody }
type channelView struct {
	ID                  string     `json:"id"`
	Type                string     `json:"type"`
	Name                string     `json:"name"`
	URL                 string     `json:"url"`
	Enabled             bool       `json:"enabled"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	DisabledReason      string     `json:"disabled_reason,omitempty"`
	LastDeliveryAt      *time.Time `json:"last_delivery_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	Secret              string     `json:"secret,omitempty" doc:"Signing secret: shown ONLY at creation and rotation"`
}
type channelOut struct{ Body channelView }
type channelListBody struct {
	Channels []channelView `json:"channels"`
}
type channelListOut struct{ Body channelListBody }
type deliveriesBody struct {
	Deliveries []alert.Delivery `json:"deliveries"`
}
type deliveriesOut struct{ Body deliveriesBody }

func toChannelView(c alert.Channel) channelView {
	return channelView{ID: c.ID, Type: c.Type, Name: c.Name, URL: c.URL, Enabled: c.Enabled, ConsecutiveFailures: c.ConsecutiveFailures,
		DisabledReason: c.DisabledReason, LastDeliveryAt: c.LastDeliveryAt, CreatedAt: c.CreatedAt}
}

func (s *Server) channelSecret(c alert.Channel) string {
	return alerting.OutboundSecret([]byte(s.Cfg.WebhookSigningKey), c.ID, c.SecretVersion)
}

func (s *Server) outboundReady() error {
	if s.Cfg.WebhookSigningKey == "" {
		return huma.Error422UnprocessableEntity("outbound webhooks are not enabled on this server (set WEBHOOK_SIGNING_KEY)")
	}
	return nil
}

func (s *Server) createChannel(ctx context.Context, in *channelIn) (*channelOut, error) {
	if err := s.outboundReady(); err != nil {
		return nil, err
	}
	name := strings.Join(strings.Fields(in.Body.Name), " ")
	raw := strings.TrimSpace(in.Body.URL)
	u, perr := url.Parse(raw)
	if name == "" || perr != nil || u.Scheme != "https" || safefetch.ValidateURL(raw) != nil {
		return nil, huma.Error422UnprocessableEntity("name is required and url must be a public https address on port 443 (no credentials in the URL)")
	}
	uid := userID(ctx)
	existing, err := s.Channels.List(ctx, uid)
	if err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	if len(existing) >= alert.MaxChannelsPerUser {
		return nil, huma.Error409Conflict("channel limit reached: delete a channel before adding another")
	}
	c := alert.Channel{ID: idgen.New(), UserID: uid, Type: "webhook", Name: name, URL: raw, Enabled: true, SecretVersion: 1, CreatedAt: time.Now().UTC()}
	if err := s.Channels.Create(ctx, c); err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	v := toChannelView(c)
	v.Secret = s.channelSecret(c)
	return &channelOut{Body: v}, nil
}

func (s *Server) listChannels(ctx context.Context, _ *struct{}) (*channelListOut, error) {
	cs, err := s.Channels.List(ctx, userID(ctx))
	if err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	out := make([]channelView, 0, len(cs))
	for _, c := range cs {
		out = append(out, toChannelView(c))
	}
	return &channelListOut{Body: channelListBody{Channels: out}}, nil
}

func (s *Server) ownedChannel(ctx context.Context, id string) (alert.Channel, error) {
	c, err := s.Channels.Get(ctx, userID(ctx), id)
	if errors.Is(err, alert.ErrNotFound) {
		return c, huma.Error404NotFound("channel not found")
	} else if err != nil {
		return c, huma.Error500InternalServerError("internal server error")
	}
	return c, nil
}

func (s *Server) deleteChannel(ctx context.Context, in *idIn) (*struct{}, error) {
	c, err := s.ownedChannel(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	if err := s.Channels.Delete(ctx, c.UserID, c.ID); err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	// Remove the channel from the user's rules so nothing keeps pointing at it.
	rs, _ := s.AlertRules.List(ctx, c.UserID)
	for _, r := range rs {
		kept := make([]string, 0, len(r.Spec.Channels))
		for _, id := range r.Spec.Channels {
			if id != c.ID {
				kept = append(kept, id)
			}
		}
		if len(kept) == len(r.Spec.Channels) {
			continue
		}
		if len(kept) == 0 {
			kept = []string{alert.InApp}
		}
		r.Spec.Channels, r.UpdatedAt = kept, time.Now().UTC()
		if err := s.AlertRules.Update(ctx, r); err == nil {
			s.Alerts.UpsertRule(r)
		}
	}
	return &struct{}{}, nil
}

func (s *Server) rotateChannelSecret(ctx context.Context, in *idIn) (*channelOut, error) {
	if err := s.outboundReady(); err != nil {
		return nil, err
	}
	c, err := s.ownedChannel(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	v, err := s.Channels.BumpSecretVersion(ctx, c.UserID, c.ID)
	if err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	c.SecretVersion = v
	out := toChannelView(c)
	out.Secret = s.channelSecret(c)
	return &channelOut{Body: out}, nil
}

func (s *Server) enableChannel(ctx context.Context, in *idIn) (*channelOut, error) {
	c, err := s.ownedChannel(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	if err := s.Channels.SetEnabled(ctx, c.UserID, c.ID, true, ""); err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	c.Enabled, c.DisabledReason, c.ConsecutiveFailures = true, "", 0
	return &channelOut{Body: toChannelView(c)}, nil
}

type testSentBody struct {
	Queued bool `json:"queued"`
}
type testSentOut struct{ Body testSentBody }

func (s *Server) testChannel(ctx context.Context, in *idIn) (*testSentOut, error) {
	c, err := s.ownedChannel(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	if !c.Enabled {
		return nil, huma.Error409Conflict("the channel is disabled: enable it first")
	}
	if s.Outbound != nil {
		s.Outbound.EnqueueTest(c.UserID, c.ID)
	}
	return &testSentOut{Body: testSentBody{Queued: true}}, nil
}

func (s *Server) listDeliveries(ctx context.Context, in *limitIn) (*deliveriesOut, error) {
	ds, err := s.Deliveries.Recent(ctx, userID(ctx), in.Limit)
	if err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	return &deliveriesOut{Body: deliveriesBody{Deliveries: ds}}, nil
}
