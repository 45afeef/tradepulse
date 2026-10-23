package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"tradepulse/internal/connector"
	"tradepulse/internal/domain/sourcedef"
	"tradepulse/internal/webhook"
)

func (s *Server) hookLimiter(id string) *limiter {
	s.hookMu.Lock()
	defer s.hookMu.Unlock()
	if s.hookLims == nil {
		s.hookLims = map[string]*limiter{}
	}
	l := s.hookLims[id]
	if l == nil {
		n := s.Cfg.WebhookRatePerMinute
		if n < 1 {
			n = 60
		}
		l = newLimiter(float64(n)/60, n)
		s.hookLims[id] = l
	}
	return l
}

const (
	hookMaxBody = 64 << 10
	hookWindow  = 5 * time.Minute
)

type hookPayload struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Summary     string   `json:"summary"`
	URL         string   `json:"url"`
	Entities    []string `json:"entities"`
	PublishedAt string   `json:"published_at"`
}

func writeJSONStatus(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) hookSecret(src sourcedef.Source) string {
	return webhook.Secret([]byte(s.Cfg.WebhookSigningKey), src.ID, src.SecretVersion)
}

// hook handles POST /api/v1/hooks/{name}. It is a plain handler because the
// signature covers the raw body (documented in the /docs overview and docs/WEBHOOKS.md).
func (s *Server) hook(w http.ResponseWriter, r *http.Request) {
	if s.Cfg.WebhookSigningKey == "" {
		writeError(w, http.StatusServiceUnavailable, "webhooks_disabled", "inbound webhooks are not enabled on this server")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, hookMaxBody))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "too_large", "request body too large (max 64 KB)")
		return
	}
	deny := func() {
		s.Metrics.Inc("webhook_deliveries_total", "result", "unauthorized")
		writeError(w, http.StatusUnauthorized, "unauthorized", "invalid webhook credentials")
	}

	src, err := s.Sources.GetByName(r.Context(), strings.ToLower(r.PathValue("name")))
	if err != nil || src.Type != sourcedef.TypeWebhook || !src.Enabled {
		deny() // unknown, disabled and wrong-secret look identical (no enumeration)
		return
	}
	secret := s.hookSecret(src)
	auth := "hmac"
	if src.Webhook != nil {
		auth = src.Webhook.Auth
	}
	switch auth {
	case "token":
		tok := r.Header.Get("X-Webhook-Token")
		if tok == "" {
			if h := r.Header.Get("Authorization"); len(h) > 7 && strings.EqualFold(h[:7], "Bearer ") {
				tok = h[7:]
			}
		}
		if !webhook.VerifyToken(secret, tok) {
			deny()
			return
		}
	default:
		if err := webhook.VerifySignature(secret, r.Header.Get("X-TP-Timestamp"), body, r.Header.Get("X-TP-Signature"), time.Now(), hookWindow); err != nil {
			deny()
			return
		}
	}

	if !s.hookLimiter(src.ID).allow(src.ID) {
		w.Header().Set("Retry-After", "10")
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many webhook deliveries for this source")
		return
	}

	var p hookPayload
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&p); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid_payload", "body must be a JSON object with title, optional summary, url, entities, id, published_at")
		return
	}
	title := strings.Join(strings.Fields(p.Title), " ")
	summary := strings.Join(strings.Fields(p.Summary), " ")
	link := strings.TrimSpace(p.URL)
	if title == "" || len(title) > 300 || len(summary) > 1000 || len(p.Entities) > 10 || len(p.ID) > 150 {
		writeError(w, http.StatusUnprocessableEntity, "invalid_payload", "title is required (max 300), summary max 1000, entities max 10, id max 150")
		return
	}
	if link != "" {
		u, err := url.Parse(link)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || len(link) > 2000 {
			writeError(w, http.StatusUnprocessableEntity, "invalid_payload", "url must be a plain http(s) link")
			return
		}
	}
	var published time.Time
	if p.PublishedAt != "" {
		if published, err = time.Parse(time.RFC3339, p.PublishedAt); err != nil {
			writeError(w, http.StatusUnprocessableEntity, "invalid_payload", "published_at must be RFC 3339")
			return
		}
	}
	key := "hook:" + p.ID
	if p.ID == "" {
		key = "hook:" + connector.HashKey(title, link) // identical retries are de-duplicated
	}
	now := time.Now().UTC()
	item := connector.Item{Key: key, Title: title, Summary: summary, URL: link, PublishedAt: published, Entities: p.Entities, Raw: body}
	inserted, err := s.Items.Insert(r.Context(), src.ID, key, body, now)
	if err != nil {
		s.Log.Error("webhook store", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal server error")
		return
	}
	if !inserted {
		s.Metrics.Inc("webhook_deliveries_total", "result", "duplicate")
		writeJSONStatus(w, http.StatusOK, map[string]string{"status": "duplicate"})
		return
	}
	ev := connector.ItemEvent(src, item, now)
	if err := s.Registry.Validate(ev); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid_payload", err.Error())
		return
	}
	if err := s.Bus.Publish(r.Context(), ev); err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "the service is shutting down")
		return
	}
	st := src.State
	st.LastRunAt, st.LastSuccessAt, st.LastItemAt = &now, &now, &now
	st.ItemsTotal++
	if err := s.Sources.SaveState(r.Context(), src.ID, st); err != nil && !errors.Is(err, sourcedef.ErrNotFound) {
		s.Log.Warn("webhook state", "err", err)
	}
	s.Metrics.Inc("webhook_deliveries_total", "result", "accepted")
	writeJSONStatus(w, http.StatusCreated, map[string]string{"status": "accepted", "event_id": ev.ID})
}
