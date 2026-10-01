package auth

import (
	"strings"
	"testing"
	"time"
)

func TestJWTRoundTripAndTamper(t *testing.T) {
	j, err := NewJWT(strings.Repeat("s", 32), "test", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	tok, _ := j.Issue("user-1")
	c, err := j.Verify(tok)
	if err != nil || c.Sub != "user-1" {
		t.Fatalf("verify failed: %v", err)
	}
	if _, err := j.Verify(tok + "x"); err == nil {
		t.Fatal("tampered token accepted")
	}
	j.now = func() time.Time { return time.Now().Add(2 * time.Minute) }
	if _, err := j.Verify(tok); err == nil {
		t.Fatal("expired token accepted")
	}
}

func TestPassword(t *testing.T) {
	h, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword("correct horse battery", h) || VerifyPassword("wrong", h) {
		t.Fatal("password verification wrong")
	}
}

func TestTicketSingleUse(t *testing.T) {
	s := NewTicketStore(time.Minute)
	tk, _ := s.Issue("u")
	if id, ok := s.Redeem(tk); !ok || id != "u" {
		t.Fatal("redeem failed")
	}
	if _, ok := s.Redeem(tk); ok {
		t.Fatal("ticket reused")
	}
}
