const rtf = typeof Intl !== 'undefined' && 'RelativeTimeFormat' in Intl ? new Intl.RelativeTimeFormat('en', { numeric: 'auto', style: 'short' }) : null;

/** "just now", "5 min ago", "yesterday", then a short date. */
export function timeAgo(iso: string | undefined, now: number = Date.now()): string {
  if (!iso) return '';
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return '';
  const s = Math.round((now - t) / 1000);
  if (s < 45) return 'just now';
  const units: [number, Intl.RelativeTimeFormatUnit][] = [[60, 'second'], [60, 'minute'], [24, 'hour'], [7, 'day']];
  let v = s;
  for (const [div, unit] of units) {
    if (v < div) return rtf ? rtf.format(-Math.round(v), unit) : `${Math.round(v)} ${unit} ago`;
    v /= div;
  }
  return new Date(t).toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
}

export function shortDateTime(iso: string | undefined): string {
  if (!iso) return '';
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? '' : d.toLocaleString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
}

export function plural(n: number, one: string, many = one + 's'): string { return `${n} ${n === 1 ? one : many}`; }

export function money(n: number): string { return new Intl.NumberFormat(undefined, { style: 'currency', currency: 'USD', maximumFractionDigits: 2 }).format(n); }

export function pct(p: number | undefined): string { return p === undefined || p === null ? 'n/a' : `${Math.round(p * 100)}%`; }

export function duration(seconds: number): string {
  if (seconds % 86400 === 0 && seconds >= 86400) return plural(seconds / 86400, 'day');
  if (seconds % 3600 === 0 && seconds >= 3600) return plural(seconds / 3600, 'hour');
  if (seconds % 60 === 0 && seconds >= 60) return plural(seconds / 60, 'minute');
  return plural(seconds, 'second');
}

/** Stable pastel hue for a name (source chips get a consistent colour). */
export function hueOf(name: string): number {
  let h = 0;
  for (let i = 0; i < name.length; i++) h = (h * 31 + name.charCodeAt(i)) % 360;
  return h;
}

export function hostOf(url: string | undefined): string {
  try { return url ? new URL(url).hostname.replace(/^www\./, '') : ''; } catch { return ''; }
}

export function initials(email: string): string {
  const base = email.split('@')[0] ?? '?';
  const parts = base.split(/[._-]+/).filter(Boolean);
  return ((parts[0]?.[0] ?? '?') + (parts[1]?.[0] ?? '')).toUpperCase();
}

export function splitList(s: string): string[] {
  return s.split(/[\n,]+/).map((x) => x.trim()).filter(Boolean);
}

/** Only http(s) links are ever rendered as hrefs (defence in depth: the server already validates them). */
export function safeUrl(u: string | undefined): string | undefined {
  if (!u) return undefined;
  try { const p = new URL(u); return p.protocol === 'http:' || p.protocol === 'https:' ? u : undefined; } catch { return undefined; }
}

/** Lower-case slug for source names: letters, digits, '-' and '_' (matches the server's rule). */
export function slugify(s: string): string {
  return s.toLowerCase().trim().replace(/[\s.]+/g, '-').replace(/[^a-z0-9_-]/g, '').replace(/^[^a-z0-9]+/, '').slice(0, 40);
}
export const validSlug = (s: string) => /^[a-z0-9][a-z0-9_-]{1,39}$/.test(s);
