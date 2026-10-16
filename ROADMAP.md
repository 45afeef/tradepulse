# ROADMAP.md — milestone status (update only when exit criteria are truly met)

## CURRENT STATE & NEXT ACTION (read this first; update at the end of every work session)
- **Date:** 2026-10-17 · **Verified by owner (Windows 11):** M0 (baseline) and M1 (generic core).
- **Written, NOT yet confirmed by running:** 0.3.0-dev (invites, roles, suspension, admin API, audit, /docs documentation, WebSocket e2e test) and 0.4.0-dev (PostgreSQL storage via env, migrations, readiness, retention, safe fetcher). Together they complete **M2** in code.
- **Next action 1 (owner):** unzip over the project **keeping your own `go.mod` and `go.sum`**, then `docker compose run --rm tidy` (adds pgx), `docker compose run --rm check` (starts the local `postgres` service; runs the integration tests) and paste the full output. Agent fixes until green; then log it in `TESTING.md` → Verification log.
- **Next action 2 (owner):** `docker compose run --rm openapi`, commit `openapi.json` + `go.sum`; `docker compose up --build`; open `/docs`; try the README walkthrough (admin login → invite → register → subscribe). Optional: put your Supabase URLs in `.env` (see `docs/DATABASE.md`) and restart: data now survives restarts.
- **Next action 3 (agent, M3):** TP-011 source model + admin CRUD, TP-012 raw store/dedupe/watchdog, TP-018 per-source scheduler, TP-013 RSS (Moneycontrol/CNBC feeds), TP-014 web-page monitor (scanx.trade), TP-015 manual post. Then M4 (X connector, webhook).
- **Decisions in force:** D-021…D-027 (managed Postgres by env; per-source latency; X/GCP by env; GitHub Actions; channels abstraction; safefetch only; 10 to 50 members).
- **Gates still ahead:** source terms/robots check for the three news sites + X before enabling (M3/M4); backup restore drill (M6).
- **Do not start:** frontend before M7.

| M | Name | Status | Exit criteria (summary) | Blocked by |
|---|---|---|---|---|
| M0 | Foundation green | ☑ (owner-verified 2026-10-17) | `check` passes on Windows + Linux; go.sum/openapi.json committed; CI green; WS e2e test | owner running first build |
| M1 | Generic core | ☑ (owner-verified 2026-10-17) | event type registry, field-condition rules, keyword/entity subscriptions, no market terms in core | M0 |
| M2 | Access & persistence | ◐ complete in code; awaiting owner run | invites + roles + audit; Postgres + migrations; restart-safe; security tests | M1 |
| M3 | Sources | ☐ | safefetch; source CRUD/health; RSS, web monitor, manual; dedupe + raw store; watchdog | M2, Q-02 |
| M4 | X + webhook | ☐ | X connector with budgets; inbound webhook HMAC/replay | M3, Q-02, Q-03 |
| M5 | Alerts & channels | ☐ | rules, templates, preview, caps/auto-pause; WS + webhook channels; feedback | M3 |
| M6 | Cloud Run & beta readiness | ☐ | staging/prod, Secret Manager, backups + restore drill, metrics/alerts, refresh tokens, load test, runbook → invite first members | M2–M5, Q-03 |
| M7 | API freeze + frontend MVP | ☐ | frozen contract, typed client, UI | M6 |
| M8 | Post-MVP | ☐ | email/Telegram/push, clustering, AI drafts, scale-out, teams, billing, privacy compliance | M7 |

Rule: connectors do not start before TP-010 (safe fetcher); frontend does not start before M7.
