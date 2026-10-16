package invite

import (
	"strings"
	"testing"
	"time"
)

func TestNewCodeIsRandomPrefixedAndHashed(t *testing.T) {
	a, ha, err := NewCode()
	if err != nil {
		t.Fatal(err)
	}
	b, _, _ := NewCode()
	if a == b {
		t.Fatal("codes must be unique")
	}
	if !strings.HasPrefix(a, CodePrefix) || len(a) < len(CodePrefix)+30 {
		t.Fatalf("code too short/unprefixed: %q", a)
	}
	if ha == a || ha != HashCode(a) || strings.Contains(ha, a) {
		t.Fatal("hash must differ from the code and be deterministic")
	}
}

func TestStatus(t *testing.T) {
	now := time.Now()
	past, future := now.Add(-time.Hour), now.Add(time.Hour)
	cases := []struct {
		name string
		inv  Invite
		want string
	}{
		{"pending", Invite{ExpiresAt: future}, "pending"},
		{"expired", Invite{ExpiresAt: past}, "expired"},
		{"used", Invite{ExpiresAt: future, UsedAt: &past}, "used"},
		{"revoked beats used", Invite{ExpiresAt: future, UsedAt: &past, RevokedAt: &past}, "revoked"},
	}
	for _, c := range cases {
		if got := c.inv.Status(now); got != c.want {
			t.Errorf("%s: got %s want %s", c.name, got, c.want)
		}
	}
}
