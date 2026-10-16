// Package invite models invite-only access: single-use, expiring, hashed codes.
package invite

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
	// ErrInvalid is deliberately generic: unknown, used, revoked, expired and
	// email-mismatch all look the same to callers (no oracle for attackers).
	ErrInvalid  = errors.New("invite invalid, used, expired or revoked")
	ErrNotFound = errors.New("invite not found")
)

const CodePrefix = "tp_inv_"

type Invite struct {
	ID        string
	CodeHash  string // SHA-256 of the code; the code itself is never stored
	Email     string // optional binding (lowercase); empty = any email
	Note      string
	CreatedBy string
	CreatedAt time.Time
	ExpiresAt time.Time
	UsedAt    *time.Time
	UsedBy    string
	RevokedAt *time.Time
}

// NewCode returns a fresh 192-bit random code and its hash.
func NewCode() (code, hash string, err error) {
	b := make([]byte, 24)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	code = CodePrefix + base64.RawURLEncoding.EncodeToString(b)
	return code, HashCode(code), nil
}

// HashCode is a fast hash, which is safe because codes have 192 bits of entropy.
func HashCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

// Status is one of: pending, used, expired, revoked.
func (i Invite) Status(now time.Time) string {
	switch {
	case i.RevokedAt != nil:
		return "revoked"
	case i.UsedAt != nil:
		return "used"
	case !now.Before(i.ExpiresAt):
		return "expired"
	}
	return "pending"
}

type Repository interface {
	Create(ctx context.Context, inv Invite) error
	// List returns invites newest first.
	List(ctx context.Context) ([]Invite, error)
	// Revoke only succeeds for pending invites (ErrNotFound / ErrInvalid otherwise).
	Revoke(ctx context.Context, id string, now time.Time) error
	// Redeem atomically marks a pending, unexpired, email-matching invite as used.
	// It must succeed for exactly one of any number of concurrent callers.
	Redeem(ctx context.Context, codeHash, email, userID string, now time.Time) (Invite, error)
	// Release undoes a Redeem by the same user (used when registration fails afterwards).
	Release(ctx context.Context, id, userID string) error
}
