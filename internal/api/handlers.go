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
	"tradepulse/internal/domain/invite"
	"tradepulse/internal/domain/session"
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

type registerBody struct {
	Email      string `json:"email" maxLength:"254" example:"member@example.com"`
	Password   string `json:"password" minLength:"10" maxLength:"128" example:"a-long-password"`
	InviteCode string `json:"invite_code" minLength:"10" maxLength:"128" example:"tp_inv_AbCdEf0123456789" doc:"Single-use code from an administrator"`
}
type registerIn struct{ Body registerBody }

type publicUser struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}
type tokenBody struct {
	Token        string     `json:"token" doc:"Short-lived access token (JWT); send as 'Authorization: Bearer <token>'"`
	RefreshToken string     `json:"refresh_token" doc:"Single-use; exchange it at POST /auth/refresh for a new pair before the access token expires"`
	ExpiresIn    int        `json:"expires_in" doc:"Access token lifetime in seconds"`
	User         publicUser `json:"user"`
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

func (s *Server) ready(ctx context.Context, _ *struct{}) (*statusOut, error) {
	if s.Ready != nil {
		cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		if err := s.Ready(cctx); err != nil {
			s.Log.Warn("readiness check failed", "err", err)
			return nil, huma.Error503ServiceUnavailable("a dependency is unavailable") // no driver details to clients
		}
	}
	return &statusOut{Body: statusBody{Status: "ok"}}, nil
}

func (s *Server) tokenFor(ctx context.Context, u user.User) (*tokenOut, error) {
	return s.issueSession(ctx, u, idgen.New())
}

// issueSession mints an access token plus a new refresh token in the given family.
func (s *Server) issueSession(ctx context.Context, u user.User, familyID string) (*tokenOut, error) {
	tok, err := s.JWT.Issue(u.ID)
	if err != nil {
		return nil, huma.Error500InternalServerError("could not issue token")
	}
	ttl := s.Cfg.RefreshTTL
	if ttl <= 0 {
		ttl = 720 * time.Hour
	}
	rt, hash, err := session.New()
	if err != nil {
		return nil, huma.Error500InternalServerError("could not issue token")
	}
	now := time.Now().UTC()
	if err := s.Sessions.Create(ctx, session.Token{ID: idgen.New(), UserID: u.ID, FamilyID: familyID, Hash: hash, CreatedAt: now, ExpiresAt: now.Add(ttl)}); err != nil {
		s.Log.Error("store refresh token", "err", err)
		return nil, huma.Error500InternalServerError("internal server error")
	}
	return &tokenOut{Body: tokenBody{Token: tok, RefreshToken: rt, ExpiresIn: int(s.JWT.TTL().Seconds()),
		User: publicUser{ID: u.ID, Email: u.Email, CreatedAt: u.CreatedAt}}}, nil
}

func (s *Server) signup(ctx context.Context, in *registerIn) (*tokenOut, error) {
	raw := strings.TrimSpace(in.Body.Email)
	addr, err := mail.ParseAddress(raw)
	if err != nil || addr.Address != raw || len(raw) > 254 {
		return nil, huma.Error422UnprocessableEntity("a valid email is required")
	}
	email := strings.ToLower(raw)
	uid := idgen.New()
	// Redeem first (cheap) so invalid codes cannot make us burn CPU hashing passwords.
	inv, err := s.Invites.Redeem(ctx, invite.HashCode(strings.TrimSpace(in.Body.InviteCode)), email, uid, time.Now().UTC())
	if err != nil {
		if !errors.Is(err, invite.ErrInvalid) {
			s.Log.Error("redeem invite", "err", err)
			return nil, huma.Error500InternalServerError("internal server error")
		}
		return nil, huma.Error403Forbidden("invalid or expired invite code")
	}
	release := func() { _ = s.Invites.Release(ctx, inv.ID, uid) }
	hash, err := auth.HashPassword(in.Body.Password)
	if err != nil {
		release()
		return nil, huma.Error500InternalServerError("could not process password")
	}
	u := user.User{
		ID: uid, Email: email, PasswordHash: hash, CreatedAt: time.Now().UTC(),
		Profile: user.DefaultProfile(), Role: user.RoleMember, Status: user.StatusActive,
	}
	if err := s.Users.Create(ctx, u); err != nil {
		release() // do not burn the invite when registration fails
		if errors.Is(err, user.ErrEmailTaken) {
			return nil, huma.Error409Conflict("email already registered")
		}
		s.Log.Error("create user", "err", err)
		return nil, huma.Error500InternalServerError("internal server error")
	}
	s.record(ctx, uid, "user.register", "user", uid, "invite "+inv.ID)
	return s.tokenFor(ctx, u)
}

func (s *Server) login(ctx context.Context, in *credentialsIn) (*tokenOut, error) {
	key := strings.ToLower(strings.TrimSpace(in.Body.Email))
	if s.throttle.blocked(key) > 0 { // per-account lockout, identical for unknown and known emails
		s.Metrics.Inc("auth_lockouts_total")
		return nil, huma.Error429TooManyRequests("too many failed attempts for this account; try again later")
	}
	u, err := s.Users.GetByEmail(ctx, strings.TrimSpace(in.Body.Email))
	hash := u.PasswordHash
	if err != nil {
		hash = s.dummyHash // equalise timing so unknown emails are indistinguishable
	}
	if !auth.VerifyPassword(in.Body.Password, hash) || err != nil {
		s.throttle.fail(key)
		s.Metrics.Inc("auth_login_failures_total")
		return nil, huma.Error401Unauthorized("invalid email or password")
	}
	s.throttle.reset(key)
	if u.Status != user.StatusActive {
		return nil, huma.Error403Forbidden("account suspended")
	}
	return s.tokenFor(ctx, u)
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
	if u, err := s.Users.GetByID(r.Context(), uid); err != nil || u.Status != user.StatusActive {
		http.Error(w, "invalid or expired ticket", http.StatusUnauthorized)
		return
	}
	s.Hub.Upgrade(w, r, uid)
}
