package auth

import (
	"context"
	"crypto/rand"
	"sync"
	"time"
)

// TicketStore issues short-lived, single-use tickets so browsers can open a
// WebSocket without putting a long-lived JWT in the URL. In-memory here; use
// Redis (SET NX EX / GETDEL) when running more than one instance.
type TicketStore struct {
	ttl time.Duration
	mu  sync.Mutex
	m   map[string]ticket
}

type ticket struct {
	userID string
	exp    time.Time
}

func NewTicketStore(ttl time.Duration) *TicketStore {
	return &TicketStore{ttl: ttl, m: map[string]ticket{}}
}

func (s *TicketStore) Issue(userID string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	t := enc.EncodeToString(b)
	s.mu.Lock()
	s.m[t] = ticket{userID: userID, exp: time.Now().Add(s.ttl)}
	s.mu.Unlock()
	return t, nil
}

// Redeem consumes the ticket; it can never be used twice.
func (s *TicketStore) Redeem(t string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tk, ok := s.m[t]
	if !ok {
		return "", false
	}
	delete(s.m, t)
	if time.Now().After(tk.exp) {
		return "", false
	}
	return tk.userID, true
}

// Run evicts expired tickets until ctx is cancelled.
func (s *TicketStore) Run(ctx context.Context) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			s.mu.Lock()
			for k, v := range s.m {
				if now.After(v.exp) {
					delete(s.m, k)
				}
			}
			s.mu.Unlock()
		}
	}
}
