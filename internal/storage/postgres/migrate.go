package postgres

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

const advisoryLockID = 727274 // serializes migrators across instances

type migration struct {
	version  int
	name     string
	up, down string
}

func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return nil, err
	}
	byVersion := map[int]*migration{}
	for _, e := range entries {
		name := e.Name() // 0001_init.up.sql
		parts := strings.SplitN(name, "_", 2)
		v, err := strconv.Atoi(parts[0])
		if err != nil || len(parts) != 2 {
			return nil, fmt.Errorf("bad migration file name %q", name)
		}
		body, err := fs.ReadFile(migrationFS, "migrations/"+name)
		if err != nil {
			return nil, err
		}
		m := byVersion[v]
		if m == nil {
			m = &migration{version: v}
			byVersion[v] = m
		}
		switch {
		case strings.HasSuffix(name, ".up.sql"):
			m.up, m.name = string(body), strings.TrimSuffix(parts[1], ".up.sql")
		case strings.HasSuffix(name, ".down.sql"):
			m.down = string(body)
		default:
			return nil, fmt.Errorf("bad migration file name %q", name)
		}
	}
	out := make([]migration, 0, len(byVersion))
	for _, m := range byVersion {
		if m.up == "" || m.down == "" {
			return nil, fmt.Errorf("migration %04d needs both .up.sql and .down.sql", m.version)
		}
		out = append(out, *m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, nil
}

func lockedConn(ctx context.Context, pool *pgxpool.Pool) (*pgxpool.Conn, func(), error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, nil, err
	}
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", advisoryLockID); err != nil {
		conn.Release()
		return nil, nil, err
	}
	release := func() {
		_, _ = conn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", advisoryLockID)
		conn.Release()
	}
	return conn, release, nil
}

// RLS is enabled on every table (managed providers such as Supabase expose the
// public schema through an HTTP API; without RLS the tables would be readable).
const createVersionTable = `CREATE TABLE IF NOT EXISTS schema_migrations (
	version int PRIMARY KEY, name text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now());
ALTER TABLE schema_migrations ENABLE ROW LEVEL SECURITY;`

func appliedVersions(ctx context.Context, conn *pgxpool.Conn) (map[int]bool, error) {
	rows, err := conn.Query(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int]bool{}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out[v] = true
	}
	return out, rows.Err()
}

// Migrate applies all pending migrations in order, each in its own transaction.
// Safe to run concurrently from several instances and safe to run repeatedly.
func Migrate(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) error {
	migs, err := loadMigrations()
	if err != nil {
		return err
	}
	conn, release, err := lockedConn(ctx, pool)
	if err != nil {
		return err
	}
	defer release()
	if _, err := conn.Exec(ctx, createVersionTable); err != nil {
		return err
	}
	applied, err := appliedVersions(ctx, conn)
	if err != nil {
		return err
	}
	for _, m := range migs {
		if applied[m.version] {
			continue
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, m.up); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("migration %04d_%s failed: %w", m.version, m.name, err)
		}
		if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version, name) VALUES ($1, $2)", m.version, m.name); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		log.Info("migration applied", "version", m.version, "name", m.name)
	}
	return nil
}

// MigrateDown reverts every applied migration, newest first. DESTRUCTIVE: tests
// and emergency rollback only; never called by the server.
func MigrateDown(ctx context.Context, pool *pgxpool.Pool) error {
	migs, err := loadMigrations()
	if err != nil {
		return err
	}
	conn, release, err := lockedConn(ctx, pool)
	if err != nil {
		return err
	}
	defer release()
	if _, err := conn.Exec(ctx, createVersionTable); err != nil {
		return err
	}
	applied, err := appliedVersions(ctx, conn)
	if err != nil {
		return err
	}
	for i := len(migs) - 1; i >= 0; i-- {
		m := migs[i]
		if !applied[m.version] {
			continue
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, m.down); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("down %04d_%s failed: %w", m.version, m.name, err)
		}
		if _, err := tx.Exec(ctx, "DELETE FROM schema_migrations WHERE version = $1", m.version); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}
