package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"tradepulse/internal/domain/session"
)

type SessionRepo struct{ db *pgxpool.Pool }

func NewSessionRepo(db *pgxpool.Pool) *SessionRepo { return &SessionRepo{db: db} }

const tokenCols = `id, user_id, family_id, token_hash, created_at, expires_at, used_at, revoked_at`

func scanToken(row scanner) (session.Token, error) {
	var t session.Token
	err := row.Scan(&t.ID, &t.UserID, &t.FamilyID, &t.Hash, &t.CreatedAt, &t.ExpiresAt, &t.UsedAt, &t.RevokedAt)
	return t, err
}

func (r *SessionRepo) Create(ctx context.Context, t session.Token) error {
	_, err := r.db.Exec(ctx, `INSERT INTO refresh_tokens (id, user_id, family_id, token_hash, created_at, expires_at) VALUES ($1,$2,$3,$4,$5,$6)`,
		t.ID, t.UserID, t.FamilyID, t.Hash, t.CreatedAt, t.ExpiresAt)
	return err
}

// Rotate is one atomic UPDATE: exactly one concurrent caller can consume a token.
func (r *SessionRepo) Rotate(ctx context.Context, hash string, now time.Time) (session.Token, error) {
	t, err := scanToken(r.db.QueryRow(ctx,
		`UPDATE refresh_tokens SET used_at = $2 WHERE token_hash = $1 AND used_at IS NULL AND revoked_at IS NULL AND expires_at > $2 RETURNING `+tokenCols,
		hash, now))
	if err == nil {
		return t, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return session.Token{}, err
	}
	old, err := scanToken(r.db.QueryRow(ctx, `SELECT `+tokenCols+` FROM refresh_tokens WHERE token_hash = $1`, hash))
	if errors.Is(err, pgx.ErrNoRows) {
		return session.Token{}, session.ErrInvalid
	}
	if err != nil {
		return session.Token{}, err
	}
	if old.UsedAt != nil {
		return old, session.ErrReused
	}
	return session.Token{}, session.ErrInvalid
}

func (r *SessionRepo) FamilyOf(ctx context.Context, hash string) (string, error) {
	var f string
	err := r.db.QueryRow(ctx, `SELECT family_id FROM refresh_tokens WHERE token_hash = $1`, hash).Scan(&f)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", session.ErrInvalid
	}
	return f, err
}

func (r *SessionRepo) RevokeFamily(ctx context.Context, familyID string, now time.Time) error {
	_, err := r.db.Exec(ctx, `UPDATE refresh_tokens SET revoked_at = $2 WHERE family_id = $1 AND revoked_at IS NULL`, familyID, now)
	return err
}

func (r *SessionRepo) RevokeUser(ctx context.Context, userID string, now time.Time) error {
	_, err := r.db.Exec(ctx, `UPDATE refresh_tokens SET revoked_at = $2 WHERE user_id = $1 AND revoked_at IS NULL`, userID, now)
	return err
}

func (r *SessionRepo) Prune(ctx context.Context, before time.Time) (int64, error) {
	tag, err := r.db.Exec(ctx, `DELETE FROM refresh_tokens WHERE expires_at < $1`, before)
	return tag.RowsAffected(), err
}
