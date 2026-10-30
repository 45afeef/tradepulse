package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"tradepulse/internal/domain/alert"
	"tradepulse/internal/domain/rule"
	"tradepulse/internal/page"
)

func jsonStr(v any) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return alert.ErrNotFound
	}
	return err
}

func affected(tag interface{ RowsAffected() int64 }, err error) error {
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return alert.ErrNotFound
	}
	return nil
}

// ---------------- rules ----------------

type AlertRuleRepo struct{ db *pgxpool.Pool }

func NewAlertRuleRepo(db *pgxpool.Pool) *AlertRuleRepo { return &AlertRuleRepo{db: db} }

const ruleCols = `id, user_id, name, scope, types, conditions, cooldown_seconds, severity, channels, enabled, paused_reason, created_at, updated_at, last_fired_at`

func scanRule(row scanner) (alert.Rule, error) {
	var r alert.Rule
	var scope, types, conds, chans []byte
	if err := row.Scan(&r.ID, &r.UserID, &r.Spec.Name, &scope, &types, &conds, &r.Spec.CooldownSeconds, &r.Spec.Severity, &chans,
		&r.Spec.Enabled, &r.PausedReason, &r.CreatedAt, &r.UpdatedAt, &r.LastFiredAt); err != nil {
		return alert.Rule{}, err
	}
	if err := json.Unmarshal(scope, &r.Spec.Scope); err != nil {
		return r, err
	}
	if err := json.Unmarshal(types, &r.Spec.Types); err != nil {
		return r, err
	}
	var cs []rule.Condition
	if err := json.Unmarshal(conds, &cs); err != nil {
		return r, err
	}
	r.Spec.Conditions = cs
	return r, json.Unmarshal(chans, &r.Spec.Channels)
}

func ruleJSON(sp alert.Spec) (scope, types, conds, chans string, err error) {
	if sp.Types == nil {
		sp.Types = []string{}
	}
	if sp.Conditions == nil {
		sp.Conditions = []rule.Condition{}
	}
	if sp.Channels == nil {
		sp.Channels = []string{alert.InApp}
	}
	if scope, err = jsonStr(sp.Scope); err != nil {
		return
	}
	if types, err = jsonStr(sp.Types); err != nil {
		return
	}
	if conds, err = jsonStr(sp.Conditions); err != nil {
		return
	}
	chans, err = jsonStr(sp.Channels)
	return
}

func (r *AlertRuleRepo) Create(ctx context.Context, x alert.Rule) error {
	sc, ty, co, ch, err := ruleJSON(x.Spec)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx,
		`INSERT INTO alert_rules (`+ruleCols+`) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		x.ID, x.UserID, x.Spec.Name, sc, ty, co, x.Spec.CooldownSeconds, x.Spec.Severity, ch, x.Spec.Enabled, x.PausedReason, x.CreatedAt, x.UpdatedAt, x.LastFiredAt)
	return err
}

func (r *AlertRuleRepo) Get(ctx context.Context, userID, id string) (alert.Rule, error) {
	x, err := scanRule(r.db.QueryRow(ctx, `SELECT `+ruleCols+` FROM alert_rules WHERE id = $1 AND user_id = $2`, id, userID))
	return x, notFound(err)
}

func (r *AlertRuleRepo) list(ctx context.Context, where string, args ...any) ([]alert.Rule, error) {
	rows, err := r.db.Query(ctx, `SELECT `+ruleCols+` FROM alert_rules `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []alert.Rule{}
	for rows.Next() {
		x, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (r *AlertRuleRepo) List(ctx context.Context, userID string) ([]alert.Rule, error) {
	return r.list(ctx, `WHERE user_id = $1 ORDER BY created_at`, userID)
}

func (r *AlertRuleRepo) ListEnabled(ctx context.Context) ([]alert.Rule, error) {
	return r.list(ctx, `WHERE enabled`)
}

func (r *AlertRuleRepo) Update(ctx context.Context, x alert.Rule) error {
	sc, ty, co, ch, err := ruleJSON(x.Spec)
	if err != nil {
		return err
	}
	return affected(r.db.Exec(ctx,
		`UPDATE alert_rules SET name=$3, scope=$4, types=$5, conditions=$6, cooldown_seconds=$7, severity=$8, channels=$9,
		   enabled=$10, paused_reason=$11, updated_at=$12 WHERE id=$1 AND user_id=$2`,
		x.ID, x.UserID, x.Spec.Name, sc, ty, co, x.Spec.CooldownSeconds, x.Spec.Severity, ch, x.Spec.Enabled, x.PausedReason, x.UpdatedAt))
}

func (r *AlertRuleRepo) Delete(ctx context.Context, userID, id string) error {
	return affected(r.db.Exec(ctx, `DELETE FROM alert_rules WHERE id=$1 AND user_id=$2`, id, userID))
}

func (r *AlertRuleRepo) MarkFired(ctx context.Context, id string, at time.Time) error {
	return affected(r.db.Exec(ctx, `UPDATE alert_rules SET last_fired_at=$2 WHERE id=$1`, id, at))
}

func (r *AlertRuleRepo) Pause(ctx context.Context, id, reason string) error {
	return affected(r.db.Exec(ctx, `UPDATE alert_rules SET enabled=false, paused_reason=$2 WHERE id=$1`, id, reason))
}

// ---------------- fires ----------------

type AlertFireRepo struct{ db *pgxpool.Pool }

func NewAlertFireRepo(db *pgxpool.Pool) *AlertFireRepo { return &AlertFireRepo{db: db} }

func (r *AlertFireRepo) Add(ctx context.Context, f alert.Fire) error {
	ents := f.Entities
	if ents == nil {
		ents = []string{}
	}
	es, err := jsonStr(ents)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx,
		`INSERT INTO alert_fires (id, rule_id, user_id, event_id, rule_name, severity, title, body, url, source, entities, reason, fired_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		f.ID, f.RuleID, f.UserID, f.EventID, f.RuleName, f.Severity, f.Title, f.Body, f.URL, f.Source, es, f.Reason, f.FiredAt)
	return err
}

func (r *AlertFireRepo) Recent(ctx context.Context, userID string, limit int) ([]alert.Fire, error) {
	if limit < 1 || limit > 500 {
		limit = 50
	}
	return r.Page(ctx, userID, page.Query{Limit: limit})
}

func (r *AlertFireRepo) Page(ctx context.Context, userID string, q page.Query) ([]alert.Fire, error) {
	const cols = `seq, id, rule_id, user_id, event_id, rule_name, severity, title, body, url, source, entities, reason, fired_at`
	var rows pgx.Rows
	var err error
	if q.Asc() {
		rows, err = r.db.Query(ctx, `SELECT `+cols+` FROM alert_fires WHERE user_id=$1 AND seq > $2 ORDER BY seq ASC LIMIT $3`, userID, q.After, q.Limit)
	} else {
		rows, err = r.db.Query(ctx, `SELECT `+cols+` FROM alert_fires WHERE user_id=$1 AND ($2::bigint = 0 OR seq < $2) ORDER BY seq DESC LIMIT $3`, userID, q.Before, q.Limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []alert.Fire{}
	for rows.Next() {
		var f alert.Fire
		var ents []byte
		if err := rows.Scan(&f.Seq, &f.ID, &f.RuleID, &f.UserID, &f.EventID, &f.RuleName, &f.Severity, &f.Title, &f.Body, &f.URL, &f.Source, &ents, &f.Reason, &f.FiredAt); err != nil {
			return nil, err
		}
		f.Cursor = page.Cursor(f.Seq)
		if err := json.Unmarshal(ents, &f.Entities); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (r *AlertFireRepo) Prune(ctx context.Context, olderThan time.Time) (int64, error) {
	tag, err := r.db.Exec(ctx, `DELETE FROM alert_fires WHERE fired_at < $1`, olderThan)
	return tag.RowsAffected(), err
}

// ---------------- channels ----------------

type ChannelRepo struct{ db *pgxpool.Pool }

func NewChannelRepo(db *pgxpool.Pool) *ChannelRepo { return &ChannelRepo{db: db} }

const chanCols = `id, user_id, type, name, url, enabled, secret_version, consecutive_failures, disabled_reason, last_delivery_at, created_at`

func scanChannel(row scanner) (alert.Channel, error) {
	var c alert.Channel
	err := row.Scan(&c.ID, &c.UserID, &c.Type, &c.Name, &c.URL, &c.Enabled, &c.SecretVersion, &c.ConsecutiveFailures, &c.DisabledReason, &c.LastDeliveryAt, &c.CreatedAt)
	return c, err
}

func (r *ChannelRepo) Create(ctx context.Context, c alert.Channel) error {
	if c.SecretVersion == 0 {
		c.SecretVersion = 1
	}
	_, err := r.db.Exec(ctx,
		`INSERT INTO notification_channels (id, user_id, type, name, url, enabled, secret_version, created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		c.ID, c.UserID, c.Type, c.Name, c.URL, c.Enabled, c.SecretVersion, c.CreatedAt)
	return err
}

func (r *ChannelRepo) Get(ctx context.Context, userID, id string) (alert.Channel, error) {
	c, err := scanChannel(r.db.QueryRow(ctx, `SELECT `+chanCols+` FROM notification_channels WHERE id=$1 AND user_id=$2`, id, userID))
	return c, notFound(err)
}

func (r *ChannelRepo) ByID(ctx context.Context, id string) (alert.Channel, error) {
	c, err := scanChannel(r.db.QueryRow(ctx, `SELECT `+chanCols+` FROM notification_channels WHERE id=$1`, id))
	return c, notFound(err)
}

func (r *ChannelRepo) List(ctx context.Context, userID string) ([]alert.Channel, error) {
	rows, err := r.db.Query(ctx, `SELECT `+chanCols+` FROM notification_channels WHERE user_id=$1 ORDER BY created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []alert.Channel{}
	for rows.Next() {
		c, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *ChannelRepo) Delete(ctx context.Context, userID, id string) error {
	return affected(r.db.Exec(ctx, `DELETE FROM notification_channels WHERE id=$1 AND user_id=$2`, id, userID))
}

func (r *ChannelRepo) SetEnabled(ctx context.Context, userID, id string, enabled bool, reason string) error {
	return affected(r.db.Exec(ctx,
		`UPDATE notification_channels SET enabled=$3::boolean, disabled_reason=CASE WHEN $3::boolean THEN '' ELSE $4::text END,
		   consecutive_failures=CASE WHEN $3::boolean THEN 0 ELSE consecutive_failures END WHERE id=$2 AND user_id=$1`,
		userID, id, enabled, reason))
}

func (r *ChannelRepo) BumpSecretVersion(ctx context.Context, userID, id string) (int, error) {
	var v int
	err := r.db.QueryRow(ctx, `UPDATE notification_channels SET secret_version = secret_version + 1 WHERE id=$1 AND user_id=$2 RETURNING secret_version`, id, userID).Scan(&v)
	return v, notFound(err)
}

func (r *ChannelRepo) RecordResult(ctx context.Context, id string, ok bool, at time.Time) (int, error) {
	var n int
	err := r.db.QueryRow(ctx,
		`UPDATE notification_channels SET last_delivery_at=$2,
		   consecutive_failures=CASE WHEN $3::boolean THEN 0 ELSE consecutive_failures + 1 END WHERE id=$1 RETURNING consecutive_failures`,
		id, at, ok).Scan(&n)
	return n, notFound(err)
}

// ---------------- deliveries ----------------

type DeliveryRepo struct{ db *pgxpool.Pool }

func NewDeliveryRepo(db *pgxpool.Pool) *DeliveryRepo { return &DeliveryRepo{db: db} }

func (r *DeliveryRepo) Add(ctx context.Context, d alert.Delivery) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO channel_deliveries (id, channel_id, user_id, fire_id, at, ok, status, attempts, error) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		d.ID, d.ChannelID, d.UserID, d.FireID, d.At, d.OK, d.Status, d.Attempts, d.Error)
	return err
}

func (r *DeliveryRepo) Recent(ctx context.Context, userID string, limit int) ([]alert.Delivery, error) {
	if limit < 1 || limit > 500 {
		limit = 50
	}
	return r.Page(ctx, userID, page.Query{Limit: limit})
}

func (r *DeliveryRepo) Page(ctx context.Context, userID string, q page.Query) ([]alert.Delivery, error) {
	const cols = `seq, id, channel_id, user_id, fire_id, at, ok, status, attempts, error`
	var rows pgx.Rows
	var err error
	if q.Asc() {
		rows, err = r.db.Query(ctx, `SELECT `+cols+` FROM channel_deliveries WHERE user_id=$1 AND seq > $2 ORDER BY seq ASC LIMIT $3`, userID, q.After, q.Limit)
	} else {
		rows, err = r.db.Query(ctx, `SELECT `+cols+` FROM channel_deliveries WHERE user_id=$1 AND ($2::bigint = 0 OR seq < $2) ORDER BY seq DESC LIMIT $3`, userID, q.Before, q.Limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []alert.Delivery{}
	for rows.Next() {
		var d alert.Delivery
		if err := rows.Scan(&d.Seq, &d.ID, &d.ChannelID, &d.UserID, &d.FireID, &d.At, &d.OK, &d.Status, &d.Attempts, &d.Error); err != nil {
			return nil, err
		}
		d.Cursor = page.Cursor(d.Seq)
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *DeliveryRepo) Prune(ctx context.Context, olderThan time.Time) (int64, error) {
	tag, err := r.db.Exec(ctx, `DELETE FROM channel_deliveries WHERE at < $1`, olderThan)
	return tag.RowsAffected(), err
}

func (r *AlertFireRepo) Get(ctx context.Context, userID, id string) (alert.Fire, error) {
	var f alert.Fire
	var ents []byte
	err := r.db.QueryRow(ctx,
		`SELECT seq, id, rule_id, user_id, event_id, rule_name, severity, title, body, url, source, entities, reason, fired_at FROM alert_fires WHERE id = $1 AND user_id = $2`, id, userID).
		Scan(&f.Seq, &f.ID, &f.RuleID, &f.UserID, &f.EventID, &f.RuleName, &f.Severity, &f.Title, &f.Body, &f.URL, &f.Source, &ents, &f.Reason, &f.FiredAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return f, alert.ErrNotFound
	}
	if err != nil {
		return f, err
	}
	f.Cursor = page.Cursor(f.Seq)
	return f, json.Unmarshal(ents, &f.Entities)
}
