# Source: X (Twitter) accounts

| Field | Value |
|---|---|
| Handles (owner-provided, D-017) | `@realDonaldTrump`, `@elonmusk`, `@narendramodi` |
| Connector | `xapi` (official X API only, D-011) — **not built yet** (TP-016, needs TP-010 safe fetcher first) |
| Method | poll recent posts per account with `since_id`; cost is proportional to NEW posts returned |
| Budget | conservative default monthly cap + stop-on-exhaust (D-018); raise via env var |
| Terms/legal | **Not yet verified.** Before enabling in production: read the current X developer terms (display requirements, caching/retention limits, redistribution) and record the decision date here. Scraping X is not allowed (D-011). |
| Credentials | X API token via Secret Manager env var; never committed or logged |
| Expected volume | Unknown; these accounts post frequently, so spend metric + cap are mandatory |
| Status | Planned (M4) |
