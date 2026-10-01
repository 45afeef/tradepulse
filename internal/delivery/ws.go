package delivery

import (
	"bufio"
	"crypto/sha1" //nolint:gosec // mandated by RFC 6455 handshake
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	opText  = 0x1
	opClose = 0x8
	opPing  = 0x9
	opPong  = 0xA

	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = 25 * time.Second
	maxInbound = 4096
	wsGUID     = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
)

var errProtocol = errors.New("websocket protocol error")

type client struct {
	hub    *Hub
	userID string
	conn   net.Conn
	br     *bufio.Reader
	send   chan []byte
	wmu    sync.Mutex
	done   chan struct{}
	once   sync.Once
}

func (c *client) close() {
	c.once.Do(func() {
		close(c.done)
		_ = c.conn.Close()
	})
}

func (c *client) writeFrame(op byte, p []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
	buf := make([]byte, 0, 10+len(p))
	buf = append(buf, 0x80|op)
	switch n := len(p); {
	case n < 126:
		buf = append(buf, byte(n))
	case n <= 65535:
		buf = append(buf, 126, byte(n>>8), byte(n))
	default:
		buf = append(buf, 127)
		buf = binary.BigEndian.AppendUint64(buf, uint64(n))
	}
	buf = append(buf, p...)
	_, err := c.conn.Write(buf)
	return err
}

// readFrame reads one client frame. Clients must mask, fragmentation is not
// supported, and payloads are capped: the server only needs control frames.
func readFrame(r *bufio.Reader) (op byte, payload []byte, err error) {
	var h [2]byte
	if _, err = io.ReadFull(r, h[:]); err != nil {
		return
	}
	fin, op := h[0]&0x80 != 0, h[0]&0x0f
	if h[0]&0x70 != 0 || !fin || op == 0 || h[1]&0x80 == 0 {
		return 0, nil, errProtocol
	}
	n := uint64(h[1] & 0x7f)
	switch n {
	case 126:
		var b [2]byte
		if _, err = io.ReadFull(r, b[:]); err != nil {
			return
		}
		n = uint64(binary.BigEndian.Uint16(b[:]))
	case 127:
		var b [8]byte
		if _, err = io.ReadFull(r, b[:]); err != nil {
			return
		}
		n = binary.BigEndian.Uint64(b[:])
	}
	if n > maxInbound || (op >= 0x8 && n > 125) {
		return 0, nil, errProtocol
	}
	var mask [4]byte
	if _, err = io.ReadFull(r, mask[:]); err != nil {
		return
	}
	payload = make([]byte, n)
	if _, err = io.ReadFull(r, payload); err != nil {
		return
	}
	for i := range payload {
		payload[i] ^= mask[i%4]
	}
	return op, payload, nil
}

func (c *client) writeLoop() {
	t := time.NewTicker(pingPeriod)
	defer t.Stop()
	defer c.close()
	for {
		select {
		case msg := <-c.send:
			if c.writeFrame(opText, msg) != nil {
				return
			}
		case <-t.C:
			if c.writeFrame(opPing, nil) != nil {
				return
			}
		case <-c.done:
			return
		}
	}
}

func (c *client) readLoop() {
	for {
		_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
		op, p, err := readFrame(c.br)
		if err != nil {
			return
		}
		switch op {
		case opClose:
			_ = c.writeFrame(opClose, p)
			return
		case opPing:
			if c.writeFrame(opPong, p) != nil {
				return
			}
		}
		// text/binary/pong frames from clients are ignored
	}
}

func headerHas(h http.Header, key, token string) bool {
	for _, v := range h.Values(key) {
		for _, part := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(part), token) {
				return true
			}
		}
	}
	return false
}

func (h *Hub) originOK(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // non-browser client
	}
	if _, ok := h.origins[origin]; ok {
		return true
	}
	u, err := url.Parse(origin)
	return err == nil && u.Host == r.Host // same-origin
}

// Upgrade performs the WebSocket handshake for an already-authenticated user
// and blocks until the connection ends.
func (h *Hub) Upgrade(w http.ResponseWriter, r *http.Request, userID string) {
	if r.Method != http.MethodGet || !headerHas(r.Header, "Connection", "upgrade") || !headerHas(r.Header, "Upgrade", "websocket") {
		http.Error(w, "websocket upgrade required", http.StatusBadRequest)
		return
	}
	if r.Header.Get("Sec-WebSocket-Version") != "13" {
		w.Header().Set("Sec-WebSocket-Version", "13")
		http.Error(w, "unsupported websocket version", http.StatusUpgradeRequired)
		return
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if raw, err := base64.StdEncoding.DecodeString(key); err != nil || len(raw) != 16 {
		http.Error(w, "bad websocket key", http.StatusBadRequest)
		return
	}
	if !h.originOK(r) {
		http.Error(w, "origin not allowed", http.StatusForbidden)
		return
	}
	conn, brw, err := http.NewResponseController(w).Hijack()
	if err != nil {
		http.Error(w, "websocket unsupported", http.StatusInternalServerError)
		return
	}
	_ = conn.SetDeadline(time.Time{})

	c := &client{hub: h, userID: userID, conn: conn, br: brw.Reader, send: make(chan []byte, 64), done: make(chan struct{})}
	sum := sha1.Sum([]byte(key + wsGUID)) //nolint:gosec
	accept := base64.StdEncoding.EncodeToString(sum[:])
	_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
	_, err = fmt.Fprintf(conn, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", accept)
	if err != nil {
		_ = conn.Close()
		return
	}
	if !h.register(c) {
		_ = c.writeFrame(opClose, []byte{0x03, 0xf0}) // 1008 policy violation
		c.close()
		return
	}
	defer func() { h.unregister(c); c.close() }()

	c.send <- []byte(`{"type":"connected"}`)
	go c.writeLoop()
	c.readLoop()
}
