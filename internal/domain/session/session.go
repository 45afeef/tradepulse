// Package session models rotating, revocable refresh tokens.
package session

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"
)

var (
	ErrInvalid = errors.New("refresh token invalid, expired or revoked")
	// ErrReused means an already-rotated token was presented again: treat as theft and
	// revoke the whole family.
	ErrReused = errors.New("refresh token reuse detected")
)

const TokenPrefix = "tp_rt_"

// Token is one refresh token. Only its SHA-256 hash is stored.
type Token struct {
	ID        string
	UserID    string
	FamilyID  string // all tokens descended from one login share a family
	Hash      string
	CreatedAt time.Time
	ExpiresAt time.Time
	UsedAt    *time.Time
	RevokedAt *time.Time
}

// New returns a fresh 256-bit token and its hash.
func New() (token, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	token = TokenPrefix + base64.RawURLEncoding.EncodeToString(b)
	return token, Hash(token), nil
}

func Hash(token string) string {
	s := sha256.Sum256([]byte(token))
	return hex.EncodeToString(s[:])
}

type Repository interface {
	Create(ctx context.Context, t Token) error
	// Rotate atomically consumes a valid token (used_at = now) and returns it, so exactly one
	// caller can rotate a given token. Presenting an already-used token returns (that token,
	// ErrReused); unknown, expired or revoked tokens return ErrInvalid.
	Rotate(ctx context.Context, hash string, now time.Time) (Token, error)
	// FamilyOf resolves a presented token to its family (ErrInvalid if unknown).
	FamilyOf(ctx context.Context, hash string) (familyID string, err error)
	RevokeFamily(ctx context.Context, familyID string, now time.Time) error
	RevokeUser(ctx context.Context, userID string, now time.Time) error
	Prune(ctx context.Context, before time.Time) (int64, error)
}
