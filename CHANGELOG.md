# CHANGELOG.md — the single record of important changes

**Rules:** newest first · every change that alters behavior, API, config, data model, security, tests or process gets an entry in the same PR · say *what changed, why, which files, what breaks, how verified, what is next*.
Related trackers: status → `ROADMAP.md` · work items → `BACKLOG.md` · tests → `TESTING.md` · decisions → `DECISIONS.md`.
Verification legend: ✅ verified by running · ⚠️ written but **not yet run/compiled** · ⛔ failed.

---

## [0.4.0-dev] — 2026-10-17 — PostgreSQL storage via env, migrations, readiness, safe fetcher (TP-007, TP-010) → M2 complete in code
**Verification:** ⚠️ not compiled or run by the author (no Go toolchain in the sandbox); owner has verified M0 and M1 (0.2.0-dev and earlier) on Windows 11. Run `docker compose run --rm check` (it now starts the local `postgres` service and runs the integration tests).
**Suggested commit message:**
```
feat(storage): managed PostgreSQL via env, embedded migrations, readiness, SSRF-safe fetcher

- pgx repositories for users, invites, audit, feed (parameterized SQL, atomic invite redeem)
- embedded up/down migrations with advisory lock; RLS enabled on every table (Supabase-safe)
- pooler-aware connection (pgbouncer/6543 -> simple protocol), separate migration URL, DB config by env only
- /readyz checks the database; production requires DATABASE_URL; hourly retention job
- internal/safefetch: SSRF guard (private/metadata IPs, DNS rebinding, redirects, size/time limits) + architecture test
- config: source interval bounds, X credentials/budget, secret-redacting LogValue
- compose: local postgres service; CI: postgres service; docs/DATABASE.md; sources docs for 3 news pages + X
- 34 new tests (24 security)
```

### Why
M2 exit criteria: restart-safe persistence and access control. The owner will run on a managed Postgres (Supabase free tier now, any provider later), so everything about the database is environment-driven and hardened for a provider that exposes an HTTP API over `public`. The safe fetcher is the prerequisite for every source connector (D-026).

### Added
- **`internal/storage/postgres`:** `UserRepo`, `InviteRepo`, `AuditRepo`, `FeedRepo` (pgx v5, parameterized SQL only). Invite `Redeem` is one atomic `UPDATE … RETURNING`. Case-insensitive unique email (`lower(email)` index). Feed ordered by a `bigserial` sequence.
- **Embedded migrator** (`migrate.go`, D-013): numbered `.up.sql`/`.down.sql`, per-migration transaction, `schema_migrations`, advisory lock (safe with several instances), `MigrateDown` for tests/emergencies.
- **Security of the schema:** `ENABLE ROW LEVEL SECURITY` on every table (including `schema_migrations`) and revoke from `anon`/`authenticated` if those roles exist (Supabase). Tests enforce it.
- **Provider flexibility:** `DATABASE_URL`, `DATABASE_MIGRATION_URL` (session/direct connection for migrations), `DB_MAX_CONNS`, `DB_SIMPLE_PROTOCOL=auto|true|false` (auto: `pgbouncer=true` or port 6543 → simple protocol; the Prisma-style `pgbouncer` param is stripped). Errors never echo the URL.
- **Readiness:** `/readyz` pings the database (503 with a generic message if down); `/healthz` stays independent.
- **Retention:** hourly job keeps the newest 1,000 feed items per user and 365 days of audit log.
- **Production guard:** server refuses to start in production without `DATABASE_URL`; without it in development, in-memory storage with a loud warning.
- **`internal/safefetch`** (TP-010, D-026): http/https only, ports 80/443, no URL credentials, metadata/localhost/`.internal` hosts blocked, IP validated at dial time and the validated IP dialed (defeats DNS rebinding and redirect tricks), loopback/private/link-local/CGNAT/multicast/doc ranges and IPv4-mapped IPv6 blocked, bounded redirects/time/size, no proxy env, no cookies. Architecture test fails if any other package makes outbound HTTP.
- **Config:** `SOURCE_MIN/MAX_INTERVAL_SECONDS` (per-source latency bounds, D-022), `X_BEARER_TOKEN`, `X_API_BASE_URL`, `X_MONTHLY_BUDGET_USD` (D-023), `Config.LogValue()` redacts secrets.
- **Dev/CI:** compose `postgres` service (local default; overridden by `.env`), `check`/`test` run integration tests, GitHub Actions Postgres service, `.env.example` rewritten with every variable.
- **Docs:** `docs/DATABASE.md` (Supabase and other providers, pooler modes, RLS), `docs/sources/news-pages.md` (Moneycontrol, CNBC, scanx.trade), updated `x-accounts.md`; decisions D-013 (resolved), D-021…D-027; backlog TP-018 (per-source scheduler), TP-019 (channels), TP-020 (deploy workflow).

### Changed
| Area | Before | After |
|---|---|---|
| Storage | in-memory only | PostgreSQL when `DATABASE_URL` is set (required in production); memory only for development |
| Dev stack | api only | api + local `postgres`; `check` starts it |
| `api.Deps` | no readiness | optional `Ready func(ctx) error` |
| `postgres.Open` | – | `Open(ctx, url, maxConns, mode)` |
| Migration 0001 | (never applied anywhere) | now includes RLS + provider-role revokes |

### Security notes
RLS everywhere; credentials never in errors or logs; injection payload tests against a real database; SSRF guard enforced by architecture test; readiness errors sanitized. Reminder: rotate the Supabase password if it was ever pasted into chat or committed.

### Known limitations / open items
- Not yet confirmed by running (see verification note). Expect possible small fixes in `internal/storage/postgres` and tests (pgx specifics).
- Per-account login throttling and refresh tokens still pending (M6).
- Supabase specifics (IPv6 direct host, pause on inactivity, connection limits) are documented from general knowledge: verify against current Supabase docs.
- Feed uses `LIMIT` not cursors yet (M7).

### Files (main)
`internal/storage/postgres/*` · `internal/safefetch/*` · `internal/config/config.go` · `internal/api/{server,handlers}.go` · `cmd/server/main.go` · `compose.yaml` · `.github/workflows/ci.yaml` · `.env.example` · `docs/DATABASE.md` · `docs/sources/*` · `DECISIONS.md` · `TESTING.md` · tests

### Next
Owner run of `check` → M3 sources: TP-011 (source model + admin CRUD), TP-012, TP-018 (per-source scheduler), TP-013/014/015 (RSS, page monitor, manual).

---

## [0.3.0-dev] — 2026-10-16 — Invite-only access, roles, admin API, audit, API docs (TP-002, TP-008, TP-009, TP-017)
**Verification:** ⚠️ not compiled or run (no Go toolchain in the authoring sandbox). Run `docker compose run --rm check` first.
**Suggested commit message:**
```
feat(access): invite-only registration, roles, suspension, admin API, audit log

- registration requires a single-use, expiring, hashed invite code (atomic redeem, optional email binding)
- roles (admin/member) and status (active/suspended); auth middleware re-checks the user on every request
- suspension rejects tokens at once, stops matching and closes WebSockets
- admin API: invites, users, audit; first admin via BOOTSTRAP_ADMIN_* env
- /docs overview + per-operation docs, enforced by test; WebSocket end-to-end test
- 27 new tests (22 security)
```

### Why
Product gate for the private beta: only invited people may join, admins must be able to remove access instantly, and every admin action must be traceable. New developers must be able to learn the API from `/docs`.

### Added
- **Invites** (`domain/invite`): 192-bit random codes (`tp_inv_…`), stored only as SHA-256, single-use, expiring (default 7 d, max 30 d), optional email binding, atomic `Redeem`, `Release` on failed registration, `Revoke` (pending only). Generic errors (no oracle).
- **Roles/status** on `User` (`admin|member`, `active|suspended`); `UserRepository.SetStatus`.
- **Auth middleware** now loads the user per request (suspension/role changes are immediate); suspended/unknown ⇒ 401, non-admin on admin route ⇒ 403. `/ws` also checks status.
- **Admin API** (`/api/v1/admin/*`): create/list/revoke invites, list users, suspend/reactivate (self-suspend blocked), audit log.
- **Audit log** (`domain/audit`): registrations, invite create/revoke, suspend/reactivate; never contains secrets.
- **First admin bootstrap** (`internal/access`): `BOOTSTRAP_ADMIN_EMAIL` + `BOOTSTRAP_ADMIN_PASSWORD` (>= 12 chars), idempotent, never overwrites (D-015). Dev defaults in `compose.yaml`.
- **Engine/Hub:** `Engine.RemoveUser` used on suspension; `Hub.DisconnectUser` closes a user's sockets.
- **Docs in /docs (TP-017):** overview (quick start, auth, subscriptions, WebSocket protocol, errors, admin), described tags, per-operation descriptions and error codes, field examples. Enforced by `TestEveryOperationIsDocumented`.
- **TP-002:** real-server WebSocket end-to-end test (handshake, live item, history, close on suspension).
- Docs: `docs/sources/x-accounts.md`, `docs/sources/news-page.md`; decisions D-015…D-020.

### Changed (⚠️ breaking vs 0.2.0-dev)
| Area | Before | After |
|---|---|---|
| `POST /auth/register` | email + password | email + password + **`invite_code`** (403 on invalid) |
| `POST /auth/login` | 401 on bad credentials | + **403** for suspended accounts |
| Private routes | valid JWT is enough | JWT **and** an active account (loaded each request) |
| `api.Deps` | no invites/audit | `Invites`, `Audit` required |
| `api.Personalizer` | `UpsertUser` | + `RemoveUser` |
| Config | — | `BOOTSTRAP_ADMIN_EMAIL`, `BOOTSTRAP_ADMIN_PASSWORD` (both or neither; password >= 12) |
| Dev flow | register directly | login as bootstrap admin → create invite → register |

### Security notes
Codes never stored or logged in plaintext; list endpoints expose neither codes nor hashes nor password hashes; failed registration releases the invite only for the same user; per-request user check prevents "valid token, removed account"; admin lock-out guard; audit trail for admin actions. Auth endpoints remain rate limited.

### Known limitations / open items
- Storage is still in memory: users, invites and audit are lost on restart (TP-007 Postgres next). Bootstrap admin is recreated on each start (idempotent).
- Per-account login throttling/lockout not yet implemented (only per-IP).
- Feed catch-up uses the latest N items; cursor pagination comes with the API freeze (M7).
- Source work (news page, X accounts) waits for TP-010 (safe fetcher) and the news URL.

### Files (main)
`internal/domain/{invite,audit,user}` · `internal/storage/memory/repos.go` · `internal/access/access.go` · `internal/api/{server,admin,authmw,docs,handlers}.go` · `internal/delivery/hub.go` · `internal/config/config.go` · `cmd/server/main.go` · `compose.yaml` · tests (`*_test.go`) · `TESTING.md` · `docs/sources/*` · `DECISIONS.md`

### Next
TP-001 (first green `check`) → TP-007 Postgres → TP-010 safe fetcher → TP-013/014/015 sources → TP-016 X connector.

---

## [0.2.0-dev] — 2026-10-16 — M1 Generic core (TP-004, TP-005, TP-006)
**Verification:** ✅ owner confirmed M0 and M1 working on Windows 11 (2026-10-17).

### Why
The platform must be domain-agnostic (D-004). The v0 core hardcoded market concepts (price/volume event types, thresholds, watchlist, `render.go`). M1 replaces them with data-driven building blocks; markets become one *example*.

### Added
- **Event type registry** (`internal/domain/event/registry.go`): types are data — id, typed payload fields (the rule-field whitelist), `{field}`/`{field:%.2f}` title/body templates, severity. Default type `content_item`. A payload field name has one kind across all types.
- **Generic Event** (`event.go`): `Entities []string`, `Title`, `Summary`, `URL`, `Source`, `DedupeKey`, `Payload`; `Field()` resolver (built-ins: type, source, title, summary, url, entity); text helpers `NormEntity`, `Tokens`, `Phrase`.
- **Rule engine** (`internal/domain/rule/rule.go`): `field op value`, ops `> >= < <= == != in contains any_of`, validated against the registry whitelist; limits (10 conditions, 50 list items, 200 chars); missing field ⇒ no match; no expression evaluation.
- **Subscriptions** (`internal/domain/user/user.go`): kinds `entity | keyword | source`, optional `types` restriction and `conditions`; `muted` list; `Profile.Normalize(registry)`.
- **Matching engine** (`internal/personalization/engine.go`): inverted indexes (entity, source, keyword-first-token); whole-word phrase matching; one item per user per event; explainability (`reason`); `RemoveUser` for suspension.
- **Endpoints:** `GET/PUT /api/v1/me/subscriptions`, `GET /api/v1/event-types`.
- **Examples** (`examples/market`, `examples/status`): two independent domains on the unchanged core (proof of D-004). Selected with `SIM_DEMO=market|status|none`.
- **Ingestion** now validates every event against the registry before publishing.
- **Tests:** 65 total (41 security-focused) — see `TESTING.md`.
- **Docs:** `TESTING.md` (register, security matrix, gaps, verification log), this file's format.

### Changed (⚠️ breaking vs 0.1.x)
| Area | Before (0.1.x) | After (0.2.0-dev) |
|---|---|---|
| Event | `Type` enum consts, single `Entity`, market payloads | `Type` string from registry, `Entities[]`, generic fields |
| Profile | `watchlist`, `event_types`, `muted`, `volume_spike_threshold`, `price_move_threshold_pct` | `subscriptions[]` (kind/value/types/conditions), `muted` |
| Endpoint | `GET/PUT /api/v1/me/profile` | `GET/PUT /api/v1/me/subscriptions`; new `GET /api/v1/event-types` |
| FeedItem | entity, title, body, severity | + `source`, `entities[]`, `url`, `reason` |
| Rules | `Rule` chain with `UserContext`, hardcoded watchlist/threshold rules | stateless `rule.Match(conditions, event)` |
| Rendering | `personalization/render.go` (market-specific) | templates in the type registry |
| Config | `SIM_ENABLED=true|false` | `SIM_DEMO=market|status|none` (default: market in dev, none in production) |
| Wiring | `NewEngine(log, feeds, out, rules)`; `ingestion.Run(ctx, log, bus, listeners, procs)` | `NewEngine(log, feeds, out, registry)`; `ingestion.Run(ctx, log, bus, registry, listeners, procs)`; `api.Deps.Registry` added |

Migration for API clients: replace `PUT /me/profile {"watchlist":["TCS"],"volume_spike_threshold":2}` with
`PUT /me/subscriptions {"subscriptions":[{"kind":"entity","value":"TCS","types":["volume_spike"],"conditions":[{"field":"relative_volume","op":">=","value":2}]}]}`.

### Removed
`internal/processing/signals.go`, `internal/source/simulated/`, `internal/personalization/render.go`, the old `rule` package types (`Rule`, `UserContext`, `WatchlistRule`, `ThresholdRule`…), `SIM_ENABLED`. Market logic now lives only under `examples/market`.

### Security notes
Rule/subscription input is whitelisted and size-limited; control characters rejected; no expression evaluation; templates only substitute whitelisted fields; per-user isolation covered by tests (engine, feed repo, API).

### Files (main)
`internal/domain/{event,rule,user,feed}/*` · `internal/personalization/engine.go` · `internal/ingestion/ingestion.go` · `internal/processing/processor.go` · `internal/api/{server,handlers}.go` · `internal/config/config.go` · `cmd/server/main.go` · `examples/{market,status}/*` · `*_test.go` (all) · `TESTING.md`

### Open items from this change
- Unverified build (TP-001). Keyword/entity matching quality (stemming, synonyms) intentionally out of scope.
- Event `DedupeKey` exists but dedupe is not enforced yet (TP-012).

### Next
TP-001 → TP-002 → TP-007 (Postgres). See `ROADMAP.md` "Next action".

---

## [0.1.5] — 2026-10-15 — Project charter & tracking (TP-003)
Added `PROJECT_BRAIN.md` (charter, architecture, agent protocol, stop conditions), `ROADMAP.md`, `BACKLOG.md`, `DECISIONS.md`, `CHANGELOG.md`. Recorded owner decisions: Cloud Run hosting (min=1/max=1, CPU always allocated), invite codes shared manually, first sources = news pages + X accounts, privacy deferred (D-005…D-008). Removed `docs/PLAN.md` (superseded). `AGENTS.md` now points to the charter.

## [0.1.4] — 2026-10-15 — Docker-only, cross-platform dev
`compose.yaml` with one-shot tool services (`check`, `test`, `openapi`, `tidy`); `.air.toml` polling for Windows/macOS file watching; `.gitattributes` forces LF; `Makefile` reduced to optional wrappers; `Dockerfile.dev` sets `GOTOOLCHAIN=auto` (fixes `air` install failure). Fix: re-added `writeError` helper in `internal/api/middleware.go` (build error after Huma rewrite).

## [0.1.3] — 2026-10-15 — Huma API layer + dev/prod tooling
Rewrote `internal/api` on Huma (typed handlers → validation + OpenAPI); `/docs` and `/openapi.json` **only when `DOCS_ENABLED`** (default off in production; absent, not merely protected; relaxed CSP only on `/docs`); production guards (rejects placeholder `JWT_SECRET`, warns if docs enabled); `healthcheck` subcommand for Docker; distroless prod `Dockerfile`, `compose.prod.yaml`, CI workflow (vet, race tests, OpenAPI diff), `AGENTS.md`/`CLAUDE.md`, README (dev + production guide).

## [0.1.2] — 2026-10-15 — Hardening of the skeleton
WebSocket one-time tickets (no JWT in URLs); origin allowlist; frame/size caps; per-IP token-bucket rate limits (strict on auth routes); CORS allowlist; security headers; panic recovery; server timeouts.

## [0.1.1] — 2026-10-15 — Auth
Stdlib-only HS256 JWT (strict header), PBKDF2-SHA256 (600k) passwords with timing-equalised login, in-memory storage behind repository interfaces.

## [0.1.0] — 2026-10-15 — Initial skeleton
Pipeline `Source → Listener → RawMessage → Processor → Event → Bus → Rules → Personalization → FeedItem → WebSocket`; in-memory event bus with bounded per-subscriber queues; supervised listeners with backoff; simulated market source and unusual-activity signal; hand-written RFC 6455 WebSocket hub.
