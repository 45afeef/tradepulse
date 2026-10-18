# Inbound webhooks

Push items from your own systems (n8n, Zapier, Make, scripts, other services) into the feed.

## Setup (admin)
1. Set `WEBHOOK_SIGNING_KEY` on the server (`openssl rand -base64 48`). Without it webhooks are disabled.
2. In `/docs`: `POST /api/v1/admin/sources` with
   `{"name":"my-hook","type":"webhook","enabled":true,"entities":["alerts"],"webhook":{"auth":"hmac"}}`
   (`auth`: `hmac` recommended, `token` for tools that cannot sign).
3. Copy `webhook_secret` from the response. **It is shown once**; rotate with `POST /api/v1/admin/sources/{id}/webhook-secret` (the old secret stops working immediately). Secrets are derived from the signing key, never stored.
4. Members subscribe to the source name (`my-hook`) or its topics (`alerts`).

## Sending an item
`POST /api/v1/hooks/my-hook`, JSON body (max 64 KB):
```json
{"id":"optional-unique-id","title":"Required headline","summary":"optional","url":"https://example.com/story","entities":["topic"],"published_at":"2026-10-17T10:00:00Z"}
```
Only `title` is required. Unknown fields are rejected. Items with the same `id` (or, without `id`, the same title+url) are de-duplicated, so **retries are safe**.

### hmac mode (default)
Headers: `X-TP-Timestamp` (unix seconds, must be within 5 minutes) and `X-TP-Signature: sha256=<hex>` where hex = HMAC-SHA256(secret, `"<timestamp>.<raw body>"`).
```bash
SECRET='whsec_...'; BODY='{"title":"Disk usage high"}'; TS=$(date +%s)
SIG=$(printf '%s.%s' "$TS" "$BODY" | openssl dgst -sha256 -hmac "$SECRET" -hex | sed 's/^.* //')
curl -X POST https://YOUR_HOST/api/v1/hooks/my-hook -H "X-TP-Timestamp: $TS" -H "X-TP-Signature: sha256=$SIG" -d "$BODY"
```
Sign the exact bytes you send.

### token mode
Send the secret as `X-Webhook-Token: <secret>` or `Authorization: Bearer <secret>`. Simpler, but a captured request can be replayed, so prefer hmac when the tool supports it.

## Responses
| Code | Meaning |
|---|---|
| 201 | accepted (`event_id` returned) |
| 200 | duplicate (already received; safe to ignore) |
| 401 | bad credentials, unknown or disabled source (always the same answer) |
| 413 | body over 64 KB |
| 422 | invalid payload (message says why) |
| 429 | rate limited (`WEBHOOK_RATE_PER_MINUTE` per source, default 60); honor `Retry-After` |
| 503 | webhooks not enabled on this server |
