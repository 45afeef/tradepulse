# DECISIONS.md — decision log (append; never rewrite history, supersede instead)

| ID | Date | Decision | Rationale / consequences | Status |
|---|---|---|---|---|
| D-001 | 2026-10 | Modular monolith, Go, Huma code-first OpenAPI, stdlib-lean | Low ops complexity; docs generated from types | Decided |
| D-002 | 2026-10 | Docker-only dev workflow | Owner on Windows 11, colleagues on Unix; `make` optional | Decided |
| D-003 | 2026-10 | `/docs`, `/openapi.json` dev-only (absent in prod) | Remove free recon surface | Decided |
| D-004 | 2026-10 | Platform is domain-agnostic; domains = configuration | Owner: generic platform for anyone aggregating sources | Decided |
| D-005 | 2026-10-15 | Invite-only; codes created by admin and shared manually (Q-01) | No email infrastructure needed for beta | Decided |
| D-006 | 2026-10-15 | Host on Google Cloud Run (+ Cloud SQL, Secret Manager) (Q-03) | Beta must run **min=1, max=1, CPU always allocated** because pollers/WS push run outside requests and state is in-memory; scale-out needs Redis/Pub/Sub first | Decided |
| D-007 | 2026-10-15 | First sources: news pages and X accounts (Q-02) | Needs RSS / web-page monitor / X API connectors; exact URLs/handles still needed | Decided (details open) |
| D-008 | 2026-10-15 | Privacy-compliance work deferred for beta (Q-04) | Security baseline retained (hashing, access control, no PII in logs, manual deletion). Revisit before public/paid use or members outside the trusted circle. **Source terms of use are NOT deferred.** | Decided |
| D-009 | 2026-10 | Alerts are structured, whitelisted rules; never user code | Security + performance | Decided |
| D-010 | 2026-10 | In-memory bus/state until a measured need | Avoid premature Kafka/NATS/Redis | Decided |
| D-011 | 2026-10-15 | X data only via official/licensed API with budget caps; no scraping X | X is pay-per-use (about $0.005 per post read per published pricing; verify at docs.x.com); scraping violates terms and breaks | **Proposed** — owner to confirm |
| D-012 | 2026-10-15 | MVP alert channels: in-app + outbound webhook; email later | "Never miss" needs an off-page channel; webhook covers Slack/Discord/Telegram cheaply | **Proposed** — owner to confirm |
| D-013 | 2026-10-17 | pgx v5 + a small embedded SQL migrator (up/down files, advisory lock, `schema_migrations`) instead of goose/golang-migrate | one fewer dependency, ~150 lines, tested up/down/up | Decided |
| D-014 | 2026-10 | Frontend only after API freeze (M7) | Avoid rework | Decided |
| D-015 | 2026-10-16 | First admin comes from `BOOTSTRAP_ADMIN_EMAIL` + `BOOTSTRAP_ADMIN_PASSWORD` at startup (idempotent, never overwrites, never over HTTP) instead of a CLI | A CLI cannot persist with in-memory storage and is awkward on Cloud Run; env bootstrap works everywhere and keeps the secret in Secret Manager | Decided |
| D-016 | 2026-10-16 | Access control (invites, roles, suspension, admin API, audit) is built against repository interfaces BEFORE Postgres (TP-007) | Verifiable without a DB; Postgres adapters implement the same interfaces next | Decided |
| D-017 | 2026-10-16 | First X accounts: Donald Trump (@realDonaldTrump), Elon Musk (@elonmusk), Narendra Modi (@narendramodi); one news page URL still to come | Owner answer to Q-02; see `docs/sources/` | Decided (news URL pending) |
| D-018 | 2026-10-16 | Owner declines to set an X budget: the platform ships a conservative default monthly spend cap (env-configurable, e.g. `X_MONTHLY_BUDGET_USD`) and stops polling when reached | Reliability/safety: a bug or popular account must never cause an unbounded bill; owner can raise it by env var without code changes | Decided |
| D-019 | 2026-10-16 | GCP project/region are deploy-time environment configuration, never in code or docs; no owner input needed (Q-03 resolved) | Owner preference | Decided |
| D-020 | 2026-10-16 | Every endpoint must be documented in /docs (summary, description, tag, errors); enforced by test | New developers learn the API from /docs | Decided |
| D-021 | 2026-10-17 | PostgreSQL is a **managed service configured only by env vars** (`DATABASE_URL`, `DATABASE_MIGRATION_URL`, `DB_MAX_CONNS`, `DB_SIMPLE_PROTOCOL`); start on Supabase free tier, later any provider (AWS/GCP/...) by changing env only. Pooler-aware (auto simple protocol for PgBouncer/Supavisor), RLS enabled on every table, production refuses to start without `DATABASE_URL` | Owner requirement (provider flexibility); Supabase exposes `public` through an HTTP API so RLS is mandatory; see `docs/DATABASE.md` | Decided |
| D-022 | 2026-10-17 | Source polling latency is **per-source configurable** (`poll_interval_seconds`) within global bounds `SOURCE_MIN_INTERVAL_SECONDS` (60) / `SOURCE_MAX_INTERVAL_SECONDS` (86400); push sources are instant; adaptive back-off + per-host rate limit | Owner (Q-07): some sources need instant, others hourly | Decided |
| D-023 | 2026-10-17 | X credentials/base URL/budget and GCP identifiers are env/config only (`X_BEARER_TOKEN`, `X_API_BASE_URL`, `X_MONTHLY_BUDGET_USD`; deploy-time `GCP_PROJECT_ID`, `GCP_REGION`, `CLOUD_RUN_SERVICE` as GitHub variables/secrets) | Owner (Q-03): swap accounts/projects without code changes | Decided |
| D-024 | 2026-10-17 | CI/CD on GitHub Actions (CI exists; Cloud Run deploy workflow in M6 using Workload Identity Federation) | Owner (Q-10) | Decided |
| D-025 | 2026-10-17 | Alert channels for MVP: in-app (WebSocket) + outbound webhook behind a `Channel` interface so email/Telegram/Slack/push can be added later without touching matching | Owner (Q-05): only in-app + webhook now, but scalable | Decided |
| D-026 | 2026-10-17 | All outbound HTTP to configured/untrusted URLs goes through `internal/safefetch` (SSRF guard); enforced by an architecture test | Security (admin-supplied source URLs, webhooks, GCP metadata server) | Decided |
| D-027 | 2026-10-17 | Capacity target for planning: 10 to 50 invited members (Q-11) → single instance + managed Postgres is enough; revisit only if metrics say otherwise | Owner (Q-11) | Decided |
| D-028 | 2026-10-17 | Add `goquery` (+ cascadia, x/net/html) for the page monitor | HTML parsing and CSS selectors are not in the stdlib; hand-rolled parsing is the riskier choice | Decided |
| D-029 | 2026-10-17 | Ingestion semantics: first successful fetch of a source is a **baseline** (items remembered, nothing published); items older than 48 h are remembered but never published; items are marked seen BEFORE publishing (at-most-once publish, no duplicates after a crash) | Avoids flooding members with old items when a source is added; for news, a rare missed item beats duplicates | Decided |
| D-030 | 2026-10-17 | Sources are created **disabled**; admins test a draft first (`POST /admin/sources/test`); source names are permanent slugs members subscribe to; `manual` sources carry admin posts | Safe onboarding of new sites; stable subscriptions | Decided |
| D-031 | 2026-10-17 | robots.txt is respected for page monitors (fails closed on 5xx); RSS/Atom feeds are fetched as intended machine endpoints; override requires a recorded reason | Politeness and legal safety | Decided |

## Open questions
Resolved 2026-10-17: Q-02 (three news URLs, `docs/sources/news-pages.md`), Q-03, Q-05, Q-07, Q-10, Q-11. Still open: Q-08 retention periods (defaults proposed and implemented: feed 1,000/user, audit 365 d), Q-09 product name/domain, Q-06 email provider (post-MVP). Source terms/robots verification for the three news sites and X is an M3/M4 gate, not a question for the owner today.
