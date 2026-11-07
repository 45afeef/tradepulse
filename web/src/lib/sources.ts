import type { Source, SourceUpdate, WebConfig } from '../api/types';

/** The PUT body that keeps a source exactly as it is (PUT replaces, so unchanged fields must be resent). */
export function toUpdate(s: Source, over: Partial<SourceUpdate> = {}): SourceUpdate {
  const u: SourceUpdate = { enabled: s.enabled, interval_seconds: s.interval_seconds || undefined, entities: s.entities };
  if (s.url) u.url = s.url;
  if (s.web) u.web = s.web;
  if (s.webhook) u.webhook = s.webhook;
  if (s.x) u.x = s.x;
  return { ...u, ...over };
}

export const INTERVALS: [number, string][] = [[60, 'Every minute'], [300, 'Every 5 minutes'], [900, 'Every 15 minutes'], [1800, 'Every 30 minutes'], [3600, 'Hourly'], [21600, 'Every 6 hours'], [86400, 'Daily']];

export const TYPE_INFO: Record<string, { title: string; blurb: string; icon: 'database' | 'eye' | 'zap' | 'megaphone' | 'users' }> = {
  rss: { title: 'RSS / Atom feed', blurb: 'The most reliable way to follow a site that publishes a feed.', icon: 'database' },
  webmonitor: { title: 'Web page monitor', blurb: 'Watch a page and pick out headlines with CSS selectors. For sites without a feed.', icon: 'eye' },
  webhook: { title: 'Webhook (push)', blurb: 'Your own systems send items in, signed. Ideal for automation.', icon: 'zap' },
  manual: { title: 'Manual posts', blurb: 'Publish announcements yourself from the Publish page.', icon: 'megaphone' },
  x: { title: 'X account', blurb: 'Follow an account through the official API (budget-capped).', icon: 'users' },
};

/** True when an error message says the site's robots.txt forbids reading the page. */
export const robotsBlocked = (msg: string | undefined | null): boolean => /robots/i.test(msg ?? '');

export const REASON_MIN = 10;
export const REASON_MAX = 200;
export const reasonOk = (r: string) => r.trim().length >= REASON_MIN && r.trim().length <= REASON_MAX;

/** The same page config with the robots.txt override switched on (a recorded reason is mandatory). */
export function withRobotsOverride(web: WebConfig, reason: string): WebConfig {
  return { ...web, ignore_robots: true, ignore_robots_reason: reason.trim() };
}
/** The same page config, respecting robots.txt again. */
export function withoutRobotsOverride(web: WebConfig): WebConfig {
  const { ignore_robots: _a, ignore_robots_reason: _b, ...rest } = web;
  return rest;
}
