// Package delivery pushes FeedItems to connected clients over WebSocket.
package delivery

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"tradepulse/internal/domain/feed"
)

type envelope struct {
	Type string `json:"type"`
	Data any    `json:"data,omitempty"`
}

// Hub tracks live connections per user and fans FeedItems out to them.
type Hub struct {
	log        *slog.Logger
	origins    map[string]struct{}
	maxPerUser int

	mu      sync.RWMutex
	clients map[string]map[*client]struct{}
	closed  bool
}

func NewHub(log *slog.Logger, allowedOrigins []string, maxPerUser int) *Hub {
	o := make(map[string]struct{}, len(allowedOrigins))
	for _, a := range allowedOrigins {
		o[a] = struct{}{}
	}
	return &Hub{log: log, origins: o, maxPerUser: maxPerUser, clients: map[string]map[*client]struct{}{}}
}

// Deliver implements personalization.Deliverer. Slow consumers are
// disconnected instead of blocking the pipeline.
func (h *Hub) Deliver(_ context.Context, item feed.FeedItem) {
	msg, err := json.Marshal(envelope{Type: "feed_item", Data: item})
	if err != nil {
		h.log.Error("marshal feed item", "err", err)
		return
	}
	var slow []*client
	h.mu.RLock()
	for c := range h.clients[item.UserID] {
		select {
		case c.send <- msg:
		default:
			slow = append(slow, c)
		}
	}
	h.mu.RUnlock()
	for _, c := range slow {
		h.log.Warn("disconnecting slow websocket client", "user", item.UserID)
		c.close()
	}
}

func (h *Hub) register(c *client) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	set := h.clients[c.userID]
	if h.closed || len(set) >= h.maxPerUser {
		return false
	}
	if set == nil {
		set = map[*client]struct{}{}
		h.clients[c.userID] = set
	}
	set[c] = struct{}{}
	return true
}

func (h *Hub) unregister(c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if set := h.clients[c.userID]; set != nil {
		delete(set, c)
		if len(set) == 0 {
			delete(h.clients, c.userID)
		}
	}
}

// Shutdown closes all connections and rejects new ones.
func (h *Hub) Shutdown() {
	h.mu.Lock()
	h.closed = true
	var all []*client
	for _, set := range h.clients {
		for c := range set {
			all = append(all, c)
		}
	}
	h.mu.Unlock()
	for _, c := range all {
		_ = c.writeFrame(opClose, []byte{0x03, 0xe9}) // 1001 going away
		c.close()
	}
}

// DisconnectUser closes every live connection of a user (e.g. on suspension).
func (h *Hub) DisconnectUser(userID string) {
	h.mu.RLock()
	var cs []*client
	for c := range h.clients[userID] {
		cs = append(cs, c)
	}
	h.mu.RUnlock()
	for _, c := range cs {
		_ = c.writeFrame(opClose, []byte{0x03, 0xf0}) // 1008 policy violation
		c.close()
	}
}
