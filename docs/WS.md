# WebSocket protocol (v1)

Live delivery of feed items and alerts. OpenAPI cannot describe WebSockets, so this file is the contract. A test fails if a message type exists in the server but is not described here.

## Connect
1. `POST /api/v1/ws-ticket` with `Authorization: Bearer <access token>` -> `{"ticket": "...", "expires_in": 30}` (single use, 30 s).
2. Open `wss://<host>/ws?ticket=<ticket>` (browsers cannot set headers on WebSockets, hence the ticket). A **new ticket is needed for every connection**, including reconnects.
3. Allowed origins are the server's `ALLOWED_ORIGINS` (and same-origin). At most **5 connections per user**; a 6th is closed with code 1008.

## Messages (server -> client, JSON text frames)
Envelope: `{"type": "<kind>", "data": <payload>}` (`connected` has no `data`). Clients must ignore unknown `type` values and unknown fields.

| type | data | when |
|---|---|---|
| `connected` | none | right after the handshake |
| `feed_item` | a feed item (`id`, `cursor`, `title`, `body`, `url`, `severity`, `source`, `entities`, `reason`, `created_at`, ...) | an event matched your subscriptions |
| `alert` | an alert fire (`id`, `rule_id`, `rule_name`, `severity`, `title`, `body`, `reason`, `fired_at`, ...) | one of your alert rules fired (channel `in_app`) |
| `alert_paused` | `{"rule_id", "name", "reason"}` | a rule fired too often and was paused automatically |
| `channel_disabled` | `{"channel_id", "name", "reason"}` | an outbound channel was disabled after repeated delivery failures |

Client -> server messages are ignored (the connection is one-way); only control frames matter.

## Keep-alive and close codes
- The server pings every 25 s and closes connections idle for 60 s without a pong. Browsers answer pings automatically; other clients must answer with a pong.
- `1001` going away (server shutdown / deploy), `1008` policy (connection cap reached or account suspended). A slow reader may be disconnected without a close frame.
- Cloud Run ends connections after the request timeout (up to 60 min) and on every deploy: expect regular reconnects.

## Reconnect and catch-up (required behavior)
Live delivery is **best effort** (at-most-once per connection). To never miss items:
1. Reconnect with exponential back-off and jitter (1 s, 2 s, 4 s ... capped at 30 s). Fetch a **new ticket** each attempt (refresh the access token first if it expired).
2. Remember the `cursor` of the newest `feed_item` you processed. After reconnecting, call `GET /api/v1/me/feed?after=<that cursor>&limit=100` and repeat with `next_cursor` until the page is empty (items arrive oldest first). Then continue with live messages.
3. De-duplicate by item `id` (a live item and a catch-up page can overlap).
4. For alerts, call `GET /api/v1/me/alerts/fires?after=<cursor>` the same way (fires are low volume; `id` de-duplication applies).
5. Do not reconnect in a tight loop on 1008: tell the user (cap or suspension).
