# Choosing and tuning sources

Good sources make good feeds. Pick the most reliable type that works for each site.

## Which type should I use?

| Type | Use it when | Notes |
|---|---|---|
| **RSS / Atom feed** | The site publishes a feed | The most reliable. Try this first. |
| **Web page monitor** | There is no feed | Reads plain HTML with CSS selectors. No JavaScript pages. |
| **Webhook (push)** | Your own systems should send items in | Signed requests, ideal for automation. |
| **Manual posts** | You write announcements yourself | Use the Publish page. |
| **X account** | You follow an account on X | Official API only; spend is capped by a monthly budget. |

## RSS and Atom

Paste the feed address in the wizard and select **Test fetch**. To find a feed, look for links named RSS or Feed on the site, try adding `/feed` or `/rss` to the address, or check the page's source for a line containing `application/rss+xml`.

## Web page monitor

1. Open the page in your browser, right-click a headline and choose **Inspect**.
2. Find the element that wraps each headline, and note a selector for it, for example `article.story`. That is the **item selector**.
3. Inside it, the **title selector** (for example `h3 a`) and **link selector** (for example `a`) point at the title and the link.
4. Select **Test fetch** and adjust until the preview lists the right headlines.

Before using it, check the website's terms of use. The monitor respects the site's `robots.txt` and will not read pages the site disallows (see [If a site blocks automated reading](#robots-txt)). The first run only records what is already there, so members are not flooded.

## If a site blocks automated reading {#robots-txt}

Sometimes **Test fetch** fails with a message about `robots.txt`: the site asks automated tools not to read that page. TradePulse respects this by default.

When you have permission, or the site's terms clearly allow it, you can choose **Override robots.txt** for that single source. You must confirm you checked the terms and give a reason of 10 to 200 characters. The decision is recorded in the audit log with your name, the time and your reason, and the source shows a "robots.txt overridden" badge until you select **Respect robots.txt again**.

> **Tip:** Before overriding, look for an official feed or API, or ask the site for permission. An override does not make reading the page allowed; it only stops TradePulse from refusing.

## Webhook sources

Choose **signed requests** when your system can compute an HMAC, or a **bearer token** for tools that cannot. After saving you see a secret once and an example request. Your system sends JSON with at least a `title`; an `id` makes retries safe.

## X accounts

The server needs an X API token and a monthly budget. Each fetch costs a small amount; collection stops at the budget and the Overview warns you at 80%. If X is not configured, the source can be saved but nothing is fetched.

## How often to check

Faster checks cost more load and, for X, more money. Five minutes is a good start for news. The system spaces requests to the same site automatically and slows down when a source keeps failing.

## Topics

Add topics to a source, such as `markets` or `india`. They are put on every item it publishes and members can follow them, so keep them few and meaningful.

## When a source fails

Open it and read **Last error**. A 403 or a robots message usually means the site blocks automated reading: choose another source or ask the site. A timeout is often temporary. A selector that stopped matching means the site changed its layout: test again and update the selectors. See the [Operator handbook](/help/operator-handbook).
