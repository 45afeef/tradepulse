# BACKLOG.md — work items (template: `PROJECT_BRAIN.md` §12.5)
Statuses: Proposed | Ready | In Progress | Blocked | Review | Done. Work top-down; one item at a time.

### TP-001 — Baseline green
- Status: Done (owner confirmed on Windows 11, 2026-10-17; `openapi.json`/`go.sum` commit is the owner's)
- Goal: code compiles and checks pass from a clean clone on Windows and Linux.
- Acceptance:
  - [ ] `docker compose run --rm check` passes
  - [ ] `go.sum` and `openapi.json` committed (`docker compose run --rm openapi`)
  - [ ] CI workflow green; `docker compose up --build` serves `/docs`
- Tests: existing unit tests. Security/cost: none.

### TP-002 — WebSocket end-to-end test
- Status: Review (implemented in 0.3.0-dev; unverified — see CHANGELOG/TESTING) · P0 · M0 · Depends: TP-001
- Acceptance:
  - [ ] test: register → set subscription/profile → ticket → WS connect → receives a feed item
  - [ ] ticket reuse and bad origin rejected

### TP-003 — Control docs aligned
- Status: Done · P0 · M0 — charter, decisions, roadmap, backlog, changelog in repo; AGENTS.md points to them.

### TP-004 — Event type registry + generic Event
- Status: Done (M1 confirmed working by owner, 2026-10-17)
- Goal: types defined as data (id, payload schema, title/body templates); default `content_item`.
- Acceptance:
  - [ ] no hardcoded market enum in core; unknown types rejected
  - [ ] rendering uses templates from the registry
- Tests: registry validation, render templates.

### TP-005 — Field-condition rules + matching index
- Status: Done (M1 confirmed working by owner, 2026-10-17)
- Acceptance:
  - [ ] ops `> >= < <= == != in contains any_of` over whitelisted fields only
  - [ ] keyword/entity inverted index; events evaluated only against interested users
  - [ ] unit tests incl. malicious/oversized input
- Security: no expression evaluation; bounded rule size.

### TP-006 — Subscriptions replace watchlist
- Status: Done (M1 confirmed working by owner, 2026-10-17)
- Acceptance:
  - [ ] subscriptions (source | keyword | entity) + mute; limits enforced
  - [ ] market simulator/rules moved to `examples/`; core has no domain terms
  - [ ] non-market demo works with zero core changes

### TP-007 — PostgreSQL + migrations
- Status: Review (implemented in 0.4.0-dev; not yet confirmed by running)
- Acceptance:
  - [ ] ADR for pgx + migration tool; fresh DB migrates; up/down tested in CI
  - [ ] repositories behind existing interfaces; parameterized SQL only; compose `postgres` service
  - [ ] `/readyz` checks DB; restart loses nothing

### TP-008 — Roles, status, invites
- Status: Review (implemented in 0.3.0-dev; unverified — see CHANGELOG/TESTING) · P0 · M2 · Depends: TP-007
- Acceptance:
  - [ ] invites: 128-bit random, hashed at rest, single-use, expiry, optional email bind
  - [ ] concurrent redeem of one code succeeds exactly once (race test)
  - [ ] registration impossible without a valid code
  - [ ] suspended users rejected on API and WebSocket

### TP-009 — Admin API, first-admin bootstrap, audit log
- Status: Review (implemented in 0.3.0-dev; unverified — see CHANGELOG/TESTING) · P0 · M2 · Depends: TP-008
- Acceptance:
  - [ ] `/admin/*` returns 403 for members; every admin action audited
  - [ ] first admin only via startup bootstrap env (D-015); per-account login throttling (follow-up)

### TP-010 — Safe fetcher (SSRF guard)
- Status: Review (implemented in 0.4.0-dev; not yet confirmed by running)
- Acceptance:
  - [ ] http/https, ports 80/443; blocks loopback, RFC1918, link-local, 169.254.169.254, metadata.google.internal
  - [ ] DNS resolved and IP pinned; redirects re-validated (max 3); 10 s timeout; 2 MB cap
  - [ ] table-driven tests incl. DNS-rebinding and redirect-to-internal cases

### TP-011 — Source model, admin CRUD, run history, health
- Status: Proposed · P0 · M3 · Depends: TP-010
- Acceptance:
  - [ ] sources stored in DB with `secret_ref` (never the secret); enable/disable; test-fetch
  - [ ] `source_runs` recorded; `/admin/health` shows last success, error rate, items/day

### TP-012 — Raw store, dedupe, silence watchdog
- Status: Proposed · P0 · M3 · Depends: TP-011
- Acceptance:
  - [ ] unique `(source_id, dedupe_key)`; replay/restart creates no duplicates
  - [ ] watchdog alerts admins when an enabled source is silent beyond N× interval
  - [ ] raw retention job

### TP-013 — RSS/Atom connector · TP-014 — Web-page monitor · TP-015 — Manual post
- Status: Proposed · P0 · M3 · Depends: TP-012
- Acceptance (each):
  - [ ] uses safefetch; conditional GET (RSS); robots.txt respected by default (web monitor); min interval 5 min
  - [ ] fixture-based tests, no live network; per-source note in `docs/sources/`
  - [ ] web monitor: CSS selectors for item/title/link/time, selector test-fetch in admin API

### TP-016 — X account connector with budgets
- Status: Blocked (Q-02, Q-03: handles, X developer account, budget) · P0 · M4 · Depends: TP-012
- Acceptance:
  - [ ] official API only; `since_id` polling; per-source monthly + global hard budget
  - [ ] polling stops at cap and raises an admin health alert; spend metric
  - [ ] token from Secret Manager; never logged
- Stop condition: paid credentials needed → ask owner.

### TP-017 — API documentation inside /docs
- Status: Review (implemented in 0.3.0-dev; unverified) · P0 · M2
- Goal: a new developer can understand and use the API from `/docs` alone.
- Acceptance:
  - [x] overview with quick start, auth, subscriptions guide, WebSocket protocol, errors, admin flow
  - [x] every operation has summary, description, tag, error codes; tags described
  - [x] enforced by `TestEveryOperationIsDocumented` / `TestAdminOperationsDeclareSecurity`
  - [ ] visually confirmed in the running /docs page (owner)
- Rule going forward: new endpoints are not "done" without docs text (see AGENTS.md).

### TP-018 — Source scheduler with per-source intervals (Q-07, D-022)
- Status: Proposed · P0 · M3 · Depends: TP-011
- Acceptance:
  - [ ] each source has `poll_interval_seconds` validated against `SOURCE_MIN/MAX_INTERVAL_SECONDS`
  - [ ] scheduler runs sources independently (one slow source never delays another), with jitter and per-host rate limit
  - [ ] adaptive back-off after unchanged responses/errors; next/last run visible in admin health
  - [ ] fixture tests with a fake clock (no sleeping tests)

### TP-019 — Channel abstraction + outbound webhook (Q-05, D-025)
- Status: Proposed · P0 · M5 · Depends: TP-010, TP-005
- Acceptance:
  - [ ] `Channel` interface (`Send(ctx, userID, item)`), in-app and webhook implementations, registry by name
  - [ ] webhook: HTTPS only via safefetch, HMAC-signed payload, retry with back-off, delivery history, auto-disable after repeated failures
  - [ ] adding email/Telegram later requires no change to matching code

### TP-020 — Deploy workflow (GitHub Actions → Cloud Run)
- Status: Proposed · P1 · M6 · Depends: TP-007
- Acceptance:
  - [ ] manual-dispatch + tag-based workflow using Workload Identity Federation (no JSON keys)
  - [ ] project/region/service from GitHub variables (`GCP_PROJECT_ID`, `GCP_REGION`, `CLOUD_RUN_SERVICE`), secrets from Secret Manager
  - [ ] smoke test on `/readyz`; rollback = previous revision (documented)
