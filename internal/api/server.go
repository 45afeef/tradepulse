// Package api exposes the HTTP/JSON (Huma, OpenAPI-documented) and WebSocket endpoints.
package api

import (
	"reflect"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	"tradepulse/internal/auth"
	"tradepulse/internal/connector"
	"tradepulse/internal/config"
	"tradepulse/internal/delivery"
	"tradepulse/internal/domain/alert"
	"tradepulse/internal/domain/audit"
	"tradepulse/internal/domain/event"
	"tradepulse/internal/domain/feed"
	"tradepulse/internal/domain/invite"
	"tradepulse/internal/domain/feedback"
	"tradepulse/internal/domain/session"
	"tradepulse/internal/domain/view"
	"tradepulse/internal/domain/sourcedef"
	"tradepulse/internal/domain/user"
	"tradepulse/internal/idgen"
	"tradepulse/internal/metrics"
)

// Personalizer keeps the matching engine in sync with account changes.
type Personalizer interface {
	UpsertUser(id string, p user.Profile)
	RemoveUser(id string)
}

type Deps struct {
	Cfg          config.Config
	Log          *slog.Logger
	Users        user.Repository
	Feeds        feed.Repository
	Invites      invite.Repository
	Audit        audit.Repository
	JWT          *auth.JWT
	Tickets      *auth.TicketStore
	Personalizer Personalizer
	Hub          *delivery.Hub
	Registry     *event.Registry
	Sources      sourcedef.Repository
	Runs         sourcedef.RunRepository
	Bus          event.Bus
	Connectors   *connector.Registry
	Items        sourcedef.ItemStore
	Alerts       AlertEngine
	AlertRules   alert.RuleRepository
	AlertFires   alert.FireRepository
	Channels     alert.ChannelRepository
	Deliveries   alert.DeliveryRepository
	Templates    *alert.Templates
	Outbound     Outbound
	Sessions     session.Repository
	Views        view.Repository
	Feedback     feedback.Repository
	Metrics      *metrics.Registry
	Usage        sourcedef.UsageStore
	// Ready (optional) checks dependencies for /readyz, e.g. the database ping.
	Ready func(ctx context.Context) error
}

type Server struct {
	Deps
	global    *limiter
	authLim   *limiter
	dummyHash string
	throttle  *throttle
	hookMu    sync.Mutex
	hookLims  map[string]*limiter
}

func New(d Deps) (*Server, error) {
	dummy, err := auth.HashPassword(idgen.New())
	if err != nil {
		return nil, err
	}
	return &Server{
		Deps:      d,
		global:    newLimiter(d.Cfg.RateRPS, d.Cfg.RateBurst),
		authLim:   newLimiter(0.2, 5), // ~12 attempts/min per IP, burst 5
		dummyHash: dummy,
		throttle:  newThrottle(10, 15*time.Minute, 15*time.Minute),
	}, nil
}

// Run starts background housekeeping until ctx is cancelled.
func (s *Server) Run(ctx context.Context) {
	go s.global.sweep(ctx)
	go s.authLim.sweep(ctx)
	go s.throttle.sweep(ctx)
	<-ctx.Done()
}

// routes registers every documented operation. To add an endpoint: input/output
// types + handler in handlers.go/admin.go, then register it here WITH a summary,
// description, tag and the error codes it can return (docs_test.go enforces this).
func (s *Server) routes(mux *http.ServeMux) huma.API {
	cfg := huma.DefaultConfig("TradePulse API", "1.0.0-rc.1")
	cfg.Components.Schemas = huma.NewMapRegistry("#/components/schemas/", schemaName)
	cfg.CreateHooks = nil // no "$schema" fields in responses
	cfg.Info.Description = overview
	cfg.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"bearer": {Type: "http", Scheme: "bearer", BearerFormat: "JWT", Description: "Token from /auth/register or /auth/login"},
	}
	if !s.Cfg.DocsEnabled { // no docs routes at all (not merely protected)
		cfg.DocsPath, cfg.OpenAPIPath, cfg.SchemasPath = "", "", ""
	}
	api := humago.New(mux, cfg)
	api.OpenAPI().Tags = []*huma.Tag{
		{Name: "ops", Description: tagOps}, {Name: "auth", Description: tagAuth},
		{Name: "me", Description: tagMe}, {Name: "catalog", Description: tagCatalog},
		{Name: "realtime", Description: tagRealtime}, {Name: "admin", Description: tagAdmin},
		{Name: "alerts", Description: tagAlerts}, {Name: "channels", Description: tagChannels},
	}

	secured := []map[string][]string{{"bearer": {}}}
	member := huma.Middlewares{s.authMiddleware(api, false)}
	admin := huma.Middlewares{s.authMiddleware(api, true)}
	const small = 64 << 10

	huma.Register(api, huma.Operation{OperationID: "health", Method: http.MethodGet, Path: "/healthz", Tags: []string{"ops"},
		Summary: "Liveness probe", Description: "Returns 200 while the process is running."}, s.health)
	huma.Register(api, huma.Operation{OperationID: "ready", Method: http.MethodGet, Path: "/readyz", Tags: []string{"ops"},
		Summary: "Readiness probe", Errors: []int{503},
		Description: "Returns 200 when the service can accept traffic: the database (if configured) answers a ping. 503 otherwise."}, s.ready)

	huma.Register(api, huma.Operation{OperationID: "signup", Method: http.MethodPost, Path: "/api/v1/auth/register", Tags: []string{"auth"},
		Summary: "Create an account with an invite code", DefaultStatus: http.StatusCreated, MaxBodyBytes: small,
		Description: "Registration is **invite-only**. Provide the single-use `invite_code` you received. " +
			"If the invite was bound to an email, you must register with that email. " +
			"Returns a JWT you can use immediately. Failed registrations do not consume the invite.",
		Errors: []int{403, 409, 422, 429}}, s.signup)
	huma.Register(api, huma.Operation{OperationID: "login", Method: http.MethodPost, Path: "/api/v1/auth/login", Tags: []string{"auth"},
		Summary: "Log in and get a JWT", MaxBodyBytes: small,
		Description: "Unknown email and wrong password return the same 401 (no account enumeration). " +
			"A suspended account gets 403. Rate limited per IP.",
		Errors: []int{401, 403, 429}}, s.login)

	huma.Register(api, huma.Operation{OperationID: "get-subscriptions", Method: http.MethodGet, Path: "/api/v1/me/subscriptions", Tags: []string{"me"},
		Summary: "Get my subscriptions and mutes", Security: secured, Middlewares: member,
		Description: "Returns what you are subscribed to. An empty profile means an empty feed.", Errors: []int{401}}, s.getSubscriptions)
	huma.Register(api, huma.Operation{OperationID: "put-subscriptions", Method: http.MethodPut, Path: "/api/v1/me/subscriptions", Tags: []string{"me"},
		Summary: "Replace my subscriptions and mutes", Security: secured, Middlewares: member, MaxBodyBytes: 256 << 10,
		Description: "Replaces your whole list (send the complete desired state). Values are normalized (case/spacing) and validated: " +
			"unknown event types or condition fields are rejected with 422. Limits: 200 subscriptions, 200 mutes, 10 conditions each. " +
			"Changes apply to live delivery immediately. See *Subscriptions* in the overview.", Errors: []int{401, 422}}, s.putSubscriptions)
	huma.Register(api, huma.Operation{OperationID: "get-feed", Method: http.MethodGet, Path: "/api/v1/me/feed", Tags: []string{"me"},
		Summary: "Recent feed items (newest first)", Security: secured, Middlewares: member,
		Description: "Your most recent delivered items, each with a `reason` explaining why it matched. " +
			"Paginated: pass `before=<next_cursor>` to scroll back; after a WebSocket reconnect call with `after=<cursor of the last item you saw>` (oldest first) and repeat until `next_cursor` is absent.", Errors: []int{401}}, s.getFeed)

	huma.Register(api, huma.Operation{OperationID: "list-event-types", Method: http.MethodGet, Path: "/api/v1/event-types", Tags: []string{"catalog"},
		Summary: "Event types and filterable fields", Security: secured, Middlewares: member,
		Description: "Lists every event type with its fields (name and kind: number, string or bool) and the built-in fields " +
			"available on all events. Only these fields can be used in subscription conditions.", Errors: []int{401}}, s.listEventTypes)
	huma.Register(api, huma.Operation{OperationID: "ws-ticket", Method: http.MethodPost, Path: "/api/v1/ws-ticket", Tags: []string{"realtime"},
		Summary: "Get a one-time WebSocket ticket", Security: secured, Middlewares: member,
		Description: "Returns a ticket valid for 30 seconds and usable once. Connect to `/ws?ticket=<ticket>`. " +
			"See *Real-time (WebSocket)* in the overview for the message format and reconnect advice.", Errors: []int{401}}, s.wsTicket)

	huma.Register(api, huma.Operation{OperationID: "create-invite", Method: http.MethodPost, Path: "/api/v1/admin/invites", Tags: []string{"admin"},
		Summary: "Create an invite code", Security: secured, Middlewares: admin, DefaultStatus: http.StatusCreated, MaxBodyBytes: small,
		Description: "Generates a single-use code. **The code is returned only in this response** (only a hash is stored). " +
			"Optionally bind it to an email and set an expiry (default 7 days, max 30).", Errors: []int{401, 403, 422}}, s.createInvite)
	huma.Register(api, huma.Operation{OperationID: "list-invites", Method: http.MethodGet, Path: "/api/v1/admin/invites", Tags: []string{"admin"},
		Summary: "List invites and their status", Security: secured, Middlewares: admin,
		Description: "Status is pending, used, expired or revoked. Codes are never shown.", Errors: []int{401, 403}}, s.listInvites)
	huma.Register(api, huma.Operation{OperationID: "revoke-invite", Method: http.MethodDelete, Path: "/api/v1/admin/invites/{id}", Tags: []string{"admin"},
		Summary: "Revoke a pending invite", Security: secured, Middlewares: admin, DefaultStatus: http.StatusNoContent,
		Description: "Only pending invites can be revoked (409 otherwise).", Errors: []int{401, 403, 404, 409}}, s.revokeInvite)
	huma.Register(api, huma.Operation{OperationID: "list-users", Method: http.MethodGet, Path: "/api/v1/admin/users", Tags: []string{"admin"},
		Summary: "List members", Security: secured, Middlewares: admin,
		Description: "All accounts with role and status. Password hashes are never exposed.", Errors: []int{401, 403}}, s.listUsers)
	huma.Register(api, huma.Operation{OperationID: "suspend-user", Method: http.MethodPost, Path: "/api/v1/admin/users/{id}/suspend", Tags: []string{"admin"},
		Summary: "Suspend a member", Security: secured, Middlewares: admin,
		Description: "Takes effect immediately: API access is rejected, matching stops and live WebSocket connections are closed. " +
			"You cannot suspend yourself.", Errors: []int{401, 403, 404, 409}}, s.suspendUser)
	huma.Register(api, huma.Operation{OperationID: "reactivate-user", Method: http.MethodPost, Path: "/api/v1/admin/users/{id}/reactivate", Tags: []string{"admin"},
		Summary: "Reactivate a member", Security: secured, Middlewares: admin,
		Description: "Restores access and live matching with the member's saved subscriptions.", Errors: []int{401, 403, 404}}, s.reactivateUser)
	huma.Register(api, huma.Operation{OperationID: "list-audit", Method: http.MethodGet, Path: "/api/v1/admin/audit", Tags: []string{"admin"},
		Summary: "Audit log", Security: secured, Middlewares: admin,
		Description: "Newest first, paginated (`before`/`after`/`next_cursor`). Records registrations, invite creation/revocation and suspensions/reactivations (never secrets).", Errors: []int{401, 403}}, s.listAudit)
	huma.Register(api, huma.Operation{OperationID: "list-catalog-sources", Method: http.MethodGet, Path: "/api/v1/sources", Tags: []string{"catalog"},
		Summary: "Sources you can subscribe to", Security: secured, Middlewares: member,
		Description: "Enabled sources with their names and topics (entities). Use a source's `name` in a `source` subscription, " +
			"or one of its `entities` in an `entity` subscription.", Errors: []int{401}}, s.catalogSources)

	huma.Register(api, huma.Operation{OperationID: "create-source", Method: http.MethodPost, Path: "/api/v1/admin/sources", Tags: []string{"admin"},
		Summary: "Add a source", Security: secured, Middlewares: admin, DefaultStatus: http.StatusCreated, MaxBodyBytes: small,
		Description: "Types: `rss` (RSS/Atom feed URL), `webmonitor` (HTML page + CSS selectors; robots.txt respected), `manual` (admin posts), " +
			"`webhook` (external systems push items; the signing secret is returned once), `x` (an X account via the official API; needs X_BEARER_TOKEN on the server, spend-capped). " +
			"`interval_seconds` is per source (bounded by the platform minimum/maximum). The URL must be a public http(s) address (SSRF-checked). " +
			"The name is a permanent slug that members subscribe to. Sources start **disabled**: use *Test a source draft* first, then enable.",
		Errors: []int{401, 403, 409, 422}}, s.createSource)
	huma.Register(api, huma.Operation{OperationID: "list-sources", Method: http.MethodGet, Path: "/api/v1/admin/sources", Tags: []string{"admin"},
		Summary: "List sources with health", Security: secured, Middlewares: admin,
		Description: "Health: ok, pending, degraded, failing, overdue (the scheduler missed its schedule: silence watchdog) or disabled.", Errors: []int{401, 403}}, s.listSources)
	huma.Register(api, huma.Operation{OperationID: "get-source", Method: http.MethodGet, Path: "/api/v1/admin/sources/{id}", Tags: []string{"admin"},
		Summary: "Source detail with recent runs", Security: secured, Middlewares: admin,
		Description: "Includes the last 20 fetch runs (status, items seen/new/published, errors).", Errors: []int{401, 403, 404}}, s.getSource)
	huma.Register(api, huma.Operation{OperationID: "update-source", Method: http.MethodPut, Path: "/api/v1/admin/sources/{id}", Tags: []string{"admin"},
		Summary: "Update a source's settings", Security: secured, Middlewares: admin, MaxBodyBytes: small,
		Description: "Replaces url, enabled, interval_seconds, entities and web settings. Name and type cannot change. Same validation as creation.",
		Errors: []int{401, 403, 404, 422}}, s.updateSource)
	huma.Register(api, huma.Operation{OperationID: "delete-source", Method: http.MethodDelete, Path: "/api/v1/admin/sources/{id}", Tags: []string{"admin"},
		Summary: "Delete a source", Security: secured, Middlewares: admin, DefaultStatus: http.StatusNoContent,
		Description: "Removes the source, its run history and stored items. Members' subscriptions to it simply stop matching.", Errors: []int{401, 403, 404}}, s.deleteSource)
	huma.Register(api, huma.Operation{OperationID: "run-source-now", Method: http.MethodPost, Path: "/api/v1/admin/sources/{id}/run", Tags: []string{"admin"},
		Summary: "Fetch a source as soon as possible", Security: secured, Middlewares: admin,
		Description: "Schedules the next fetch immediately (the scheduler picks it up within seconds). Only for enabled rss/webmonitor sources.",
		Errors: []int{401, 403, 404, 409}}, s.runSourceNow)
	huma.Register(api, huma.Operation{OperationID: "test-source", Method: http.MethodPost, Path: "/api/v1/admin/sources/test", Tags: []string{"admin"},
		Summary: "Test a source draft without saving", Security: secured, Middlewares: admin, MaxBodyBytes: small,
		Description: "Fetches the URL with the given settings and returns the first items it would extract (nothing is stored or published). " +
			"Use it to tune CSS selectors. Fetch problems are reported in the response (`ok: false`), invalid settings return 422.",
		Errors: []int{401, 403, 422}}, s.testSource)
	huma.Register(api, huma.Operation{OperationID: "post-item", Method: http.MethodPost, Path: "/api/v1/admin/items", Tags: []string{"admin"},
		Summary: "Publish a manual item", Security: secured, Middlewares: admin, DefaultStatus: http.StatusCreated, MaxBodyBytes: small,
		Description: "Publishes one item through an enabled `manual` source; matching members receive it live. Useful for announcements and corrections.",
		Errors: []int{401, 403, 404, 409, 422}}, s.postItem)
	huma.Register(api, huma.Operation{OperationID: "rotate-webhook-secret", Method: http.MethodPost, Path: "/api/v1/admin/sources/{id}/webhook-secret", Tags: []string{"admin"},
		Summary: "Rotate a webhook source's secret", Security: secured, Middlewares: admin,
		Description: "Generates a new secret and returns it **once**; the old secret stops working immediately. Only for `webhook` sources.",
		Errors: []int{401, 403, 404, 409, 422}}, s.rotateWebhookSecret)
	huma.Register(api, huma.Operation{OperationID: "get-usage", Method: http.MethodGet, Path: "/api/v1/admin/usage", Tags: []string{"admin"},
		Summary: "Paid API usage this month", Security: secured, Middlewares: admin,
		Description: "X API spend for the current month against the configured cap (X_MONTHLY_BUDGET_USD). Polling stops automatically when the cap is reached.",
		Errors: []int{401, 403}}, s.getUsage)
	huma.Register(api, huma.Operation{OperationID: "list-alert-templates", Method: http.MethodGet, Path: "/api/v1/alert-templates", Tags: []string{"alerts"},
		Summary: "Alert templates", Security: secured, Middlewares: member,
		Description: "Shipped, pre-filled rules. Use one with *Create an alert from a template* (you only supply the values to watch), or copy its settings into a custom rule.", Errors: []int{401}}, s.listTemplates)
	huma.Register(api, huma.Operation{OperationID: "list-alerts", Method: http.MethodGet, Path: "/api/v1/me/alerts", Tags: []string{"alerts"},
		Summary: "My alert rules", Security: secured, Middlewares: member,
		Description: "Includes `paused_reason` when the system auto-paused a rule that fired too often.", Errors: []int{401}}, s.listRules)
	huma.Register(api, huma.Operation{OperationID: "create-alert", Method: http.MethodPost, Path: "/api/v1/me/alerts", Tags: []string{"alerts"},
		Summary: "Create an alert rule", Security: secured, Middlewares: member, DefaultStatus: http.StatusCreated, MaxBodyBytes: 64 << 10,
		Description: "A rule = **scope** (entities, keywords or sources) + optional **types** and **conditions** (same fields as subscription conditions, see `GET /event-types`) + behavior " +
			"(`cooldown_seconds`, default 300, minimum 10; `severity`; `channels`: `in_app` and/or your channel ids). At most 50 rules. A rule that fires more than the hourly cap is paused automatically. " +
			"Try it first with *Preview an alert rule*.", Errors: []int{401, 409, 422}}, s.createRule)
	huma.Register(api, huma.Operation{OperationID: "create-alert-from-template", Method: http.MethodPost, Path: "/api/v1/me/alerts/from-template", Tags: []string{"alerts"},
		Summary: "Create an alert from a template", Security: secured, Middlewares: member, DefaultStatus: http.StatusCreated, MaxBodyBytes: 64 << 10,
		Description: "Pick a template id from `GET /alert-templates` and give the values to watch; everything else comes pre-filled. Edit the rule afterwards to tweak it.", Errors: []int{401, 404, 409, 422}}, s.createFromTemplate)
	huma.Register(api, huma.Operation{OperationID: "preview-alert", Method: http.MethodPost, Path: "/api/v1/me/alerts/preview", Tags: []string{"alerts"},
		Summary: "Preview an alert rule", Security: secured, Middlewares: member, MaxBodyBytes: 64 << 10,
		Description: "Replays recently seen events (since the server started, last 24 hours, up to 1000) against the draft and reports how many alerts it would have produced, with examples, " +
			"after applying cooldown. Nothing is saved or sent. Use it to avoid noisy rules.", Errors: []int{401, 422}}, s.previewRule)
	huma.Register(api, huma.Operation{OperationID: "list-alert-fires", Method: http.MethodGet, Path: "/api/v1/me/alerts/fires", Tags: []string{"alerts"},
		Summary: "Recent alerts that fired", Security: secured, Middlewares: member,
		Description: "Newest first, each with a `reason` explaining why it fired. Paginated like the feed (`before`/`after`/`next_cursor`).", Errors: []int{401}}, s.listFires)
	huma.Register(api, huma.Operation{OperationID: "get-alert", Method: http.MethodGet, Path: "/api/v1/me/alerts/{id}", Tags: []string{"alerts"},
		Summary: "Get an alert rule", Security: secured, Middlewares: member, Description: "Only your own rules are visible (404 otherwise).", Errors: []int{401, 404}}, s.getRule)
	huma.Register(api, huma.Operation{OperationID: "update-alert", Method: http.MethodPut, Path: "/api/v1/me/alerts/{id}", Tags: []string{"alerts"},
		Summary: "Replace an alert rule", Security: secured, Middlewares: member, MaxBodyBytes: 64 << 10,
		Description: "Send the full rule. Saving with `enabled: true` re-enables a paused rule and clears its paused reason.", Errors: []int{401, 404, 422}}, s.updateRule)
	huma.Register(api, huma.Operation{OperationID: "delete-alert", Method: http.MethodDelete, Path: "/api/v1/me/alerts/{id}", Tags: []string{"alerts"},
		Summary: "Delete an alert rule", Security: secured, Middlewares: member, DefaultStatus: http.StatusNoContent, Description: "Deletes the rule and stops it immediately.", Errors: []int{401, 404}}, s.deleteRule)

	huma.Register(api, huma.Operation{OperationID: "list-channels", Method: http.MethodGet, Path: "/api/v1/me/channels", Tags: []string{"channels"},
		Summary: "My notification channels", Security: secured, Middlewares: member,
		Description: "`in_app` (live WebSocket) is built in and always available. This lists your outbound webhook channels with their health. Secrets are never shown here.", Errors: []int{401}}, s.listChannels)
	huma.Register(api, huma.Operation{OperationID: "create-channel", Method: http.MethodPost, Path: "/api/v1/me/channels", Tags: []string{"channels"},
		Summary: "Add an outbound webhook channel", Security: secured, Middlewares: member, DefaultStatus: http.StatusCreated, MaxBodyBytes: 64 << 10,
		Description: "Alerts are POSTed to your HTTPS URL as JSON, signed like inbound webhooks (`X-TP-Timestamp`, `X-TP-Signature: sha256=HMAC(secret, \"<timestamp>.<body>\")`). " +
			"The secret is returned **once**. Failed deliveries are retried with back-off; after 10 consecutive failures the channel is disabled until you re-enable it. At most 5 channels.", Errors: []int{401, 409, 422}}, s.createChannel)
	huma.Register(api, huma.Operation{OperationID: "list-deliveries", Method: http.MethodGet, Path: "/api/v1/me/channels/deliveries", Tags: []string{"channels"},
		Summary: "Recent outbound deliveries", Security: secured, Middlewares: member,
		Description: "Delivery history (status, attempts, error) for your channels, newest first. Paginated (`before`/`after`/`next_cursor`).", Errors: []int{401}}, s.listDeliveries)
	huma.Register(api, huma.Operation{OperationID: "delete-channel", Method: http.MethodDelete, Path: "/api/v1/me/channels/{id}", Tags: []string{"channels"},
		Summary: "Delete a channel", Security: secured, Middlewares: member, DefaultStatus: http.StatusNoContent,
		Description: "Also removes it from your alert rules (rules left without a channel fall back to `in_app`).", Errors: []int{401, 404}}, s.deleteChannel)
	huma.Register(api, huma.Operation{OperationID: "rotate-channel-secret", Method: http.MethodPost, Path: "/api/v1/me/channels/{id}/rotate-secret", Tags: []string{"channels"},
		Summary: "Rotate a channel's signing secret", Security: secured, Middlewares: member,
		Description: "Returns the new secret **once**; the old one stops being used immediately.", Errors: []int{401, 404, 422}}, s.rotateChannelSecret)
	huma.Register(api, huma.Operation{OperationID: "enable-channel", Method: http.MethodPost, Path: "/api/v1/me/channels/{id}/enable", Tags: []string{"channels"},
		Summary: "Re-enable a channel", Security: secured, Middlewares: member,
		Description: "Re-enables a channel that was disabled after repeated failures and resets its failure counter.", Errors: []int{401, 404}}, s.enableChannel)
	huma.Register(api, huma.Operation{OperationID: "test-channel", Method: http.MethodPost, Path: "/api/v1/me/channels/{id}/test", Tags: []string{"channels"},
		Summary: "Send a test delivery", Security: secured, Middlewares: member,
		Description: "Queues a clearly marked test alert (`\"test\": true`) to the channel. Check `GET /me/channels/deliveries` for the result.", Errors: []int{401, 404, 409}}, s.testChannel)
	huma.Register(api, huma.Operation{OperationID: "refresh", Method: http.MethodPost, Path: "/api/v1/auth/refresh", Tags: []string{"auth"},
		Summary: "Exchange a refresh token for a new token pair", MaxBodyBytes: small,
		Description: "Access tokens are short-lived (see `expires_in`). Send the refresh token you received at login to get a new access token **and a new refresh token**; the old refresh token stops working. " +
			"Presenting an already-used refresh token is treated as theft: the whole session is revoked and you must log in again. Refresh tokens last 30 days by default.", Errors: []int{401, 429}}, s.refresh)
	huma.Register(api, huma.Operation{OperationID: "logout", Method: http.MethodPost, Path: "/api/v1/auth/logout", Tags: []string{"auth"},
		Summary: "Log out (revoke this session)", MaxBodyBytes: small, DefaultStatus: http.StatusNoContent,
		Description: "Revokes the session the refresh token belongs to. Always answers 204, whether or not the token was valid.", Errors: []int{429}}, s.logout)
	huma.Register(api, huma.Operation{OperationID: "logout-all", Method: http.MethodPost, Path: "/api/v1/me/logout-all", Tags: []string{"me"},
		Summary: "Log out everywhere", Security: secured, Middlewares: member, DefaultStatus: http.StatusNoContent,
		Description: "Revokes every refresh token and invalidates every access token issued before now (including the one you used).", Errors: []int{401}}, s.logoutAll)
	huma.Register(api, huma.Operation{OperationID: "change-password", Method: http.MethodPost, Path: "/api/v1/me/password", Tags: []string{"me"},
		Summary: "Change my password", Security: secured, Middlewares: member, MaxBodyBytes: small,
		Description: "Requires the current password. All other sessions are signed out; the response contains a fresh token pair for this one. Repeated wrong current passwords are throttled.",
		Errors: []int{401, 403, 422, 429}}, s.changePassword)
	huma.Register(api, huma.Operation{OperationID: "reset-user-password", Method: http.MethodPost, Path: "/api/v1/admin/users/{id}/reset-password", Tags: []string{"admin"},
		Summary: "Reset a member's password", Security: secured, Middlewares: admin,
		Description: "Generates a temporary password, **shown once** in the response; the member's sessions are signed out immediately. Hand it over securely and ask the member to change it (`POST /me/password`).", Errors: []int{401, 403, 404}}, s.resetPassword)
	huma.Register(api, huma.Operation{OperationID: "get-me", Method: http.MethodGet, Path: "/api/v1/me", Tags: []string{"me"},
		Summary: "Who am I", Security: secured, Middlewares: member,
		Description: "Your id, email and role (`member` or `admin`). Clients use the role to show or hide the operator console; the server enforces it regardless.", Errors: []int{401}}, s.getMe)
	huma.Register(api, huma.Operation{OperationID: "list-views", Method: http.MethodGet, Path: "/api/v1/me/views", Tags: []string{"me"},
		Summary: "My saved views", Security: secured, Middlewares: member,
		Description: "Saved views are named filters over your feed (sources, topics, keywords, event types, time window). Use one with `GET /me/feed?view=<id>`. At most 20; returned whole.", Errors: []int{401}}, s.listViews)
	huma.Register(api, huma.Operation{OperationID: "create-view", Method: http.MethodPost, Path: "/api/v1/me/views", Tags: []string{"me"},
		Summary: "Save a view", Security: secured, Middlewares: member, DefaultStatus: http.StatusCreated, MaxBodyBytes: 64 << 10,
		Description: "All given criteria must match; within one list any value matches. Names are unique per member (case-insensitive). A view only narrows what is already in your feed; it does not change what you receive.", Errors: []int{401, 409, 422}}, s.createView)
	huma.Register(api, huma.Operation{OperationID: "update-view", Method: http.MethodPut, Path: "/api/v1/me/views/{id}", Tags: []string{"me"},
		Summary: "Replace a saved view", Security: secured, Middlewares: member, MaxBodyBytes: 64 << 10,
		Description: "Send the full view. Only your own views are accessible (404 otherwise).", Errors: []int{401, 404, 409, 422}}, s.updateView)
	huma.Register(api, huma.Operation{OperationID: "delete-view", Method: http.MethodDelete, Path: "/api/v1/me/views/{id}", Tags: []string{"me"},
		Summary: "Delete a saved view", Security: secured, Middlewares: member, DefaultStatus: http.StatusNoContent, Description: "Deletes the view only, never feed items.", Errors: []int{401, 404}}, s.deleteView)
	huma.Register(api, huma.Operation{OperationID: "rate-feed-item", Method: http.MethodPost, Path: "/api/v1/me/feed/{id}/feedback", Tags: []string{"me"},
		Summary: "Rate a feed item (useful / not useful)", Security: secured, Middlewares: member, MaxBodyBytes: 16 << 10,
		Description: "Idempotent: rating again replaces your earlier rating. Only your own feed items can be rated (404 otherwise). Ratings measure how relevant sources are; the rating is also returned as `feedback` when listing the feed.", Errors: []int{401, 404, 422}}, s.rateFeedItem)
	huma.Register(api, huma.Operation{OperationID: "clear-feed-item-feedback", Method: http.MethodDelete, Path: "/api/v1/me/feed/{id}/feedback", Tags: []string{"me"},
		Summary: "Remove my rating of a feed item", Security: secured, Middlewares: member, DefaultStatus: http.StatusNoContent, Description: "Always 204, also when there was no rating.", Errors: []int{401}}, s.clearFeedItemFeedback)
	huma.Register(api, huma.Operation{OperationID: "rate-alert", Method: http.MethodPost, Path: "/api/v1/me/alerts/fires/{id}/feedback", Tags: []string{"alerts"},
		Summary: "Rate an alert that fired (useful / not useful)", Security: secured, Middlewares: member, MaxBodyBytes: 16 << 10,
		Description: "Idempotent. This is how alert precision is measured, so rate the alerts that were noise. Only your own alerts (404 otherwise).", Errors: []int{401, 404, 422}}, s.rateAlertFire)
	huma.Register(api, huma.Operation{OperationID: "clear-alert-feedback", Method: http.MethodDelete, Path: "/api/v1/me/alerts/fires/{id}/feedback", Tags: []string{"alerts"},
		Summary: "Remove my rating of an alert", Security: secured, Middlewares: member, DefaultStatus: http.StatusNoContent, Description: "Always 204, also when there was no rating. Only affects your own rating.", Errors: []int{401}}, s.clearAlertFireFeedback)
	huma.Register(api, huma.Operation{OperationID: "feedback-summary", Method: http.MethodGet, Path: "/api/v1/admin/feedback/summary", Tags: []string{"admin"},
		Summary: "Feedback and alert precision", Security: secured, Middlewares: admin,
		Description: "Totals of useful / not useful ratings for feed items and alerts in the last `days`, the precision (useful share), and the most-rated sources and alert rules. Contains no member identities.", Errors: []int{401, 403}}, s.feedbackSummary)
	return api
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	s.routes(mux)
	mux.HandleFunc("GET /metrics", s.metricsHandler) // Prometheus text; 404 unless METRICS_TOKEN is set
	mux.HandleFunc("/", s.notFound) // unknown paths/methods get the same JSON error shape as everything else
	mux.HandleFunc("GET /ws", s.ws)                            // WebSocket: plain handler, outside OpenAPI
	mux.HandleFunc("POST /api/v1/hooks/{name}", s.hook)       // inbound webhook: signs the raw body, documented in the overview

	var h http.Handler = mux
	h = bodyLimit(h)
	h = s.rateLimit(h)
	h = s.cors(h)
	h = s.securityHeaders(h)
	h = s.logging(h)
	h = s.recoverer(h)
	h = requestID(h)
	return h
}

// OpenAPISpec renders the spec without any dependencies (used by cmd/openapi and CI).
func OpenAPISpec() ([]byte, error) {
	api := (&Server{}).routes(http.NewServeMux())
	return json.MarshalIndent(api.OpenAPI(), "", "  ")
}

func (s *Server) notFound(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotFound, "not_found", "no such endpoint")
}

// Huma names OpenAPI schemas after the bare Go type name and panics when two different types share
// one. Types that would collide get an explicit, descriptive schema name here.
var schemaNames = map[string]string{
	"tradepulse/internal/domain/alert.Spec": "RuleSpec",
	"tradepulse/internal/domain/view.Spec":  "ViewSpec",
}

func schemaName(t reflect.Type, hint string) string {
	if n, ok := schemaNames[t.PkgPath()+"."+t.Name()]; ok {
		return n
	}
	return huma.DefaultSchemaNamer(t, hint)
}
