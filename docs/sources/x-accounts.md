# Source: X (Twitter) accounts

| Field | Value |
|---|---|
| Accounts (owner-provided, D-017) | Donald Trump `realDonaldTrump`, Elon Musk `elonmusk`, Narendra Modi `narendramodi` |
| Connector | `x`: official X API v2 only (D-011). **Built (0.6.0-dev); not yet tested by the owner.** Available only when `X_BEARER_TOKEN` is set. |
| Configuration (env, D-023) | `X_BEARER_TOKEN` (secret), `X_API_BASE_URL`, `X_MONTHLY_BUDGET_USD` (hard cap, default 20; `0` = never call X), `X_COST_PER_RESOURCE_USD` (default 0.005: keep in sync with your plan's price), `X_MAX_POSTS_PER_FETCH` (5-100). Swap accounts or tokens by changing env only. |
| Cost control | Polls with `since_id`, so only NEW posts are returned and billed. Every call is checked against the persistent monthly ledger BEFORE it is made; the page size shrinks to what the remaining budget allows; below the API minimum (5 posts) polling stops and the source shows `failing` with "monthly X budget reached". Retweets and replies are excluded. A handle lookup (one read) happens when the source is saved. `GET /api/v1/admin/usage` shows spend and remaining budget. |
| Terms/legal | **Not yet verified.** Before enabling in production read the current X developer terms (display requirements, caching/retention, redistribution) and record the date here. Scraping X is not allowed (D-011). |
| Interval | per source (`interval_seconds`); with `since_id` a shorter interval does not cost more per post, but each poll counts toward X rate limits |

## Add an account (admin, in /docs)
1. Put `X_BEARER_TOKEN` (and optionally a budget) in `.env`, restart.
2. `POST /api/v1/admin/sources/test` with `{"name":"modi","type":"x","interval_seconds":300,"entities":["india","politics"],"x":{"handle":"narendramodi"}}` previews recent posts (this spends a few reads).
3. `POST /api/v1/admin/sources` with the same body plus `"enabled": true`. The first fetch only remembers the latest posts (baseline); new posts arrive live.
4. Watch `GET /api/v1/admin/sources/{id}` and `GET /api/v1/admin/usage`.
