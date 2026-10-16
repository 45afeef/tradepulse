// Package access holds startup access-control helpers (first-admin bootstrap).
package access

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"tradepulse/internal/auth"
	"tradepulse/internal/domain/user"
	"tradepulse/internal/idgen"
)

// EnsureAdmin creates the first admin if no user with that email exists. It never
// changes an existing account (no password overwrite), so it is safe to run on
// every start. It is not reachable over HTTP.
func EnsureAdmin(ctx context.Context, users user.Repository, email, password string) (created bool, err error) {
	email = strings.TrimSpace(email)
	addr, perr := mail.ParseAddress(email)
	if perr != nil || addr.Address != email {
		return false, errors.New("invalid admin email")
	}
	if len(password) < 12 {
		return false, errors.New("admin password must be at least 12 characters")
	}
	if _, err := users.GetByEmail(ctx, email); err == nil {
		return false, nil
	} else if !errors.Is(err, user.ErrNotFound) {
		return false, err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return false, fmt.Errorf("hash password: %w", err)
	}
	err = users.Create(ctx, user.User{
		ID: idgen.New(), Email: strings.ToLower(email), PasswordHash: hash, CreatedAt: time.Now().UTC(),
		Profile: user.DefaultProfile(), Role: user.RoleAdmin, Status: user.StatusActive,
	})
	if errors.Is(err, user.ErrEmailTaken) {
		return false, nil
	}
	return err == nil, err
}
