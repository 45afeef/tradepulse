# Sources: news pages (owner-provided, Q-02)

| # | URL | Likely best method | Status |
|---|---|---|---|
| 1 | https://www.moneycontrol.com/markets/indian-indices/ | Moneycontrol appears to publish RSS feeds (listed by feed directories, e.g. under `moneycontrol.com/rss/`): verify the official feed URL with the admin *test-fetch* and prefer it. The indices page itself is a data/quote page, not an article list. | to verify (M3) |
| 2 | https://www.cnbc.com/ | CNBC publishes official RSS feeds per topic: prefer RSS over scraping the home page. | to verify (M3) |
| 3 | https://scanx.trade/stock-market-news | No known feed. Likely needs the web-page monitor (CSS selectors) or a JSON endpoint the page itself uses (check with the browser's network tab, allowed only if the terms permit). | to verify (M3) |

## Before enabling any of them
1. Terms of use and `robots.txt` allow automated reading (record the date and result in this file).
2. Prefer an official RSS/Atom feed; use the page monitor only where allowed.
3. Keep attribution and the "view original" link on every item; do not republish full article text (title, short summary and link only).
4. Choose a polite interval per source (see below); the platform enforces the global bounds.

## Latency is configurable per source (D-022)
Each source has its own `poll_interval_seconds`, limited only by the global bounds `SOURCE_MIN_INTERVAL_SECONDS` (default 60) and `SOURCE_MAX_INTERVAL_SECONDS` (default 86400). A breaking-news source can poll every minute; a rarely changing page once a day. Push sources (webhooks, streams) are instant. Adaptive back-off after unchanged responses and a per-host rate limit protect the target sites. The admin sees the effective interval and last/next run in the source health view.
