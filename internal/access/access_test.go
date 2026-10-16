package access

import (
	"context"
	"strings"
	"testing"

	"tradepulse/internal/auth"
	"tradepulse/internal/domain/user"
	"tradepulse/internal/storage/memory"
)

func TestEnsureAdminCreatesOnceAndNeverOverwrites(t *testing.T) {
	repo := memory.NewUserRepo()
	ctx := context.Background()
	created, err := EnsureAdmin(ctx, repo, "Admin@Example.com", "first-password-123")
	if err != nil || !created {
		t.Fatalf("expected creation, got %v %v", created, err)
	}
	u, err := repo.GetByEmail(ctx, "admin@example.com")
	if err != nil || u.Role != user.RoleAdmin || u.Status != user.StatusActive {
		t.Fatalf("unexpected admin: %+v %v", u, err)
	}
	if !auth.VerifyPassword("first-password-123", u.PasswordHash) || strings.Contains(u.PasswordHash, "first-password-123") {
		t.Fatal("password not stored as a verifiable hash")
	}
	created, err = EnsureAdmin(ctx, repo, "admin@example.com", "different-password-456")
	if err != nil || created {
		t.Fatalf("second run must be a no-op: %v %v", created, err)
	}
	u2, _ := repo.GetByEmail(ctx, "admin@example.com")
	if !auth.VerifyPassword("first-password-123", u2.PasswordHash) {
		t.Fatal("existing admin password was overwritten")
	}
}

func TestEnsureAdminValidation(t *testing.T) {
	repo := memory.NewUserRepo()
	for _, c := range []struct{ email, pw string }{
		{"not-an-email", "long-enough-password"},
		{"Bob <b@example.com>", "long-enough-password"},
		{"a@example.com", "short"},
	} {
		if _, err := EnsureAdmin(context.Background(), repo, c.email, c.pw); err == nil {
			t.Errorf("expected error for %+v", c)
		}
	}
}
