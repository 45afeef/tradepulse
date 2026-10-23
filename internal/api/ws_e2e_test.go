package api

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"tradepulse/internal/auth"
	"tradepulse/internal/config"
	"tradepulse/internal/delivery"
	"tradepulse/internal/domain/event"
	"tradepulse/internal/personalization"
	"tradepulse/internal/storage/memory"
)

// readServerFrame reads one unmasked server frame.
func readServerFrame(conn net.Conn, br *bufio.Reader) (op byte, payload []byte, err error) {
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	var h [2]byte
	if _, err = io.ReadFull(br, h[:]); err != nil {
		return
	}
	op = h[0] & 0x0f
	n := int(h[1] & 0x7f)
	switch n {
	case 126:
		var b [2]byte
		if _, err = io.ReadFull(br, b[:]); err != nil {
			return
		}
		n = int(binary.BigEndian.Uint16(b[:]))
	case 127:
		var b [8]byte
		if _, err = io.ReadFull(br, b[:]); err != nil {
			return
		}
		n = int(binary.BigEndian.Uint64(b[:]))
	}
	payload = make([]byte, n)
	_, err = io.ReadFull(br, payload)
	return
}

// TP-002: real HTTP server + raw WebSocket client + real matching engine and hub.
func TestWebSocketEndToEnd(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	reg := event.NewRegistry()
	hub := delivery.NewHub(log, nil, 2)
	feeds := memory.NewFeedRepo(10)
	engine := personalization.NewEngine(log, feeds, hub, reg)
	jwt, _ := auth.NewJWT(strings.Repeat("k", 32), "test", time.Minute)
	s, err := New(Deps{
		Cfg: config.Config{Env: "development", RateRPS: 1000, RateBurst: 1000}, Log: log,
		Users: memory.NewUserRepo(), Feeds: feeds, Invites: memory.NewInviteRepo(), Audit: memory.NewAuditRepo(100),
		JWT: jwt, Tickets: auth.NewTicketStore(time.Minute), Personalizer: engine, Hub: hub, Registry: reg, Sessions: memory.NewSessionRepo(),
	})
	if err != nil {
		t.Fatal(err)
	}
	s.authLim = newLimiter(1000, 1000)
	lastServer = s
	h := s.Handler()
	srv := httptest.NewServer(h)
	defer srv.Close()
	defer hub.Shutdown()

	member := register(t, h, "live@example.com")
	if rec := do(h, "PUT", "/api/v1/me/subscriptions", member, `{"subscriptions":[{"kind":"keyword","value":"breaking news"}]}`); rec.Code != http.StatusOK {
		t.Fatalf("subscribe: %d %s", rec.Code, rec.Body)
	}
	var tk struct{ Ticket string }
	_ = json.Unmarshal(do(h, "POST", "/api/v1/ws-ticket", member, "").Body.Bytes(), &tk)

	conn, err := net.Dial("tcp", srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef"))
	fmt.Fprintf(conn, "GET /ws?ticket=%s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\n\r\n",
		tk.Ticket, srv.Listener.Addr(), key)
	br := bufio.NewReader(conn)
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	resp, err := http.ReadResponse(br, nil)
	if err != nil || resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("handshake failed: %v %v", resp, err)
	}
	sum := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	if resp.Header.Get("Sec-WebSocket-Accept") != base64.StdEncoding.EncodeToString(sum[:]) {
		t.Fatal("wrong Sec-WebSocket-Accept")
	}

	if _, p, err := readServerFrame(conn, br); err != nil || !strings.Contains(string(p), "connected") {
		t.Fatalf("expected connected message, got %q %v", p, err)
	}

	ev := event.New("content_item", "RSS", []string{"newsroom"}, time.Now(), nil)
	ev.Title = "Breaking news: markets rally"
	engine.Handle(context.Background(), ev)
	_, p, err := readServerFrame(conn, br)
	if err != nil || !strings.Contains(string(p), `"feed_item"`) || !strings.Contains(string(p), "Breaking news: markets rally") || !strings.Contains(string(p), "reason") {
		t.Fatalf("expected feed_item, got %q %v", p, err)
	}
	if rec := do(h, "GET", "/api/v1/me/feed", member, ""); !strings.Contains(rec.Body.String(), "Breaking news: markets rally") {
		t.Fatalf("history missing item: %s", rec.Body)
	}

	// Suspension must close the live connection.
	claims, _ := s.JWT.Verify(member)
	_, admin := adminToken(t)
	if rec := do(h, "POST", "/api/v1/admin/users/"+claims.Sub+"/suspend", admin, ""); rec.Code != http.StatusOK {
		t.Fatalf("suspend: %d", rec.Code)
	}
	closed := false
	for i := 0; i < 5; i++ {
		op, _, err := readServerFrame(conn, br)
		if err != nil || op == 0x8 {
			closed = true
			break
		}
	}
	if !closed {
		t.Fatal("connection stayed open after suspension")
	}
}
