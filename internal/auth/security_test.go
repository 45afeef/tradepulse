package auth

import (
	"strings"
	"testing"
	"time"
)

func newJWT(t *testing.T, secret, iss string) *JWT {
	t.Helper()
	j, err := NewJWT(secret, iss, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return j
}

func TestJWTRejectsShortSecret(t *testing.T) {
	if _, err := NewJWT("short", "x", time.Minute); err == nil {
		t.Fatal("short secret accepted")
	}
}

func TestJWTRejectsAlgNoneAndForgeries(t *testing.T) {
	j := newJWT(t, strings.Repeat("a", 32), "iss")
	good, _ := j.Issue("user-1")
	parts := strings.Split(good, ".")

	noneHeader := enc.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	forged := []string{
		noneHeader + "." + parts[1] + ".",                // alg=none, no signature
		noneHeader + "." + parts[1] + "." + parts[2],     // alg=none with a real signature
		parts[0] + "." + parts[1],                         // missing signature
		parts[0] + "." + parts[1] + "." + parts[2] + ".x", // extra segment
		"",
		"a.b.c",
	}
	for _, tok := range forged {
		if _, err := j.Verify(tok); err == nil {
			t.Errorf("forged token accepted: %q", tok)
		}
	}
}

func TestJWTRejectsWrongSecretAndIssuer(t *testing.T) {
	a := newJWT(t, strings.Repeat("a", 32), "iss")
	tok, _ := a.Issue("user-1")
	if _, err := newJWT(t, strings.Repeat("b", 32), "iss").Verify(tok); err == nil {
		t.Fatal("token verified with a different secret")
	}
	if _, err := newJWT(t, strings.Repeat("a", 32), "other").Verify(tok); err == nil {
		t.Fatal("token verified with a different issuer")
	}
}

func TestJWTRejectsTamperedPayload(t *testing.T) {
	j := newJWT(t, strings.Repeat("a", 32), "iss")
	tok, _ := j.Issue("user-1")
	parts := strings.Split(tok, ".")
	evil := enc.EncodeToString([]byte(`{"sub":"admin","iss":"iss","iat":1,"exp":99999999999}`))
	if _, err := j.Verify(parts[0] + "." + evil + "." + parts[2]); err == nil {
		t.Fatal("payload tampering not detected")
	}
}

func TestPasswordMalformedHashesRejected(t *testing.T) {
	for _, h := range []string{"", "plain", "pbkdf2-sha256$x$y$z", "pbkdf2-sha256$0$AA$AA", "pbkdf2-sha256$99999999$AA$AA", "md5$1$AA$AA", "pbkdf2-sha256$1$!!$!!"} {
		if VerifyPassword("anything", h) {
			t.Errorf("malformed hash accepted: %q", h)
		}
	}
}

func TestPasswordHashesAreSalted(t *testing.T) {
	a, _ := HashPassword("same-password-123")
	b, _ := HashPassword("same-password-123")
	if a == b {
		t.Fatal("identical hashes: salt missing")
	}
	if strings.Contains(a, "same-password-123") {
		t.Fatal("hash contains plaintext")
	}
}

func TestTicketExpiresAndIsUnguessable(t *testing.T) {
	s := NewTicketStore(time.Millisecond)
	tk, _ := s.Issue("u")
	time.Sleep(10 * time.Millisecond)
	if _, ok := s.Redeem(tk); ok {
		t.Fatal("expired ticket accepted")
	}
	a, _ := NewTicketStore(time.Minute).Issue("u")
	b, _ := NewTicketStore(time.Minute).Issue("u")
	if a == b || len(a) < 40 {
		t.Fatal("tickets must be long and random")
	}
	if _, ok := NewTicketStore(time.Minute).Redeem("guess"); ok {
		t.Fatal("unknown ticket accepted")
	}
}
