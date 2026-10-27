# Building a client (typed, resilient)

## 1. Generate types from the contract
```bash
npx openapi-typescript openapi.json -o src/api/schema.d.ts      # types only, no runtime dependency
# or a full client: npx @hey-api/openapi-ts -i openapi.json -o src/api
```
Regenerate whenever `openapi.json` changes (CI keeps it in sync with the server; read the "API" notes in `CHANGELOG.md`).

## 2. Token handling
- Keep the access token in memory; keep the refresh token in the most protected storage available (an httpOnly cookie via your own backend-for-frontend is best; `localStorage` is readable by any injected script).
- Refresh **before** expiry (use `expires_in`) or on the first 401, with a **single in-flight refresh** shared by all requests. Replace *both* tokens from the response. If refresh returns 401 the session is over: go to the login screen.
```ts
let refreshing: Promise<void> | null = null;
async function api(path: string, init: RequestInit = {}, retry = true): Promise<Response> {
  const res = await fetch(BASE + path, { ...init, headers: { ...init.headers, Authorization: `Bearer ${tokens.access}` } });
  if (res.status === 401 && retry) {
    refreshing ??= refreshTokens().finally(() => (refreshing = null));
    await refreshing;                      // throws -> redirect to login
    return api(path, init, false);
  }
  return res;
}
```

## 3. Errors
Parse `application/problem+json`; show `detail`; log `X-Request-ID`. On 429 wait `Retry-After`. Do not retry non-idempotent POSTs blindly.

## 4. Live updates
Follow `docs/WS.md`: ticket -> connect -> on every (re)connect catch up with `GET /me/feed?after=<last cursor>` until empty -> then render live messages; de-duplicate by `id`.

## 5. Lists
Infinite scroll: `GET /me/feed?limit=30`, then `before=<next_cursor>`. Never assume `next_cursor` absent means "no more" without having requested it when present.

## 6. CORS
The browser origin must be in the server's `ALLOWED_ORIGINS`. `X-Request-ID` and `Retry-After` are exposed to scripts.
