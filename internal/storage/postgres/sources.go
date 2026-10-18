package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"tradepulse/internal/domain/sourcedef"
)

// ---------------- sources ----------------

type SourceRepo struct{ db *pgxpool.Pool }

func NewSourceRepo(db *pgxpool.Pool) *SourceRepo { return &SourceRepo{db: db} }

const sourceCols = `id, name, type, url, enabled, interval_seconds, entities, web, created_at, updated_at,
	last_run_at, last_success_at, next_run_at, last_item_at, last_error, consecutive_failures, unchanged_streak,
	etag, last_modified, items_total, webhook, xcfg, secret_version, cursor`

func scanSource(row scanner) (sourcedef.Source, error) {
	var s sourcedef.Source
	var typ string
	var ents, web, webhook, xcfg []byte
	st := &s.State
	err := row.Scan(&s.ID, &s.Name, &typ, &s.URL, &s.Enabled, &s.IntervalSeconds, &ents, &web, &s.CreatedAt, &s.UpdatedAt,
		&st.LastRunAt, &st.LastSuccessAt, &st.NextRunAt, &st.LastItemAt, &st.LastError, &st.ConsecutiveFailures, &st.UnchangedStreak,
		&st.ETag, &st.LastModified, &st.ItemsTotal, &webhook, &xcfg, &s.SecretVersion, &st.Cursor)
	if err != nil {
		return sourcedef.Source{}, err
	}
	s.Type = sourcedef.Type(typ)
	if err := json.Unmarshal(ents, &s.Entities); err != nil {
		return sourcedef.Source{}, err
	}
	if len(web) > 0 {
		var w sourcedef.WebConfig
		if err := json.Unmarshal(web, &w); err != nil {
			return sourcedef.Source{}, err
		}
		s.Web = &w
	}
	if len(webhook) > 0 {
		var w sourcedef.WebhookConfig
		if err := json.Unmarshal(webhook, &w); err != nil {
			return sourcedef.Source{}, err
		}
		s.Webhook = &w
	}
	if len(xcfg) > 0 {
		var x sourcedef.XConfig
		if err := json.Unmarshal(xcfg, &x); err != nil {
			return sourcedef.Source{}, err
		}
		s.X = &x
	}
	return s, nil
}

type cfgJSON struct {
	ents                string
	web, webhook, xcfg any
}

func encodeConfig(s sourcedef.Source) (cfgJSON, error) {
	if s.Entities == nil {
		s.Entities = []string{}
	}
	eb, err := json.Marshal(s.Entities)
	if err != nil {
		return cfgJSON{}, err
	}
	c := cfgJSON{ents: string(eb)}
	enc := func(v any) (any, error) {
		b, err := json.Marshal(v)
		return string(b), err
	}
	if s.Web != nil {
		if c.web, err = enc(s.Web); err != nil {
			return c, err
		}
	}
	if s.Webhook != nil {
		if c.webhook, err = enc(s.Webhook); err != nil {
			return c, err
		}
	}
	if s.X != nil {
		if c.xcfg, err = enc(s.X); err != nil {
			return c, err
		}
	}
	return c, nil
}

func (r *SourceRepo) Create(ctx context.Context, s sourcedef.Source) error {
	c, err := encodeConfig(s)
	if err != nil {
		return err
	}
	if s.SecretVersion == 0 {
		s.SecretVersion = 1
	}
	_, err = r.db.Exec(ctx,
		`INSERT INTO sources (id, name, type, url, enabled, interval_seconds, entities, web, webhook, xcfg, secret_version, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		s.ID, s.Name, string(s.Type), s.URL, s.Enabled, s.IntervalSeconds, c.ents, c.web, c.webhook, c.xcfg, s.SecretVersion, s.CreatedAt, s.UpdatedAt)
	if isUniqueViolation(err) {
		return sourcedef.ErrNameTaken
	}
	return err
}

func (r *SourceRepo) one(ctx context.Context, where string, arg any) (sourcedef.Source, error) {
	s, err := scanSource(r.db.QueryRow(ctx, `SELECT `+sourceCols+` FROM sources WHERE `+where, arg))
	if errors.Is(err, pgx.ErrNoRows) {
		return sourcedef.Source{}, sourcedef.ErrNotFound
	}
	return s, err
}

func (r *SourceRepo) Get(ctx context.Context, id string) (sourcedef.Source, error) {
	return r.one(ctx, `id = $1`, id)
}

func (r *SourceRepo) GetByName(ctx context.Context, name string) (sourcedef.Source, error) {
	return r.one(ctx, `name = $1`, name)
}

func (r *SourceRepo) List(ctx context.Context) ([]sourcedef.Source, error) {
	rows, err := r.db.Query(ctx, `SELECT `+sourceCols+` FROM sources ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []sourcedef.Source{}
	for rows.Next() {
		s, err := scanSource(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *SourceRepo) Update(ctx context.Context, s sourcedef.Source) error {
	c, err := encodeConfig(s)
	if err != nil {
		return err
	}
	tag, err := r.db.Exec(ctx,
		`UPDATE sources SET url=$2, enabled=$3, interval_seconds=$4, entities=$5, web=$6, webhook=$7, xcfg=$8, updated_at=$9 WHERE id=$1`,
		s.ID, s.URL, s.Enabled, s.IntervalSeconds, c.ents, c.web, c.webhook, c.xcfg, s.UpdatedAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return sourcedef.ErrNotFound
	}
	return nil
}

func (r *SourceRepo) Delete(ctx context.Context, id string) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM sources WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return sourcedef.ErrNotFound
	}
	return nil
}

func (r *SourceRepo) SaveState(ctx context.Context, id string, st sourcedef.State) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE sources SET last_run_at=$2, last_success_at=$3, next_run_at=$4, last_item_at=$5, last_error=$6,
		   consecutive_failures=$7, unchanged_streak=$8, etag=$9, last_modified=$10, items_total=$11, cursor=$12 WHERE id=$1`,
		id, st.LastRunAt, st.LastSuccessAt, st.NextRunAt, st.LastItemAt, st.LastError,
		st.ConsecutiveFailures, st.UnchangedStreak, st.ETag, st.LastModified, st.ItemsTotal, st.Cursor)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return sourcedef.ErrNotFound
	}
	return nil
}

func (r *SourceRepo) BumpSecretVersion(ctx context.Context, id string) (int, error) {
	var v int
	err := r.db.QueryRow(ctx, `UPDATE sources SET secret_version = secret_version + 1 WHERE id = $1 RETURNING secret_version`, id).Scan(&v)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, sourcedef.ErrNotFound
	}
	return v, err
}

// ---------------- usage ----------------

type UsageRepo struct{ db *pgxpool.Pool }

func NewUsageRepo(db *pgxpool.Pool) *UsageRepo { return &UsageRepo{db: db} }

func period(now time.Time) string { return now.UTC().Format("2006-01") }

// Add is atomic across instances (single upsert).
func (u *UsageRepo) Add(ctx context.Context, key string, micro int64, now time.Time) (int64, error) {
	var total int64
	err := u.db.QueryRow(ctx,
		`INSERT INTO usage_monthly (period, key, micro_usd) VALUES ($1,$2,$3)
		 ON CONFLICT (period, key) DO UPDATE SET micro_usd = usage_monthly.micro_usd + EXCLUDED.micro_usd
		 RETURNING micro_usd`, period(now), key, micro).Scan(&total)
	return total, err
}

func (u *UsageRepo) Month(ctx context.Context, key string, now time.Time) (int64, error) {
	var total int64
	err := u.db.QueryRow(ctx, `SELECT COALESCE((SELECT micro_usd FROM usage_monthly WHERE period=$1 AND key=$2), 0)`, period(now), key).Scan(&total)
	return total, err
}

// ---------------- runs ----------------

type RunRepo struct{ db *pgxpool.Pool }

func NewRunRepo(db *pgxpool.Pool) *RunRepo { return &RunRepo{db: db} }

func (r *RunRepo) Add(ctx context.Context, run sourcedef.Run) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO source_runs (id, source_id, started_at, duration_ms, ok, http_status, items_seen, items_new, items_published, error)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		run.ID, run.SourceID, run.StartedAt, run.DurationMS, run.OK, run.HTTPStatus, run.ItemsSeen, run.ItemsNew, run.ItemsPublished, run.Error)
	return err
}

func (r *RunRepo) Recent(ctx context.Context, sourceID string, limit int) ([]sourcedef.Run, error) {
	if limit < 1 || limit > 200 {
		limit = 20
	}
	rows, err := r.db.Query(ctx,
		`SELECT id, source_id, started_at, duration_ms, ok, http_status, items_seen, items_new, items_published, error
		 FROM source_runs WHERE source_id = $1 ORDER BY started_at DESC LIMIT $2`, sourceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []sourcedef.Run{}
	for rows.Next() {
		var x sourcedef.Run
		if err := rows.Scan(&x.ID, &x.SourceID, &x.StartedAt, &x.DurationMS, &x.OK, &x.HTTPStatus, &x.ItemsSeen, &x.ItemsNew, &x.ItemsPublished, &x.Error); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (r *RunRepo) Prune(ctx context.Context, olderThan time.Time) (int64, error) {
	tag, err := r.db.Exec(ctx, `DELETE FROM source_runs WHERE started_at < $1`, olderThan)
	return tag.RowsAffected(), err
}

// ---------------- raw items / dedupe ----------------

type ItemStore struct{ db *pgxpool.Pool }

func NewItemStore(db *pgxpool.Pool) *ItemStore { return &ItemStore{db: db} }

// Insert is idempotent and atomic: exactly one caller ever gets true for a key.
func (s *ItemStore) Insert(ctx context.Context, sourceID, key string, raw []byte, now time.Time) (bool, error) {
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	tag, err := s.db.Exec(ctx,
		`INSERT INTO source_items (source_id, dedupe_key, fetched_at, raw) VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`,
		sourceID, key, now, string(raw))
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func (s *ItemStore) Prune(ctx context.Context, olderThan time.Time) (int64, error) {
	tag, err := s.db.Exec(ctx, `DELETE FROM source_items WHERE fetched_at < $1`, olderThan)
	return tag.RowsAffected(), err
}
