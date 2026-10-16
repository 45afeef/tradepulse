package api

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"tradepulse/internal/domain/audit"
	"tradepulse/internal/domain/invite"
	"tradepulse/internal/domain/user"
	"tradepulse/internal/idgen"
)

// record appends an audit entry (failures are logged, never shown to clients).
func (s *Server) record(ctx context.Context, actor, action, targetType, targetID, detail string) {
	err := s.Audit.Append(ctx, audit.Entry{
		ID: idgen.New(), At: time.Now().UTC(), ActorID: actor,
		Action: action, TargetType: targetType, TargetID: targetID, Detail: detail,
	})
	if err != nil {
		s.Log.Error("audit append failed", "action", action, "err", err)
	}
}

type adminUser struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role" enum:"admin,member"`
	Status    string    `json:"status" enum:"active,suspended"`
	CreatedAt time.Time `json:"created_at"`
}

func toAdminUser(u user.User) adminUser {
	return adminUser{ID: u.ID, Email: u.Email, Role: u.Role, Status: u.Status, CreatedAt: u.CreatedAt}
}

type createInviteBody struct {
	Email          string `json:"email,omitempty" maxLength:"254" doc:"Bind the invite to this email (optional). Empty = usable by any email." example:"friend@example.com"`
	ExpiresInHours int    `json:"expires_in_hours,omitempty" minimum:"1" maximum:"720" doc:"Default 168 (7 days), max 720." example:"168"`
	Note           string `json:"note,omitempty" maxLength:"200" doc:"Private label for your own reference." example:"for Asha"`
}
type createInviteIn struct{ Body createInviteBody }

type inviteCreatedBody struct {
	ID        string    `json:"id"`
	Code      string    `json:"code" doc:"Shown ONCE. Give it to the invitee; it cannot be retrieved later."`
	Email     string    `json:"email,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
}
type inviteCreatedOut struct{ Body inviteCreatedBody }

type inviteItem struct {
	ID        string     `json:"id"`
	Email     string     `json:"email,omitempty"`
	Note      string     `json:"note,omitempty"`
	CreatedBy string     `json:"created_by"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt time.Time  `json:"expires_at"`
	Status    string     `json:"status" enum:"pending,used,expired,revoked"`
	UsedBy    string     `json:"used_by,omitempty"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
}
type inviteListBody struct {
	Invites []inviteItem `json:"invites"`
}
type inviteListOut struct{ Body inviteListBody }

type idIn struct {
	ID string `path:"id" maxLength:"64" doc:"Resource id"`
}

type userListBody struct {
	Users []adminUser `json:"users"`
}
type userListOut struct{ Body userListBody }
type userOut struct{ Body adminUser }

type auditIn struct {
	Limit int `query:"limit" default:"100" minimum:"1" maximum:"500" doc:"Max entries, newest first"`
}
type auditBody struct {
	Entries []audit.Entry `json:"entries"`
}
type auditOut struct{ Body auditBody }

func (s *Server) createInvite(ctx context.Context, in *createInviteIn) (*inviteCreatedOut, error) {
	email := ""
	if e := strings.TrimSpace(in.Body.Email); e != "" {
		addr, err := mail.ParseAddress(e)
		if err != nil || addr.Address != e {
			return nil, huma.Error422UnprocessableEntity("email is not valid")
		}
		email = strings.ToLower(e)
	}
	hours := in.Body.ExpiresInHours
	if hours == 0 {
		hours = 168
	}
	code, hash, err := invite.NewCode()
	if err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	now := time.Now().UTC()
	inv := invite.Invite{
		ID: idgen.New(), CodeHash: hash, Email: email, Note: in.Body.Note, CreatedBy: userID(ctx),
		CreatedAt: now, ExpiresAt: now.Add(time.Duration(hours) * time.Hour),
	}
	if err := s.Invites.Create(ctx, inv); err != nil {
		s.Log.Error("create invite", "err", err)
		return nil, huma.Error500InternalServerError("internal server error")
	}
	s.record(ctx, userID(ctx), "invite.create", "invite", inv.ID, "") // never log the code
	return &inviteCreatedOut{Body: inviteCreatedBody{ID: inv.ID, Code: code, Email: email, ExpiresAt: inv.ExpiresAt}}, nil
}

func (s *Server) listInvites(ctx context.Context, _ *struct{}) (*inviteListOut, error) {
	invs, err := s.Invites.List(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	now := time.Now().UTC()
	items := make([]inviteItem, 0, len(invs))
	for _, i := range invs {
		items = append(items, inviteItem{
			ID: i.ID, Email: i.Email, Note: i.Note, CreatedBy: i.CreatedBy, CreatedAt: i.CreatedAt,
			ExpiresAt: i.ExpiresAt, Status: i.Status(now), UsedBy: i.UsedBy, UsedAt: i.UsedAt,
		})
	}
	return &inviteListOut{Body: inviteListBody{Invites: items}}, nil
}

func (s *Server) revokeInvite(ctx context.Context, in *idIn) (*struct{}, error) {
	err := s.Invites.Revoke(ctx, in.ID, time.Now().UTC())
	switch {
	case errors.Is(err, invite.ErrNotFound):
		return nil, huma.Error404NotFound("invite not found")
	case errors.Is(err, invite.ErrInvalid):
		return nil, huma.Error409Conflict("only pending invites can be revoked")
	case err != nil:
		return nil, huma.Error500InternalServerError("internal server error")
	}
	s.record(ctx, userID(ctx), "invite.revoke", "invite", in.ID, "")
	return &struct{}{}, nil
}

func (s *Server) listUsers(ctx context.Context, _ *struct{}) (*userListOut, error) {
	us, err := s.Users.List(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	out := make([]adminUser, 0, len(us))
	for _, u := range us {
		out = append(out, toAdminUser(u))
	}
	return &userListOut{Body: userListBody{Users: out}}, nil
}

func (s *Server) setUserStatus(ctx context.Context, id, status, action string) (*userOut, error) {
	actor := userID(ctx)
	if status == user.StatusSuspended && id == actor {
		return nil, huma.Error409Conflict("you cannot suspend your own account")
	}
	u, err := s.Users.GetByID(ctx, id)
	if errors.Is(err, user.ErrNotFound) {
		return nil, huma.Error404NotFound("user not found")
	} else if err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	if err := s.Users.SetStatus(ctx, id, status); err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	if status == user.StatusSuspended {
		s.Personalizer.RemoveUser(id) // stop matching immediately
		s.Hub.DisconnectUser(id)      // cut live connections
	} else {
		s.Personalizer.UpsertUser(id, u.Profile)
	}
	s.record(ctx, actor, action, "user", id, "")
	u.Status = status
	return &userOut{Body: toAdminUser(u)}, nil
}

func (s *Server) suspendUser(ctx context.Context, in *idIn) (*userOut, error) {
	return s.setUserStatus(ctx, in.ID, user.StatusSuspended, "user.suspend")
}

func (s *Server) reactivateUser(ctx context.Context, in *idIn) (*userOut, error) {
	return s.setUserStatus(ctx, in.ID, user.StatusActive, "user.reactivate")
}

func (s *Server) listAudit(ctx context.Context, in *auditIn) (*auditOut, error) {
	es, err := s.Audit.Recent(ctx, in.Limit)
	if err != nil {
		return nil, huma.Error500InternalServerError("internal server error")
	}
	return &auditOut{Body: auditBody{Entries: es}}, nil
}
