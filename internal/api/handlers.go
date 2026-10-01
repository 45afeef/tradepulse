package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"tradepulse/internal/auth"
	"tradepulse/internal/domain/user"
	"tradepulse/internal/idgen"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": msg}})
}

// decode reads a size-limited, strict JSON body.
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "malformed request body")
		return false
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_json", "unexpected trailing data")
		return false
	}
	return true
}

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type publicUser struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Server) tokenResponse(w http.ResponseWriter, status int, u user.User) {
	tok, err := s.JWT.Issue(u.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not issue token")
		return
	}
	writeJSON(w, status, map[string]any{
		"token": tok, "user": publicUser{ID: u.ID, Email: u.Email, CreatedAt: u.CreatedAt},
	})
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if !decode(w, r, &c) {
		return
	}
	addr, err := mail.ParseAddress(strings.TrimSpace(c.Email))
	if err != nil || addr.Name != "" || len(addr.Address) > 254 {
		writeError(w, http.StatusBadRequest, "invalid_email", "a valid email is required")
		return
	}
	if len(c.Password) < 10 || len(c.Password) > 128 {
		writeError(w, http.StatusBadRequest, "invalid_password", "password must be 10-128 characters")
		return
	}
	hash, err := auth.HashPassword(c.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not process password")
		return
	}
	u := user.User{
		ID: idgen.New(), Email: strings.ToLower(addr.Address), PasswordHash: hash,
		CreatedAt: time.Now().UTC(), Profile: user.DefaultProfile(),
	}
	if err := s.Users.Create(r.Context(), u); err != nil {
		if errors.Is(err, user.ErrEmailTaken) {
			writeError(w, http.StatusConflict, "email_taken", "email already registered")
			return
		}
		s.Log.Error("create user", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal server error")
		return
	}
	s.tokenResponse(w, http.StatusCreated, u)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if !decode(w, r, &c) {
		return
	}
	u, err := s.Users.GetByEmail(r.Context(), strings.TrimSpace(c.Email))
	hash := u.PasswordHash
	if err != nil {
		hash = s.dummyHash // equalise timing so unknown emails are not distinguishable
	}
	if !auth.VerifyPassword(c.Password, hash) || err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid email or password")
		return
	}
	s.tokenResponse(w, http.StatusOK, u)
}

func (s *Server) getProfile(w http.ResponseWriter, r *http.Request) {
	u, err := s.Users.GetByID(r.Context(), userID(r))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unknown user")
		return
	}
	writeJSON(w, http.StatusOK, u.Profile)
}

func (s *Server) putProfile(w http.ResponseWriter, r *http.Request) {
	var in user.Profile
	if !decode(w, r, &in) {
		return
	}
	p, err := in.Normalize()
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid_profile", err.Error())
		return
	}
	id := userID(r)
	if err := s.Users.UpdateProfile(r.Context(), id, p); err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "unknown user")
		return
	}
	s.Personalizer.UpsertUser(id, p)
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) getFeed(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 200 {
			writeError(w, http.StatusBadRequest, "invalid_limit", "limit must be 1-200")
			return
		}
		limit = n
	}
	items, err := s.Feeds.Recent(r.Context(), userID(r), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) wsTicket(w http.ResponseWriter, r *http.Request) {
	t, err := s.Tickets.Issue(userID(r))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ticket": t, "expires_in": 30})
}

func (s *Server) ws(w http.ResponseWriter, r *http.Request) {
	uid, ok := s.Tickets.Redeem(r.URL.Query().Get("ticket"))
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid or expired ticket")
		return
	}
	s.Hub.Upgrade(w, r, uid)
}
