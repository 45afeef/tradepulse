package memory

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"tradepulse/internal/domain/feed"
	"tradepulse/internal/domain/user"
)

func TestUserRepoEmailUniqueCaseInsensitive(t *testing.T) {
	r := NewUserRepo()
	ctx := context.Background()
	if err := r.Create(ctx, user.User{ID: "1", Email: "A@x.com"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Create(ctx, user.User{ID: "2", Email: "a@X.com"}); !errors.Is(err, user.ErrEmailTaken) {
		t.Fatalf("expected ErrEmailTaken, got %v", err)
	}
	if u, err := r.GetByEmail(ctx, "A@X.COM"); err != nil || u.ID != "1" {
		t.Fatalf("lookup failed: %v", err)
	}
	if _, err := r.GetByID(ctx, "nope"); !errors.Is(err, user.ErrNotFound) {
		t.Fatal("expected ErrNotFound")
	}
	if err := r.UpdateProfile(ctx, "nope", user.Profile{}); !errors.Is(err, user.ErrNotFound) {
		t.Fatal("expected ErrNotFound on update")
	}
}

func TestFeedRepoCapOrderAndIsolation(t *testing.T) {
	r := NewFeedRepo(3)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		_ = r.Add(ctx, feed.FeedItem{ID: fmt.Sprint(i), UserID: "alice"})
	}
	_ = r.Add(ctx, feed.FeedItem{ID: "b0", UserID: "bob"})
	got, _ := r.Recent(ctx, "alice", 10)
	if len(got) != 3 || got[0].ID != "4" || got[2].ID != "2" {
		t.Fatalf("cap/order wrong: %+v", got)
	}
	for _, it := range got {
		if it.UserID != "alice" {
			t.Fatal("cross-user leak")
		}
	}
	if none, _ := r.Recent(ctx, "carol", 10); len(none) != 0 {
		t.Fatal("unknown user must have empty feed")
	}
}
