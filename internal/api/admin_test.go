package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"tradepulse/internal/domain/invite"
)

func TestRegistrationRequiresValidInvite(t *testing.T) {
	_, h := newServer(t, nil)
	body := func(code string) string {
		return `{"email":"new@example.com","password":"correct-horse-battery","invite_code":"` + code + `"}`
	}
	if rec := do(h, "POST", "/api/v1/auth/register", "", `{"email":"new@example.com","password":"correct-horse-battery"}`); rec.Code < 400 || rec.Code >= 500 {
		t.Errorf("missing invite_code: got %d, want 4xx", rec.Code)
	}
	if rec := do(h, "POST", "/api/v1/auth/register", "", body("tp_inv_totally-made-up-code")); rec.Code != http.StatusForbidden {
		t.Errorf("unknown code: got %d, want 403", rec.Code)
	}
	if rec := do(h, "POST", "/api/v1/auth/register", "", body(inviteCode(t))); rec.Code != http.StatusCreated {
		t.Errorf("valid code: got %d %s", rec.Code, rec.Body)
	}
}

func TestInviteIsSingleUseOverHTTP(t *testing.T) {
	_, h := newServer(t, nil)
	code := inviteCode(t)
	mk := func(email string) string {
		return `{"email":"` + email + `","password":"correct-horse-battery","invite_code":"` + code + `"}`
	}
	if rec := do(h, "POST", "/api/v1/auth/register", "", mk("first@example.com")); rec.Code != http.StatusCreated {
		t.Fatalf("first use: %d", rec.Code)
	}
	if rec := do(h, "POST", "/api/v1/auth/register", "", mk("second@example.com")); rec.Code != http.StatusForbidden {
		t.Fatalf("reuse must be 403, got %d", rec.Code)
	}
}

func TestFailedRegistrationDoesNotBurnInvite(t *testing.T) {
	_, h := newServer(t, nil)
	register(t, h, "taken@example.com")
	code := inviteCode(t)
	dup := do(h, "POST", "/api/v1/auth/register", "", `{"email":"taken@example.com","password":"correct-horse-battery","invite_code":"`+code+`"}`)
	if dup.Code != http.StatusConflict {
		t.Fatalf("duplicate email: %d", dup.Code)
	}
	ok := do(h, "POST", "/api/v1/auth/register", "", `{"email":"other@example.com","password":"correct-horse-battery","invite_code":"`+code+`"}`)
	if ok.Code != http.StatusCreated {
		t.Fatalf("invite was burned by a failed registration: %d", ok.Code)
	}
}

func TestEmailBoundInviteOnlyWorksForThatEmail(t *testing.T) {
	_, h := newServer(t, nil)
	_, admin := adminToken(t)
	rec := do(h, "POST", "/api/v1/admin/invites", admin, `{"email":"asha@example.com"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create invite: %d %s", rec.Code, rec.Body)
	}
	var out struct{ Code string }
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	wrong := do(h, "POST", "/api/v1/auth/register", "", `{"email":"mallory@example.com","password":"correct-horse-battery","invite_code":"`+out.Code+`"}`)
	if wrong.Code != http.StatusForbidden {
		t.Fatalf("wrong email must be 403, got %d", wrong.Code)
	}
	right := do(h, "POST", "/api/v1/auth/register", "", `{"email":"ASHA@example.com","password":"correct-horse-battery","invite_code":"`+out.Code+`"}`)
	if right.Code != http.StatusCreated {
		t.Fatalf("bound email must work (case-insensitive), got %d", right.Code)
	}
}

func TestAdminRoutesRejectMembersAndAnonymous(t *testing.T) {
	_, h := newServer(t, nil)
	member := register(t, h, "member@example.com")
	routes := []struct{ method, path string }{
		{"POST", "/api/v1/admin/invites"}, {"GET", "/api/v1/admin/invites"}, {"DELETE", "/api/v1/admin/invites/x"},
		{"GET", "/api/v1/admin/users"}, {"POST", "/api/v1/admin/users/x/suspend"},
		{"POST", "/api/v1/admin/users/x/reactivate"}, {"GET", "/api/v1/admin/audit"},
	}
	for _, r := range routes {
		if rec := do(h, r.method, r.path, member, `{}`); rec.Code != http.StatusForbidden {
			t.Errorf("member %s %s: got %d, want 403", r.method, r.path, rec.Code)
		}
		if rec := do(h, r.method, r.path, "", `{}`); rec.Code != http.StatusUnauthorized {
			t.Errorf("anonymous %s %s: got %d, want 401", r.method, r.path, rec.Code)
		}
	}
}

func TestInviteListNeverExposesCodes(t *testing.T) {
	_, h := newServer(t, nil)
	_, admin := adminToken(t)
	rec := do(h, "POST", "/api/v1/admin/invites", admin, `{"note":"for asha"}`)
	var out struct{ Code string }
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if !strings.HasPrefix(out.Code, invite.CodePrefix) {
		t.Fatalf("unexpected code %q", out.Code)
	}
	list := do(h, "GET", "/api/v1/admin/invites", admin, "")
	if strings.Contains(list.Body.String(), out.Code) || strings.Contains(list.Body.String(), invite.HashCode(out.Code)) {
		t.Fatal("invite list leaks the code or its hash")
	}
	if !strings.Contains(list.Body.String(), `"pending"`) {
		t.Fatalf("expected pending status: %s", list.Body)
	}
}

func TestRevokeInvite(t *testing.T) {
	_, h := newServer(t, nil)
	_, admin := adminToken(t)
	var created struct{ ID, Code string }
	_ = json.Unmarshal(do(h, "POST", "/api/v1/admin/invites", admin, `{}`).Body.Bytes(), &created)
	if rec := do(h, "DELETE", "/api/v1/admin/invites/"+created.ID, admin, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("revoke: %d", rec.Code)
	}
	if rec := do(h, "POST", "/api/v1/auth/register", "", `{"email":"r@example.com","password":"correct-horse-battery","invite_code":"`+created.Code+`"}`); rec.Code != http.StatusForbidden {
		t.Fatalf("revoked invite must not work: %d", rec.Code)
	}
	if rec := do(h, "DELETE", "/api/v1/admin/invites/"+created.ID, admin, ""); rec.Code != http.StatusConflict {
		t.Fatalf("second revoke: %d, want 409", rec.Code)
	}
	if rec := do(h, "DELETE", "/api/v1/admin/invites/nope", admin, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown invite: %d, want 404", rec.Code)
	}
}

func TestSuspensionIsImmediateAndReversible(t *testing.T) {
	s, h := newServer(t, nil)
	_, admin := adminToken(t)
	member := register(t, h, "victim@example.com")
	claims, _ := s.JWT.Verify(member)

	if rec := do(h, "GET", "/api/v1/me/feed", member, ""); rec.Code != http.StatusOK {
		t.Fatalf("before suspension: %d", rec.Code)
	}
	if rec := do(h, "POST", "/api/v1/admin/users/"+claims.Sub+"/suspend", admin, ""); rec.Code != http.StatusOK {
		t.Fatalf("suspend: %d %s", rec.Code, rec.Body)
	}
	if rec := do(h, "GET", "/api/v1/me/feed", member, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("suspended token must be rejected, got %d", rec.Code)
	}
	login := do(h, "POST", "/api/v1/auth/login", "", `{"email":"victim@example.com","password":"correct-horse-battery"}`)
	if login.Code != http.StatusForbidden {
		t.Fatalf("suspended login: %d, want 403", login.Code)
	}
	if rec := do(h, "POST", "/api/v1/admin/users/"+claims.Sub+"/reactivate", admin, ""); rec.Code != http.StatusOK {
		t.Fatalf("reactivate: %d", rec.Code)
	}
	if rec := do(h, "GET", "/api/v1/me/feed", member, ""); rec.Code != http.StatusOK {
		t.Fatalf("after reactivation: %d", rec.Code)
	}
}

func TestAdminCannotSuspendSelfAndUnknownUser404(t *testing.T) {
	_, h := newServer(t, nil)
	id, admin := adminToken(t)
	if rec := do(h, "POST", "/api/v1/admin/users/"+id+"/suspend", admin, ""); rec.Code != http.StatusConflict {
		t.Fatalf("self-suspend: %d, want 409", rec.Code)
	}
	if rec := do(h, "POST", "/api/v1/admin/users/nope/suspend", admin, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown user: %d, want 404", rec.Code)
	}
}

func TestUserListNeverExposesPasswordHashes(t *testing.T) {
	_, h := newServer(t, nil)
	_, admin := adminToken(t)
	register(t, h, "someone@example.com")
	rec := do(h, "GET", "/api/v1/admin/users", admin, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "someone@example.com") {
		t.Fatalf("unexpected: %d %s", rec.Code, rec.Body)
	}
	if strings.Contains(strings.ToLower(rec.Body.String()), "pbkdf2") || strings.Contains(strings.ToLower(rec.Body.String()), "password") {
		t.Fatal("password hash leaked in user list")
	}
}

func TestAuditTrailRecordsAdminActions(t *testing.T) {
	_, h := newServer(t, nil)
	adminID, admin := adminToken(t)
	var created struct{ ID, Code string }
	_ = json.Unmarshal(do(h, "POST", "/api/v1/admin/invites", admin, `{}`).Body.Bytes(), &created)
	do(h, "DELETE", "/api/v1/admin/invites/"+created.ID, admin, "")
	rec := do(h, "GET", "/api/v1/admin/audit", admin, "")
	body := rec.Body.String()
	for _, want := range []string{"invite.create", "invite.revoke", adminID} {
		if !strings.Contains(body, want) {
			t.Errorf("audit log missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, created.Code) {
		t.Fatal("audit log leaked an invite code")
	}
}

func TestTokenOfDeletedOrUnknownUserIsRejected(t *testing.T) {
	s, h := newServer(t, nil)
	tok, _ := s.JWT.Issue("ghost-user-id") // validly signed, but no such account
	if rec := do(h, "GET", "/api/v1/me/feed", tok, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("got %d, want 401", rec.Code)
	}
}
