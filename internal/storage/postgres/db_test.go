package postgres

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestPrepareConfigStripsPgbouncerAndUsesSimpleProtocol(t *testing.T) {
	cfg, err := prepareConfig("postgres://u:p@host:5432/db?sslmode=require&pgbouncer=true", 5, "auto")
	if err != nil {
		t.Fatal(err)
	}
	if _, present := cfg.ConnConfig.RuntimeParams["pgbouncer"]; present {
		t.Fatal("pgbouncer param must be removed (server would reject it as a startup parameter)")
	}
	if cfg.ConnConfig.DefaultQueryExecMode != pgx.QueryExecModeSimpleProtocol {
		t.Fatal("pgbouncer=true must select simple protocol")
	}
	if cfg.MaxConns != 5 {
		t.Fatalf("MaxConns = %d", cfg.MaxConns)
	}
}

func TestPrepareConfigAutoDetectsTransactionPoolerPort(t *testing.T) {
	pooled, _ := prepareConfig("postgres://u:p@host:6543/db", 5, "auto")
	if pooled.ConnConfig.DefaultQueryExecMode != pgx.QueryExecModeSimpleProtocol {
		t.Fatal("port 6543 must select simple protocol")
	}
	direct, _ := prepareConfig("postgres://u:p@host:5432/db", 5, "auto")
	if direct.ConnConfig.DefaultQueryExecMode == pgx.QueryExecModeSimpleProtocol {
		t.Fatal("direct connections should keep prepared statements")
	}
}

func TestPrepareConfigExplicitModesOverrideAuto(t *testing.T) {
	on, _ := prepareConfig("postgres://u:p@host:5432/db", 5, "true")
	if on.ConnConfig.DefaultQueryExecMode != pgx.QueryExecModeSimpleProtocol {
		t.Fatal("mode=true must force simple protocol")
	}
	off, _ := prepareConfig("postgres://u:p@host:6543/db", 5, "false")
	if off.ConnConfig.DefaultQueryExecMode == pgx.QueryExecModeSimpleProtocol {
		t.Fatal("mode=false must disable simple protocol")
	}
}

func TestInvalidURLErrorNeverLeaksPassword(t *testing.T) {
	_, err := Open(context.Background(), "postgres://admin:SuperSecret123@host:notaport/db", 5, "auto")
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "SuperSecret123") || strings.Contains(err.Error(), "admin") {
		t.Fatalf("error leaks credentials: %v", err)
	}
}

func TestUnreachableDatabaseErrorNeverLeaksPassword(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
	defer cancel()
	_, err := Open(ctx, "postgres://admin:SuperSecret123@127.0.0.1:1/db?sslmode=disable", 5, "auto")
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "SuperSecret123") {
		t.Fatalf("error leaks the password: %v", err)
	}
}

func TestMigrationFilesAreComplete(t *testing.T) {
	migs, err := loadMigrations()
	if err != nil || len(migs) == 0 {
		t.Fatalf("loadMigrations: %v (%d)", err, len(migs))
	}
	for i, m := range migs {
		if m.version != i+1 || m.up == "" || m.down == "" {
			t.Fatalf("migration %d malformed or out of sequence: %+v", i, m.version)
		}
		if strings.Contains(strings.ToLower(m.up), "create table") && !strings.Contains(strings.ToLower(m.up), "enable row level security") {
			t.Fatalf("migration %04d creates tables without enabling row level security", m.version)
		}
	}
}
