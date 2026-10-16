// Package postgres implements the storage interfaces on PostgreSQL (pgx).
// Rules: parameterized SQL only (never string-built), no SQL outside this package.
package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// prepareConfig builds the pool config. Managed providers (e.g. Supabase) offer
// transaction-mode poolers (PgBouncer/Supavisor, usually port 6543) that do not
// support prepared statements, so simple protocol is used for them:
//
//	mode "true"  -> always simple protocol
//	mode "false" -> never
//	mode "auto"  -> simple when the URL has pgbouncer=true or port 6543
//
// The Prisma-style "pgbouncer" query parameter is removed (the server would reject
// it as an unknown startup parameter).
func prepareConfig(url string, maxConns int, mode string) (*pgxpool.Config, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, errors.New("invalid DATABASE_URL") // never echo the URL: it contains the password
	}
	viaPooler := false
	if v, ok := cfg.ConnConfig.RuntimeParams["pgbouncer"]; ok {
		delete(cfg.ConnConfig.RuntimeParams, "pgbouncer")
		viaPooler = strings.EqualFold(v, "true")
	}
	simple := false
	switch strings.ToLower(mode) {
	case "true":
		simple = true
	case "false":
	default:
		simple = viaPooler || cfg.ConnConfig.Port == 6543
	}
	if simple {
		cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	}
	if maxConns < 1 {
		maxConns = 10
	}
	cfg.MaxConns = int32(maxConns)
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.HealthCheckPeriod = time.Minute
	cfg.ConnConfig.ConnectTimeout = 10 * time.Second
	return cfg, nil
}

// Open connects and waits briefly for the database to accept connections. Errors
// never include the URL or driver details that could contain credentials.
func Open(ctx context.Context, url string, maxConns int, mode string) (*pgxpool.Pool, error) {
	cfg, err := prepareConfig(url, maxConns, mode)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, errors.New("cannot create database pool")
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err = pool.Ping(pctx)
		cancel()
		if err == nil {
			return pool, nil
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			pool.Close()
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, errors.New("database not reachable")
		}
		select {
		case <-ctx.Done():
			pool.Close()
			return nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}
