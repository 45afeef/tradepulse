# Alerts and channels

Subscriptions fill your **feed** (everything relevant). **Alerts** are the subset you must not miss: they are pushed to you, with a reason, the moment they fire.

## Concepts
- **Rule** = scope + optional types/conditions + behavior.
  - *Scope*: `entity` (topics/symbols the item lists), `keyword` (whole word or phrase in title/summary) or `source` (items from a named source). Up to 20 values; any one matches.
  - *Types / conditions*: same fields as subscription conditions (`GET /api/v1/event-types`), e.g. `abs_change_pct >= 2` on `price_changed`.
  - *Behavior*: `cooldown_seconds` (default 300, minimum 10), `severity` (`info|notable|high`), `channels` (`in_app` and/or your channel ids), `enabled`.
- **Template** = a shipped, pre-filled rule (`GET /api/v1/alert-templates`). Use `POST /api/v1/me/alerts/from-template` with just the values to watch.
- **Preview** (`POST /api/v1/me/alerts/preview`) replays recent events (since the server started, last 24 h, up to 1000) against a draft and reports how many alerts it would have produced and whether it would be auto-paused. Nothing is saved or sent.
- **Fire** = one alert (`GET /api/v1/me/alerts/fires`), always with a `reason`.

## Guardrails (protect trust)
- Per-rule **cooldown**; at most **50 rules** and **5 channels** per user.
- **Auto-pause:** a rule that fires more than `ALERT_MAX_FIRES_PER_HOUR` (default 30) in one hour is paused; you get a live `alert_paused` message and the rule shows `paused_reason`. Save it with `enabled: true` after tightening it.
- Suspended members' rules stop immediately and resume on reactivation.

## Channels
- `in_app`: built in. WebSocket message `{"type":"alert","data":{...fire...}}` (also `alert_paused`, `channel_disabled`).
- **Outbound webhook** (`POST /api/v1/me/channels`, needs `WEBHOOK_SIGNING_KEY` on the server): HTTPS URL on a public address (port 443). Each alert is POSTed as JSON:
```json
{"type":"alert","id":"...","rule_id":"...","rule_name":"Big movers","severity":"high","title":"TCS up 3.10%","body":"Last price 3500.00","url":"https://...","source":"nse","entities":["tcs"],"reason":"Matched your alert ...","fired_at":"2026-10-18T10:00:00Z"}
```
  Headers: `X-TP-Timestamp`, `X-TP-Signature: sha256=HMAC-SHA256(secret, "<timestamp>.<raw body>")`, `X-TP-Event: alert`, `X-TP-Delivery: <id>`. Verify the signature and reject old timestamps (replay). The secret is shown once at creation or rotation.
  - Retries: 2 s, 10 s, 60 s for network errors, 5xx, 408 and 429; 3xx and other 4xx are not retried; redirects are never followed.
  - After 10 consecutive failed deliveries the channel is disabled (you get `channel_disabled`); re-enable with `POST /api/v1/me/channels/{id}/enable`.
  - `POST /api/v1/me/channels/{id}/test` sends a marked test (`"test": true`); history at `GET /api/v1/me/channels/deliveries`.
- Delivery is at-least-once and best effort (the retry queue is in memory). Add email/Telegram later by implementing another channel type; matching code does not change.

## Notes
- Previews use events seen since the server started, so they are thin right after a restart.
- Cooldown state survives restarts (`last_fired_at`); hourly counters do not (they reset on restart).
