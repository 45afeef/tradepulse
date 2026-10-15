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
	"tradepulse/internal/domain/feed"
	"tradepulse/internal/domain/user"
	"tradepulse/internal/idgen"
)

// Personalizer is notified when a user's profile changes.
type Personalizer interface {
	UpsertUser(id string, p user.Profile)
}

type Deps struct {
	Cfg          config.Config
	Log          *slog.Logger
	Users        user.Repository
	Feeds        feed.Repository
	JWT          *auth.JWT
	Tickets      *auth.TicketStore
	Personalizer Personalizer
	Hub          *delivery.Hub
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

// routes registers every documented operation. To add an endpoint: add input/
// output types + a handler in handlers.go, then register it here.
func (s *Server) routes(mux *http.ServeMux) huma.API {
	cfg := huma.DefaultConfig("TradePulse API", "1.0.0")
	cfg.CreateHooks = nil // no "$schema" fields in responses
	cfg.Info.Description = "Personalized real-time information API. Real-time feed: WebSocket `GET /ws?ticket=` (not representable in OpenAPI; see README)."
	cfg.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"bearer": {Type: "http", Scheme: "bearer", BearerFormat: "JWT"},
	}
	if !s.Cfg.DocsEnabled { // no docs routes at all (not merely protected)
		cfg.DocsPath, cfg.OpenAPIPath, cfg.SchemasPath = "", "", ""
	}
	api := humago.New(mux, cfg)

	secured := []map[string][]string{{"bearer": {}}}
	needAuth := huma.Middlewares{s.authMiddleware(api)}
	const small = 64 << 10

	huma.Register(api, huma.Operation{OperationID: "health", Method: http.MethodGet, Path: "/healthz",
		Summary: "Liveness probe", Tags: []string{"ops"}}, s.health)
	huma.Register(api, huma.Operation{OperationID: "ready", Method: http.MethodGet, Path: "/readyz",
		Summary: "Readiness probe", Tags: []string{"ops"}}, s.health)

	huma.Register(api, huma.Operation{OperationID: "signup", Method: http.MethodPost, Path: "/api/v1/auth/register",
		Summary: "Create an account", Tags: []string{"auth"}, DefaultStatus: http.StatusCreated, MaxBodyBytes: small}, s.signup)
	huma.Register(api, huma.Operation{OperationID: "login", Method: http.MethodPost, Path: "/api/v1/auth/login",
		Summary: "Log in and get a JWT", Tags: []string{"auth"}, MaxBodyBytes: small}, s.login)

	huma.Register(api, huma.Operation{OperationID: "get-profile", Method: http.MethodGet, Path: "/api/v1/me/profile",
		Summary: "Get my personalization profile", Tags: []string{"me"}, Security: secured, Middlewares: needAuth}, s.getProfile)
	huma.Register(api, huma.Operation{OperationID: "put-profile", Method: http.MethodPut, Path: "/api/v1/me/profile",
		Summary: "Replace my personalization profile", Tags: []string{"me"}, Security: secured, Middlewares: needAuth, MaxBodyBytes: small}, s.putProfile)
	huma.Register(api, huma.Operation{OperationID: "get-feed", Method: http.MethodGet, Path: "/api/v1/me/feed",
		Summary: "Recent feed items (newest first)", Tags: []string{"me"}, Security: secured, Middlewares: needAuth}, s.getFeed)
	huma.Register(api, huma.Operation{OperationID: "ws-ticket", Method: http.MethodPost, Path: "/api/v1/ws-ticket",
		Summary: "Get a one-time, 30s ticket for GET /ws?ticket=", Tags: []string{"me"}, Security: secured, Middlewares: needAuth}, s.wsTicket)
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
