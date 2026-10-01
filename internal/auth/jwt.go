// Package auth provides stdlib-only JWT (HS256), password hashing and
// short-lived one-time WebSocket tickets.
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var ErrInvalidToken = errors.New("invalid token")

type Claims struct {
	Sub string `json:"sub"`
	Iss string `json:"iss"`
	Iat int64  `json:"iat"`
	Exp int64  `json:"exp"`
}

type JWT struct {
	secret []byte
	issuer string
	ttl    time.Duration
	now    func() time.Time
}

var (
	enc       = base64.RawURLEncoding
	jwtHeader = enc.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
)

func NewJWT(secret, issuer string, ttl time.Duration) (*JWT, error) {
	if len(secret) < 32 {
		return nil, errors.New("jwt secret must be at least 32 bytes")
	}
	return &JWT{secret: []byte(secret), issuer: issuer, ttl: ttl, now: time.Now}, nil
}

func (j *JWT) sign(s string) []byte {
	m := hmac.New(sha256.New, j.secret)
	m.Write([]byte(s))
	return m.Sum(nil)
}

func (j *JWT) Issue(sub string) (string, error) {
	now := j.now()
	b, err := json.Marshal(Claims{Sub: sub, Iss: j.issuer, Iat: now.Unix(), Exp: now.Add(j.ttl).Unix()})
	if err != nil {
		return "", err
	}
	signing := jwtHeader + "." + enc.EncodeToString(b)
	return signing + "." + enc.EncodeToString(j.sign(signing)), nil
}

// Verify checks signature, issuer and expiry. The header must match exactly the
// one we issue, which rules out alg=none and algorithm-confusion attacks.
func (j *JWT) Verify(token string) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != jwtHeader {
		return Claims{}, ErrInvalidToken
	}
	sig, err := enc.DecodeString(parts[2])
	if err != nil || !hmac.Equal(sig, j.sign(parts[0]+"."+parts[1])) {
		return Claims{}, ErrInvalidToken
	}
	raw, err := enc.DecodeString(parts[1])
	if err != nil {
		return Claims{}, ErrInvalidToken
	}
	var c Claims
	if err := json.Unmarshal(raw, &c); err != nil {
		return Claims{}, ErrInvalidToken
	}
	if c.Sub == "" || c.Iss != j.issuer || j.now().Unix() >= c.Exp {
		return Claims{}, ErrInvalidToken
	}
	return c, nil
}
