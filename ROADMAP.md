# ROADMAP.md — milestone status (update only when exit criteria are truly met)

## CURRENT STATE & NEXT ACTION (read this first; update at the end of every work session)
- **Date:** 2026-10-18 · **Verified by owner (Windows 11):** M0, M1, M2. Owner ran M4: only 8 failures, fixed in 0.6.1-dev (re-run not yet reported).
- **Delivered, NOT yet confirmed:** 0.6.1-dev (M4 fixes) and **0.7.0-dev = M5**: alert rules, templates, preview, guardrails (cooldown/limits/auto-pause), fires history, channels (in-app + signed outbound webhook with retries and auto-disable), `safefetch.Post`, migration `0004_alerts`. Guide: `docs/ALERTS.md`.
- **Next action 1 (owner):** apply/unzip, keep your `go.mod`/`go.sum`, `docker compose run --rm check`, paste the output (agent fixes until green); `docker compose run --rm openapi`, commit `openapi.json`.
- **Next action 2 (owner):** in `/docs`: create a manual source (or use your webhook), create a keyword alert from a template, preview it, post an item, watch the alert arrive (WebSocket `type: alert`); add an outbound channel pointing at your automation and use *Send a test delivery*. TP-021: enable the three news sources.
- **Next (agent):** M6: GitHub Actions to Cloud Run deploy (project/region by variables), refresh tokens + per-account login throttling, metrics/alerts for the operator, backup restore drill, runbook; then invite the first 10-50 members.
- **Decisions in force:** D-021...D-038. **Do not start:** frontend before M7.

| M | Name | Status | Exit criteria (summary) | Blocked by |
|---|---|---|---|---|
| M0 | Foundation green | ☑ (owner-verified 2026-10-17) | `check` passes on Windows + Linux; go.sum/openapi.json committed; CI green; WS e2e test | owner running first build |
| M1 | Generic core | ☑ (owner-verified 2026-10-17) | event type registry, field-condition rules, keyword/entity subscriptions, no market terms in core | M0 |
| M2 | Access & persistence | ☑ (owner-verified 2026-10-17) | invites + roles + audit; Postgres + migrations; restart-safe; security tests | M1 |
| M3 | Sources | ◐ delivered 0.5.0-dev; awaiting owner run + source enablement (TP-021) | safefetch; source CRUD/health; RSS, web monitor, manual; dedupe + raw store; watchdog | M2, Q-02 |
| M4 | X + webhook | ◐ delivered (0.6.0/0.6.1-dev); owner ran it (8 failures, fixed); re-run pending; X untested by owner | X connector with budgets; inbound webhook HMAC/replay | M3, Q-02, Q-03 |
| M5 | Alerts & channels | ◐ complete in code (0.7.0-dev); awaiting owner run | rules, templates, preview, caps/auto-pause; WS + webhook channels; feedback | M3 |
| M6 | Cloud Run & beta readiness | ☐ | staging/prod, Secret Manager, backups + restore drill, metrics/alerts, refresh tokens, load test, runbook → invite first members | M2–M5, Q-03 |
| M7 | API freeze + frontend MVP | ☐ | frozen contract, typed client, UI | M6 |
| M8 | Post-MVP | ☐ | email/Telegram/push, clustering, AI drafts, scale-out, teams, billing, privacy compliance | M7 |

Rule: connectors do not start before TP-010 (safe fetcher); frontend does not start before M7.
