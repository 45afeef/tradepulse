package api

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"tradepulse/internal/auth"
	"tradepulse/internal/domain/session"
	"tradepulse/internal/domain/user"
	"tradepulse/internal/metrics"
)

type meOut struct{ Body publicUser }

func (s *Server) getMe(ctx context.Context, _ *struct{}) (*meOut, error) {
	u, err := s.Users.GetByID(ctx, userID(ctx))
	if err != nil {
		return nil, huma.Error401Unauthorized("unknown user")
	}
	return &meOut{Body: publicUser{ID: u.ID, Email: u.Email, Role: u.Role, CreatedAt: u.CreatedAt}}, nil
}

type refreshBody struct {
	RefreshToken string `json:"refresh_token" minLength:"10" maxLength:"200"`
}
type refreshIn struct{ Body refreshBody }

func (s *Server) refresh(ctx context.Context, in *refreshIn) (*tokenOut, error) {
	now := time.Now().UTC()
	t, err := s.Sessions.Rotate(ctx, session.Hash(strings.TrimSpace(in.Body.RefreshToken)), now)
	bad := huma.Error401Unauthorized("invalid or expired refresh token")
	switch {
	case errors.Is(err, session.ErrReused):
		_ = s.Sessions.RevokeFamily(ctx, t.FamilyID, now) // theft signal: kill the whole session
		s.Metrics.Inc("auth_refresh_reuse_total")
		s.Log.Warn("refresh token reuse detected; session revoked", "user", t.UserID)
		return nil, bad
	case errors.Is(err, session.ErrInvalid):
		return nil, bad
	case err != nil:
		return nil, huma.Error500InternalServerError("internal server error")
	}
	u, err := s.Users.GetByID(ctx, t.UserID)
	if err != nil || u.Status != user.StatusActive {
		_ = s.Sessions.RevokeFamily(ctx, t.FamilyID, now)
		return nil, bad
	}
	s.Metrics.Inc("auth_refresh_total")
	return s.issueSession(ctx, u, t.FamilyID)
}

func (s *Server) logout(ctx context.Context, in *refreshIn) (*struct{}, error) {
	h := session.Hash(strings.TrimSpace(in.Body.RefreshToken))
	if fam, err := s.Sessions.FamilyOf(ctx, h); err == nil {
		_ = s.Sessions.RevokeFamily(ctx, fam, time.Now().UTC())
	}
	return &struct{}{}, nil // same answer for valid, expired and unknown tokens
}

func (s *Server) revokeEverywhere(ctx context.Context, uid string) error {
	now := time.Now().UTC()
	if err := s.Sessions.RevokeUser(ctx, uid, now); err != nil {
		return err
	}
	return s.Users.RevokeSessions(ctx, uid, now)
}

func (s *Server) logoutAll(ctx context.Context, _ *struct{}) (*struct{}, error) {
	uid := userID(ctx)
	if err := s.revokeEverywhere(ctx, uid); err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	s.record(ctx, uid, "auth.logout_all", "user", uid, "")
	return &struct{}{}, nil
}

type passwordBody struct {
	CurrentPassword string `json:"current_password" minLength:"1" maxLength:"128"`
	NewPassword     string `json:"new_password" minLength:"10" maxLength:"128"`
}
type passwordIn struct{ Body passwordBody }

func (s *Server) changePassword(ctx context.Context, in *passwordIn) (*tokenOut, error) {
	uid := userID(ctx)
	key := "pw:" + uid
	if s.throttle.blocked(key) > 0 {
		return nil, huma.Error429TooManyRequests("too many failed attempts; try again later")
	}
	u, err := s.Users.GetByID(ctx, uid)
	if err != nil {
		return nil, huma.Error401Unauthorized("unknown user")
	}
	if !auth.VerifyPassword(in.Body.CurrentPassword, u.PasswordHash) {
		s.throttle.fail(key)
		return nil, huma.Error403Forbidden("the current password is incorrect")
	}
	s.throttle.reset(key)
	if in.Body.NewPassword == in.Body.CurrentPassword {
		return nil, huma.Error422UnprocessableEntity("the new password must differ from the current one")
	}
	hash, err := auth.HashPassword(in.Body.NewPassword)
	if err != nil {
		return nil, huma.Error500InternalServerError("could not process password")
	}
	now := time.Now().UTC()
	if err := s.Sessions.RevokeUser(ctx, uid, now); err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	if err := s.Users.SetPassword(ctx, uid, hash, now); err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	s.record(ctx, uid, "user.change_password", "user", uid, "")
	return s.tokenFor(ctx, u) // fresh pair for this device; every other session is signed out
}

type tempPasswordBody struct {
	TemporaryPassword string `json:"temporary_password" doc:"Shown ONCE; hand it over securely"`
}
type tempPasswordOut struct{ Body tempPasswordBody }

func (s *Server) resetPassword(ctx context.Context, in *idIn) (*tempPasswordOut, error) {
	u, err := s.Users.GetByID(ctx, in.ID)
	if errors.Is(err, user.ErrNotFound) {
		return nil, huma.Error404NotFound("user not found")
	} else if err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	temp := base64.RawURLEncoding.EncodeToString(b)
	hash, err := auth.HashPassword(temp)
	if err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	now := time.Now().UTC()
	if err := s.Sessions.RevokeUser(ctx, u.ID, now); err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	if err := s.Users.SetPassword(ctx, u.ID, hash, now); err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	s.record(ctx, userID(ctx), "user.reset_password", "user", u.ID, "") // never the password
	return &tempPasswordOut{Body: tempPasswordBody{TemporaryPassword: temp}}, nil
}

// metricsHandler serves Prometheus text; it is a 404 unless METRICS_TOKEN is configured.
func (s *Server) metricsHandler(w http.ResponseWriter, r *http.Request) {
	if s.Cfg.MetricsToken == "" || s.Metrics == nil {
		http.NotFound(w, r)
		return
	}
	const p = "Bearer "
	h := r.Header.Get("Authorization")
	if len(h) <= len(p) || !strings.EqualFold(h[:len(p)], p) || subtle.ConstantTimeCompare([]byte(h[len(p):]), []byte(s.Cfg.MetricsToken)) != 1 {
		w.Header().Set("WWW-Authenticate", `Bearer realm="metrics"`)
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid metrics credentials")
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = s.Metrics.WriteTo(w)
}

// RegisterMetrics describes the counters and registers scrape-time gauges.
func (s *Server) RegisterMetrics(busDropped func() uint64) {
	m := s.Metrics
	if m == nil {
		return
	}
	m.Describe("http_requests_total", "counter", "HTTP requests by status class")
	m.Describe("http_request_duration_seconds", "histogram", "HTTP request duration")
	m.Describe("auth_login_failures_total", "counter", "Failed logins")
	m.Describe("auth_lockouts_total", "counter", "Logins refused because the account is temporarily locked")
	m.Describe("auth_refresh_total", "counter", "Refresh token rotations")
	m.Describe("auth_refresh_reuse_total", "counter", "Refresh token reuse detections (possible theft)")
	m.Describe("webhook_deliveries_total", "counter", "Inbound webhook deliveries by result")
	m.Describe("alert_fires_total", "counter", "Alerts fired by severity")
	m.Describe("alert_rules_auto_paused_total", "counter", "Alert rules paused for firing too often")
	m.Describe("outbound_deliveries_total", "counter", "Outbound webhook deliveries by result")
	m.Describe("source_runs_total", "counter", "Source fetch runs by result")
	m.Describe("feedback_total", "counter", "Useful / not useful ratings by target and rating")
	if s.Hub != nil {
		m.GaugeFunc("ws_connections", "Live WebSocket connections", func() []metrics.Sample {
			return []metrics.Sample{{Value: float64(s.Hub.Count())}}
		})
	}
	if busDropped != nil {
		m.GaugeFunc("bus_dropped_events", "Events dropped because a subscriber queue was full (since start)", func() []metrics.Sample {
			return []metrics.Sample{{Value: float64(busDropped())}}
		})
	}
	if s.Sources != nil {
		m.GaugeFunc("sources_by_health", "Configured sources by health", func() []metrics.Sample {
			list, err := s.Sources.List(context.Background())
			if err != nil {
				return nil
			}
			counts := map[string]float64{"ok": 0, "pending": 0, "degraded": 0, "failing": 0, "overdue": 0, "disabled": 0}
			now := time.Now()
			for _, src := range list {
				counts[src.Health(now)]++
			}
			out := make([]metrics.Sample, 0, len(counts))
			for k, v := range counts {
				out = append(out, metrics.Sample{Labels: []string{"health", k}, Value: v})
			}
			return out
		})
	}
	if s.Usage != nil {
		m.GaugeFunc("x_spend_usd", "X API spend this month (USD)", func() []metrics.Sample {
			v, err := s.Usage.Month(context.Background(), "x", time.Now())
			if err != nil {
				return nil
			}
			return []metrics.Sample{{Value: float64(v) / 1e6}}
		})
		m.GaugeFunc("x_budget_usd", "Configured monthly X budget (USD)", func() []metrics.Sample {
			return []metrics.Sample{{Value: s.Cfg.XMonthlyBudgetUSD}}
		})
	}
}
