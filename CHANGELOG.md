# CHANGELOG.md — the single record of important changes

**Rules:** newest first · every change that alters behavior, API, config, data model, security, tests or process gets an entry in the same PR · say *what changed, why, which files, what breaks, how verified, what is next*.
Related trackers: status → `ROADMAP.md` · work items → `BACKLOG.md` · tests → `TESTING.md` · decisions → `DECISIONS.md`.
Verification legend: ✅ verified by running · ⚠️ written but **not yet run/compiled** · ⛔ failed.

---

## [0.8.0-dev] — 2026-10-20 — M6: session hardening, operator metrics, Cloud Run deployment, runbook (TP-020, TP-024, TP-025, TP-026)
**Verification:** ⚠️ not compiled or run by the author (no Go toolchain in the sandbox); no new Go dependencies. Run `docker compose run --rm check` (Postgres integration tests included). The deploy workflow and service template cannot be exercised without a GCP project: first staging deploy is an owner gate.
**Suggested commit message:**
```
feat(m6): refresh tokens and session hardening, metrics, Cloud Run deployment and runbook

- rotating hashed refresh tokens with reuse detection; logout, logout-all, password change, admin reset; exact revocation via sessions_valid_after
- per-account login lockout (uniform responses); access tokens now 15 min (JWT_TTL), refresh 30 days
- Cloud Run readiness: PORT, SHUTDOWN_TIMEOUT, TRUSTED_PROXY_HOPS; /metrics (METRICS_TOKEN) with curated counters and gauges
- deploy workflow (WIF, Secret Manager, GitHub environments), Cloud Run service template, CI govulncheck/gosec/Trivy
- docs: DEPLOY, RUNBOOK (alerts, playbooks, restore drill), LOADTEST + k6 script; migration 0005
- 27 new tests (21 security)
```

### Why
The beta needs sessions that can be ended instantly, protection against credential attacks, a repeatable deployment to Cloud Run, and enough observability to run it (D-039...D-044).

### Added
- **Sessions:** `POST /auth/refresh` (rotation; reuse revokes the family), `POST /auth/logout` (uniform 204), `POST /me/logout-all`, `POST /me/password` (current password required, other sessions signed out, fresh pair returned), `POST /admin/users/{id}/reset-password` (one-time temporary password, audited). Login/signup responses gain `refresh_token` and `expires_in`.
- **Exact revocation:** access tokens carry `ian` (issue time in ns); `users.sessions_valid_after` is set by logout-all/password change/reset; tokens issued before it are rejected immediately, tokens issued after work immediately.
- **Lockout:** 10 failed logins per 15 min lock the account for 15 min (keyed by the submitted email; same response for unknown accounts); password-change attempts are throttled too.
- **Cloud Run readiness:** `PORT` honored, `SHUTDOWN_TIMEOUT` (default 8 s), `TRUSTED_PROXY_HOPS` for the client IP behind proxies/load balancers, Docker healthcheck follows `PORT`.
- **Metrics:** `GET /metrics` (404 unless `METRICS_TOKEN`; bearer-protected): HTTP counts/latency, auth failures/lockouts/refresh/reuse, webhook deliveries, alert fires/pauses, outbound deliveries, source runs, WebSocket connections, bus drops, sources by health, X spend/budget.
- **Deployment:** `.github/workflows/deploy.yaml` (manual dispatch, GitHub environments, Workload Identity Federation, Secret Manager, smoke test), `deploy/cloudrun.service.yaml` (min=max=1, no CPU throttling, 3600 s timeout, probes), CI `scan` job (govulncheck blocking; gosec and Trivy advisory).
- **Docs:** `docs/DEPLOY.md`, `docs/RUNBOOK.md`, `docs/LOADTEST.md`, `deploy/loadtest/k6.js`.
- **Storage:** migration `0005_sessions` (`users.sessions_valid_after`, `refresh_tokens` with RLS), Postgres and in-memory repos, hourly pruning of expired refresh tokens.
- **Tests:** 27 new (see `TESTING.md`).

### Changed (⚠️)
| Area | Before | After |
|---|---|---|
| Access token lifetime (`JWT_TTL`) | 12 h | **15 min** (clients must use the refresh token) |
| `POST /auth/login`, `/auth/register` response | `token`, `user` | + `refresh_token`, `expires_in` |
| Password change | none | `POST /me/password` |
| Client IP behind a proxy | last `X-Forwarded-For` entry | entry `TRUSTED_PROXY_HOPS` from the right (default 1 = same as before) |
| Shutdown budget | 15 s | `SHUTDOWN_TIMEOUT` (8 s) |
| `user.Repository` | - | + `SetPassword`, `RevokeSessions` |

### Security notes
Refresh tokens are 256-bit, stored hashed, single-use; reuse is treated as theft; logout answers uniformly; revocation is exact; lockouts and failures reveal nothing about account existence; metrics are opt-in, token-protected and label-bounded (no user data); deployment uses OIDC (no long-lived keys) and Secret Manager.

### Known limitations / open items
- Not yet run by the owner; the Cloud Run YAML and workflow are templates verified only by reading (first staging deploy will confirm).
- Lockouts, rate limiters and tickets are per instance and reset on restart (single-instance design). A targeted lockout of a known email is possible for 15 minutes (documented trade-off).
- No session list UI/endpoint yet; no email-based password reset (admin reset instead).
- Privacy compliance remains deferred (D-008); restore drill and load test are owner-run gates.

### Files (main)
`internal/domain/session/*` · `internal/api/{session,throttle,handlers,authmw,middleware,server}.go` · `internal/metrics/*` · `internal/storage/{memory,postgres}/sessions*` + `migrations/0005_*` · `internal/config/config.go` · `cmd/server/main.go` · `deploy/*` · `.github/workflows/{deploy,ci}.yaml` · `docs/{DEPLOY,RUNBOOK,LOADTEST}.md` · tests

### Next
Owner gates (check, staging deploy, load test, restore drill) -> invite first members; agent starts M7 (API freeze).

---

## [0.7.0-dev] — 2026-10-18 — M5: alerts, templates, preview, guardrails, channels (TP-019, TP-023)
**Verification:** ⚠️ not compiled or run by the author (no Go toolchain in the sandbox). No new dependencies. Run `docker compose run --rm check` (Postgres integration tests included).
**Suggested commit message:**
```
feat(m5): alert rules with guardrails, templates and preview; in-app and signed outbound webhook channels

- rules: scope (entity/keyword/source) + types/conditions + cooldown/severity/channels; explainable fires and history
- guardrails: cooldown, 50 rules / 5 channels per user, hourly cap with automatic pause (visible reason, live notice)
- templates (generic + market/status examples), create-from-template, preview over recent events
- channels: in_app via WebSocket; outbound webhooks (HTTPS only, HMAC-signed, retries, auto-disable, secret shown once, test delivery, history)
- safefetch.Post (no redirects, same SSRF rules); Hub.Notify; suspension stops/resumes alerts
- migration 0004 (rules, fires, channels, deliveries; RLS); docs/ALERTS.md; /docs overview + 16 documented endpoints
- 50 new tests (35 security)
```

### Why
M5 completes the product loop: sources -> feed -> alerts you can trust. Spam control and explainability are first-class (D-035...D-038).

### Added
- **Domain** `alert`: `Spec`/`Rule`, `Fire`, `Channel`, `Delivery`, templates registry, `Preview`; strict `Spec.Normalize` (names, scope, types, conditions via the whitelist, cooldown 10 s...24 h, severity, own channels only).
- **Engine** (`internal/alerting`): inverted indexes (entity/source/keyword token), whole-phrase keyword matching, per-rule cooldown (restored from `last_fired_at` after restart), hourly cap -> automatic pause (persisted reason, `alert_paused` message), suspension handling, ring buffer of recent events for previews, `Preview` with cooldown/cap simulation.
- **Sender**: workers, bounded queue (overflow recorded), HMAC-signed JSON, retries (2/10/60 s) for network/5xx/408/429, no retry for 3xx/4xx, delivery history, auto-disable after 10 consecutive failures with `channel_disabled` notice, test deliveries, owner/enabled checks before every send.
- **API (all documented in /docs):** `GET /alert-templates`; `GET/POST /me/alerts`, `POST /me/alerts/from-template`, `POST /me/alerts/preview`, `GET /me/alerts/fires`, `GET/PUT/DELETE /me/alerts/{id}`; `GET/POST /me/channels`, `GET /me/channels/deliveries`, `DELETE /me/channels/{id}`, `POST /me/channels/{id}/rotate-secret|enable|test`. Tags `alerts`, `channels`; overview section.
- **Storage:** migration `0004_alerts` (RLS + revokes); Postgres and in-memory repositories; hourly retention (30 days) of fires and deliveries.
- **safefetch.Post:** same destination rules as Get, never follows redirects, 4 KB response cap. `Hub.Notify` for non-feed messages.
- **Examples:** alert templates for the market (big mover, volume spike, unusual activity) and status (service not up) domains.
- **Config:** `ALERT_MAX_FIRES_PER_HOUR` (default 30).
- **Docs:** `docs/ALERTS.md`; decisions D-035...D-038.

### Changed
- `api.Deps` gained `Alerts`, `AlertRules`, `AlertFires`, `Channels`, `Deliveries`, `Templates`, `Outbound`; suspend/reactivate also stop/restore alert rules; `main.go` wires the engine, sender, retention and example templates.
- `delivery.Hub.Deliver` now delegates to the new `Notify`.

### Security notes
Rules are validated against the field whitelist (no code); users can only see or change their own rules/channels (404 otherwise); channel URLs must be public HTTPS (SSRF-checked at save and at send); outbound secrets are derived with a separate namespace and never listed; redirects are never followed; every send re-checks owner and enabled state; spam is bounded by cooldown, limits and auto-pause.

### Known limitations / open items
- Not yet run by the owner. Retry queue is in memory (a restart loses pending retries); hourly fire counters reset on restart.
- Previews only know events seen since the server started.
- No email/Telegram/push channels yet (add behind `Dispatcher`).
- Alerts evaluate every event for every enabled rule via indexes; fine for 10-50 members, revisit if volume grows.

### Files (main)
`internal/domain/alert/*` · `internal/alerting/*` · `internal/api/{alerts,server,docs,admin}.go` · `internal/storage/{memory/alerts.go,postgres/alerts.go,postgres/migrations/0004_*}` · `internal/safefetch/safefetch.go` · `internal/delivery/hub.go` · `examples/*` · `cmd/server/main.go` · `docs/ALERTS.md` · tests

### Next
Owner run of `check`; try alerts end to end; then M6 (deploy to Cloud Run via GitHub Actions, auth hardening, operator metrics, restore drill).

---

## [0.6.1-dev] — 2026-10-18 — Fix: webhook sources refused; test stabilization
**Verification:** owner ran `check` on 0.6.0-dev: 8 failures (listed below); fixes applied, not yet re-run.
**Suggested commit message:** `fix(m4): allow webhook sources in connector validation; stabilize tests`
- **Bug fixed:** `connector.Registry.Validate` demanded a connector for `webhook` sources, so no webhook source could be created (7 tests). Push sources are now skipped like `manual`.
- **Tests corrected (author mistakes):** webhook tests decoded `webhook_secret`/`webhook_path` into Go-named fields without JSON tags (now tagged); rate-limit test used a refilling limiter that raced slow PBKDF2 (now no refill); manual-post test dropped topics via full-replace PUT (documented behavior); architecture test matched the injected `x.http.Get` field (regex now requires a non-identifier prefix); memory item-store prune expectation 4 -> 3.

---

## [0.6.0-dev] — 2026-10-17 — M4: inbound webhooks and X connector (TP-016, TP-022)
**Verification:** owner ran `check`: only the 8 failures fixed in 0.6.1-dev (everything else in 0.5.0-dev and 0.6.0-dev passed, including Postgres integration tests).
**Suggested commit message:**
```
feat(m4): inbound webhooks and budget-capped X connector, configured by env

- webhook sources: POST /api/v1/hooks/{name}, hmac (replay window) or token auth, derived secrets (nothing stored), rotation, identical 401s, 64 KB + per-source rate limits, idempotent retries
- X connector (official API only): enabled by X_BEARER_TOKEN, user id resolved on save, since_id cursor, persistent monthly spend ledger checked before every call, page size shrinks to remaining budget
- usage endpoint, connector cursor in scheduler state, migration 0003 (webhook/x config, secret_version, cursor, usage_monthly with RLS)
- config: WEBHOOK_SIGNING_KEY, WEBHOOK_RATE_PER_MINUTE, X_COST_PER_RESOURCE_USD, X_MAX_POSTS_PER_FETCH
- docs: WEBHOOKS.md, x-accounts.md, /docs overview section; 31 new tests (21 security)
```

### Why
The owner will feed items through their own automation (webhooks) and wants the X connector built and ready, configured purely by environment, without risk of runaway API spend.

### Added
- **Webhooks:** source type `webhook`. `POST /api/v1/hooks/{name}` accepts `{id?, title, summary?, url?, entities?, published_at?}`. Auth `hmac` (default: `X-TP-Timestamp` within 5 min + `X-TP-Signature: sha256=` over `timestamp.body`) or `token` (`X-Webhook-Token` / Bearer). Secrets are derived from `WEBHOOK_SIGNING_KEY` + source id + version, never stored; shown once at creation and on `POST /admin/sources/{id}/webhook-secret` (rotation kills the old secret). Unknown, disabled and wrong-secret requests all get the same 401. Dedupe by `id` or title+url; 64 KB cap; per-source rate limit; source stats updated; deliveries publish normal `content_item` events.
- **X connector:** source type `x` (`x.handle`); exists only when `X_BEARER_TOKEN` is set. Handle resolved to a user id when the source is saved (client-supplied ids ignored; same handle on update keeps the id, no extra billed read). Polling uses `since_id` (cursor persisted in source state), excludes retweets/replies, and every call is checked against a persistent monthly ledger first; when fewer than 5 posts are affordable polling stops with a clear error. Errors never contain the token.
- **Usage ledger:** `usage_monthly` (Postgres) / memory; `GET /api/v1/admin/usage`.
- **Scheduler:** persists connector cursors, never polls webhook sources.
- **Config (env):** `WEBHOOK_SIGNING_KEY`, `WEBHOOK_RATE_PER_MINUTE`, `X_COST_PER_RESOURCE_USD`, `X_MAX_POSTS_PER_FETCH` (plus existing `X_BEARER_TOKEN`, `X_API_BASE_URL`, `X_MONTHLY_BUDGET_USD`). `LogValue` redacts the new secrets.
- **Docs:** `docs/WEBHOOKS.md`, `docs/sources/x-accounts.md` (how to add the three accounts), /docs overview section; decisions D-032…D-034.
- **Tests:** 31 new (see `TESTING.md`).

### Changed
- `sourcedef.Source` gained `Webhook`, `X`, `SecretVersion`; `State` gained `Cursor`; `Repository` gained `BumpSecretVersion`; new `UsageStore`.
- `connector.Item` gained `Entities`; `Result` gained `Cursor`; `HashKey` exported; new optional `Resolver` interface.
- Source API accepts types `webhook` and `x`; responses show `webhook_path`, `webhook_secret` (create/rotate only), `x`.
- Migration `0003` (relaxes the type CHECK, adds columns and `usage_monthly`, RLS + revokes).

### Security notes
Webhook secrets never stored, logged, listed or audited; constant-time comparisons; replay window + idempotent dedupe; uniform failure response; bounded bodies and rates. X token only from env, redacted from errors; paid calls gated by a persistent budget (also across restarts and instances); user ids are never taken from the client.

### Known limitations / open items
- X connector has not been run against the real API (owner is not testing X yet): response shapes follow the documented v2 format; endpoint access depends on your X plan. Pricing is configured by env, not fetched.
- Terms of use for X and the three news sites still need to be checked before production use.
- Webhook endpoint is a plain handler (not an OpenAPI operation), documented in the overview and `docs/WEBHOOKS.md`.

### Files (main)
`internal/webhook/*` · `internal/connector/x.go` · `internal/api/{hooks,sources,server,docs}.go` · `internal/domain/sourcedef/sourcedef.go` · `internal/storage/{memory/repos.go,postgres/sources.go,postgres/migrations/0003_*}` · `internal/ingestion/scheduler.go` · `internal/config/config.go` · `cmd/server/main.go` · `docs/WEBHOOKS.md` · `docs/sources/x-accounts.md` · tests

### Next
Owner run of `check`; send a real webhook delivery; then M5 (alerts and channels).

---

## [0.5.0-dev] — 2026-10-17 — M3 Sources: model, scheduler, RSS/Atom + page monitor connectors, manual posts (TP-011, 012, 013, 014, 015, 018)
**Verification:** ⚠️ not compiled or run by the author (no Go toolchain in the sandbox). Owner has verified everything up to 0.4.0-dev. Run `docker compose run --rm tidy` (new deps: goquery, cascadia, x/net) then `check`.
**Suggested commit message:**
```
feat(sources): configurable sources, per-source scheduler, RSS/Atom and page-monitor connectors

- source model + admin API (create, test draft, update, run-now, delete, health) and member catalog
- scheduler: per-source intervals, independent bounded runs, per-host spacing, failure back-off, adaptive interval, silence watchdog
- ingestion semantics: baseline first run, stale-item guard, atomic dedupe (Postgres PK), run history, retention
- connectors: RSS/Atom (conditional GET, sanitized text, safe links) and web monitor (CSS selectors, robots.txt) via safefetch
- manual posts through manual sources; migration 0002 with RLS
- fix(safefetch): scheme checked before host; ValidateURL for config-time checks
- 39 new tests (23 security)
```

### Why
M3 delivers the product's core promise: connect real sources without a deploy and see new items live. Latency is configurable per source (D-022); everything fetched goes through the SSRF-safe fetcher (D-026).

### Added
- **Domain** `sourcedef`: `Source` (permanent slug name, type `rss|webmonitor|manual`, URL, interval, topics, web selectors), `State`, `Run`, repositories, `Normalize` and `Health` (ok/pending/degraded/failing/overdue/disabled).
- **Connectors** (`internal/connector`): RSS 2.0 + Atom (encoding/xml: no DTD/entity expansion, charset decoding, 100-item cap, conditional GET with ETag/Last-Modified), web monitor (goquery CSS selectors, validated; relative links resolved), `Robots` (robots.txt groups, longest match, wildcards, cached, fails closed on 5xx), text sanitizing (`stripTags`, `safeLink`: plain http(s) only, no credentials).
- **Scheduler** (`internal/ingestion/scheduler.go`): each source on its own interval; parallelism cap; one fetch per source at a time; per-host spacing (2 s default); ±10% jitter; failure back-off (30 s doubling, capped by the source's interval, floored at the global minimum); interval stretches after 10/30 unchanged runs; per-run timeout; watchdog logs on failing/overdue; hourly pruning (30 days) of runs and raw items.
- **Ingestion rules (D-029):** first success is a baseline; items older than 48 h never published; dedupe by `(source, key)` before publishing; storage errors fail the run without advancing state.
- **Storage:** migration `0002_sources` (sources, source_runs, source_items; RLS + revokes); Postgres and in-memory repositories.
- **API** (all documented in /docs): `GET /sources` (members), admin `POST/GET /admin/sources`, `GET/PUT/DELETE /admin/sources/{id}`, `POST /admin/sources/{id}/run`, `POST /admin/sources/test`, `POST /admin/items`. Sources start disabled; names are permanent; SSRF checks at create/update/test and again at fetch time.
- **Config:** `FETCH_USER_AGENT` (optional).
- **Docs:** how to add the three owner sources in `docs/sources/news-pages.md`; decisions D-028…D-031.

### Changed
- `safefetch.validateURL` reports a disallowed scheme before host problems; new `safefetch.ValidateURL` for configuration-time checks (fix for the failing test the owner reported).
- `api.Deps` gained `Sources`, `Runs`, `Bus`, `Connectors`; `main.go` wires the fetcher, connectors and scheduler.

### Security notes
SSRF guard on every fetch; robots.txt respected by default (override needs a recorded reason, audited via source config); feed/HTML content reduced to plain text; links limited to plain http(s); XML entity expansion not possible; catalog never exposes source URLs; admin-only management with audit entries; scheduler isolation prevents one bad source from starving others.

### Known limitations / open items
- Not yet run against the three real sites: feed URLs, terms of use, robots.txt and CSS selectors must be confirmed (TP-021).
- Web monitor does not execute JavaScript (pages that render items client-side need a feed or JSON endpoint).
- Publication is at-most-once (D-029). No per-source secrets yet (needed for X in M4).
- Subscriptions matching on `source` use the permanent source name; deleting a source stops matching but keeps members' subscriptions.

### Files (main)
`internal/domain/sourcedef/*` · `internal/connector/*` · `internal/ingestion/scheduler.go` · `internal/storage/{memory/repos.go,postgres/sources.go,postgres/migrations/0002_*}` · `internal/api/{sources,server,docs}.go` · `cmd/server/main.go` · `internal/safefetch/*` · tests · `TESTING.md` · `DECISIONS.md` · `docs/sources/news-pages.md`

### Next
Owner run + TP-021 (enable the three sources), then M4 (X connector with budget cap, inbound webhook).

---

## [0.4.0-dev] — 2026-10-17 — PostgreSQL storage via env, migrations, readiness, safe fetcher (TP-007, TP-010) → M2 complete in code
**Verification:** ✅ owner confirmed working on Windows 11 (2026-10-17), including data persisting across restarts (one safefetch test needed the scheme-order fix, shipped in 0.5.0-dev).
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
**Verification:** ✅ owner confirmed working on Windows 11 (2026-10-17).
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
