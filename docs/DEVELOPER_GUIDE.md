# TradePulse Developer Guide (the Dev Advocate and Developer Bible)

One file, ordered by **who you are** and **what you need**. Pick your track, follow the numbered steps, tick the boxes. Everything here is copy-paste. Nothing here is hidden magic: every claim points to a doc, an endpoint or a test.

> **Status honesty.** Milestones M0 to M2 were confirmed working by the owner. Later features (sources, webhooks, alerts, sessions, pagination) are delivered and covered by tests, but a full verification run is still pending (see `ROADMAP.md`). If a command here fails, you found something we want to know: send the `X-Request-ID` response header, the command and the response.

---

## 0. The system in five minutes

**What it is.** A backend that collects information from many **sources** (RSS feeds, web pages, X accounts, your own systems through webhooks, manual posts), normalizes everything into **events**, matches them to what each member cares about (**subscriptions**), shows one live **feed**, and pushes the must-not-miss items as **alerts** (in-app, or signed webhooks to your own endpoint). It is **invite-only**.

```
Sources -> events -> matching (subscriptions, alert rules) -> feed + alerts -> WebSocket / REST / outbound webhook
```

| Word | Meaning |
|---|---|
| **Source** | Where items come from. Types: `rss`, `webmonitor` (web page + CSS selectors), `x`, `webhook` (push), `manual`. Named with a permanent slug. |
| **Event / item** | One piece of content: title, summary, URL, time, topics (entities). |
| **Entity** | A topic tag on an item (for example `markets`, `tcs`). Subscribe to it. |
| **Subscription** | What a member wants in their feed: an `entity`, a `keyword` (whole phrase) or a `source`, optionally with type and conditions. |
| **Feed** | Items that match the member's subscriptions, newest first, each with a `reason` ("why am I seeing this"). |
| **Alert rule** | A rule (scope + conditions + cooldown) that pushes the items you must not miss. Can be created from a **template**, checked with **preview**. |
| **Channel** | Where alerts go: `in_app` (WebSocket) or your own HTTPS **webhook** (signed). |
| **Cursor** | Opaque position token for paging and reconnect catch-up. Never parse it. |

**Two ready-made demo worlds** (switch with `SIM_DEMO=status` or `market`, default `market` in development): a stock-ticks simulator and a service-health simulator. They prove the platform is domain-agnostic; real sources are configured through the API.

**Where to read more:** API contract `docs/API.md` · live reference `/docs` (running server) · WebSocket `docs/WS.md` · typed client `docs/CLIENT.md` · webhooks `docs/WEBHOOKS.md` · alerts `docs/ALERTS.md` · sources `docs/sources/news-pages.md` · database `docs/DATABASE.md` · deploy `docs/DEPLOY.md` · operations `docs/RUNBOOK.md` · load test `docs/LOADTEST.md` · architecture and rules `PROJECT_BRAIN.md`, `AGENTS.md` · history `CHANGELOG.md` · plan `ROADMAP.md`, `BACKLOG.md`.

---

## 1. Pick your track

| I am / I need... | Go to |
|---|---|
| A PM, DevRel or newcomer who wants to **see it working** in 15 minutes | **Track A** |
| A frontend dev with **no backend running** (mock first) | **Track B** |
| A frontend dev who wants the **live dev server** and real data | **Track C** |
| To **build the UI** (types, auth, paging, live updates) | **Track D** |
| To **integrate** my own systems (webhooks in and out, automation) | **Track E** |
| To **change the backend** | **Track F** |
| To **deploy and operate** it | **Track G** |
| To **review its security** | **Track H** |
| To **run a demo or workshop** | **Section 9 (Dev Advocate playbook)** |

---

## 2. Setup shared by every track (Windows 11, macOS, Linux)

Only **Docker** is required to run the backend.

```bash
# 1. get the code, then from the project root:
docker compose up --build            # API + local Postgres, hot reload
# 2. wait for the log line: {"msg":"listening","addr":":8080",...}
```
- API: `http://localhost:8080` · interactive docs: `http://localhost:8080/docs` · raw contract: `http://localhost:8080/openapi.json`
- Dev admin (development only): `admin@example.com` / `dev-admin-password-change-me`
- Stop: `Ctrl+C`; reset the database: `docker compose down -v`
- Windows tip: clone inside WSL 2 for speed; PowerShell, CMD and WSL all work.

**Command helpers.** The recipes below use `tp METHOD PATH [JSON]`. Define it once per terminal.

*bash / zsh / Git Bash / WSL* (needs `curl`; `jq` makes it nicer):
```bash
export BASE=http://localhost:8080
tp() { curl -sS -X "$1" "$BASE$2" -H "Content-Type: application/json" ${TOKEN:+-H "Authorization: Bearer $TOKEN"} ${3:+-d "$3"}; echo; }
```
*PowerShell:*
```powershell
$BASE = "http://localhost:8080"
function tp($method, $path, $body = $null) {
  $h = @{}; if ($TOKEN) { $h["Authorization"] = "Bearer $TOKEN" }
  $a = @{ Method = $method; Uri = "$BASE$path"; Headers = $h; ContentType = "application/json" }
  if ($body) { $a.Body = ($body | ConvertTo-Json -Depth 10) }
  Invoke-RestMethod @a
}
# PowerShell bodies are hashtables: tp POST /api/v1/auth/login @{ email = "admin@example.com"; password = "dev-admin-password-change-me" }
```
Prefer a GUI? Import `http://localhost:8080/openapi.json` into Postman, Bruno or Insomnia.

---

## Track A: Explorer / 15-minute tour (no code)

Goal: see an item travel from a source to a member's screen and trigger an alert.

- [ ] 1. `docker compose up --build`, open `http://localhost:8080/docs`. Read the **Quick start** section at the top.
- [ ] 2. Log in as admin: `POST /api/v1/auth/login` with the dev admin. Copy `token`. Click **Authorize** and paste it.
- [ ] 3. Create an invite: `POST /api/v1/admin/invites` with `{}`. Copy `code` (shown once).
- [ ] 4. Register a member: `POST /api/v1/auth/register` with `{"email":"me@example.com","password":"a-long-password","invite_code":"<code>"}`. Authorize with the **member** token.
- [ ] 5. Subscribe to the simulated world: `PUT /api/v1/me/subscriptions` with `{"subscriptions":[{"kind":"entity","value":"RELIANCE"}]}` (market demo). Use `GET /api/v1/event-types` to see what you can filter on.
- [ ] 6. Watch items arrive: `GET /api/v1/me/feed` (repeat every few seconds). Each item has a `reason`.
- [ ] 7. Create an alert from a template: `GET /api/v1/alert-templates`, then `POST /api/v1/me/alerts/from-template` with `{"template_id":"topic-updates","values":["RELIANCE"]}`.
- [ ] 8. Preview before saving: `POST /api/v1/me/alerts/preview` with `{"name":"p","scope":{"kind":"entity","values":["RELIANCE"]},"enabled":true}`.
- [ ] 9. See the alerts: `GET /api/v1/me/alerts/fires`.
- [ ] 10. (Admin) see who did what: `GET /api/v1/admin/audit`.

You have now seen: invite-only access, sources, matching, the explainable feed, templates, preview and alerts.

---

## Track B: Frontend, mock first (nothing running)

Goal: start building screens today, before you have a backend. You need only `openapi.json` (ask for it or generate it: `docker compose run --rm openapi`).

- [ ] 1. **Types:** `npx openapi-typescript openapi.json -o src/api/schema.d.ts`
- [ ] 2. **REST mock** (returns schema-shaped example data and validates your requests):
  ```bash
  npx @stoplight/prism-cli mock openapi.json -p 4010
  # secured endpoints need a header, any value works: -H "Authorization: Bearer mock"
  ```
  Point your app's `BASE_URL` at `http://localhost:4010`. Values are generic placeholders, so also keep a few **hand-written fixtures** for the screens you design (see the shapes in Section 6).
- [ ] 3. **WebSocket mock** (Prism cannot mock WebSockets). Save as `mock-ws.mjs`, run `npm i ws && node mock-ws.mjs`, connect to `ws://localhost:8081/ws`:
  ```js
  import { WebSocketServer } from 'ws';
  const wss = new WebSocketServer({ port: 8081, path: '/ws' });
  let n = 0;
  const cursor = (i) => 'c1_' + Buffer.from(String(i)).toString('base64url');
  wss.on('connection', (ws) => {
    ws.send(JSON.stringify({ type: 'connected' }));
    const t = setInterval(() => {
      n++;
      ws.send(JSON.stringify({ type: 'feed_item', data: { id: 'i' + n, cursor: cursor(n), title: 'Mock item ' + n, body: '', severity: 'info',
        source: 'mock', entities: ['demo'], reason: 'Matched your source subscription "mock"', created_at: new Date().toISOString() } }));
      if (n % 5 === 0) ws.send(JSON.stringify({ type: 'alert', data: { id: 'a' + n, rule_name: 'Demo rule', severity: 'high', title: 'Mock alert ' + n,
        body: '', reason: 'Matched your alert "Demo rule"', fired_at: new Date().toISOString(), cursor: cursor(n) } }));
    }, 3000);
    ws.on('close', () => clearInterval(t));
  });
  ```
  Message shapes: `docs/WS.md`.
- [ ] 4. Build against the mock, using the **error shape** (`application/problem+json`: `title`, `status`, `detail`) and the **paging shape** (`items`, `order`, `next_cursor`).
- [ ] 5. When the real server is available, change only `BASE_URL` and the WebSocket URL. Go to Track C.

---

## Track C: Frontend, live dev server

- [ ] 1. `docker compose up --build` (Section 2).
- [ ] 2. **Allow your dev origin** (CORS and WebSocket origin): create `.env` next to `compose.yaml`:
  ```
  ALLOWED_ORIGINS=http://localhost:5173,http://localhost:3000
  ```
  then `docker compose up -d --force-recreate api`. (Vite uses 5173; Next/CRA use 3000.)
- [ ] 3. **Seed demo data** with the script in Section 4 (creates a member, a manual source, items, a subscription and an alert).
- [ ] 4. Open `/docs` next to your app; use "Try it" to reproduce any call your UI makes.
- [ ] 5. Browser dev tools: every response has `X-Request-ID`; the server log (`docker compose logs -f api`) shows the same id.
- [ ] 6. Reset to a clean slate any time: `docker compose down -v && docker compose up --build`.
- [ ] 7. Want different live data? `SIM_DEMO=status` in `.env` (service-health events, subscribe with `{"kind":"entity","value":"checkout"}`) or `none` and add real sources as admin (Track E).

---

## Track D: Build the UI

Order of work (each step is a vertical slice you can demo):

- [ ] 1. **Auth shell.** Login screen (`POST /api/v1/auth/login`), token store, refresh before expiry (`POST /api/v1/auth/refresh`), logout (`POST /api/v1/auth/logout`), logout everywhere (`POST /api/v1/me/logout-all`). Algorithm and code: `docs/CLIENT.md`. Access tokens last 15 minutes; refresh tokens are single use (use the *new* one every time).
- [ ] 2. **Invite signup.** `POST /api/v1/auth/register` (email, password of at least 10 characters, invite code).
- [ ] 3. **Feed.** `GET /api/v1/me/feed` with `limit` and `before=<next_cursor>` for infinite scroll. Show `title`, `body`, `source`, time, a "view original" link from `url`, and a "why am I seeing this" popover from `reason`. Add 👍/👎 buttons: `POST /api/v1/me/feed/{id}/feedback` with `{"rating":"useful"}` or `"not_useful"` (and `DELETE /api/v1/me/feed/{id}/feedback` to undo); the member's own rating comes back as `feedback` on each item. Add **saved views**: `GET /api/v1/me/views`, `POST /api/v1/me/views`, `PUT /api/v1/me/views/{id}`, `DELETE /api/v1/me/views/{id}`, then list through one with `GET /api/v1/me/feed?view=<id>` (paging and catch-up work unchanged).
- [ ] 4. **Live updates.** `POST /api/v1/ws-ticket` then `wss://.../ws?ticket=...`. On every (re)connect catch up with `GET /api/v1/me/feed?after=<last cursor>` until the page is empty, de-duplicate by `id`. Reference client in Section 6.
- [ ] 5. **Subscriptions.** `GET /api/v1/me/subscriptions`, `PUT /api/v1/me/subscriptions` (send the complete list), pickers fed by `GET /api/v1/event-types` and `GET /api/v1/sources`.
- [ ] 6. **Alerts.** `GET /api/v1/alert-templates` as the starting point, `POST /api/v1/me/alerts/from-template`, a rule builder with live **preview** (`POST /api/v1/me/alerts/preview`), list/edit/delete (`GET /api/v1/me/alerts`, `PUT /api/v1/me/alerts/{id}`, `DELETE /api/v1/me/alerts/{id}`), history (`GET /api/v1/me/alerts/fires`), with 👍/👎 on each fired alert (`POST /api/v1/me/alerts/fires/{id}/feedback`, undo with `DELETE`). Show `paused_reason` when the system paused a rule.
- [ ] 7. **Channels.** `POST /api/v1/me/channels` (show the secret once), `GET /api/v1/me/channels`, `POST /api/v1/me/channels/{id}/test`, `GET /api/v1/me/channels/deliveries`, `POST /api/v1/me/channels/{id}/enable`.
- [ ] 8. **Account.** `POST /api/v1/me/password`.
- [ ] 9. **Admin area** (role `admin`): invites (`POST /api/v1/admin/invites`, `GET /api/v1/admin/invites`), members (`GET /api/v1/admin/users`, suspend/reactivate/reset-password), sources with a **test-before-save** flow (`POST /api/v1/admin/sources/test`, then `POST /api/v1/admin/sources`), source health (`GET /api/v1/admin/sources`), audit (`GET /api/v1/admin/audit`), X spend (`GET /api/v1/admin/usage`), alert precision and noisy sources/rules (`GET /api/v1/admin/feedback/summary`).
- [ ] 10. **Polish rules:** mobile first; show loading, empty and error states; for 429 honor `Retry-After`; never block the UI on the WebSocket; test the "server restarted" path (reconnect + catch-up).

**Acceptance for the whole UI:** a newly invited user reaches a first useful alert in under 10 minutes.

---

## Track E: Integrate (automation, webhooks, real sources)

**E1. Push your own items in (inbound webhook).**
- [ ] 1. Server needs `WEBHOOK_SIGNING_KEY` (in `.env`: `openssl rand -base64 48`), restart.
- [ ] 2. As admin: `POST /api/v1/admin/sources` with `{"name":"my-hook","type":"webhook","enabled":true,"webhook":{"auth":"hmac"}}`. Copy `webhook_secret` (shown once).
- [ ] 3. Send an item (signature = HMAC-SHA256 of `"<timestamp>.<raw body>"`):
  ```bash
  SECRET='whsec_...'; BODY='{"title":"Disk usage high","summary":"92%","entities":["infra"]}'; TS=$(date +%s)
  SIG=$(printf '%s.%s' "$TS" "$BODY" | openssl dgst -sha256 -hmac "$SECRET" -hex | sed 's/^.* //')
  curl -i -X POST "$BASE/api/v1/hooks/my-hook" -H "X-TP-Timestamp: $TS" -H "X-TP-Signature: sha256=$SIG" -d "$BODY"
  ```
  Endpoint reference: `POST /api/v1/hooks/{name}` (details, token mode, responses: `docs/WEBHOOKS.md`). Retries are safe: same `id` is a no-op.
  Node: `crypto.createHmac('sha256', secret).update(ts + '.' + body).digest('hex')`. Python: `hmac.new(secret.encode(), f"{ts}.{body}".encode(), 'sha256').hexdigest()`.
- [ ] 4. n8n/Zapier/Make cannot sign? Create the source with `"webhook":{"auth":"token"}` and send `Authorization: Bearer <secret>`.

**E2. Receive alerts in your own system (outbound webhook).**
- [ ] 1. As a member: `POST /api/v1/me/channels` with `{"type":"webhook","name":"my-system","url":"https://example.com/hooks/tp"}` (HTTPS, public address, port 443). Copy `secret`.
- [ ] 2. `POST /api/v1/me/channels/{id}/test` then check `GET /api/v1/me/channels/deliveries`.
- [ ] 3. Verify every delivery: recompute `HMAC-SHA256(secret, timestamp + "." + rawBody)`, compare with `X-TP-Signature`, reject timestamps older than 5 minutes.
- [ ] 4. Attach the channel id to rules: `"channels":["in_app","<channel id>"]`. Failures are retried; 10 failures in a row disable the channel (re-enable with `POST /api/v1/me/channels/{id}/enable`).
- [ ] Local receiver without a public URL: expose it with a tunnel and use that `https` URL.

**E3. Add real sources (admin).**
- [ ] RSS: `POST /api/v1/admin/sources/test` with `{"name":"cnbc-top","type":"rss","url":"<feed url>","interval_seconds":300,"entities":["markets"]}` shows what would be read (nothing is saved). Then `POST /api/v1/admin/sources` with the same body plus `"enabled": true`.
- [ ] Web page: type `webmonitor` with `"web":{"item_selector":"<css>","title_selector":"<css>","link_selector":"a"}`; iterate with the test endpoint until the preview looks right. `robots.txt` is respected; check the site's terms first (`docs/sources/news-pages.md`).
- [ ] X account: set `X_BEARER_TOKEN` and a budget (`X_MONTHLY_BUDGET_USD`), then a source of type `x` (`docs/sources/x-accounts.md`). Spend is capped and visible at `GET /api/v1/admin/usage`.
- [ ] Per-source latency is `interval_seconds` (bounded by `SOURCE_MIN_INTERVAL_SECONDS`).
- [ ] Manual announcements: a `manual` source plus `POST /api/v1/admin/items`.
- [ ] Health and troubleshooting: `GET /api/v1/admin/sources`, `GET /api/v1/admin/sources/{id}`, `POST /api/v1/admin/sources/{id}/run` (fetch now).

---

## Track F: Backend contributor

- [ ] 1. Read `PROJECT_BRAIN.md` (what and why), `AGENTS.md` (rules), `ROADMAP.md` (what is next), `BACKLOG.md` (work items), `DECISIONS.md` (why).
- [ ] 2. Run everything: `docker compose run --rm check` (vet + all tests including Postgres integration). One package: `docker compose run --rm test go test ./internal/alerting/...`.
- [ ] 3. Map of the code: `cmd/server` (wiring only), `internal/domain/*` (pure rules and interfaces), `internal/api` (HTTP, one file per area), `internal/connector` (RSS, web monitor, X), `internal/ingestion` (scheduler), `internal/alerting`, `internal/personalization`, `internal/storage/{memory,postgres}`, `internal/safefetch` (the only place that makes outbound HTTP), `examples/` (demo worlds).
- [ ] 4. **Add an endpoint:** typed input/output + handler, register it with summary, description, tag and error codes (a test fails if undocumented), add tests, regenerate `docker compose run --rm openapi`, note API-visible changes in `CHANGELOG.md`. Changes in `/api/v1` are additive only.
- [ ] 5. **Add a source type:** implement `connector.Connector`, register it in `main.go`, validate config in `sourcedef`, use only `safefetch`.
- [ ] 6. **New database table:** numbered `.up.sql`/`.down.sql`, `ENABLE ROW LEVEL SECURITY` (tests enforce it), repository interface in the domain, memory and Postgres implementations.
- [ ] 7. **Definition of done:** tests (with `-race`) green, security checklist in `AGENTS.md`, docs and `/docs` text updated, `CHANGELOG.md` entry with a Conventional Commit message, `TESTING.md` register updated, `ROADMAP.md` "current state" rewritten.
- [ ] 8. Working with an AI assistant? Point it at `AGENTS.md`; it carries the architecture and security rules in few tokens.

---

## Track G: Deploy and operate

- [ ] 1. Cloud Run + GitHub Actions setup, secrets, variables, rollback: `docs/DEPLOY.md`.
- [ ] 2. Health: `GET /healthz` (alive), `GET /readyz` (database reachable). Metrics: `GET /metrics` with `Authorization: Bearer <METRICS_TOKEN>` (404 unless the token is configured).
- [ ] 3. Alert thresholds and incident playbooks: `docs/RUNBOOK.md`.
- [ ] 4. Before inviting members: staging deploy, load test (`docs/LOADTEST.md`, `deploy/loadtest/k6.js`), restore drill (`docs/RUNBOOK.md`), first admin via `BOOTSTRAP_ADMIN_*` secrets then delete them.
- [ ] 5. Database provider (Supabase now, anything later) is only environment variables: `docs/DATABASE.md`.

---

## Track H: Security review

- [ ] 1. Threat controls are listed per area in `AGENTS.md` and mapped to tests in `TESTING.md` (security coverage matrix: auth, sessions, SSRF, webhooks, invites, isolation, database RLS, secrets).
- [ ] 2. Try the boundaries yourself: bad tokens, reused refresh tokens (session revoked), another member's ids (always 404), private-IP source URLs (rejected), forged webhook signatures (identical 401), oversized bodies (413), brute-force logins (lockout, identical for unknown emails).
- [ ] 3. Open items are written down: `ROADMAP.md`, `PROJECT_BRAIN.md` risks, privacy compliance deliberately deferred (`DECISIONS.md` D-008).

---

## 3. Endpoint to screen map (frontend cheat sheet)

| Screen | Calls |
|---|---|
| Login / signup | `POST /api/v1/auth/login`, `POST /api/v1/auth/register`, `POST /api/v1/auth/refresh`, `POST /api/v1/auth/logout` |
| Feed | `GET /api/v1/me/feed`, `POST /api/v1/ws-ticket`, WebSocket `/ws`, `POST /api/v1/me/feed/{id}/feedback`, `DELETE /api/v1/me/feed/{id}/feedback` |
| Saved views | `GET /api/v1/me/views`, `POST /api/v1/me/views`, `PUT /api/v1/me/views/{id}`, `DELETE /api/v1/me/views/{id}`, then `GET /api/v1/me/feed?view=<id>` |
| Subscriptions | `GET /api/v1/me/subscriptions`, `PUT /api/v1/me/subscriptions`, `GET /api/v1/event-types`, `GET /api/v1/sources` |
| Alerts | `GET /api/v1/alert-templates`, `GET /api/v1/me/alerts`, `POST /api/v1/me/alerts`, `POST /api/v1/me/alerts/from-template`, `POST /api/v1/me/alerts/preview`, `GET /api/v1/me/alerts/fires`, `POST /api/v1/me/alerts/fires/{id}/feedback` |
| Channels | `GET /api/v1/me/channels`, `POST /api/v1/me/channels`, `POST /api/v1/me/channels/{id}/test`, `GET /api/v1/me/channels/deliveries` |
| Account | `POST /api/v1/me/password`, `POST /api/v1/me/logout-all` |
| Admin: members | `GET /api/v1/admin/users`, `POST /api/v1/admin/users/{id}/suspend`, `POST /api/v1/admin/users/{id}/reactivate`, `POST /api/v1/admin/users/{id}/reset-password` |
| Admin: invites | `POST /api/v1/admin/invites`, `GET /api/v1/admin/invites`, `DELETE /api/v1/admin/invites/{id}` |
| Admin: sources | `GET /api/v1/admin/sources`, `POST /api/v1/admin/sources/test`, `POST /api/v1/admin/sources`, `PUT /api/v1/admin/sources/{id}`, `POST /api/v1/admin/sources/{id}/run`, `POST /api/v1/admin/sources/{id}/webhook-secret`, `POST /api/v1/admin/items` |
| Admin: oversight | `GET /api/v1/admin/audit`, `GET /api/v1/admin/usage`, `GET /api/v1/admin/feedback/summary` |

---

## 4. Seed script (a populated dev environment in one go)

Needs `curl` and `jq` (bash / Git Bash / WSL). Safe to run on a fresh dev database.
```bash
#!/usr/bin/env bash
set -euo pipefail
BASE=${BASE:-http://localhost:8080}
j() { curl -sS -X "$1" "$BASE$2" -H "Content-Type: application/json" ${TOKEN:+-H "Authorization: Bearer $TOKEN"} ${3:+-d "$3"}; }

TOKEN=$(j POST /api/v1/auth/login '{"email":"admin@example.com","password":"dev-admin-password-change-me"}' | jq -r .token)
ADMIN=$TOKEN
CODE=$(j POST /api/v1/admin/invites '{}' | jq -r .code)
MEMBER=$(j POST /api/v1/auth/register "{\"email\":\"demo@example.com\",\"password\":\"demo-password-123\",\"invite_code\":\"$CODE\"}" | jq -r .token)

# a manual source the admin can post announcements through
SRC=$(j POST /api/v1/admin/sources '{"name":"feed","type":"manual","entities":["news"]}' | jq -r .id)
j PUT /api/v1/admin/sources/$SRC '{"enabled":true,"entities":["news"]}' >/dev/null

# the member follows it and wants an alert on "outage"
TOKEN=$MEMBER
j PUT /api/v1/me/subscriptions '{"subscriptions":[{"kind":"source","value":"feed"}]}' >/dev/null
j POST /api/v1/me/alerts/from-template '{"template_id":"keyword-mention","values":["outage"]}' >/dev/null

# some content
TOKEN=$ADMIN
for t in "Welcome to TradePulse" "Quarterly update published" "Major outage at provider" "Service restored"; do
  j POST /api/v1/admin/items "{\"source\":\"feed\",\"title\":\"$t\"}" >/dev/null; sleep 1
done
echo "member token: $MEMBER"
echo "log in as demo@example.com / demo-password-123"
```
Then open `GET /api/v1/me/feed` and `GET /api/v1/me/alerts/fires` with the member token.

---

## 5. Recipes (common needs)

| Need | Do |
|---|---|
| Reset everything | `docker compose down -v && docker compose up --build` |
| Get the contract file | `docker compose run --rm openapi` (writes `openapi.json`) |
| Types for the UI | `npx openapi-typescript openapi.json -o src/api/schema.d.ts` |
| See live server logs | `docker compose logs -f api` |
| Run the tests | `docker compose run --rm check` |
| Use a hosted database | put `DATABASE_URL` in `.env` (`docs/DATABASE.md`) |
| Different demo data | `SIM_DEMO=status` or `none` in `.env`, restart |
| Allow my frontend origin | `ALLOWED_ORIGINS=http://localhost:5173` in `.env`, recreate `api` |
| New dev admin | change `BOOTSTRAP_ADMIN_EMAIL` and `BOOTSTRAP_ADMIN_PASSWORD` in `.env` (an existing account is never overwritten) |

---

## 6. Reference snippets for the UI

**Live connection with reconnect and catch-up** (TypeScript, framework-free):
```ts
type Item = { id: string; cursor: string; title: string };
let lastCursor = '';
const seen = new Set<string>();

async function connect(attempt = 0): Promise<void> {
  const t = await api('/api/v1/ws-ticket', { method: 'POST' }).then((r) => r.json());   // api() = wrapper from docs/CLIENT.md
  const ws = new WebSocket(`${WS_BASE}/ws?ticket=${t.ticket}`);
  ws.onopen = async () => { attempt = 0; await catchUp(); };
  ws.onmessage = (e) => {
    const m = JSON.parse(e.data);
    if (m.type === 'feed_item') render(m.data);
    if (m.type === 'alert') notify(m.data);
    if (m.type === 'alert_paused' || m.type === 'channel_disabled') warn(m.data);
  };
  ws.onclose = () => setTimeout(() => connect(attempt + 1), Math.min(30000, 1000 * 2 ** attempt) * (0.5 + Math.random()));
}
function render(it: Item) { if (seen.has(it.id)) return; seen.add(it.id); lastCursor = it.cursor || lastCursor; /* update state */ }
async function catchUp() {
  while (lastCursor) {                                     // skip on first start: the first page load sets lastCursor
    const p = await api(`/api/v1/me/feed?after=${lastCursor}&limit=100`).then((r) => r.json());
    if (!p.items.length) break;
    p.items.forEach(render);
    if (!p.next_cursor) break; lastCursor = p.next_cursor;
  }
}
```
**Shapes you will render** (full schemas in `openapi.json`):
```json
{"items":[{"id":"...","cursor":"c1_...","title":"Major outage at provider","body":"","url":"https://...","source":"feed","entities":["news"],
           "severity":"notable","reason":"Matched your source subscription \"feed\"","created_at":"2026-10-22T10:00:00Z"}],
 "order":"desc","next_cursor":"c1_..."}
```
```json
{"title":"Unprocessable Entity","status":422,"detail":"interval_seconds must be between 60 and 86400"}
```

---

## 6a. Troubleshooting

| Symptom | Cause and fix |
|---|---|
| `401` right after it worked | Access token expired (15 min): refresh (`docs/CLIENT.md`). After `logout-all`, password change or reset every old token dies at once. |
| `403` on register | Invite code wrong, used, expired or bound to another email. Create a new one as admin. |
| `422` | Read `detail` (field-level `errors` may follow). Subscriptions and rules are validated against `GET /api/v1/event-types`. |
| `429` | Rate limit or lockout. Wait `Retry-After`. Ten failed logins lock the account for 15 minutes. |
| Browser CORS error | Add your origin to `ALLOWED_ORIGINS`, recreate the `api` container. |
| WebSocket closes immediately | Ticket used twice or older than 30 s (get a new one every time), origin not allowed, or more than 5 connections for the user (close code 1008). |
| Feed empty | You have no subscriptions yet, or nothing matched. Subscribe to `{"kind":"source","value":"feed"}` after posting, or use the demo world's entities. |
| Webhook returns 401 | Wrong secret, timestamp older than 5 minutes, or body changed after signing. Sign the exact bytes you send. |
| New source shows `pending` | Sources start disabled and run on their interval; use `POST /api/v1/admin/sources/{id}/run` for an immediate fetch. First fetch only remembers existing items (no flood). |
| `docker compose up` fails on Windows | Use WSL 2 backend; make sure Docker Desktop is running; run `docker compose down -v` to clear a bad volume. |
| Something else | Note the `X-Request-ID` response header and the log line with that id; open an issue with the command and response. |

---

## 7. Learning path by level

| Level | Time | Do |
|---|---|---|
| **L0 Curious** | 15 min | Track A |
| **L1 Junior frontend** | 1 day | Setup, Track C steps 1-4, Track D steps 1-3 (login, signup, feed) |
| **L2 Frontend** | 1 week | Track B, then Track D complete |
| **L3 Senior / lead** | 2 days | `docs/API.md`, `docs/WS.md`, `docs/CLIENT.md`, then design the auth/refresh and reconnect layers; review Track H |
| **L4 Integrator** | 1 day | Track E |
| **L5 Backend contributor** | 1 week | Track F; first task: a small item from `BACKLOG.md` |
| **L6 Operator** | 2 days | Track G; run the load test and restore drill |

---

## 8. Rules of the road

- The contract is `openapi.json` plus `docs/API.md`, `docs/WS.md`. Within `/api/v1` changes are additive; ignore unknown fields and enum values in your client.
- Cursors are opaque. IDs are opaque. Timestamps are UTC RFC 3339.
- Never log tokens, secrets or webhook signatures.
- Do not point tests or demos at production. Use staging or the local stack.
- Respect sources: robots.txt, site terms, X pricing and budgets.

---

## 9. Dev Advocate playbook

**The 10-minute demo (live).** Prepare: `docker compose up --build`, run the seed script, keep `/docs` and a terminal open.
1. *Problem (1 min):* "Information is scattered; people miss what matters."
2. *Invite-only access (1 min):* create an invite, register a member (Track A steps 3-4).
3. *Sources (2 min):* in `/docs`, `POST /api/v1/admin/sources/test` on a real RSS URL: "no deploy, no code, preview first".
4. *Personal feed with reasons (2 min):* subscribe, post an item (`POST /api/v1/admin/items`), show `GET /api/v1/me/feed` and the `reason`.
5. *Alerts you can trust (2 min):* template, preview ("would have fired 7 times"), then fire it live; mention cooldown and auto-pause.
6. *Your systems (2 min):* send a signed webhook in, receive a signed alert out (Track E).

**The 30-minute workshop.** Setup (5, Section 2) -> Track A (10) -> pick one of: Track B mock, Track E webhook (10) -> Q&A (5). Success = each attendee has a token, a subscription and has seen an alert.

**Say this / do not say that**
- Say: invite-only, explainable ("why am I seeing this"), spam-resistant (cooldown, preview, auto-pause), safe by design (SSRF guard, signed webhooks, row-level security), configurable per source.
- Do not claim: real-time exchange data (licensing is the owner's call), end-user privacy compliance (deliberately deferred), multi-instance scale (single instance for 10 to 50 members), that X works without a paid token.

**Metrics to track from day one** (from `PROJECT_BRAIN.md`): time to first useful alert (target under 10 minutes), alert usefulness rate (target over 70%: `GET /api/v1/admin/feedback/summary` returns the precision once members rate alerts with 👍/👎), weekly active members, source freshness.

**Feedback loop.** Collect: the command, the response, the `X-Request-ID`, and what the developer expected. Triage into `BACKLOG.md`; user-visible fixes go in `CHANGELOG.md`.

**Keep this file true.** When an endpoint changes, update the tables here; a test (`TestDeveloperGuideOnlyMentionsRealEndpoints`) fails if this guide names an endpoint or doc file that does not exist.
