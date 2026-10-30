package api

import "strings"

// Markdown shown at the top of /docs (dev only). § stands for a backtick.
const overviewRaw = `
**TradePulse** collects information from many sources, normalizes it into *events*, and delivers
**the items relevant to you** in one live feed, with automatic alerts. Access is **invite-only**.

## Quick start (5 minutes)
1. **Get an invite code** from an administrator (looks like §tp_inv_…§). Codes are single-use and expire.
2. §POST /api/v1/auth/register§ with your email, a password (10+ characters) and the invite code. The response contains a **token**.
3. Click **Authorize** in this page (or send §Authorization: Bearer <token>§ on every request).
4. §GET /api/v1/event-types§ shows what kinds of events exist and **which fields you can filter on**.
5. §PUT /api/v1/me/subscriptions§ tells the system what you care about (see *Subscriptions* below). **Until you subscribe, your feed is empty.**
6. §GET /api/v1/me/feed§ returns recent items. For live delivery: §POST /api/v1/ws-ticket§, then open the WebSocket (see *Real-time*).

## Authentication
- Register or log in to get a JWT; send it as §Authorization: Bearer <token>§.
- Access tokens are short-lived (15 minutes by default, see §expires_in§). Keep the §refresh_token§ and call §POST /api/v1/auth/refresh§ before expiry: it returns a **new pair** and the old refresh token stops working (reusing an old one revokes the whole session). §POST /api/v1/auth/logout§ ends a session, §POST /api/v1/me/logout-all§ ends all of them, §POST /api/v1/me/password§ changes your password (other sessions are signed out).
- Repeated failed logins lock the account for a while (the same answer is given whether or not the account exists).
- A **suspended** account is rejected everywhere immediately and its live connections are closed.
- Brute force is limited: authentication endpoints allow only a few attempts per minute per IP (HTTP 429, see §Retry-After§).

## Subscriptions (what you receive)
A subscription says *what to watch*; optional **types** and **conditions** narrow it. An item is delivered if **any** subscription matches, unless it is **muted**.

| kind | matches when |
|---|---|
| §entity§ | the event lists that entity (case-insensitive), e.g. §tcs§, §checkout§ |
| §keyword§ | the whole word/phrase appears in the item's title or text, e.g. §repo rate§ |
| §source§ | the event comes from that source id, e.g. §status_sim§ |

**Conditions** are §field operator value§ checks; **all** must hold. Operators: §>§ §>=§ §<§ §<=§ §==§ §!=§ §in§ §contains§ §any_of§.
Fields come from §GET /api/v1/event-types§ (built-in: §type§, §source§, §title§, §summary§, §url§, §entity§; plus each type's own fields).
If an event does not have the field, the condition is simply *false*.

Example: notify me when *checkout* is anything other than up:
§§§json
{"subscriptions":[{"kind":"entity","value":"checkout","conditions":[{"field":"status","op":"!=","value":"up"}]}],
 "muted":["maintenance"]}
§§§
Every delivered item has a **reason** explaining which subscription matched.

## Sources
Items come from **sources** that administrators configure (RSS/Atom feeds, web pages read with CSS selectors, manual posts). §GET /api/v1/sources§ lists the enabled ones: subscribe with kind §source§ and the source's name, or with kind §entity§ and one of its topics. Each source is polled on its own schedule; new items appear within one polling interval.

## Alerts and channels
The feed shows everything that matches your subscriptions; **alerts** are the subset you must not miss, pushed to you immediately.
1. §GET /api/v1/alert-templates§ lists ready-made rules, or build your own: **scope** (entities / keywords / sources) + optional **types** and **conditions** (same fields as in subscriptions).
2. §POST /api/v1/me/alerts/preview§ replays recent events against your draft ("would have fired 7 times") before you save it. §POST /api/v1/me/alerts§ (or §/from-template§) saves it.
3. When a rule fires you get a live WebSocket message §{"type":"alert","data":{...}}§ (channel §in_app§) and/or a signed POST to your own HTTPS endpoint (§POST /api/v1/me/channels§; signed like inbound webhooks, retried, auto-disabled after repeated failures). Every fire has a **reason**; history: §GET /api/v1/me/alerts/fires§.
4. Guardrails: per-rule **cooldown** (default 5 minutes), at most 50 rules and 5 channels per user, and a rule that fires too often in an hour is **paused automatically** (WebSocket message §alert_paused§, §paused_reason§ on the rule). Re-enable it by saving it with §enabled: true§ after tightening it.

## Inbound webhooks (push items from your own systems)
An admin creates a source of type §webhook§; the response shows its **secret once** and its path (§/api/v1/hooks/<name>§). Then POST JSON from any system (n8n, Zapier, scripts):
§§§json
{"id":"optional-unique-id","title":"Required headline","summary":"optional","url":"https://example.com/story","entities":["topic"],"published_at":"2026-10-17T10:00:00Z"}
§§§
- **hmac** (default): send §X-TP-Timestamp§ (unix seconds, within 5 minutes) and §X-TP-Signature: sha256=<hex>§ where hex = HMAC-SHA256(secret, "<timestamp>.<raw body>").
- **token**: send the secret as §X-Webhook-Token§ or §Authorization: Bearer <secret>§ (for tools that cannot sign).
- Responses: §201§ accepted, §200§ duplicate (same §id§ or same title+url: safe to retry), §401§ bad credentials (also for unknown sources), §413§ over 64 KB, §422§ invalid payload, §429§ rate limited, §503§ webhooks not enabled on the server. Details and code samples: §docs/WEBHOOKS.md§ in the repository.

## Saved views and feedback
**Views** (§/me/views§) are named filters over your feed (sources, topics, keywords, event types, time window): create one, then list the feed through it with §GET /me/feed?view=<id>§ (paging works as usual). **Feedback**: rate a feed item (§POST /me/feed/{id}/feedback§) or an alert that fired (§POST /me/alerts/fires/{id}/feedback§) as §useful§ or §not_useful§; your rating comes back as §feedback§ on the item. Ratings tell the operators which sources and rules are noisy (alert precision is the share rated useful).

## Pagination
Growing lists (feed, alert fires, deliveries, audit) are paged with opaque **cursors**. Responses contain §next_cursor§ and §order§. Scroll back: §?before=<next_cursor>§ (newest first). Catch up after a reconnect: §?after=<cursor of the last item you saw>§ returns the newer items **oldest first**; repeat with the new §next_cursor§ until it is absent. Every item has a §cursor§ (live WebSocket feed items too). Never parse cursors. Short, bounded lists (rules, channels, sources, members) are returned whole.

## Real-time (WebSocket)
1. §POST /api/v1/ws-ticket§ (with your token) → a **one-time ticket valid for 30 seconds**.
2. Connect to §ws(s)://<host>/ws?ticket=<ticket>§ (browsers cannot set headers on WebSockets, hence the ticket). A new ticket is needed for every connection.
3. Messages are JSON text frames:
§§§json
{"type":"connected"}
{"type":"feed_item","data":{"id":"…","title":"checkout is down","body":"Latency 950 ms","severity":"notable","reason":"Matched your entity subscription \"checkout\"","created_at":"…"}}
§§§
4. **Reconnect with backoff** when the connection drops (deploys and idle limits close connections). After reconnecting, call §GET /api/v1/me/feed§ to catch up on anything missed.
5. Limits: at most 5 simultaneous connections per user; slow readers are disconnected. Close code 1008 = policy (cap/suspended), 1001 = server going away.
The WebSocket cannot be tried from this page; use a client such as Postman or §websocat§.

## Errors
Every error, including unknown paths, uses **RFC 9457 problem JSON** (§Content-Type: application/problem+json§): §{"title":"Unauthorized","status":401,"detail":"…"}§. Responses carry §X-Request-ID§ (quote it when reporting problems) and rate-limit responses carry §Retry-After§. Validation failures (422) list the offending fields.
Common codes: §401§ missing/invalid token · §403§ not allowed (e.g. not an admin, bad invite) · §409§ conflict · §422§ invalid input · §429§ rate limited · §413§ body too large.

## Administrators
Endpoints under **admin** require the §admin§ role (others get 403). Typical flow: create an invite → share the code → watch its status → suspend/reactivate members → review the audit log.
`

var overview = strings.ReplaceAll(overviewRaw, "§", "`")

const (
	tagOps      = "Health probes for load balancers and monitoring. No authentication."
	tagAuth     = "Create an account (invite required) and log in. Rate limited per IP."
	tagMe       = "Your own data: subscriptions (what you receive) and your feed history. Requires a bearer token."
	tagCatalog  = "Discover event types and the fields usable in subscription conditions."
	tagRealtime = "Live delivery over WebSocket. Get a one-time ticket here, then connect to /ws."
	tagAlerts   = "Rules that push the items you cannot afford to miss: templates, custom rules, preview, history. Requires a bearer token."
	tagChannels = "Where alerts are delivered: in-app (built in) and signed outbound webhooks you register."
	tagAdmin    = "Administrator-only: invites, members and the audit log. Every action is audited."
)
