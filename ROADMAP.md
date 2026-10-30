# ROADMAP.md — milestone status (update only when exit criteria are truly met)

## CURRENT STATE & NEXT ACTION (read this first; update at the end of every work session)
- **Date:** 2026-10-22 · **Verified by owner (Windows 11):** M0, M1, M2. Everything after (0.5.0 to 0.9.0-dev) is delivered but **not yet confirmed by a full `check` run**; the author could only run static pre-checks (imports, duplicates, interface completeness).
- **Delivered:** M3 sources, M4 X + webhooks, M5 alerts + channels, M6 sessions/metrics/Cloud Run deploy/runbook, **0.9.0-dev = M7 backend half**: cursor pagination, uniform errors, `docs/WS.md`, `docs/API.md`, `docs/CLIENT.md`, version 1.0.0-rc.1, migration `0006_cursors`.
- **Next action 1 (owner, most important):** unzip keeping your `go.mod`/`go.sum`; `docker compose run --rm check`; paste the failing lines only (the agent fixes until green; expect a few after this much unrun code). Then `docker compose run --rm openapi` and commit `openapi.json`.
- **Next action 2 (owner gates before inviting members):** staging deploy (`docs/DEPLOY.md`), load test (`docs/LOADTEST.md`), restore drill (`docs/RUNBOOK.md`), enable the three news sources, try alerts + webhooks end to end.
- **Then:** TP-028 frontend MVP, generated from `openapi.json` (`docs/CLIENT.md`). The backend contract is frozen except for bug fixes and additive changes (`docs/API.md`).
- **Onboarding:** `docs/DEVELOPER_GUIDE.md` (0.9.1-dev) is the entry point for frontend developers, integrators and the dev advocate.
- **Decisions in force:** D-021...D-048.

| M | Name | Status | Exit criteria (summary) | Blocked by |
|---|---|---|---|---|
| M0 | Foundation green | ☑ (owner-verified 2026-10-17) | `check` passes on Windows + Linux; go.sum/openapi.json committed; CI green; WS e2e test | owner running first build |
| M1 | Generic core | ☑ (owner-verified 2026-10-17) | event type registry, field-condition rules, keyword/entity subscriptions, no market terms in core | M0 |
| M2 | Access & persistence | ☑ (owner-verified 2026-10-17) | invites + roles + audit; Postgres + migrations; restart-safe; security tests | M1 |
| M3 | Sources | ◐ delivered 0.5.0-dev; awaiting owner run + source enablement (TP-021) | safefetch; source CRUD/health; RSS, web monitor, manual; dedupe + raw store; watchdog | M2, Q-02 |
| M4 | X + webhook | ◐ delivered (0.6.0/0.6.1-dev); owner ran it (8 failures, fixed); re-run pending; X untested by owner | X connector with budgets; inbound webhook HMAC/replay | M3, Q-02, Q-03 |
| M5 | Alerts & channels | ◐ complete in code (0.7.0-dev); awaiting owner run | rules, templates, preview, caps/auto-pause; WS + webhook channels; feedback | M3 |
| M6 | Cloud Run & beta readiness | ◐ complete in code (0.8.0-dev); gates left for the owner: staging deploy, load test, restore drill | staging/prod, Secret Manager, backups + restore drill, metrics/alerts, refresh tokens, load test, runbook → invite first members | M2–M5, Q-03 |
| M7 | API freeze + frontend MVP | ◐ backend half complete in code (0.9.0-dev); frontend not started | frozen contract, typed client, UI | M6 |
| M8 | Post-MVP | ☐ | email/Telegram/push, clustering, AI drafts, scale-out, teams, billing, privacy compliance | M7 |

Rule: connectors do not start before TP-010 (safe fetcher); frontend does not start before M7.
