package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"tradepulse/internal/domain/audit"
	"tradepulse/internal/domain/feed"
	"tradepulse/internal/domain/invite"
	"tradepulse/internal/domain/user"
	"tradepulse/internal/page"
)

type scanner interface{ Scan(dest ...any) error }

func isUniqueViolation(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23505"
}

// ---------------- users ----------------

type UserRepo struct{ db *pgxpool.Pool }

func NewUserRepo(db *pgxpool.Pool) *UserRepo { return &UserRepo{db: db} }

const userCols = `id, email, password_hash, role, status, profile, created_at, sessions_valid_after`

func scanUser(row scanner) (user.User, error) {
	var u user.User
	var prof []byte
	if err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &u.Status, &prof, &u.CreatedAt, &u.SessionsValidAfter); err != nil {
		return user.User{}, err
	}
	if err := json.Unmarshal(prof, &u.Profile); err != nil {
		return user.User{}, fmt.Errorf("decode profile: %w", err)
	}
	return u, nil
}

func (r *UserRepo) Create(ctx context.Context, u user.User) error {
	prof, err := json.Marshal(u.Profile)
	if err != nil {
		return err
	}
	if u.Role == "" {
		u.Role = user.RoleMember
	}
	if u.Status == "" {
		u.Status = user.StatusActive
	}
	_, err = r.db.Exec(ctx,
		`INSERT INTO users (id, email, password_hash, role, status, profile, created_at) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		u.ID, u.Email, u.PasswordHash, u.Role, u.Status, string(prof), u.CreatedAt)
	if isUniqueViolation(err) {
		return user.ErrEmailTaken
	}
	return err
}

func (r *UserRepo) GetByID(ctx context.Context, id string) (user.User, error) {
	u, err := scanUser(r.db.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return user.User{}, user.ErrNotFound
	}
	return u, err
}

func (r *UserRepo) GetByEmail(ctx context.Context, email string) (user.User, error) {
	u, err := scanUser(r.db.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE lower(email) = lower($1)`, email))
	if errors.Is(err, pgx.ErrNoRows) {
		return user.User{}, user.ErrNotFound
	}
	return u, err
}

func (r *UserRepo) exec1(ctx context.Context, sql string, args ...any) error {
	tag, err := r.db.Exec(ctx, sql, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return user.ErrNotFound
	}
	return nil
}

func (r *UserRepo) UpdateProfile(ctx context.Context, id string, p user.Profile) error {
	prof, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return r.exec1(ctx, `UPDATE users SET profile = $2 WHERE id = $1`, id, string(prof))
}

func (r *UserRepo) SetStatus(ctx context.Context, id, status string) error {
	return r.exec1(ctx, `UPDATE users SET status = $2 WHERE id = $1`, id, status)
}

func (r *UserRepo) SetPassword(ctx context.Context, id, hash string, validAfter time.Time) error {
	return r.exec1(ctx, `UPDATE users SET password_hash = $2, sessions_valid_after = $3 WHERE id = $1`, id, hash, validAfter)
}

func (r *UserRepo) RevokeSessions(ctx context.Context, id string, at time.Time) error {
	return r.exec1(ctx, `UPDATE users SET sessions_valid_after = $2 WHERE id = $1`, id, at)
}

func (r *UserRepo) List(ctx context.Context) ([]user.User, error) {
	rows, err := r.db.Query(ctx, `SELECT `+userCols+` FROM users ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []user.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// ---------------- invites ----------------

type InviteRepo struct{ db *pgxpool.Pool }

func NewInviteRepo(db *pgxpool.Pool) *InviteRepo { return &InviteRepo{db: db} }

const inviteCols = `id, code_hash, email, note, created_by, created_at, expires_at, used_at, used_by, revoked_at`

func scanInvite(row scanner) (invite.Invite, error) {
	var i invite.Invite
	err := row.Scan(&i.ID, &i.CodeHash, &i.Email, &i.Note, &i.CreatedBy, &i.CreatedAt, &i.ExpiresAt, &i.UsedAt, &i.UsedBy, &i.RevokedAt)
	return i, err
}

func (r *InviteRepo) Create(ctx context.Context, i invite.Invite) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO invites (id, code_hash, email, note, created_by, created_at, expires_at) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		i.ID, i.CodeHash, strings.ToLower(i.Email), i.Note, i.CreatedBy, i.CreatedAt, i.ExpiresAt)
	return err
}

func (r *InviteRepo) List(ctx context.Context) ([]invite.Invite, error) {
	rows, err := r.db.Query(ctx, `SELECT `+inviteCols+` FROM invites ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []invite.Invite
	for rows.Next() {
		i, err := scanInvite(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func (r *InviteRepo) Revoke(ctx context.Context, id string, now time.Time) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE invites SET revoked_at = $2 WHERE id = $1 AND revoked_at IS NULL AND used_at IS NULL AND expires_at > $2`, id, now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	var exists bool
	if err := r.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM invites WHERE id = $1)`, id).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return invite.ErrNotFound
	}
	return invite.ErrInvalid
}

// Redeem is a single atomic UPDATE: exactly one concurrent caller can win.
func (r *InviteRepo) Redeem(ctx context.Context, codeHash, email, userID string, now time.Time) (invite.Invite, error) {
	i, err := scanInvite(r.db.QueryRow(ctx,
		`UPDATE invites SET used_at = $4, used_by = $3
		 WHERE code_hash = $1 AND used_at IS NULL AND revoked_at IS NULL AND expires_at > $4
		   AND (email = '' OR email = lower($2))
		 RETURNING `+inviteCols, codeHash, email, userID, now))
	if errors.Is(err, pgx.ErrNoRows) {
		return invite.Invite{}, invite.ErrInvalid
	}
	return i, err
}

func (r *InviteRepo) Release(ctx context.Context, id, userID string) error {
	_, err := r.db.Exec(ctx, `UPDATE invites SET used_at = NULL, used_by = '' WHERE id = $1 AND used_by = $2`, id, userID)
	return err
}

// ---------------- audit ----------------

type AuditRepo struct{ db *pgxpool.Pool }

func NewAuditRepo(db *pgxpool.Pool) *AuditRepo { return &AuditRepo{db: db} }

func (r *AuditRepo) Append(ctx context.Context, e audit.Entry) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO audit_log (id, at, actor_id, action, target_type, target_id, detail) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		e.ID, e.At, e.ActorID, e.Action, e.TargetType, e.TargetID, e.Detail)
	return err
}

func (r *AuditRepo) Recent(ctx context.Context, limit int) ([]audit.Entry, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	return r.Page(ctx, page.Query{Limit: limit})
}

func (r *AuditRepo) Page(ctx context.Context, q page.Query) ([]audit.Entry, error) {
	const cols = `seq, id, at, actor_id, action, target_type, target_id, detail`
	var rows pgx.Rows
	var err error
	if q.Asc() {
		rows, err = r.db.Query(ctx, `SELECT `+cols+` FROM audit_log WHERE seq > $1 ORDER BY seq ASC LIMIT $2`, q.After, q.Limit)
	} else {
		rows, err = r.db.Query(ctx, `SELECT `+cols+` FROM audit_log WHERE ($1::bigint = 0 OR seq < $1) ORDER BY seq DESC LIMIT $2`, q.Before, q.Limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []audit.Entry{}
	for rows.Next() {
		var e audit.Entry
		if err := rows.Scan(&e.Seq, &e.ID, &e.At, &e.ActorID, &e.Action, &e.TargetType, &e.TargetID, &e.Detail); err != nil {
			return nil, err
		}
		e.Cursor = page.Cursor(e.Seq)
		out = append(out, e)
	}
	return out, rows.Err()
}

// ---------------- feed ----------------

type FeedRepo struct{ db *pgxpool.Pool }

func NewFeedRepo(db *pgxpool.Pool) *FeedRepo { return &FeedRepo{db: db} }

func (r *FeedRepo) Add(ctx context.Context, it feed.FeedItem) error {
	_, err := r.AddItem(ctx, it)
	return err
}

func (r *FeedRepo) AddItem(ctx context.Context, it feed.FeedItem) (feed.FeedItem, error) {
	ents, err := json.Marshal(it.Entities)
	if err != nil {
		return it, err
	}
	if it.Entities == nil {
		ents = []byte("[]")
	}
	err = r.db.QueryRow(ctx,
		`INSERT INTO feed_items (id, user_id, event_id, event_type, source, entities, title, body, url, severity, reason, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING seq`,
		it.ID, it.UserID, it.EventID, it.EventType, it.Source, string(ents), it.Title, it.Body, it.URL, string(it.Severity), it.Reason, it.CreatedAt).Scan(&it.Seq)
	it.Cursor = page.Cursor(it.Seq)
	return it, err
}

func (r *FeedRepo) Recent(ctx context.Context, userID string, limit int) ([]feed.FeedItem, error) {
	if limit < 1 || limit > 500 {
		limit = 50
	}
	return r.Page(ctx, userID, page.Query{Limit: limit})
}

func (r *FeedRepo) Page(ctx context.Context, userID string, q page.Query) ([]feed.FeedItem, error) {
	const cols = `seq, id, user_id, event_id, event_type, source, entities, title, body, url, severity, reason, created_at`
	var rows pgx.Rows
	var err error
	if q.Asc() {
		rows, err = r.db.Query(ctx, `SELECT `+cols+` FROM feed_items WHERE user_id = $1 AND seq > $2 ORDER BY seq ASC LIMIT $3`, userID, q.After, q.Limit)
	} else {
		rows, err = r.db.Query(ctx, `SELECT `+cols+` FROM feed_items WHERE user_id = $1 AND ($2::bigint = 0 OR seq < $2) ORDER BY seq DESC LIMIT $3`, userID, q.Before, q.Limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []feed.FeedItem{}
	for rows.Next() {
		var it feed.FeedItem
		var ents []byte
		var sev string
		if err := rows.Scan(&it.Seq, &it.ID, &it.UserID, &it.EventID, &it.EventType, &it.Source, &ents, &it.Title, &it.Body, &it.URL, &sev, &it.Reason, &it.CreatedAt); err != nil {
			return nil, err
		}
		it.Severity = feed.Severity(sev)
		it.Cursor = page.Cursor(it.Seq)
		if err := json.Unmarshal(ents, &it.Entities); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (r *FeedRepo) Get(ctx context.Context, userID, id string) (feed.FeedItem, error) {
	var it feed.FeedItem
	var ents []byte
	var sev string
	err := r.db.QueryRow(ctx,
		`SELECT seq, id, user_id, event_id, event_type, source, entities, title, body, url, severity, reason, created_at FROM feed_items WHERE id = $1 AND user_id = $2`, id, userID).
		Scan(&it.Seq, &it.ID, &it.UserID, &it.EventID, &it.EventType, &it.Source, &ents, &it.Title, &it.Body, &it.URL, &sev, &it.Reason, &it.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return it, feed.ErrNotFound
	}
	if err != nil {
		return it, err
	}
	it.Severity, it.Cursor = feed.Severity(sev), page.Cursor(it.Seq)
	return it, json.Unmarshal(ents, &it.Entities)
}
