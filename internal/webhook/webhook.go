// Package webhook implements inbound-webhook authentication.
//
// Secrets are NOT stored: each source's secret is derived as
// HMAC-SHA256(WEBHOOK_SIGNING_KEY, "tp-webhook|<sourceID>|<version>"). Rotating a
// secret bumps the source's version. Losing the database therefore never leaks
// webhook secrets, and the signing key lives only in the environment.
package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const SecretPrefix = "whsec_"

var (
	ErrBadTimestamp = errors.New("timestamp missing, malformed or outside the allowed window")
	ErrBadSignature = errors.New("signature mismatch")
)

// Secret derives the secret for a source generation.
func Secret(key []byte, sourceID string, version int) string {
	m := hmac.New(sha256.New, key)
	fmt.Fprintf(m, "tp-webhook|%s|%d", sourceID, version)
	return SecretPrefix + base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// Sign returns "sha256=<hex>" over "<timestamp>.<body>".
func Sign(secret, timestamp string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(timestamp))
	m.Write([]byte{'.'})
	m.Write(body)
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

// VerifySignature checks the timestamp window (replay protection) and the HMAC in
// constant time. got may be "sha256=<hex>" or bare hex.
func VerifySignature(secret, timestamp string, body []byte, got string, now time.Time, window time.Duration) error {
	sec, err := strconv.ParseInt(strings.TrimSpace(timestamp), 10, 64)
	if err != nil {
		return ErrBadTimestamp
	}
	d := now.Sub(time.Unix(sec, 0))
	if d < 0 {
		d = -d
	}
	if d > window {
		return ErrBadTimestamp
	}
	got = strings.ToLower(strings.TrimSpace(got))
	if !strings.HasPrefix(got, "sha256=") {
		got = "sha256=" + got
	}
	want := Sign(secret, strings.TrimSpace(timestamp), body)
	if !hmac.Equal([]byte(got), []byte(want)) {
		return ErrBadSignature
	}
	return nil
}

// VerifyToken compares a static token in constant time.
func VerifyToken(secret, got string) bool {
	return subtle.ConstantTimeCompare([]byte(secret), []byte(strings.TrimSpace(got))) == 1
}
