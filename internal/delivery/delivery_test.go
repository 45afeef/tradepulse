package delivery

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"tradepulse/internal/domain/feed"
)

func discardLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func maskedFrame(op byte, fin bool, payload []byte) []byte {
	b0 := op
	if fin {
		b0 |= 0x80
	}
	mask := []byte{1, 2, 3, 4}
	out := []byte{b0, 0x80 | byte(len(payload))}
	out = append(out, mask...)
	for i, c := range payload {
		out = append(out, c^mask[i%4])
	}
	return out
}

func read(b []byte) (byte, []byte, error) { return readFrame(bufio.NewReader(bytes.NewReader(b))) }

func TestReadFrameAcceptsMaskedText(t *testing.T) {
	op, p, err := read(maskedFrame(opText, true, []byte("hello")))
	if err != nil || op != opText || string(p) != "hello" {
		t.Fatalf("got op=%d payload=%q err=%v", op, p, err)
	}
}

func TestReadFrameRejectsProtocolViolations(t *testing.T) {
	cases := map[string][]byte{
		"unmasked":        {0x81, 0x02, 'h', 'i'},
		"fragmented":      maskedFrame(opText, false, []byte("x")),
		"continuation":    maskedFrame(0x0, true, []byte("x")),
		"reserved bits":   append([]byte{0xC1, 0x80}, 1, 2, 3, 4),
		"oversized":       {0x81, 0x80 | 126, 0x13, 0x88, 1, 2, 3, 4},       // 5000 > maxInbound
		"huge 64-bit len": {0x81, 0x80 | 127, 0, 0, 0, 0, 0xFF, 0xFF, 0xFF, 0xFF, 1, 2, 3, 4},
		"long control":    {0x89, 0x80 | 126, 0, 200, 1, 2, 3, 4},           // ping > 125 bytes
	}
	for name, frame := range cases {
		if _, _, err := read(frame); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestOriginCheck(t *testing.T) {
	h := NewHub(discardLog(), []string{"https://app.example"}, 2)
	mk := func(origin, host string) *http.Request {
		r, _ := http.NewRequest(http.MethodGet, "http://"+host+"/ws", nil)
		r.Host = host
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		return r
	}
	if !h.originOK(mk("https://app.example", "api.example")) {
		t.Error("allowlisted origin rejected")
	}
	if !h.originOK(mk("", "api.example")) {
		t.Error("non-browser client (no Origin) rejected")
	}
	if !h.originOK(mk("https://api.example", "api.example")) {
		t.Error("same-origin rejected")
	}
	if h.originOK(mk("https://evil.example", "api.example")) {
		t.Error("foreign origin accepted (cross-site WebSocket hijacking)")
	}
}

func TestWriteFrameEncoding(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()
	c := &client{conn: c1}
	for _, n := range []int{2, 200} {
		payload := bytes.Repeat([]byte("a"), n)
		go func() { _ = c.writeFrame(opText, payload) }()
		hdr := 2
		if n >= 126 {
			hdr = 4
		}
		buf := make([]byte, hdr+n)
		if _, err := io.ReadFull(c2, buf); err != nil {
			t.Fatal(err)
		}
		if buf[0] != 0x81 {
			t.Fatalf("first byte %x", buf[0])
		}
		if n < 126 && buf[1] != byte(n) {
			t.Fatal("short length encoding wrong")
		}
		if n >= 126 && (buf[1] != 126 || int(buf[2])<<8|int(buf[3]) != n) {
			t.Fatal("extended length encoding wrong")
		}
	}
}

func newTestClient(user string, buf int) *client {
	c1, c2 := net.Pipe()
	go func() { _, _ = io.Copy(io.Discard, c2) }() // keep writes from blocking
	return &client{userID: user, conn: c1, send: make(chan []byte, buf), done: make(chan struct{})}
}

func TestHubConnectionCapPerUser(t *testing.T) {
	h := NewHub(discardLog(), nil, 2)
	a, b, c := newTestClient("u", 1), newTestClient("u", 1), newTestClient("u", 1)
	if !h.register(a) || !h.register(b) {
		t.Fatal("registration under cap failed")
	}
	if h.register(c) {
		t.Fatal("per-user connection cap not enforced")
	}
	h.unregister(a)
	if !h.register(c) {
		t.Fatal("slot not freed after unregister")
	}
}

func TestDeliverTargetsOnlyOwnerAndDropsSlowClients(t *testing.T) {
	h := NewHub(discardLog(), nil, 5)
	alice, bob := newTestClient("alice", 1), newTestClient("bob", 1)
	h.register(alice)
	h.register(bob)
	h.Deliver(context.Background(), feed.FeedItem{UserID: "alice", Title: "t"})
	if len(alice.send) != 1 || len(bob.send) != 0 {
		t.Fatal("item delivered to the wrong user")
	}
	h.Deliver(context.Background(), feed.FeedItem{UserID: "alice", Title: "t2"}) // alice's queue is full
	select {
	case <-alice.done:
	default:
		t.Fatal("slow client was not disconnected")
	}
}

func TestShutdownRejectsNewConnections(t *testing.T) {
	h := NewHub(discardLog(), nil, 5)
	h.Shutdown()
	if h.register(newTestClient("u", 1)) {
		t.Fatal("registration accepted after shutdown")
	}
}

func TestDisconnectUserClosesOnlyThatUsersConnections(t *testing.T) {
	h := NewHub(discardLog(), nil, 5)
	alice, bob := newTestClient("alice", 1), newTestClient("bob", 1)
	h.register(alice)
	h.register(bob)
	h.DisconnectUser("alice")
	select {
	case <-alice.done:
	case <-time.After(time.Second):
		t.Fatal("alice not disconnected")
	}
	select {
	case <-bob.done:
		t.Fatal("bob must stay connected")
	default:
	}
}
