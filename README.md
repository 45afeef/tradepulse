# TradePulse — personalized real-time information backend (Go)

```
Source → Listener → RawMessage → Processor → Event → EventBus → Signals/Rules → Personalization → FeedItem → WebSocket
```
Modular monolith. Go 1.24, [Huma](https://huma.rocks) for typed, self-documenting REST (like FastAPI), hand-written WebSocket hub.
Working with an AI coding assistant? Read [`AGENTS.md`](AGENTS.md) (architecture + security rules, token-efficient).

> **Status:** data is stored in memory (lost on restart) and a simulated NSE feed generates events. See [Before real production](#before-real-production).
> This code was authored without a Go toolchain. The first `go mod tidy`/`make check` may surface small fixes (Huma API details).

---
## 1. Start developing

### Option A — Docker (recommended, nothing to install but Docker)
```bash
docker compose up --build
```
Hot reload (air) rebuilds on every `.go` change. First start runs `go mod tidy` and creates `go.mod`/`go.sum` changes — commit them.

### Option B — local Go
```bash
go mod tidy                                   # once; creates go.sum
export JWT_SECRET=$(openssl rand -base64 48)  # PowerShell: $env:JWT_SECRET="<random 32+ chars>"
make run
```

Then open **http://localhost:8080/docs** — interactive API docs (auto-listed endpoints, "Try it out"), like FastAPI's Swagger.
Raw spec: `http://localhost:8080/openapi.json`. Docs are **on in development, off in production** (`DOCS_ENABLED`).

### Manual test walkthrough (in /docs, Postman or curl)
1. `POST /api/v1/auth/register` `{"email":"me@example.com","password":"a-long-password"}` → copy `token`.
2. In /docs click **Auth/Authorize** → Bearer → paste token (Postman: Authorization → Bearer Token).
3. `PUT /api/v1/me/profile` `{"watchlist":["RELIANCE","TCS","SBIN"],"price_move_threshold_pct":0.5}` — no watchlist = empty feed.
4. `POST /api/v1/ws-ticket` → `ticket` (valid 30 s, single use).
5. **WebSocket** (not testable in /docs): Postman → New → WebSocket → `ws://localhost:8080/ws?ticket=<ticket>`, or `websocat "ws://localhost:8080/ws?ticket=<ticket>"`. You get `{"type":"connected"}` then `feed_item` messages. Get a fresh ticket per connection.
6. `GET /api/v1/me/feed?limit=20` for history.

### Everyday commands
| Command | What |
|---|---|
| `make dev` | Docker dev server, hot reload |
| `make check` | `go vet` + `go test -race` (run before commit) |
| `make openapi` | regenerate `openapi.json` (**commit it**; CI fails if stale) |
| `make tidy` | resolve dependencies |

First-time setup: run `make openapi` once and commit `openapi.json` (CI diffs against it).

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
| `SIM_ENABLED` / `SIM_INTERVAL` / `SIM_SYMBOLS` | true / 2s / 10 NSE symbols | disable in production |
| `ADDR`, `LOG_LEVEL` | :8080, info | |

### Adding things
Endpoint, source, event type, rule, storage: follow the checklist in [`AGENTS.md`](AGENTS.md). Endpoints are code-first: typed structs + `huma.Register` produce validation **and** docs; then `make openapi`.

---
## 2. Deploy to production

### Before real production
The current build is a solid skeleton, but these are **required** for a real deployment:
1. **PostgreSQL** repositories for users/feed (implement `user.Repository`, `feed.Repository`; swap in `main.go`). Until then every restart wipes accounts.
2. **Real data source:** implement `source.Listener` + `processing.Processor` for NSE/BSE (licensed feed) and set `SIM_ENABLED=false`.
3. **Redis** for rate limits and WS tickets before running >1 instance (they are in-memory now). Until then run exactly **one** instance.
4. Recommended: refresh tokens/revocation, Prometheus metrics, error tracking.

### Deploy on a single host with Docker
```bash
cp .env.example .env.production      # then edit:
#   APP_ENV=production
#   JWT_SECRET=$(openssl rand -base64 48)
#   ALLOWED_ORIGINS=https://app.yourdomain.com
#   (compose.prod.yaml forces DOCS_ENABLED=false, TRUST_PROXY=true, SIM_ENABLED=false)
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
- [ ] CI green (`make check`, OpenAPI up to date); image built from tagged commit
- [ ] Postgres backups + tested restore; log shipping (JSON logs on stdout)
- [ ] Monitor `/healthz`; alert on restarts

### Security design (summary)
PBKDF2-SHA256 (600k) passwords, timing-equalised login; HS256 JWT with strict header check; one-time WebSocket tickets (no JWT in URLs); per-IP token-bucket rate limits; strict, size-limited JSON; WS origin check + frame/size caps + timeouts; security headers, CORS allowlist; server timeouts; panic recovery; docs absent (not just protected) in production.
