# TradePulse — personalized real-time information backend (Go)

```
Source → Listener → RawMessage → Processor → Event → EventBus → Signals/Rules → Personalization → FeedItem → WebSocket
```
Modular monolith. Go 1.24, [Huma](https://huma.rocks) for typed, self-documenting REST (like FastAPI), hand-written WebSocket hub.
Where we are & what is next: top of [`ROADMAP.md`](ROADMAP.md) · all changes: [`CHANGELOG.md`](CHANGELOG.md) · tests: [`TESTING.md`](TESTING.md).
Project charter and plan: [`PROJECT_BRAIN.md`](PROJECT_BRAIN.md) · status: [`ROADMAP.md`](ROADMAP.md) · work items: [`BACKLOG.md`](BACKLOG.md) · decisions: [`DECISIONS.md`](DECISIONS.md).
Working with an AI coding assistant? Read [`AGENTS.md`](AGENTS.md) (rules, token-efficient).

> **Status:** data is stored in memory (lost on restart) and a simulated NSE feed generates events. See [Before real production](#before-real-production).
> This code was authored without a Go toolchain. The first `go mod tidy`/`check` run may surface small fixes (Huma API details).

---
## 1. Start developing

**Requirement: Docker only** (Docker Desktop on Windows/macOS, Docker Engine + Compose on Linux). No Go, no `make` needed. Commands are identical on every OS.

```bash
docker compose up --build
```
Hot reload (air) rebuilds on every `.go` change. The first start runs `go mod tidy` and creates `go.sum` — commit it.

<details><summary>Windows 11 notes</summary>

- Use Docker Desktop with the **WSL 2 backend**. For much faster builds and file watching, clone the repo **inside WSL** (e.g. `~/code/tradepulse`) rather than under `C:\`.
- PowerShell, CMD and WSL all work with the commands above. If port 8080 is taken, change the left side of `"8080:8080"` in `compose.yaml`.
- `.gitattributes` forces LF line endings so Windows checkouts don't break Docker/shell files.
</details>

<details><summary>Linux note</summary>

The dev container runs as root, so files it creates on the host (`go.sum`, `openapi.json`, `tmp/`) are root-owned. Fix with `sudo chown -R $USER:$USER .` or use rootless Docker/Podman.
</details>

Then open **http://localhost:8080/docs** — interactive API docs (auto-listed endpoints, "Try it out"), like FastAPI's Swagger.
Raw spec: `http://localhost:8080/openapi.json`. Docs are **on in development, off in production** (`DOCS_ENABLED`).

### Manual test walkthrough (in /docs, Postman or curl)
0. **The API reference is the running `/docs` page** (overview, quick start, every endpoint with examples, WebSocket protocol, error format). New developers should start there.
1. Registration is **invite-only**. In dev a first admin exists: `admin@example.com` / `dev-admin-password-change-me` (from `compose.yaml`; change via `BOOTSTRAP_ADMIN_EMAIL` / `BOOTSTRAP_ADMIN_PASSWORD`). `POST /api/v1/auth/login` as admin → copy `token`.
2. In /docs click **Authorize** → paste the token. `POST /api/v1/admin/invites` `{}` → copy the one-time `code`. Then `POST /api/v1/auth/register` `{"email":"me@example.com","password":"a-long-password","invite_code":"<code>"}` → the member `token`; authorize with that one for the steps below. (Postman: Authorization → Bearer Token.)
3. `PUT /api/v1/me/subscriptions` — without subscriptions the feed is empty. Market demo (`SIM_DEMO=market`):
   `{"subscriptions":[{"kind":"entity","value":"RELIANCE","types":["volume_spike","unusual_activity"]},{"kind":"entity","value":"SBIN","conditions":[{"field":"abs_change_pct","op":">=","value":0.5}]}]}`
   Status demo (`SIM_DEMO=status`): `{"subscriptions":[{"kind":"source","value":"STATUS_SIM","conditions":[{"field":"status","op":"!=","value":"up"}]}]}`.
   `GET /api/v1/event-types` lists the fields you can use in conditions.
4. `POST /api/v1/ws-ticket` → `ticket` (valid 30 s, single use).
5. **WebSocket** (not testable in /docs): Postman → New → WebSocket → `ws://localhost:8080/ws?ticket=<ticket>`, or `websocat "ws://localhost:8080/ws?ticket=<ticket>"`. You get `{"type":"connected"}` then `feed_item` messages. Get a fresh ticket per connection.
6. `GET /api/v1/me/feed?limit=20` for history.

### Everyday commands
| Task | Any OS (Docker) | Optional Unix shortcut |
|---|---|---|
| Start dev server (hot reload) | `docker compose up --build` | `make dev` |
| Vet + race tests (before every commit) | `docker compose run --rm check` | `make check` |
| Tests only | `docker compose run --rm test` | `make test` |
| Regenerate `openapi.json` (**commit it**; CI fails if stale) | `docker compose run --rm openapi` | `make openapi` |
| Resolve dependencies / `go.sum` | `docker compose run --rm tidy` | `make tidy` |
| Logs / stop | `docker compose logs -f api` / `docker compose down` | |
| Clean rebuild | `docker compose build --no-cache` | |

First-time setup: run the `openapi` command once and commit `openapi.json` (CI diffs against it).

### Project layout
```
cmd/server        wiring (main.go)          cmd/openapi   prints OpenAPI spec
internal/domain   event, user, feed, rule   (pure; no infrastructure imports)
internal/source   Listener implementations (simulated NSE now)
internal/processing  Processors + unusual-activity signal
internal/ingestion   supervises listeners (restart w/ backoff)
internal/personalization  entity→users index, rule chain, rendering
internal/delivery WebSocket hub     internal/api  Huma operations + middleware
internal/auth     JWT, PBKDF2, WS tickets
internal/storage/memory, internal/infrastructure/eventbus   replaceable implementations
```

### Configuration (env vars, see `.env.example`)
| Var | Default | Notes |
|---|---|---|
| `APP_ENV` | development | `production` flips safe defaults |
| `JWT_SECRET` | — (required) | ≥32 chars; `change-me*` rejected in production |
| `JWT_TTL` | 12h | |
| `ALLOWED_ORIGINS` | empty | CORS + WebSocket origin allowlist (comma-separated) |
| `TRUST_PROXY` | false | true only behind a trusted reverse proxy (uses last `X-Forwarded-For`) |
| `DOCS_ENABLED` | on in dev, off in prod | serves `/docs`, `/openapi.json` |
| `RATE_LIMIT_RPS` / `_BURST` | 20 / 40 | per IP; auth routes are stricter (~12/min) |
| `SIM_DEMO` / `SIM_INTERVAL` / `SIM_SYMBOLS` | market (dev) or none (prod) / 2s / 10 symbols | example domain: `market`, `status`, `none` |
| `ADDR`, `LOG_LEVEL` | :8080, info | |

### Database
Dev uses the local `postgres` compose service automatically. To use a managed database (Supabase now, any provider later) set `DATABASE_URL` (and `DATABASE_MIGRATION_URL`) in `.env`: nothing else changes. Production **requires** `DATABASE_URL`. Details, Supabase setup and security notes: [`docs/DATABASE.md`](docs/DATABASE.md). Migrations run automatically at startup; `docker compose run --rm check` also runs the Postgres integration tests.

### Sources, webhooks and X
Admins add sources in `/docs` (RSS, web pages, manual, **webhook**, **x**); see `docs/sources/` and [`docs/WEBHOOKS.md`](docs/WEBHOOKS.md). Webhooks need `WEBHOOK_SIGNING_KEY`; X needs `X_BEARER_TOKEN` and is spend-capped by `X_MONTHLY_BUDGET_USD`.

### Access control (invite-only)
- Admin creates a single-use invite (optionally bound to an email, expiring in 7 days by default); the code is shown **once** and stored only as a hash.
- Members register with the code. Suspending a member rejects their token at once, stops matching and closes their WebSockets. All admin actions are in the audit log (`GET /api/v1/admin/audit`).
- First admin: set `BOOTSTRAP_ADMIN_EMAIL` + `BOOTSTRAP_ADMIN_PASSWORD` (idempotent, never overwrites an existing account). In production set them from Secret Manager and remove them after the first start.

### Adding things
Endpoint, source, event type, rule, storage: follow the checklist in [`AGENTS.md`](AGENTS.md). Endpoints are code-first: typed structs + `huma.Register` produce validation **and** docs; then regenerate the spec (`docker compose run --rm openapi`).

---
## 2. Deploy to production

### Before real production
The current build is a solid skeleton, but these are **required** for a real deployment:
1. **PostgreSQL** repositories for users/feed (implement `user.Repository`, `feed.Repository`; swap in `main.go`). Until then every restart wipes accounts.
2. **Real data source:** implement `source.Listener` + `processing.Processor` for NSE/BSE (licensed feed) and set `SIM_DEMO=none`.
3. **Redis** for rate limits and WS tickets before running >1 instance (they are in-memory now). Until then run exactly **one** instance.
4. Recommended: refresh tokens/revocation, Prometheus metrics, error tracking.

### Deploy on a single host with Docker
```bash
cp .env.example .env.production      # then edit:
#   APP_ENV=production
#   JWT_SECRET=$(openssl rand -base64 48)
#   ALLOWED_ORIGINS=https://app.yourdomain.com
#   (compose.prod.yaml forces DOCS_ENABLED=false, TRUST_PROXY=true, SIM_DEMO=none)
docker compose -f compose.prod.yaml up -d --build
docker compose -f compose.prod.yaml ps      # should become "healthy"
```
The container is distroless, non-root, read-only, capabilities dropped, and only listens on `127.0.0.1:8080`.

### TLS reverse proxy (required — the app speaks plain HTTP)
Caddy (automatic HTTPS, WebSocket works out of the box). `Caddyfile`:
```
api.yourdomain.com {
    reverse_proxy 127.0.0.1:8080
}
```
Nginx users: forward `Upgrade`/`Connection` headers for `/ws` and set a long `proxy_read_timeout`.

### Production checklist
- [ ] `APP_ENV=production`, strong unique `JWT_SECRET` from a secret manager (not in git)
- [ ] `DOCS_ENABLED=false` (default in production) — verify `/docs` returns 404
- [ ] HTTPS only; `ALLOWED_ORIGINS` lists only your real frontend origins
- [ ] Firewall: expose 80/443 only; app port bound to localhost/private network
- [ ] `TRUST_PROXY=true` only if a trusted proxy sets `X-Forwarded-For`
- [ ] CI green (`check`, OpenAPI up to date); image built from tagged commit
- [ ] Postgres backups + tested restore; log shipping (JSON logs on stdout)
- [ ] Monitor `/healthz`; alert on restarts

### Security design (summary)
PBKDF2-SHA256 (600k) passwords, timing-equalised login; HS256 JWT with strict header check; one-time WebSocket tickets (no JWT in URLs); per-IP token-bucket rate limits; strict, size-limited JSON; WS origin check + frame/size caps + timeouts; security headers, CORS allowlist; server timeouts; panic recovery; docs absent (not just protected) in production.
