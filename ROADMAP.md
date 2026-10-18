# ROADMAP.md — milestone status (update only when exit criteria are truly met)

## CURRENT STATE & NEXT ACTION (read this first; update at the end of every work session)
- **Date:** 2026-10-17 · **Verified by owner (Windows 11):** M0, M1, M2. M3 (0.5.0-dev) was delivered; the owner has not reported running it yet.
- **Written, NOT yet confirmed:** 0.5.0-dev (M3 sources) and **0.6.0-dev = M4**: inbound webhooks (`POST /api/v1/hooks/{name}`, hmac/token, derived secrets, rotation), X connector (env-configured, budget-capped, off unless `X_BEARER_TOKEN` is set), usage endpoint, migration `0003_webhook_x`.
- **Next action 1 (owner):** unzip keeping your own `go.mod`/`go.sum`; `docker compose run --rm tidy`, `docker compose run --rm check`, paste the output (agent fixes until green); `docker compose run --rm openapi`, commit `openapi.json`.
- **Next action 2 (owner):** set `WEBHOOK_SIGNING_KEY` in `.env`, create a webhook source in `/docs`, send a delivery from your automation (`docs/WEBHOOKS.md`). X: nothing to do until you want it (`docs/sources/x-accounts.md`). TP-021: enable the three news sources.
- **Next (agent):** M5 alerts + channels (`Channel` interface, outbound webhook, templates, cooldowns, preview), then M6 (GitHub Actions to Cloud Run deploy, refresh tokens, metrics, backup drill).
- **Decisions in force:** D-021…D-034. **Do not start:** frontend before M7.

| M | Name | Status | Exit criteria (summary) | Blocked by |
|---|---|---|---|---|
| M0 | Foundation green | ☑ (owner-verified 2026-10-17) | `check` passes on Windows + Linux; go.sum/openapi.json committed; CI green; WS e2e test | owner running first build |
| M1 | Generic core | ☑ (owner-verified 2026-10-17) | event type registry, field-condition rules, keyword/entity subscriptions, no market terms in core | M0 |
| M2 | Access & persistence | ☑ (owner-verified 2026-10-17) | invites + roles + audit; Postgres + migrations; restart-safe; security tests | M1 |
| M3 | Sources | ◐ delivered 0.5.0-dev; awaiting owner run + source enablement (TP-021) | safefetch; source CRUD/health; RSS, web monitor, manual; dedupe + raw store; watchdog | M2, Q-02 |
| M4 | X + webhook | ◐ complete in code (0.6.0-dev); awaiting owner run; X untested by owner | X connector with budgets; inbound webhook HMAC/replay | M3, Q-02, Q-03 |
| M5 | Alerts & channels | ☐ | rules, templates, preview, caps/auto-pause; WS + webhook channels; feedback | M3 |
| M6 | Cloud Run & beta readiness | ☐ | staging/prod, Secret Manager, backups + restore drill, metrics/alerts, refresh tokens, load test, runbook → invite first members | M2–M5, Q-03 |
| M7 | API freeze + frontend MVP | ☐ | frozen contract, typed client, UI | M6 |
| M8 | Post-MVP | ☐ | email/Telegram/push, clustering, AI drafts, scale-out, teams, billing, privacy compliance | M7 |

Rule: connectors do not start before TP-010 (safe fetcher); frontend does not start before M7.
