package postgres

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Prune keeps at most feedCap newest items per user and drops audit entries older
// than auditMaxAge. It is idempotent and safe to run concurrently.
func Prune(ctx context.Context, db *pgxpool.Pool, feedCap int, auditMaxAge time.Duration) (feedDeleted, auditDeleted int64, err error) {
	tag, err := db.Exec(ctx,
		`DELETE FROM feed_items WHERE id IN (
		   SELECT id FROM (
		     SELECT id, row_number() OVER (PARTITION BY user_id ORDER BY seq DESC) AS rn FROM feed_items
		   ) t WHERE rn > $1)`, feedCap)
	if err != nil {
		return 0, 0, err
	}
	feedDeleted = tag.RowsAffected()
	tag, err = db.Exec(ctx, `DELETE FROM audit_log WHERE at < $1`, time.Now().Add(-auditMaxAge))
	if err != nil {
		return feedDeleted, 0, err
	}
	return feedDeleted, tag.RowsAffected(), nil
}

// RunRetention prunes hourly until ctx is cancelled.
func RunRetention(ctx context.Context, db *pgxpool.Pool, log *slog.Logger, feedCap int, auditMaxAge time.Duration) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			f, a, err := Prune(ctx, db, feedCap, auditMaxAge)
			if err != nil {
				log.Error("retention prune failed", "err", err)
				continue
			}
			log.Info("retention prune", "feed_deleted", f, "audit_deleted", a)
		}
	}
}
