# ROADMAP.md — milestone status (update only when exit criteria are truly met)

## CURRENT STATE & NEXT ACTION (read this first; update at the end of every work session)
- **Date:** 2026-10-16 · **Build state:** all code so far is WRITTEN BUT NEVER COMPILED OR RUN (no Go in the authoring sandbox).
- **Done (unverified):** TP-003 docs · TP-004/005/006 generic core (M1) · TP-002 WS end-to-end test · TP-008/009 invites, roles, suspension, admin API, audit (M2 part) · TP-017 API docs in /docs. See `CHANGELOG.md` [0.3.0-dev], [0.2.0-dev].
- **Next action 1 (owner):** `docker compose run --rm check` → paste the full output; agent fixes until green; log the result in `TESTING.md` → Verification log; tick TP-001.
- **Next action 2 (owner):** `docker compose run --rm openapi` → commit `openapi.json` + `go.sum`; open http://localhost:8080/docs and confirm the overview/quick start render; try: login as dev admin → create invite → register → subscribe (README walkthrough).
- **Next action 3 (agent):** TP-007 Postgres (ADR: pgx + migration tool; repos for users, invites, audit, feed; `/readyz` checks DB), then TP-010 safe fetcher.
- **Waiting on owner:** the one news page URL (Q-02). X handles received (D-017). No GCP details needed (D-019); X spend cap default (D-018).
- **Do not start:** connectors before TP-010; frontend before M7.

| M | Name | Status | Exit criteria (summary) | Blocked by |
|---|---|---|---|---|
| M0 | Foundation green | ◐ (written, unverified) | `check` passes on Windows + Linux; go.sum/openapi.json committed; CI green; WS e2e test | owner running first build |
| M1 | Generic core | ◐ (written, unverified) | event type registry, field-condition rules, keyword/entity subscriptions, no market terms in core | M0 |
| M2 | Access & persistence | ◐ (access done-unverified; Postgres pending) | invites + roles + audit; Postgres + migrations; restart-safe; security tests | M1 |
| M3 | Sources | ☐ | safefetch; source CRUD/health; RSS, web monitor, manual; dedupe + raw store; watchdog | M2, Q-02 |
| M4 | X + webhook | ☐ | X connector with budgets; inbound webhook HMAC/replay | M3, Q-02, Q-03 |
| M5 | Alerts & channels | ☐ | rules, templates, preview, caps/auto-pause; WS + webhook channels; feedback | M3 |
| M6 | Cloud Run & beta readiness | ☐ | staging/prod, Secret Manager, backups + restore drill, metrics/alerts, refresh tokens, load test, runbook → invite first members | M2–M5, Q-03 |
| M7 | API freeze + frontend MVP | ☐ | frozen contract, typed client, UI | M6 |
| M8 | Post-MVP | ☐ | email/Telegram/push, clustering, AI drafts, scale-out, teams, billing, privacy compliance | M7 |

Rule: connectors do not start before TP-010 (safe fetcher); frontend does not start before M7.
