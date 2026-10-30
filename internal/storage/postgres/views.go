package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"tradepulse/internal/domain/feedback"
	"tradepulse/internal/domain/view"
)

type ViewRepo struct{ db *pgxpool.Pool }

func NewViewRepo(db *pgxpool.Pool) *ViewRepo { return &ViewRepo{db: db} }

const viewCols = `id, user_id, name, filter, created_at, updated_at`

func scanView(row scanner) (view.View, error) {
	var v view.View
	var f []byte
	if err := row.Scan(&v.ID, &v.UserID, &v.Spec.Name, &f, &v.CreatedAt, &v.UpdatedAt); err != nil {
		return v, err
	}
	return v, json.Unmarshal(f, &v.Spec.Filter)
}

func (r *ViewRepo) Create(ctx context.Context, v view.View) error {
	f, err := json.Marshal(v.Spec.Filter)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `INSERT INTO views (`+viewCols+`) VALUES ($1,$2,$3,$4,$5,$6)`, v.ID, v.UserID, v.Spec.Name, string(f), v.CreatedAt, v.UpdatedAt)
	if isUniqueViolation(err) {
		return view.ErrDuplicate
	}
	return err
}

func (r *ViewRepo) List(ctx context.Context, userID string) ([]view.View, error) {
	rows, err := r.db.Query(ctx, `SELECT `+viewCols+` FROM views WHERE user_id = $1 ORDER BY created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []view.View{}
	for rows.Next() {
		v, err := scanView(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *ViewRepo) Get(ctx context.Context, userID, id string) (view.View, error) {
	v, err := scanView(r.db.QueryRow(ctx, `SELECT `+viewCols+` FROM views WHERE id = $1 AND user_id = $2`, id, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return v, view.ErrNotFound
	}
	return v, err
}

func (r *ViewRepo) Update(ctx context.Context, v view.View) error {
	f, err := json.Marshal(v.Spec.Filter)
	if err != nil {
		return err
	}
	tag, err := r.db.Exec(ctx, `UPDATE views SET name = $3, filter = $4, updated_at = $5 WHERE id = $1 AND user_id = $2`, v.ID, v.UserID, v.Spec.Name, string(f), v.UpdatedAt)
	if isUniqueViolation(err) {
		return view.ErrDuplicate
	}
	if err == nil && tag.RowsAffected() == 0 {
		return view.ErrNotFound
	}
	return err
}

func (r *ViewRepo) Delete(ctx context.Context, userID, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM views WHERE id = $1 AND user_id = $2`, id, userID)
	if err == nil && tag.RowsAffected() == 0 {
		return view.ErrNotFound
	}
	return err
}

type FeedbackRepo struct{ db *pgxpool.Pool }

func NewFeedbackRepo(db *pgxpool.Pool) *FeedbackRepo { return &FeedbackRepo{db: db} }

func (r *FeedbackRepo) Upsert(ctx context.Context, e feedback.Entry) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO feedback (user_id, target_type, target_id, rating, note, grp, label, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$8)
		 ON CONFLICT (user_id, target_type, target_id) DO UPDATE SET rating = EXCLUDED.rating, note = EXCLUDED.note,
		   grp = EXCLUDED.grp, label = EXCLUDED.label, updated_at = EXCLUDED.updated_at`,
		e.UserID, e.TargetType, e.TargetID, e.Rating, e.Note, e.Group, e.Label, e.UpdatedAt)
	return err
}

func (r *FeedbackRepo) Delete(ctx context.Context, userID, targetType, targetID string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM feedback WHERE user_id = $1 AND target_type = $2 AND target_id = $3`, userID, targetType, targetID)
	return err
}

func (r *FeedbackRepo) Ratings(ctx context.Context, userID, targetType string, ids []string) (map[string]string, error) {
	out := map[string]string{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.db.Query(ctx, `SELECT target_id, rating FROM feedback WHERE user_id = $1 AND target_type = $2 AND target_id = ANY($3)`, userID, targetType, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, rating string
		if err := rows.Scan(&id, &rating); err != nil {
			return nil, err
		}
		out[id] = rating
	}
	return out, rows.Err()
}

func (r *FeedbackRepo) Summary(ctx context.Context, since time.Time) (feedback.Summary, error) {
	var s feedback.Summary
	rows, err := r.db.Query(ctx, `SELECT target_type, rating, count(*) FROM feedback WHERE updated_at >= $1 GROUP BY 1, 2`, since)
	if err != nil {
		return s, err
	}
	for rows.Next() {
		var tt, rating string
		var n int
		if err := rows.Scan(&tt, &rating, &n); err != nil {
			rows.Close()
			return s, err
		}
		t := &s.FeedItems
		if tt == feedback.TargetAlertFire {
			t = &s.AlertFires
		}
		if rating == feedback.Useful {
			t.Useful += n
		} else {
			t.NotUseful += n
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return s, err
	}
	g, err := r.db.Query(ctx,
		`SELECT target_type, grp, max(label), sum(CASE WHEN rating = 'useful' THEN 1 ELSE 0 END), sum(CASE WHEN rating = 'not_useful' THEN 1 ELSE 0 END)
		 FROM feedback WHERE updated_at >= $1 GROUP BY target_type, grp ORDER BY count(*) DESC, grp LIMIT 20`, since)
	if err != nil {
		return s, err
	}
	defer g.Close()
	for g.Next() {
		var st feedback.GroupStat
		if err := g.Scan(&st.TargetType, &st.Group, &st.Label, &st.Useful, &st.NotUseful); err != nil {
			return s, err
		}
		s.Groups = append(s.Groups, st)
	}
	return s, g.Err()
}

func (r *FeedbackRepo) Prune(ctx context.Context, before time.Time) (int64, error) {
	tag, err := r.db.Exec(ctx, `DELETE FROM feedback WHERE updated_at < $1`, before)
	return tag.RowsAffected(), err
}
