# TradePulse — Project Brain & Execution Charter

**Document ID:** TP-PROJECT-BRAIN · **Version:** 1.0.0 · **Status:** Baseline / ready for implementation planning
**Last updated:** 2026-10-15 · **Working name:** TradePulse (code module `tradepulse`; the product is domain-agnostic, rename is an open decision, Q-09)
**Product type:** Invite-only SaaS that aggregates information from many sources, shows one personalized live feed, and alerts automatically
**Stack:** Go 1.24 + Huma (OpenAPI) · PostgreSQL (Cloud SQL) · Docker · Google Cloud Run · React/TypeScript frontend (later)
**Purpose:** Single source of truth for product intent, architecture, build order, quality gates and AI-agent execution.

---

## 0. How to use this document

Humans and coding agents read this before any change. Companion files at the repo root:

| File | Purpose |
|---|---|
| `PROJECT_BRAIN.md` | this charter (what & why & rules) |
| `AGENTS.md` | short agent rules (kept in sync; `CLAUDE.md` imports it) |
| `ROADMAP.md` | milestone status |
| `BACKLOG.md` | work items with acceptance criteria |
| `DECISIONS.md` | decisions, open questions, rationale |
| `CHANGELOG.md` | **single record of important changes** (what/why/files/breaking/verification/next) |
| `TESTING.md` | test register, security coverage matrix, gaps, verification log |
| `docs/adr/`, `docs/runbooks/`, `docs/sources/` | ADRs, ops procedures, per-source legal/config notes (create when first needed) |

**Precedence if instructions conflict:** (1) decision in `DECISIONS.md` → (2) this charter → (3) approved ADRs → (4) existing tests/behavior that don't violate 1–3 → (5) agent assumptions.
Never silently resolve a conflict touching security, access control, cost, external-source terms or data retention. Record it and ask.

**Executing work:** one bounded work item at a time (ID, goal, scope, acceptance criteria, dependencies, tests, status). Run checks, summarize files/risks, update `BACKLOG.md`/`ROADMAP.md` only when exit criteria are truly met.

---

# 1. Product vision

## 1.1 Statement
TradePulse lets a person or team **connect many information sources** (news pages, RSS feeds, social accounts, webhooks, manual posts), normalizes everything into one event stream, shows **everything relevant to them in a single real-time view**, and **alerts them automatically** by rules they define or enable from templates.

It is domain-agnostic: *domains are configuration* (sources + keywords/entities + alert rules), never code branches.

## 1.2 Users
1. **Platform Admin** — owns the platform: invites members, manages sources, sees system health, suspends accounts.
2. **Member** — invited user: subscribes to topics/sources, builds views and alerts, receives the live feed.
3. *(Later)* **Workspace owner/team member** — shared sources, views and alerts.

## 1.3 Principles
- **Trust before breadth:** accurate, deduplicated, traceable items; no alert spam; every item links back to its original source ("view original").
- **Explainable:** every feed item/alert records *why* it matched (rule, values, source).
- **Config over code:** admins/users add sources and alerts via validated structured config. Never execute user-supplied code or expressions.
- **Source isolation:** one failing/slow source never affects others.
- **Cost-aware:** paid sources (e.g. X API) have explicit budgets and hard caps.
- **Fail closed:** authorization, validation and egress rules default to deny.
- **Provider independence:** sources, channels, storage and bus sit behind interfaces.
- **Incremental complexity:** reliable single-instance MVP before scale-out, billing or AI.

## 1.4 Non-goals for MVP (need a recorded decision to start)
Native mobile apps · billing/subscriptions · posting to X or any source · scraping logged-in or private content · scraping X (official/licensed API only) · headless-browser/JS rendering · recommendations or advice · AI summaries/clustering · multi-instance fan-out · user-supplied code/scripts · public sign-up.

---

# 2. MVP definition

## 2.1 Required capabilities (all working end-to-end in Docker and on Cloud Run)
1. Invite-only registration (admin-generated codes handed out manually), login, secure tokens.
2. Roles (admin/member) and status (active/suspended); suspension cuts live connections.
3. Admin source management: create/enable/disable/test sources via API (secrets by reference).
4. Connectors: **RSS/Atom, web-page monitor (CSS-selector extraction), manual post, X account (official API, budget-capped)**; inbound webhook and JSON-poll as P1.
5. Normalization to a generic Event; deduplication; raw-item storage with retention.
6. Subscriptions (sources, keywords/phrases, entity tags) and mute list.
7. Unified live feed (WebSocket + REST with cursor) with saved views and "view original".
8. Alerts: templates + custom rules (whitelisted fields/operators), cooldowns, fire caps, **preview**, explainability, auto-pause of noisy rules.
9. Channels: in-app (WebSocket) and outbound webhook (Slack/Discord/Telegram-compatible); email is P1.
10. Feedback (useful / not useful) on feed items and alerts.
11. Audit log for admin and security-sensitive actions.
12. Source health dashboard data (last success, error rate, silence watchdog) via admin API.
13. PostgreSQL persistence with migrations; `/readyz` checks DB.
14. Automated unit, integration, security-boundary and WebSocket end-to-end tests.
15. Docker-only local setup that works from a clean clone on Windows/macOS/Linux.
16. Staging and production on Cloud Run with CI/CD, secrets in Secret Manager, runbook.

## 2.2 Success measures (track from first beta use)
- Admin adds an RSS source and a web-page source **without a deploy or DB edit**.
- A new member reaches a first useful alert in **≤ 10 minutes**.
- Duplicate items: zero for same source; reported cross-source duplicates tracked.
- Alert precision (rated useful) **> 70%**; alert-spam reports near zero.
- Source freshness: median detection delay ≤ configured poll interval + 1 min.
- Zero cross-user data access in security tests; zero silent account/rule loss across restarts and deploys.
- A clean machine runs the stack from the README.
Performance targets are *measured first* (load test in M6), then set — do not invent benchmarks.

---

# 3. Functional requirements
Priority: **P0** release blocker · **P1** important · **P2** later.

## 3.1 Identity, invites, access
- P0: passwords hashed with PBKDF2-SHA256 (or argon2/bcrypt if approved); HS256 JWT with strict header check (exists).
- P0: registration requires a valid invite code: 128-bit random, **stored hashed**, single-use, expires (default 7 days), optional email binding, **atomic redeem** (race-tested).
- P0: roles `admin|member`, status `active|suspended`; server-side checks on every protected operation; admin routes under `/api/v1/admin/*` return 403 to members.
- P0: first admin created at startup from `BOOTSTRAP_ADMIN_EMAIL`/`BOOTSTRAP_ADMIN_PASSWORD` (idempotent, never overwrites, never over HTTP) — D-015.
- P0: login/registration rate-limited per IP **and** per account.
- P1: refresh tokens (rotating, DB-stored, revocable, reuse detection), logout, password change, admin-triggered reset.
- P1: session list/revocation.

**Acceptance:** no registration without a valid unused code; the same code used concurrently succeeds exactly once; suspended user's API calls and WebSocket are rejected within seconds.

## 3.2 Sources and connectors
Every source has: `id, name, type, enabled, schedule/interval, config (declarative), secret_ref, mapping, budget, health`.

| Connector | Priority | Notes |
|---|---|---|
| RSS/Atom | P0 | conditional GET (ETag/Last-Modified), min interval 5 min |
| Web-page monitor | P0 | fetch URL → extract item list with CSS selectors (title, link, time) → events; no JS rendering; respects robots.txt by default (override only with recorded reason); polite UA; min interval 5 min |
| Manual post | P0 | admin posts an item (title, summary, url, entities) |
| X account | P0 (gated by Q-02/Q-03) | **official X API only**, pay-per-use credits; poll with `since_id` so cost ≈ new posts returned; per-source monthly budget + global hard cap; stops polling when exhausted and raises a health alert |
| Inbound webhook | P1 | per-source secret, HMAC signature, timestamp/replay window, size limit |
| HTTP-poll JSON | P1 | declarative JSON-path mapping |
| Stream (WS/SSE) | P2 | supervised reconnect already exists |

**Every connector must:** use the **safe fetcher** (below), time-box and size-limit responses, back off with jitter on errors, use a circuit breaker, store the raw item (retention-limited), set a deterministic **dedupe key** (`source_id + guid | canonical_url | content hash`), emit UTC timestamps, and record a run row (`source_runs`).

**Safe fetcher (P0, built before any connector):** http/https only, ports 80/443, DNS resolve → block loopback, RFC1918, link-local and **cloud metadata addresses (169.254.169.254, metadata.google.internal)**, pin the resolved IP for the request, re-validate on each redirect (max 3), 10 s timeout, 2 MB body cap, no cookies/credentials from admin-supplied URLs.

**Legal/terms gate:** each shipped source gets `docs/sources/<name>.md` (terms/robots checked, rate limit, cost, attribution, decision date). Unknown terms → do not enable in production. Owner has deferred *privacy* compliance (D-008); **source terms of use are not deferred.**

## 3.3 Events and normalization
Generic Event: `id, source_id, type, title, summary, url, occurred_at, ingested_at, entities[], payload{}, dedupe_key, raw_ref`.
- P0: **event type registry** (type id, payload field schema, title/body templates). Default type `content_item`; sources may define more.
- P0: unique `(source_id, dedupe_key)`; idempotent ingestion (restart/replay safe).
- P1: cross-source duplicate flagging (canonical URL + title similarity); P2: story clustering.
- Core code contains **no domain terms** (price, stock, volume); the current market example moves to `examples/` (see BASELINE §5.6).

## 3.4 Subscriptions, views, feed
- P0: subscriptions of kind `source | keyword/phrase | entity`; mute list; limits per user.
- P0: matching via inverted index (keyword/entity → subscribers) so each event is evaluated only against interested users.
- P0: feed = items matching subscriptions, persisted per user (capped), cursor pagination, newest first, `GET /me/feed?after=<cursor>`.
- P0: **WebSocket reconnect + catch-up:** clients reconnect with backoff and fetch missed items via the cursor (Cloud Run closes long connections; deploys drop them).
- P1: saved views (filters over sources/keywords/types/time).

## 3.5 Alerts
- Rule = **scope** (sources/entities/keywords/types) + **conditions** (`field op value`, ops: `> >= < <= == != in contains any_of`; AND first, OR later) + **behavior** (cooldown, severity, channels, enabled).
- P0: templates (shipped, pre-filled, tweakable); custom rules from whitelisted fields of the event-type schema; validation server-side.
- P0: **preview** — replay recent events, report "would have fired N times (examples)".
- P0: guardrails — max rules/user, max fires/rule/hour, auto-pause + notify on runaway rules, per-rule cooldown.
- P0: explainability stored with each fire (rule id, matched values, event id).
- P1: quiet hours; AND/OR groups; digest mode. P2: natural-language → *draft* rule (AI), user-reviewed, same validation, never executes anything.

## 3.6 Channels
- P0: in-app (WebSocket) with persisted feed fallback. P0: outbound webhook (HTTPS only, SSRF-guarded, signed payload, retry with backoff, delivery history, disable after repeated failure).
- P1: email (provider TBD, Q-06); P1: Telegram bot. P2: web push.
- Delivery history per channel; failures visible; retries idempotent.

## 3.7 Admin & operations
- P0: source CRUD/test-fetch/enable/disable; invites; users; audit log read; source health (last success, error rate, items/day, X budget used).
- P0: silence watchdog — alert admins when an enabled source yields nothing for N× its expected interval.
- P1: usage counters per source/user for cost visibility.

---

# 4. Non-functional requirements
## 4.1 Security
Auth on every private route; user ID derived from token only; all user data scoped by `user_id`; strict size-limited JSON; parameterized SQL only; secrets only from env/Secret Manager and never logged; constant-time secret comparison; security headers/CSP; CORS allowlist; WS tickets (no JWT in URLs) + origin check; docs absent in production; dependency/vuln/image scanning in CI; non-root distroless image; SSRF guard on **all** outbound fetches (sources, webhooks).
## 4.2 Privacy (deferred by owner — D-008)
No formal compliance work in the beta. **Retained baseline anyway:** hashed passwords/invites, least-privilege access, no PII or tokens in logs, manual deletion on request, minimal data collected. **Revisit before:** any non-invited/public users, any paid launch, or any member outside the trusted circle.
## 4.3 Reliability
Single instance first; graceful shutdown; supervised connectors; at-least-once ingestion with dedupe; bounded queues (drop + metric, never block ingestion); DB backups with a tested restore; Cloud Run revision rollback documented.
## 4.4 Observability
Structured JSON logs (request id, source id, run id) to Cloud Logging; metrics: request latency/error rate, bus drops/queue depth, WS connections, per-source freshness/errors, alert fires, X spend; alerts on source silence, error spikes, budget >80%.
## 4.5 Usability (for the later frontend)
Mobile-first, accessible, clear loading/empty/error states, every item shows source + time + "view original" + "why am I seeing this".

---

# 5. Architecture and constraints
## 5.1 Pipeline
```
Connectors → RawItem (stored) → Normalizer (mapping) → Event (deduped)
 → Bus → (Signals) → Matching (subscription index + alert rules)
 → FeedItem / Alert fire → Channels (WebSocket, webhook, later email) → API/UI
Admin plane: sources, invites, users, audit, health.   User plane: subscriptions, views, alerts, channels.
```
Modular monolith with a `ROLE` flag (`all` default; `api`/`worker` splits possible later).

## 5.2 Google Cloud Run topology (beta)
| Concern | Decision / constraint |
|---|---|
| Compute | One Cloud Run service. **min-instances=1, max-instances=1, CPU always allocated** — required because pollers and WebSocket pushes run outside request handling (default Cloud Run throttles CPU between requests; scale-to-zero would stop polling). |
| Why max=1 | Bus, rate limits, WS tickets and the subscription index are in-memory. Scaling out requires Redis/Pub/Sub first (post-MVP, measured need). |
| WebSocket | Supported, but connections end at the request timeout (configurable, up to 60 min) and on every deploy → clients must reconnect + catch up (§3.4). Verify current limits in GCP docs. |
| Database | Cloud SQL for PostgreSQL (smallest viable), private connectivity/Cloud SQL connector; pgx pool sized below instance connection limit; automated backups + point-in-time recovery; restore drill before beta. |
| Secrets | Secret Manager → env vars: `JWT_SECRET`, DB credentials, X API token, webhook signing keys. Never in images, git or logs. |
| Registry/CI | Artifact Registry; GitHub Actions (or chosen CI) → deploy using **Workload Identity Federation** (no long-lived service-account keys). |
| Environments | `staging` and `prod` as separate Cloud Run services (ideally separate projects) with separate DBs and secrets. |
| Ingress/TLS | Cloud Run managed HTTPS + custom domain; `TRUST_PROXY=true`; docs disabled; least-privilege runtime service account. |
| Cost | Always-on instance + Cloud SQL + X API credits are recurring costs: estimate with the GCP pricing calculator in M0 and set budget alerts. |
| Local parity | `docker compose` mirrors env vars; a fake-source HTTP server provides fixtures so CI never calls live sites or X. |

## 5.3 Repository structure (target)
```
cmd/server  cmd/openapi  cmd/adminctl?    wiring only
internal/domain/{event,user,feed,rule,source,alert}   pure; no infrastructure imports
internal/connector/{rss,webmonitor,manual,xapi,webhook}  + internal/safefetch
internal/processing  internal/ingestion  internal/personalization  internal/channel/{ws,webhook,email}
internal/api  internal/auth  internal/storage/{postgres,memory}  internal/infrastructure/eventbus
migrations/  examples/  docs/{adr,runbooks,sources}  deploy/ (Cloud Run service YAML, scripts)
```
## 5.4 Backend boundaries
Domain defines interfaces; infrastructure implements; `cmd/server` wires. Handlers are thin (validate → call service → map errors). No SQL outside `storage/postgres`. No outbound HTTP except through `safefetch`. Every goroutine takes a context and stops on cancel.
## 5.5 API conventions
Huma code-first; `/api/v1`; RFC 9457 problem errors; cursor pagination; request-id header; idempotent create where retried (`Idempotency-Key` for invite/source create); `openapi.json` committed and CI-checked; WebSocket described in `docs/WS.md` (OpenAPI cannot).
## 5.6 Baseline (as built, v0 skeleton)
Done: pipeline skeleton, in-memory bus/repos, simulated market source + unusual-activity signal, JWT/PBKDF2/WS tickets, Huma API with `/docs` (dev-only), hand-written WebSocket hub, rate limits, Docker-only dev (Windows/Linux), prod Dockerfile + compose example.
Gaps: not yet compiled/verified by the owner; market-specific code in core (event types, thresholds, `render.go`, watchlist); no persistence/invites/roles/real connectors/alerts; single-instance in-memory state.

---

# 6. Data model baseline
**Entities:** `users, invites, refresh_tokens, audit_log, sources, source_runs, raw_items, event_types, events, subscriptions, views, alert_templates, alert_rules, alert_fires, feed_items, feedback, channels, channel_deliveries, usage_counters`.
**Integrity:** unique `(source_id, dedupe_key)` on events/raw_items; invites store only a hash + `used_at`/`used_by` (redeem in one transaction with row lock); FKs from all user-owned rows to `users`; feed capped per user by retention job; `ON DELETE` rules explicit; timestamps `timestamptz` UTC; migrations reversible or documented irreversible.
**Isolation:** every user-owned query includes `user_id` from the token; repository methods take `userID` explicitly; negative tests for cross-user reads/writes.
**Retention defaults (proposed, confirm Q-08):** raw items 30 days, events 180 days, feed per user last 1,000 items, audit log 1 year, source_runs 30 days.

---

# 7. API inventory (v1)
- **Public:** `GET /healthz, /readyz`; `POST /auth/register` (invite code required), `/auth/login`, `/auth/refresh`, `/auth/logout`.
- **Me:** `GET/PUT /me/subscriptions`, `GET/POST/PUT/DELETE /me/views`, `GET /me/feed?after=`, `POST /me/feed/{id}/feedback`, `GET/POST/PUT/DELETE /me/alerts`, `POST /me/alerts/preview`, `GET /alert-templates`, `GET/POST/DELETE /me/channels`, `POST /ws-ticket`.
- **Realtime:** `GET /ws?ticket=` (messages documented in `docs/WS.md`).
- **Admin (role=admin):** `POST/GET/DELETE /admin/invites`, `GET /admin/users`, `POST /admin/users/{id}/suspend|reactivate`, `CRUD /admin/sources`, `POST /admin/sources/{id}/test`, `GET /admin/sources/{id}/runs`, `GET /admin/health`, `POST /admin/items` (manual post), `GET /admin/audit`.
- **Inbound (P1):** `POST /hooks/{source_id}` (HMAC-verified).

---

# 8. UX (frontend starts only after API contract freeze, milestone M7)
Screens: invite signup/login · live feed (filters, saved views, original-link, "why this", 👍/👎) · subscriptions · alert builder with template picker + **preview** · channels · admin: sources (+ test fetch, health), invites, users, audit.
Rules: mobile-first; reconnect banner + catch-up; no color-only status; empty states explain next step.

---

# 9. Testing and quality
## 9.1 Layers
Unit (rules, mapping, dedupe, safefetch, auth) · integration (real Postgres in compose/CI) · **connector tests from recorded fixtures — no live network in CI** · security-boundary (authz, cross-user, invite race, SSRF, webhook signature/replay) · WebSocket end-to-end · load test (many WS clients, ingest burst) in M6.
## 9.2 Minimum end-to-end journeys
1. Admin creates invite → member registers → redeem again fails.
2. Admin adds RSS source → item appears in member's feed once, even after restart/replay.
3. Member subscribes to keyword → creates alert from template → preview → live WebSocket delivery with explanation.
4. Suspended member loses API and WebSocket access.
5. Source outage → health alert; recovery resumes without duplicates.
6. X budget exhausted → polling stops, admin alerted, no overspend.
## 9.3 Definition of Done (every change)
Tests incl. `-race` · `docker compose run --rm check` green · `openapi.json` regenerated · migrations + rollback notes · security review against §4.1 · no new dependency without a decision entry · backlog/roadmap updated · **CHANGELOG.md entry and TESTING.md register updated** · ROADMAP "Current state & next action" rewritten · no secrets or production data in code/fixtures/logs.

---

# 10. Docker, local development, CI/CD
**Local:** Docker only (Windows 11/macOS/Linux): `docker compose up --build` (hot reload, `/docs`), `docker compose run --rm check|test|openapi|tidy`. Compose will add: `postgres`, `fakesource` (fixtures), `mailpit` (when email lands).
**CI gates:** vet · `test -race` · OpenAPI diff · migration up/down on fresh DB · `govulncheck` · `gosec` · container image scan · secret scan.
**CD:** build image → Artifact Registry → deploy **staging** → smoke tests (`/readyz`, login, ingest fixture) → manual approval → **prod** → watch error rate → rollback = route traffic to previous revision.

---

# 11. Roadmap and milestones (status lives in `ROADMAP.md`)
| M | Name | Exit criteria |
|---|---|---|
| M0 | Foundation green | compose `check` passes on Windows + Linux; `go.sum`/`openapi.json` committed; CI green; control docs present; WS end-to-end test |
| M1 | Generic core | event type registry, field-condition rules, keyword/entity subscriptions; no market terms in core; a non-market demo works unchanged |
| M2 | Access & persistence | invites + roles + audit; Postgres + migrations; restart loses nothing; security tests pass |
| M3 | Sources | safefetch; source CRUD/health; RSS, web-page monitor, manual post; dedupe + raw store; source silence watchdog |
| M4 | X + webhook | X connector with budgets (needs owner approval of credentials/budget); inbound webhook with HMAC/replay protection |
| M5 | Alerts & channels | rules, templates, preview, cooldown/caps/auto-pause; WS + outbound webhook channels; feedback |
| M6 | Cloud Run & beta readiness | staging/prod on Cloud Run, Secret Manager, backups + restore drill, metrics/alerts, refresh tokens, load test, runbook → **invite first members** |
| M7 | API freeze + frontend MVP | pagination/WS docs/error shape frozen; typed client; UI for feed, subscriptions, alerts, admin |
| M8 | Post-MVP | email/Telegram/push, story clustering, AI-drafted rules/summaries, Redis/Pub/Sub scale-out, teams/workspaces, billing, privacy compliance |

Do **not** start connectors before M2's security tests and the safe fetcher (TP-012). Do not start the frontend before M7.

---

# 12. AI-agent execution protocol (mandatory)
## 12.1 Before coding
Read `AGENTS.md`, this file, `DECISIONS.md`, `ROADMAP.md` and the backlog item; inspect the repo (don't assume files/libs exist); confirm dependencies and acceptance criteria; check ADRs; if a critical decision is missing, ask in a short decision request; write a brief plan including tests.
## 12.2 While coding
Implement only the assigned scope; follow existing patterns; small reviewable changes; no framework changes or large rewrites without approval; explain every new dependency; never weaken validation, auth, SSRF guards, tests or limits to make something pass; no secrets in code/fixtures/logs; no production data locally; no live network in tests; never claim an external integration works without verifying it against the provider's sandbox/live behavior; update migrations, OpenAPI and tests with the code.
## 12.3 Before reporting completion
Run `docker compose run --rm check`; inspect the diff for unrelated changes/secrets; verify each acceptance criterion; update backlog/roadmap only if truly done; add a changelog entry; update this file or an ADR if design changed. Report: summary · files changed · commands run + outcomes · security implications · known limitations · suggested next task.
## 12.4 Stop and ask a human if
Requirements conflict or an approval is missing · a change could expose another user's data · a migration could destroy data · a source needs paid credentials, new terms acceptance or a live account · a source's terms/robots are unclear or forbid access · outbound-request rules (SSRF) would be relaxed · a change raises recurring cloud/API cost · a request asks to bypass auth, audit, tests or limits · scope exceeds the assigned milestone.
## 12.5 Work item template (`BACKLOG.md`)
```markdown
### TP-XXX — Short title
- Status: Proposed | Ready | In Progress | Blocked | Review | Done
- Priority: P0 | P1 | P2        - Milestone: Mx
- Goal / user value:
- Scope included / excluded:
- Dependencies:
- Acceptance criteria:
  - [ ] ...
- Tests required:
- Security/cost considerations:
- Docs to update:
- Notes / decision links:
```
## 12.6 Pull request template
```markdown
## Summary / Work item
## User-visible changes
## Implementation notes
## Tests run
## Security & cost review (authz, SSRF, secrets, API spend)
## Database/API changes (migration + rollback)
## Known limitations
```

---

# 13. Decisions log (detail in `DECISIONS.md`)
| ID | Decision | Status |
|---|---|---|
| D-001 | Modular monolith, Go, Huma code-first OpenAPI, stdlib-lean | Decided |
| D-002 | Docker-only dev workflow (Windows/macOS/Linux) | Decided |
| D-003 | `/docs` and `/openapi.json` exist only in development | Decided |
| D-004 | Platform is domain-agnostic; domains are configuration | Decided |
| D-005 | Invite-only; codes generated by admin and shared manually; no email infra yet | Decided (Q-01) |
| D-006 | Hosting: Google Cloud Run + Cloud SQL + Secret Manager; beta runs one always-on instance | Decided (Q-03) |
| D-007 | First sources: news pages and X accounts → RSS / web-page monitor / X API connectors | Decided (Q-02) |
| D-008 | Privacy-compliance work deferred for beta; security baseline retained; revisit before public/paid | Decided (Q-04) |
| D-009 | Alerts are structured whitelisted rules, never user code | Decided |
| D-010 | In-memory bus/state until measured need; Redis/Pub/Sub later | Decided |
| D-011 | X data only via official/licensed API with budget caps; no scraping X | **Proposed** — confirm |
| D-012 | MVP alert channels: in-app + outbound webhook; email post-MVP | **Proposed** — confirm |
| D-013 | Migrations tool (goose vs golang-migrate) + pgx | **Proposed** — ADR in TP-007 |
| D-014 | Frontend only after API freeze (M7) | Decided |

### ADR template (`docs/adr/ADR-XXX.md`)
`Status · Date · Owner · Context · Options · Decision · Consequences · Security/cost implications · Migration/rollback · Related items`

---

# 14. Risks and mitigations
| Risk | Impact | Mitigation |
|---|---|---|
| X API cost grows with volume | High | pay-per-use budgets per source + global hard cap, `since_id` polling, spend metric/alerts, stop at cap |
| Source terms/robots forbid access | High | per-source legal note, robots respected, official feeds first, no X scraping |
| SSRF via admin/webhook URLs (incl. GCP metadata server) | Critical | safefetch (IP/DNS/redirect rules), tests, only fetch path allowed |
| Web-page monitor breaks on layout change | Medium | health watchdog, fixtures, selector test-fetch, raw store for replay |
| Alert spam destroys trust | High | cooldowns, caps, preview, auto-pause, feedback loop |
| Cloud Run scale-to-zero/CPU throttling stops polling | High | min=1 + CPU always allocated, documented and verified in staging |
| In-memory state lost on deploy/restart | Medium | persistence for users/feed/sources; WS catch-up cursor; restart tests |
| Single-instance ceiling | Medium | documented limit; Redis/Pub/Sub plan triggered by metrics |
| Invite code leakage/race | High | hashed codes, expiry, single-use atomic redeem, audit |
| Privacy deferral becomes liability | Medium | retained baseline; explicit revisit triggers (§4.2) |
| Scope creep / agent drift | High | milestones, stop conditions, decision precedence |
| Unverified code (never compiled in authoring sandbox) | Medium | M0 compile/test gate first |

---

# 15. Release, versioning, change management
Product: semantic versioning (MAJOR incompatible API/data, MINOR features, PATCH fixes). This document: PATCH = clarification, MINOR = new requirement/phase, MAJOR = change to product model, access model or architecture.
**Change procedure:** describe change/value → find impacted requirements/API/schema/tests/cost/security → if it alters an approved decision write an ADR/decision entry → update brain, backlog, OpenAPI, roadmap → implement in small items → migration + rollback notes → verify acceptance → changelog. Never rewrite history: supersede decisions and say why.
**Release checklist:** P0 acceptance passes · CI + scans green · migrations tested on representative data · backup exists and restore tested · secrets verified in Secret Manager · security-boundary + SSRF tests pass · budgets/alerts active · runbook + rollback ready · owner approves.

---

# 16. First sprint: exact task order (seed backlog)
1. **TP-001** Baseline green (compile, `check`, `go.sum`, `openapi.json`, CI) — M0
2. **TP-002** WebSocket end-to-end test — M0
3. **TP-003** Control docs in repo; AGENTS.md aligned — M0
4. **TP-004** Event type registry + generic Event (`content_item`) — M1
5. **TP-005** Field-condition rule engine + keyword/entity matching index — M1
6. **TP-006** Subscriptions replace watchlist; move market example to `examples/` — M1
7. **TP-007** Postgres + migrations + repositories + `/readyz` (ADR D-013) — M2
8. **TP-008** Roles/status + invites (hashed, single-use, atomic redeem) — M2
9. **TP-009** Admin API (invites/users/suspend), env-bootstrapped first admin, audit log — M2
10. **TP-010** Safe fetcher (SSRF guard) with tests — M3 (**blocks all connectors**)
11. **TP-011** Source model + admin CRUD + secret refs + run history + health — M3
12. **TP-012** Raw store + dedupe + silence watchdog — M3
13. **TP-013** RSS connector · **TP-014** Web-page monitor · **TP-015** Manual post — M3
14. **TP-016** X connector with budgets — M4 (blocked on Q-02/Q-03 credentials + budget)
Full items with acceptance criteria: `BACKLOG.md`.

---

# 17. Owner questions
| ID | Question | Needed by |
|---|---|---|
| Q-01 | ~~Invite delivery~~ → manual admin-copied codes | resolved |
| Q-02 | Exact first sources: **news page URLs** (do they offer RSS?) and **X handles** | M3 / M4 |
| Q-03 | GCP project(s), region (e.g. asia-south1), monthly budget cap, **X API monthly budget** and who owns the X developer account | M4 / M6 |
| Q-04 | ~~Privacy~~ → deferred for beta | resolved |
| Q-05 | Alert channel after in-app/webhook: Telegram, Slack, email, push? | M5 |
| Q-06 | Email provider (if email) | M8 |
| Q-07 | Acceptable alert latency per source type (e.g. news 5 min, X 1–5 min — cost-driven) | M3 |
| Q-08 | Retention periods (defaults proposed in §6) | M2 |
| Q-09 | Product name / domain | before M7 |
| Q-10 | Repo host and CI (GitHub Actions assumed) | M0 |
| Q-11 | Expected number of beta members and sources | M6 |

---

# 18. Final operating rule
**Build a trustworthy pipeline first — correct, deduplicated, explainable items from sources we are allowed to use — then add breadth.**
The first successful release is not the one with the most screens: it is the one where an admin adds sources without a deploy, an invited member sees one clean live feed, gets a useful alert from a rule they understand, and the system keeps its data across restarts and deploys while staying inside its security and cost limits.
When uncertain: preserve data, preserve audit history, fail closed, respect source terms and budgets, and ask for a decision rather than inventing behavior.
