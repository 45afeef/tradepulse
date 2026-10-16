package api

import (
	"context"
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"tradepulse/internal/auth"
	"tradepulse/internal/domain/event"
	"tradepulse/internal/domain/feed"
	"tradepulse/internal/domain/user"
	"tradepulse/internal/idgen"
)

func userID(ctx context.Context) string { id, _ := ctx.Value(userKey).(string); return id }

// ---- types (these ARE the API docs: tags drive validation + OpenAPI) ----

type statusBody struct {
	Status string `json:"status" example:"ok"`
}
type statusOut struct{ Body statusBody }

type credentialsBody struct {
	Email    string `json:"email" maxLength:"254" example:"trader@example.com"`
	Password string `json:"password" minLength:"10" maxLength:"128" example:"a-long-password"`
}
type credentialsIn struct{ Body credentialsBody }

type publicUser struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}
type tokenBody struct {
	Token string     `json:"token" doc:"JWT; send as 'Authorization: Bearer <token>'"`
	User  publicUser `json:"user"`
}
type tokenOut struct{ Body tokenBody }

type subscriptionsIn struct{ Body user.Profile }
type subscriptionsOut struct{ Body user.Profile }

type eventTypesBody struct {
	Types         []event.TypeDef  `json:"types"`
	BuiltinFields []event.FieldDef `json:"builtin_fields" doc:"Available on every event; usable in conditions"`
}
type eventTypesOut struct{ Body eventTypesBody }

type feedIn struct {
	Limit int `query:"limit" default:"50" minimum:"1" maximum:"200" doc:"Max items, newest first"`
}
type feedBody struct {
	Items []feed.FeedItem `json:"items"`
}
type feedOut struct{ Body feedBody }

type ticketBody struct {
	Ticket    string `json:"ticket" doc:"Single-use; connect to /ws?ticket=<ticket> within expires_in seconds"`
	ExpiresIn int    `json:"expires_in" example:"30"`
}
type ticketOut struct{ Body ticketBody }

// ---- handlers ----

func (s *Server) health(_ context.Context, _ *struct{}) (*statusOut, error) {
	return &statusOut{Body: statusBody{Status: "ok"}}, nil
}

func (s *Server) tokenFor(u user.User) (*tokenOut, error) {
	tok, err := s.JWT.Issue(u.ID)
	if err != nil {
		return nil, huma.Error500InternalServerError("could not issue token")
	}
	return &tokenOut{Body: tokenBody{Token: tok, User: publicUser{ID: u.ID, Email: u.Email, CreatedAt: u.CreatedAt}}}, nil
}

func (s *Server) signup(ctx context.Context, in *credentialsIn) (*tokenOut, error) {
	raw := strings.TrimSpace(in.Body.Email)
	addr, err := mail.ParseAddress(raw)
	if err != nil || addr.Address != raw || len(raw) > 254 {
		return nil, huma.Error422UnprocessableEntity("a valid email is required")
	}
	hash, err := auth.HashPassword(in.Body.Password)
	if err != nil {
		return nil, huma.Error500InternalServerError("could not process password")
	}
	u := user.User{
		ID: idgen.New(), Email: strings.ToLower(raw), PasswordHash: hash,
		CreatedAt: time.Now().UTC(), Profile: user.DefaultProfile(),
	}
	if err := s.Users.Create(ctx, u); err != nil {
		if errors.Is(err, user.ErrEmailTaken) {
			return nil, huma.Error409Conflict("email already registered")
		}
		s.Log.Error("create user", "err", err)
		return nil, huma.Error500InternalServerError("internal server error")
	}
	return s.tokenFor(u)
}

func (s *Server) login(ctx context.Context, in *credentialsIn) (*tokenOut, error) {
	u, err := s.Users.GetByEmail(ctx, strings.TrimSpace(in.Body.Email))
	hash := u.PasswordHash
	if err != nil {
		hash = s.dummyHash // equalise timing so unknown emails are indistinguishable
	}
	if !auth.VerifyPassword(in.Body.Password, hash) || err != nil {
		return nil, huma.Error401Unauthorized("invalid email or password")
	}
	return s.tokenFor(u)
}

func (s *Server) getSubscriptions(ctx context.Context, _ *struct{}) (*subscriptionsOut, error) {
	u, err := s.Users.GetByID(ctx, userID(ctx))
	if err != nil {
		return nil, huma.Error401Unauthorized("unknown user")
	}
	return &subscriptionsOut{Body: u.Profile}, nil
}

func (s *Server) putSubscriptions(ctx context.Context, in *subscriptionsIn) (*subscriptionsOut, error) {
	p, err := in.Body.Normalize(s.Registry)
	if err != nil {
		return nil, huma.Error422UnprocessableEntity(err.Error())
	}
	id := userID(ctx)
	if err := s.Users.UpdateProfile(ctx, id, p); err != nil {
		return nil, huma.Error401Unauthorized("unknown user")
	}
	s.Personalizer.UpsertUser(id, p)
	return &subscriptionsOut{Body: p}, nil
}

func (s *Server) listEventTypes(_ context.Context, _ *struct{}) (*eventTypesOut, error) {
	return &eventTypesOut{Body: eventTypesBody{Types: s.Registry.List(), BuiltinFields: event.BuiltinFieldDefs()}}, nil
}

func (s *Server) getFeed(ctx context.Context, in *feedIn) (*feedOut, error) {
	items, err := s.Feeds.Recent(ctx, userID(ctx), in.Limit)
	if err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	return &feedOut{Body: feedBody{Items: items}}, nil
}

func (s *Server) wsTicket(ctx context.Context, _ *struct{}) (*ticketOut, error) {
	t, err := s.Tickets.Issue(userID(ctx))
	if err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	return &ticketOut{Body: ticketBody{Ticket: t, ExpiresIn: 30}}, nil
}

// ws is a plain handler: WebSocket is not expressible in OpenAPI.
func (s *Server) ws(w http.ResponseWriter, r *http.Request) {
	uid, ok := s.Tickets.Redeem(r.URL.Query().Get("ticket"))
	if !ok {
		http.Error(w, "invalid or expired ticket", http.StatusUnauthorized)
		return
	}
	s.Hub.Upgrade(w, r, uid)
}
