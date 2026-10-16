# Database guide (managed PostgreSQL, provider-agnostic)

The app talks to **any PostgreSQL 14+** through environment variables only. Switching from Supabase to AWS RDS/Aurora, GCP Cloud SQL, Neon, etc. means changing the variables below. No code change.

| Variable | Purpose |
|---|---|
| `DATABASE_URL` | Runtime connection string. **Required in production.** Empty in development = in-memory storage (data lost on restart). |
| `DATABASE_MIGRATION_URL` | Optional. A *direct or session-mode* connection used only for migrations (they take a session-level advisory lock). Defaults to `DATABASE_URL`. |
| `DB_MAX_CONNS` | Pool size (default 10). Keep it below your plan's connection limit. |
| `DB_SIMPLE_PROTOCOL` | `auto` (default) / `true` / `false`. Transaction-mode poolers (PgBouncer, Supavisor port 6543) do not support prepared statements; `auto` switches to the simple protocol when the URL has `pgbouncer=true` or port `6543`. |

Migrations run automatically at startup (embedded SQL in `internal/storage/postgres/migrations`, tracked in `schema_migrations`, safe with several instances).

## Supabase (free tier) quick setup
1. Create a project; note the database password.
2. *Project Settings → Database → Connection string*. Use the **pooler** (Supavisor) strings:
   - **Session mode** (port 5432) → `DATABASE_MIGRATION_URL` (and fine for `DATABASE_URL` too).
   - **Transaction mode** (port 6543) → `DATABASE_URL` for the app (best for serverless like Cloud Run). `DB_SIMPLE_PROTOCOL=auto` handles it.
   - Keep `sslmode=require` in the URL.
3. Put them in `.env` (local) or Secret Manager / GitHub secrets (deployed). Never commit them.
4. `docker compose up --build` now uses Supabase instead of the local Postgres container.

Notes (verify against current Supabase docs, they change):
- Supabase's *direct* connection host has historically been IPv6-only on free plans; Cloud Run egress is IPv4 by default, so use the **pooler** host.
- Free projects can pause when inactive. Plan for that in staging, and upgrade before real members depend on it.
- Free-tier connection limits are small: keep `DB_MAX_CONNS` low (5 to 10).

## Security: Row Level Security is mandatory
Supabase exposes the `public` schema over an HTTP API with publicly known roles (`anon`, `authenticated`). Our migrations therefore **enable RLS on every table with no policies** and revoke grants from those roles. The app connects as the table owner (`postgres`), which bypasses RLS, so it keeps working while the HTTP API can read nothing.
- A test (`TestEveryTableHasRowLevelSecurity`) fails if any table lacks RLS; a unit test fails if a migration creates tables without enabling it.
- New migrations that create tables MUST `ENABLE ROW LEVEL SECURITY` (see `AGENTS.md`).
- Optional hardening in the Supabase dashboard: remove `public` from *Exposed schemas* (Data API settings) since the app does not use the HTTP API.

## Other providers
- **AWS RDS/Aurora, GCP Cloud SQL, Azure:** set `DATABASE_URL` (add `sslmode=require` or `verify-full`); no pooler → leave `DB_SIMPLE_PROTOCOL=auto`. For Cloud SQL on Cloud Run use the instance's connection string/Unix socket as the provider documents.
- **PgBouncer in transaction mode (self-run):** set `DB_SIMPLE_PROTOCOL=true`, and use a direct URL for `DATABASE_MIGRATION_URL`.

## Operations
- **Retention:** hourly job keeps the newest 1,000 feed items per user and deletes audit entries older than 365 days (`internal/storage/postgres/retention.go`).
- **Backups:** use the provider's automated backups/PITR and do a restore drill before inviting members (M6).
- **Rollback of a migration:** `MigrateDown` exists for tests and emergencies only and is destructive; prefer forward fixes.
- **Local development:** the compose `postgres` service is used unless `DATABASE_URL` is set in `.env`.
- **Integration tests:** run automatically in `docker compose run --rm check` (they create and drop isolated schemas) and in CI; skipped when `TEST_DATABASE_URL` is unset.
