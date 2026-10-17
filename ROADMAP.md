# ROADMAP.md — milestone status (update only when exit criteria are truly met)

## CURRENT STATE & NEXT ACTION (read this first; update at the end of every work session)
- **Date:** 2026-10-17 · **Verified by owner (Windows 11):** M0, M1 and M2 (access control, admin API, /docs, WebSocket e2e, Postgres persistence, safe fetcher).
- **Written, NOT yet confirmed:** 0.5.0-dev = **M3 Sources**: source model + admin API (create/test/update/run/delete, health), member catalog, manual posts, scheduler with per-source intervals, baseline/stale/dedupe rules, RSS/Atom and web-page connectors, robots.txt, Postgres migration `0002_sources`.
- **Next action 1 (owner):** unzip keeping your own `go.mod`/`go.sum`; `docker compose run --rm tidy` (adds goquery/cascadia/x-net), `docker compose run --rm check`, paste the output (agent fixes until green); `docker compose run --rm openapi` and commit `openapi.json`.
- **Next action 2 (owner + agent, TP-021):** in `/docs` use *Test a source draft* for each URL (Moneycontrol, CNBC, scanx.trade), check each site's terms/robots, create the sources, enable, subscribe (`GET /api/v1/sources` shows names), and watch items arrive. Share any selector/feed problems and the agent tunes them.
- **Next (agent):** M4: X connector (official API, budget cap by env), inbound webhook; then M5 alerts + channels (`Channel` interface, webhook), M6 deploy (GitHub Actions to Cloud Run), refresh tokens, metrics.
- **Decisions in force:** D-021…D-031. **Do not start:** frontend before M7.

| M | Name | Status | Exit criteria (summary) | Blocked by |
|---|---|---|---|---|
| M0 | Foundation green | ☑ (owner-verified 2026-10-17) | `check` passes on Windows + Linux; go.sum/openapi.json committed; CI green; WS e2e test | owner running first build |
| M1 | Generic core | ☑ (owner-verified 2026-10-17) | event type registry, field-condition rules, keyword/entity subscriptions, no market terms in core | M0 |
| M2 | Access & persistence | ☑ (owner-verified 2026-10-17) | invites + roles + audit; Postgres + migrations; restart-safe; security tests | M1 |
| M3 | Sources | ◐ complete in code; awaiting owner run + source enablement (TP-021) | safefetch; source CRUD/health; RSS, web monitor, manual; dedupe + raw store; watchdog | M2, Q-02 |
| M4 | X + webhook | ☐ | X connector with budgets; inbound webhook HMAC/replay | M3, Q-02, Q-03 |
| M5 | Alerts & channels | ☐ | rules, templates, preview, caps/auto-pause; WS + webhook channels; feedback | M3 |
| M6 | Cloud Run & beta readiness | ☐ | staging/prod, Secret Manager, backups + restore drill, metrics/alerts, refresh tokens, load test, runbook → invite first members | M2–M5, Q-03 |
| M7 | API freeze + frontend MVP | ☐ | frozen contract, typed client, UI | M6 |
| M8 | Post-MVP | ☐ | email/Telegram/push, clustering, AI drafts, scale-out, teams, billing, privacy compliance | M7 |

Rule: connectors do not start before TP-010 (safe fetcher); frontend does not start before M7.
