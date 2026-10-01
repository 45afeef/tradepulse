// Package api exposes the HTTP/JSON and WebSocket endpoints.
package api

import (
	"context"
	"log/slog"
	"net/http"

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

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, map[string]string{"status": "ready"}) })

	mux.Handle("POST /api/v1/auth/register", s.authLimit(http.HandlerFunc(s.register)))
	mux.Handle("POST /api/v1/auth/login", s.authLimit(http.HandlerFunc(s.login)))

	mux.Handle("GET /api/v1/me/profile", s.requireAuth(s.getProfile))
	mux.Handle("PUT /api/v1/me/profile", s.requireAuth(s.putProfile))
	mux.Handle("GET /api/v1/me/feed", s.requireAuth(s.getFeed))
	mux.Handle("POST /api/v1/ws-ticket", s.requireAuth(s.wsTicket))
	mux.HandleFunc("GET /ws", s.ws)

	var h http.Handler = mux
	h = s.rateLimit(h)
	h = s.cors(h)
	h = s.securityHeaders(h)
	h = s.logging(h)
	h = s.recoverer(h)
	h = requestID(h)
	return h
}
