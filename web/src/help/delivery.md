# Webhooks: alerts in your own tools

A webhook sends each alert to an address you control, so you can feed chat, automation tools or your own servers. You can add up to 5 webhooks.

## Adding one

Under **Delivery**, select **Add webhook**, give it a name and enter the address. It must be a public **https** address on the standard port; private and internal addresses are refused for safety. When you save you see a **signing secret** once. Copy it into your receiving system straight away; if you lose it, rotate it.

Then edit an alert and tick the webhook under **Deliver to**. Use **Send test** to check the connection; the result appears in the delivery log.

## What you receive

A `POST` with a JSON body, for example:

```json
{"type":"alert","id":"...","rule_name":"Rate decisions","severity":"high","title":"RBI holds rates","body":"...","url":"https://...","source":"cnbc","entities":["rates"],"reason":"Matched your alert ...","fired_at":"2026-10-30T10:00:00Z"}
```

Test deliveries carry `"test": true`. Four headers are added: `X-TP-Timestamp`, `X-TP-Signature`, `X-TP-Event` and `X-TP-Delivery`.

## Verify every delivery

Compute `HMAC-SHA256(secret, timestamp + "." + rawBody)`, compare it with the `X-TP-Signature` header (which starts with `sha256=`) and reject timestamps older than 5 minutes. Use the raw request body, exactly as received.

```js
import crypto from 'node:crypto';

export function verify(secret, timestamp, rawBody, signatureHeader) {
  const expected = 'sha256=' + crypto.createHmac('sha256', secret).update(`${timestamp}.${rawBody}`).digest('hex');
  const a = Buffer.from(signatureHeader), b = Buffer.from(expected);
  const fresh = Math.abs(Date.now() / 1000 - Number(timestamp)) < 300;
  return a.length === b.length && crypto.timingSafeEqual(a, b) && fresh;
}
```

```python
import hashlib, hmac, time

def verify(secret: str, timestamp: str, raw_body: bytes, signature_header: str) -> bool:
    expected = "sha256=" + hmac.new(secret.encode(), timestamp.encode() + b"." + raw_body, hashlib.sha256).hexdigest()
    fresh = abs(time.time() - int(timestamp)) < 300
    return hmac.compare_digest(signature_header, expected) and fresh
```

## Retries and disabled webhooks

If your server is down or answers with an error, TradePulse retries a few times with growing pauses (2, 10 and 60 seconds). Redirects are not followed, and client errors such as 400 or 404 are not retried. After 10 failed deliveries in a row the webhook is **disabled** and you are told; fix your receiver and select **Re-enable**.

## Rotating the secret

Select **Rotate secret** if it may have leaked. The old secret stops working at once, so update your receiver right after.
