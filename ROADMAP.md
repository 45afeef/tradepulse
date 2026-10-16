# ROADMAP.md — milestone status (update only when exit criteria are truly met)

## CURRENT STATE & NEXT ACTION (read this first; update at the end of every work session)
- **Date:** 2026-10-16 · **Build state:** code for M0 baseline + M1 generic core is WRITTEN BUT NEVER COMPILED OR RUN.
- **Done (unverified):** TP-003 (docs), TP-004 (event registry), TP-005 (rule engine + index), TP-006 (subscriptions, examples). See `CHANGELOG.md [0.2.0-dev]`.
- **Next action 1 (owner):** run `docker compose run --rm check` and paste the full output. Agent fixes compile/test errors until green. Then record the result in `TESTING.md` → Verification log and tick TP-001.
- **Next action 2:** run `docker compose run --rm openapi`, commit `openapi.json` and `go.sum`; verify `docker compose up --build` serves `/docs` and the walkthrough in README works with `SIM_DEMO=market` and `status`.
- **Next action 3 (agent):** TP-002 WebSocket end-to-end test, then M2: TP-007 Postgres (ADR for pgx + migration tool).
- **Blocked on owner input:** Q-02 exact sources, Q-03 GCP project/region/budgets + X budget (needed from M3/M4).
- **Do not start:** connectors before TP-010 (safe fetcher); frontend before M7.

Legend: ☐ not started · ◐ in progress · ☑ done · ⛔ blocked

| M | Name | Status | Exit criteria (summary) | Blocked by |
|---|---|---|---|---|
| M0 | Foundation green | ◐ (written, unverified) | `check` passes on Windows + Linux; go.sum/openapi.json committed; CI green; WS e2e test | owner running first build |
| M1 | Generic core | ◐ (written, unverified) | event type registry, field-condition rules, keyword/entity subscriptions, no market terms in core | M0 |
| M2 | Access & persistence | ☐ | invites + roles + audit; Postgres + migrations; restart-safe; security tests | M1 |
| M3 | Sources | ☐ | safefetch; source CRUD/health; RSS, web monitor, manual; dedupe + raw store; watchdog | M2, Q-02 |
| M4 | X + webhook | ☐ | X connector with budgets; inbound webhook HMAC/replay | M3, Q-02, Q-03 |
| M5 | Alerts & channels | ☐ | rules, templates, preview, caps/auto-pause; WS + webhook channels; feedback | M3 |
| M6 | Cloud Run & beta readiness | ☐ | staging/prod, Secret Manager, backups + restore drill, metrics/alerts, refresh tokens, load test, runbook → invite first members | M2–M5, Q-03 |
| M7 | API freeze + frontend MVP | ☐ | frozen contract, typed client, UI | M6 |
| M8 | Post-MVP | ☐ | email/Telegram/push, clustering, AI drafts, scale-out, teams, billing, privacy compliance | M7 |

Rule: connectors do not start before TP-010 (safe fetcher); frontend does not start before M7.
