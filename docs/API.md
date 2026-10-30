# API contract (v1)

Status: **1.0.0-rc.1 (freeze candidate).** The spec is `openapi.json` (generated from the code, committed, checked in CI); the live reference is `/docs` in development. This file states the rules that make the API safe to build a frontend on.

## Compatibility
- Everything lives under `/api/v1`. **Within v1 changes are additive only**: new endpoints, new optional request fields, new response fields, new enum values. Breaking changes (removing/renaming fields, changing types or meanings, tightening validation) require `/api/v2`.
- Clients must **ignore unknown response fields and unknown enum values**, and treat a missing optional/`omitempty` field as empty.
- Deprecations: the old behavior keeps working for at least 90 days, responses carry `Deprecation: true` and `Sunset: <HTTP date>` headers, and the change is announced in `CHANGELOG.md`.
- CI compares `openapi.json` with the main branch (oasdiff) and flags breaking changes.

## Conventions
- JSON, `snake_case`, timestamps RFC 3339 in UTC, ids are opaque strings, numbers are JSON numbers.
- **Cursors are opaque** (`c1_...`). Never construct or parse them.
- Every response has `X-Request-ID`; quote it when reporting a problem.
- Create endpoints (`POST`) are **not idempotent** except webhook deliveries (same `id` or same title+url is a no-op). Do not blindly retry creates after a timeout; list first.

## Authentication
`Authorization: Bearer <access token>` (15 minutes). `POST /auth/refresh` rotates the refresh token (30 days, single use; reusing one revokes the session). `POST /auth/logout`, `POST /me/logout-all`, `POST /me/password` end sessions. WebSockets use a one-time ticket (see `docs/WS.md`). See `docs/CLIENT.md` for the client algorithm.

## Errors
All errors, including unknown paths, are RFC 9457 problem JSON with `Content-Type: application/problem+json`:
```json
{"title": "Unprocessable Entity", "status": 422, "detail": "interval_seconds must be between 60 and 86400"}
```
Validation errors may also carry an `errors` array (field-level). Codes: 400/422 invalid input, 401 not authenticated or expired, 403 not allowed, 404 not found (also for other users' resources), 409 conflict or limit, 413 body too large, 429 rate limited (`Retry-After`), 5xx server (retry with back-off). `detail` is safe to show to users; it never contains internals or secrets.

## Pagination
Growing lists (`/me/feed`, `/me/alerts/fires`, `/me/channels/deliveries`, `/admin/audit`): query `limit` (1-200; audit 1-500), `before`, `after`. Response: items, `order` (`desc`|`asc`), `next_cursor`.
- Default and `before=<cursor>`: **newest first**; `next_cursor` is the `before` value for the next (older) page.
- `after=<cursor>`: **oldest first** (catch-up); `next_cursor` is the `after` value for the next (newer) page.
- `next_cursor` is present whenever a page is full; the request that follows may return an empty page (and no cursor): that means you are done. `before` and `after` together are a 422.
- Bounded collections (rules <= 50, channels <= 5, saved views <= 20, sources, members, invites, subscriptions) are returned whole.

## Saved views and feedback
- `GET /me/feed?view=<id>` returns the feed narrowed by a saved view (`/me/views`); paging and catch-up behave exactly as for the plain feed. A view only filters what the member already has.
- Feed items and alert fires carry `feedback` (`useful` | `not_useful`, absent when unrated) with the member's own rating. Rating is idempotent (`POST .../feedback`), undo with `DELETE`. Operators read aggregates only (`GET /admin/feedback/summary`): precision per source and rule, never identities.

## Limits
Per-IP request limits (429 + `Retry-After`), stricter on `/auth/*`; request bodies <= 64 KB (subscriptions 256 KB); webhook deliveries <= 64 KB and rate limited per source. See `docs/RUNBOOK.md`.

## Not in OpenAPI
`GET /ws` (WebSocket, `docs/WS.md`), `POST /api/v1/hooks/{name}` (inbound webhook, `docs/WEBHOOKS.md`), `GET /metrics` (operators), `/healthz`, `/readyz`.
