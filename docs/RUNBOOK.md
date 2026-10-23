# Operations runbook

## Health and signals
| Endpoint | Meaning |
|---|---|
| `GET /healthz` | process alive (no dependencies) |
| `GET /readyz` | database reachable (503 otherwise) |
| `GET /metrics` | Prometheus text; **404 unless `METRICS_TOKEN` is set**, then `Authorization: Bearer <token>` |
| `GET /api/v1/admin/sources` | per-source health, last/next run, errors |
| `GET /api/v1/admin/usage` | X spend vs budget |
| `GET /api/v1/admin/audit` | admin actions |

### Metrics and suggested alerts
| Metric | Alert when |
|---|---|
| `http_requests_total{class="5xx"}` | 5xx rate above 1% for 5 min |
| `http_request_duration_seconds` | p95 above 1 s for 10 min |
| `sources_by_health{health="failing"}` / `overdue` | any for 15 min |
| `source_runs_total{result="failed"}` | sustained growth |
| `outbound_deliveries_total{result="failed"}` | sustained growth (a member's endpoint or SSRF block) |
| `auth_refresh_reuse_total` | **any increase** (possible token theft) |
| `auth_lockouts_total`, `auth_login_failures_total` | sudden spike (credential stuffing) |
| `bus_dropped_events` | above 0 and rising (a subscriber is too slow) |
| `x_spend_usd` vs `x_budget_usd` | above 80% of budget |
| `ws_connections` | unexpected drop to 0 after a deploy that does not recover |
| `alert_rules_auto_paused_total` | growth (noisy rules; tell the member) |

## Playbooks
**Database down (`/readyz` 503).** Check the provider status page and connection limits (`DB_MAX_CONNS` below the plan limit). The app keeps serving liveness; deploys fail their startup probe until the database answers. Do not restart repeatedly: each start retries for 30 s.

**A source is `failing`.** `GET /admin/sources/{id}` shows the error. 4xx/robots: the site changed or blocked us (check terms, selectors with `POST /admin/sources/test`, or disable the source). X "budget reached": raise `X_MONTHLY_BUDGET_USD` or wait for the month. Never bypass robots.txt without a recorded reason.

**Suspected token theft.** `auth_refresh_reuse_total` rising: the affected session is already revoked. For one member: `POST /admin/users/{id}/suspend` then `reset-password`. For everyone: rotate `JWT_SECRET` (every access token dies at once) and also revoke all refresh tokens, otherwise they could mint new access tokens: `UPDATE refresh_tokens SET revoked_at = now() WHERE revoked_at IS NULL;`.

**Webhook secret leaked.** Inbound: `POST /admin/sources/{id}/webhook-secret`. Outbound channel: the member rotates it (`POST /me/channels/{id}/rotate-secret`). Everything at once: rotate `WEBHOOK_SIGNING_KEY` (invalidates all inbound and outbound secrets; reissue them).

**Locked-out admin.** Another admin resets the password (`reset-password`). If none exists: promote a member with SQL (`UPDATE users SET role='admin' WHERE lower(email)=lower('x');`) or set `BOOTSTRAP_ADMIN_*` for a **new** email (an existing account is never overwritten) and redeploy; remove the bootstrap secrets afterwards.

**A member says "I never get alerts".** `GET /me/alerts` (paused? `paused_reason`), `GET /me/alerts/fires`, `GET /me/channels/deliveries` (HTTP status of their endpoint), `POST /me/channels/{id}/test`.

**Secret rotation schedule.** `JWT_SECRET` and `WEBHOOK_SIGNING_KEY` yearly or on suspicion; database password per provider policy; `X_BEARER_TOKEN` when staff change. Rotating `JWT_SECRET` signs everyone out (acceptable).

## Backups and the restore drill (required before inviting members)
- Use the provider's automated backups / point-in-time recovery (Supabase: check the plan; free tiers may not include PITR: upgrade before real use).
- **Drill (quarterly, and once before beta):**
  1. Restore the latest backup into a **new, empty** project/database.
  2. Point a staging deployment at it (`DATABASE_URL`, `DATABASE_MIGRATION_URL`); the app applies any missing migrations at startup.
  3. Verify: `/readyz` 200; log in; row counts for `users`, `sources`, `alert_rules` match expectations; sources resume and do not republish old items (dedupe table restored).
  4. Record date, backup age (RPO) and total time (RTO) in the table below.

| Date | Backup used | Restored in | Result | Notes |
|---|---|---|---|---|
| _not yet performed_ | | | | |

## Data retention (defaults)
Feed 1,000 items/user · audit 365 days · source runs and raw items 30 days · alert fires and deliveries 30 days · refresh tokens kept 7 days after expiry. Privacy compliance work is deferred for the beta (D-008): revisit before any public or paid launch.

## Known limits (single instance)
In-memory: event bus, WebSocket tickets, rate limiters, login lockouts, outbound retry queue, hourly alert counters. A restart loses pending retries and resets counters/lockouts (cooldowns survive). Scale-out needs Redis/Pub-Sub first.
