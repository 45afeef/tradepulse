# ROADMAP.md — milestone status (update only when exit criteria are truly met)

## CURRENT STATE & NEXT ACTION (read this first; update at the end of every work session)
- **Date:** 2026-10-30 · **Backend:** M0 to M7 verified by the owner's full test run (all green, 0.10.1-dev).
- **Delivered, NOT yet run by a human:** **0.11.0-dev = web app MVP** (`web/`): member screens (onboarding, live feed with views and ratings, alerts with preview, following, delivery, settings) and the operator console (overview, source wizard, invites, members, publish, audit); backend additions `GET /me` and `role` on the user payload. The author type-checked it and ran 33 of its 40 tests in a Node harness; the browser has not been used.
- **Next action 1 (owner):** unzip keeping your `go.mod`/`go.sum`; `docker compose run --rm check` (Go: expect green; one new test); then `docker compose run --rm web-check` (installs packages, typechecks, tests, builds) and paste failures only; commit `web/package-lock.json`.
- **Next action 2 (owner):** `docker compose up --build`, open http://localhost:5173, sign in as the dev operator, add a source with the wizard, publish an announcement, invite yourself, sign up in a private window and walk the member flow on a phone-sized window. Send every point of friction.
- **Next action 3 (owner gates before real members):** `openapi.json` committed; staging deploy (`docs/DEPLOY.md`), load test, restore drill, source terms (TP-021).
- **Then:** TP-032 (Playwright e2e, generated types, accessibility audit, BFF cookie), private beta with 10 to 50 members.
- **Decisions in force:** D-021...D-055.

| M | Name | Status | Exit criteria (summary) | Blocked by |
|---|---|---|---|---|
| M0 | Foundation green | ☑ (owner-verified 2026-10-17) | `check` passes on Windows + Linux; go.sum/openapi.json committed; CI green; WS e2e test | owner running first build |
| M1 | Generic core | ☑ (owner-verified 2026-10-17) | event type registry, field-condition rules, keyword/entity subscriptions, no market terms in core | M0 |
| M2 | Access & persistence | ☑ (owner-verified 2026-10-17) | invites + roles + audit; Postgres + migrations; restart-safe; security tests | M1 |
| M3 | Sources | ✅ tests green (owner, 2026-10-30); still to do: enable the three news sources (TP-021) | safefetch; source CRUD/health; RSS, web monitor, manual; dedupe + raw store; watchdog | M2, Q-02 |
| M4 | X + webhook | ✅ tests green; X untested against the real API; try webhooks end to end | X connector with budgets; inbound webhook HMAC/replay | M3, Q-02, Q-03 |
| M5 | Alerts & channels | ✅ tests green; try alerts + an outbound webhook end to end | rules, templates, preview, caps/auto-pause; WS + webhook channels; feedback | M3 |
| M6 | Cloud Run & beta readiness | ◐ tests green; owner gates left: staging deploy, load test, restore drill | staging/prod, Secret Manager, backups + restore drill, metrics/alerts, refresh tokens, load test, runbook → invite first members | M2–M5, Q-03 |
| M7 | API freeze + frontend MVP | ✅ backend contract verified; ◐ web app MVP delivered (0.11.0-dev), awaiting first human run | frozen contract, typed client, UI | M6 |
| M8 | Post-MVP | ☐ | email/Telegram/push, clustering, AI drafts, scale-out, teams, billing, privacy compliance | M7 |

Rule: connectors do not start before TP-010 (safe fetcher); frontend does not start before M7.
