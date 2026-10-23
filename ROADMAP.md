# ROADMAP.md — milestone status (update only when exit criteria are truly met)

## CURRENT STATE & NEXT ACTION (read this first; update at the end of every work session)
- **Date:** 2026-10-20 · **Verified by owner (Windows 11):** M0, M1, M2. M3-M5 were delivered and (M4) partly run; owner re-runs for 0.6.1/0.7.0/0.8.0 not yet reported.
- **Delivered, NOT yet confirmed:** **0.8.0-dev = M6**: refresh tokens + logout/logout-all + password change/admin reset + per-account lockout; Cloud Run readiness (`PORT`, shutdown timeout, trusted proxy hops); `/metrics`; deploy workflow, Cloud Run service template, CI scans; `docs/DEPLOY.md`, `docs/RUNBOOK.md`, `docs/LOADTEST.md`, `deploy/loadtest/k6.js`; migration `0005_sessions`.
- **Next action 1 (owner):** unzip keeping your `go.mod`/`go.sum`; `docker compose run --rm check` (expect to paste failures; the agent fixes until green); `docker compose run --rm openapi` and commit `openapi.json`.
- **Next action 2 (owner, before inviting members):** (a) first **staging deploy** following `docs/DEPLOY.md`; (b) run the **load test** (`docs/LOADTEST.md`) and record results; (c) perform the **restore drill** (`docs/RUNBOOK.md`); (d) enable the three news sources (TP-021) and try alerts/webhooks end to end; (e) wire the alerts listed in the runbook.
- **Then:** invite the first 10-50 members (private beta). In parallel the agent starts **M7: API freeze** (cursor pagination, `docs/WS.md`, uniform errors, typed client from `openapi.json`) so the frontend can be built.
- **Decisions in force:** D-021...D-044. **Do not start:** frontend before M7.

| M | Name | Status | Exit criteria (summary) | Blocked by |
|---|---|---|---|---|
| M0 | Foundation green | ☑ (owner-verified 2026-10-17) | `check` passes on Windows + Linux; go.sum/openapi.json committed; CI green; WS e2e test | owner running first build |
| M1 | Generic core | ☑ (owner-verified 2026-10-17) | event type registry, field-condition rules, keyword/entity subscriptions, no market terms in core | M0 |
| M2 | Access & persistence | ☑ (owner-verified 2026-10-17) | invites + roles + audit; Postgres + migrations; restart-safe; security tests | M1 |
| M3 | Sources | ◐ delivered 0.5.0-dev; awaiting owner run + source enablement (TP-021) | safefetch; source CRUD/health; RSS, web monitor, manual; dedupe + raw store; watchdog | M2, Q-02 |
| M4 | X + webhook | ◐ delivered (0.6.0/0.6.1-dev); owner ran it (8 failures, fixed); re-run pending; X untested by owner | X connector with budgets; inbound webhook HMAC/replay | M3, Q-02, Q-03 |
| M5 | Alerts & channels | ◐ complete in code (0.7.0-dev); awaiting owner run | rules, templates, preview, caps/auto-pause; WS + webhook channels; feedback | M3 |
| M6 | Cloud Run & beta readiness | ◐ complete in code (0.8.0-dev); gates left for the owner: staging deploy, load test, restore drill | staging/prod, Secret Manager, backups + restore drill, metrics/alerts, refresh tokens, load test, runbook → invite first members | M2–M5, Q-03 |
| M7 | API freeze + frontend MVP | ☐ | frozen contract, typed client, UI | M6 |
| M8 | Post-MVP | ☐ | email/Telegram/push, clustering, AI drafts, scale-out, teams, billing, privacy compliance | M7 |

Rule: connectors do not start before TP-010 (safe fetcher); frontend does not start before M7.
