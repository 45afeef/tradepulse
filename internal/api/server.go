// Package api exposes the HTTP/JSON (Huma, OpenAPI-documented) and WebSocket endpoints.
package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	"tradepulse/internal/auth"
	"tradepulse/internal/config"
	"tradepulse/internal/delivery"
	"tradepulse/internal/domain/audit"
	"tradepulse/internal/domain/event"
	"tradepulse/internal/domain/feed"
	"tradepulse/internal/domain/invite"
	"tradepulse/internal/domain/user"
	"tradepulse/internal/idgen"
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
	// Ready (optional) checks dependencies for /readyz, e.g. the database ping.
	Ready func(ctx context.Context) error
}

type Server struct {
	Deps
	global    *limiter
	authLim   *limiter
	dummyHash string
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
	}, nil
}

// Run starts background housekeeping until ctx is cancelled.
func (s *Server) Run(ctx context.Context) {
	go s.global.sweep(ctx)
	go s.authLim.sweep(ctx)
	<-ctx.Done()
}

// routes registers every documented operation. To add an endpoint: input/output
// types + handler in handlers.go/admin.go, then register it here WITH a summary,
// description, tag and the error codes it can return (docs_test.go enforces this).
func (s *Server) routes(mux *http.ServeMux) huma.API {
	cfg := huma.DefaultConfig("TradePulse API", "0.3.0")
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
			"Use after a WebSocket reconnect to catch up on anything missed.", Errors: []int{401}}, s.getFeed)

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
		Description: "Newest first. Records registrations, invite creation/revocation and suspensions/reactivations (never secrets).", Errors: []int{401, 403}}, s.listAudit)
	return api
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	s.routes(mux)
	mux.HandleFunc("GET /ws", s.ws) // WebSocket: plain handler, outside OpenAPI

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
