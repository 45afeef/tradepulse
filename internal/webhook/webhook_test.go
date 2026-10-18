package webhook

import (
	"strings"
	"testing"
	"time"
)

func TestSecretDerivation(t *testing.T) {
	key := []byte(strings.Repeat("k", 40))
	a := Secret(key, "src-1", 1)
	if a != Secret(key, "src-1", 1) {
		t.Fatal("derivation must be deterministic")
	}
	if !strings.HasPrefix(a, SecretPrefix) || len(a) < 40 {
		t.Fatalf("unexpected secret shape: %q", a)
	}
	for name, other := range map[string]string{
		"other source":  Secret(key, "src-2", 1),
		"other version": Secret(key, "src-1", 2),
		"other key":     Secret([]byte(strings.Repeat("z", 40)), "src-1", 1),
	} {
		if other == a {
			t.Errorf("%s must yield a different secret", name)
		}
	}
	if strings.Contains(a, string(key)) {
		t.Fatal("secret must not contain the signing key")
	}
}

func TestSignAndVerify(t *testing.T) {
	secret, body := Secret([]byte(strings.Repeat("k", 40)), "s", 1), []byte(`{"title":"x"}`)
	now := time.Unix(1_800_000_000, 0)
	ts := "1800000000"
	good := Sign(secret, ts, body)
	if err := VerifySignature(secret, ts, body, good, now, 5*time.Minute); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
	if err := VerifySignature(secret, ts, body, strings.TrimPrefix(good, "sha256="), now, 5*time.Minute); err != nil {
		t.Fatalf("bare hex must be accepted: %v", err)
	}
	if err := VerifySignature(secret, ts, body, strings.ToUpper(good), now, 5*time.Minute); err != nil {
		t.Fatalf("hex case must not matter: %v", err)
	}
	cases := map[string]struct {
		secret, ts, sig string
		body            []byte
		err             error
	}{
		"tampered body":   {secret, ts, good, []byte(`{"title":"evil"}`), ErrBadSignature},
		"wrong secret":    {"whsec_other", ts, good, body, ErrBadSignature},
		"empty signature": {secret, ts, "", body, ErrBadSignature},
		"garbage":         {secret, ts, "sha256=zzzz", body, ErrBadSignature},
		"missing ts":      {secret, "", good, body, ErrBadTimestamp},
		"malformed ts":    {secret, "yesterday", good, body, ErrBadTimestamp},
		"too old":         {secret, "1799999000", Sign(secret, "1799999000", body), body, ErrBadTimestamp},
		"too new":         {secret, "1800001000", Sign(secret, "1800001000", body), body, ErrBadTimestamp},
		"ts swapped":      {secret, "1800000001", good, body, ErrBadSignature}, // signature is bound to the timestamp
	}
	for name, c := range cases {
		if err := VerifySignature(c.secret, c.ts, c.body, c.sig, now, 5*time.Minute); err != c.err {
			t.Errorf("%s: got %v, want %v", name, err, c.err)
		}
	}
}

func TestVerifyToken(t *testing.T) {
	if !VerifyToken("whsec_abc", " whsec_abc ") || VerifyToken("whsec_abc", "whsec_abd") || VerifyToken("whsec_abc", "") || VerifyToken("whsec_abc", "whsec_ab") {
		t.Fatal("token comparison wrong")
	}
}
