# Web app guide

The first-party web app lives in `web/` (React 18, TypeScript strict, Vite, TanStack Query, React Router). It talks to the public API only (`docs/API.md`, `docs/WS.md`); there is no private channel. Every screen below works for members; operators additionally get the operator console.

## Run it
```bash
docker compose up --build              # api + postgres + web (hot reload)
# http://localhost:5173   dev operator: admin@example.com / dev-admin-password-change-me
docker compose run --rm web-check      # typecheck + tests + production build
```
In development the browser only talks to the Vite dev server, which proxies `/api` and `/ws` to the API: no CORS configuration, and the API's WebSocket origin list already includes `http://localhost:5173` (`compose.yaml`). After the first install commit `web/package-lock.json`.

## What members get
| Screen | Highlights |
|---|---|
| **Sign in / Join** | invite code prefilled from `?invite=`; clear errors; password hints |
| **Get started** (`/welcome`) | 3 steps in about two minutes: follow sources/topics/keywords, pick alert templates with a live "would have fired N times" preview, done. Aim: first useful alert in under 10 minutes |
| **Feed** | live updates (new items slide in at the top; a "N new" pill when you have scrolled), infinite scroll with cursors, saved **views** as tabs, 👍/👎 on every item (optimistic), "Why this" explanation, safe "Open original" links, skeleton loading, empty states that tell you what to do |
| **Alerts** | rule cards with on/off switch, auto-pause banner with one-click resume, builder with templates, advanced filters (event types and conditions), cooldown and importance, delivery channels, **live preview** while you edit, history with 👍/👎 |
| **Following** | sources, topics, keywords and muted words as editable chips with suggestions; unsaved-changes guard |
| **Delivery** | add signed webhooks (secret shown once with copy), test, rotate, re-enable, delete, delivery log |
| **Settings** | theme (system/light/dark), change password, sign out / sign out everywhere |

## What operators get (`/admin`, role `admin`)
| Screen | Highlights |
|---|---|
| **Overview** | setup checklist until the workspace is running, attention banners (failing sources, X budget, noisy sources/rules), members/invites/sources/precision cards, X spend bar, recent activity |
| **Sources** | health table with filters, **add-source wizard** (type -> settings -> *test fetch preview* -> create), detail view with runs, run now, start/stop, edit interval/topics, webhook secret rotate, delete |
| **Invites** | create with optional email/note/expiry, copy or share the signup link, status tabs, revoke |
| **Members** | search, suspend/reactivate, reset password (temporary password shown once); cannot act on yourself |
| **Publish** | post an announcement through a manual source |
| **Audit log** | readable actions, who did it, filter, CSV export (formula-safe) |

## Architecture
```
web/src
  api/        client.ts (fetch wrapper), endpoints.ts (every call), types.ts (contract mirror)
  auth/       AuthProvider (session bootstrap, login/register/logout)
  live/       LiveClient.ts (framework-free WebSocket logic) + LiveProvider.tsx
  ui/         design-system components (Button, Modal, Tabs, ChipInput, toasts, confirm ...)
  components/ FeedCard, AlertEditor, ViewEditor, SourceWizard, AppShell
  pages/      one file per screen; pages/admin/* for the operator console
  lib/        pure helpers (format, backoff, conditions, feedCache, audit, sources, subscriptions)
  styles/     app.css (tokens, light/dark, components, layout)
  test/       setup
```
Principles: all HTTP goes through `api/client.ts`; all endpoint calls live in `api/endpoints.ts` (so `npm run contract` can verify them against `openapi.json`); server state lives in TanStack Query, UI state in components; pure logic is kept out of components so it is unit-tested without a DOM.

### Behaviors worth knowing
- **Tokens:** the access token (15 min) stays in memory; the refresh token is in `localStorage`. Refresh is **single-flight** (refresh tokens are single use, so concurrent refreshes would revoke the session) and happens before expiry or after a 401. A failed refresh signs the user out; a network failure does not.
- **Live updates:** `LiveClient` follows `docs/WS.md`: ticket per connection, back-off with jitter, catch-up with `after=<cursor>` after every reconnect, de-duplication by id, 60 s pause after a policy close.
- **Errors:** `ApiError` carries status, `detail`, request id and field errors; 5xx messages never show server text, only a reference id.
- **Safety:** no `dangerouslySetInnerHTML`; external links are `http(s)` only with `rel="noopener noreferrer"`; CSV export neutralises spreadsheet formulas; the production nginx config sets a strict CSP (`script-src 'self'`).
- **Accessibility:** semantic landmarks, labelled controls, focus-visible rings, focus trap and Escape in dialogs, `aria-live` for toasts and previews, keyboard-operable tabs, `prefers-reduced-motion` respected, colour is never the only signal.

## Tests
`npm test` (Vitest): API client (single-flight refresh, 401 retry, error mapping), live client (reconnect, catch-up, dedupe, close codes), pure helpers, and component tests (feed card, chip input, dialog, tabs). The author ran the 33 logic tests in a Node harness; the 7 component tests need `npm install` (jsdom) and are run by `web-check` and CI.

## Adding a screen or an endpoint call
1. Add the call to `api/endpoints.ts` (use `get/post/put/del`) and its types to `api/types.ts`.
2. `npm run contract` fails until the path exists in `openapi.json`: regenerate it first (`docker compose run --rm openapi`).
3. Put logic in `lib/` with a test; keep the page component thin; use `Banner`/`Empty`/`SkeletonCards` for error, empty and loading states.
4. Update `docs/DEVELOPER_GUIDE.md` if the user-visible flow changes.

## Deploying
`web/Dockerfile` builds the static site and serves it with nginx on port 8080 (SPA fallback, caching, security headers).
```bash
docker build --build-arg VITE_API_BASE=https://api.example.com -t tradepulse-web web
```
Host it as a second Cloud Run service (or any static host/CDN). Because the app and API then have different origins, add the app's origin to the API's `ALLOWED_ORIGINS` (CORS and WebSocket origin), and keep `VITE_API_BASE` pointing at the API. Same-origin hosting behind one load balancer (path routing `/api`, `/ws` to the API) avoids CORS entirely and is the recommended production shape.

## Known limitations (MVP)
- Hand-written API types (guarded by `npm run contract`, not generated); switch to generated types when the contract is stable.
- The refresh token lives in `localStorage` (readable by injected scripts; mitigated by the CSP and no HTML injection). A backend-for-frontend with an httpOnly cookie is the stronger option later.
- No offline/PWA, push or email notifications yet; no end-to-end browser tests (Playwright is the planned next step); English only.
- Views filter the retained feed (last 1,000 items); per-rule precision is operator-only.
