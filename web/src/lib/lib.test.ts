import { describe, expect, it } from 'vitest';
import { duration, hostOf, hueOf, initials, plural, safeUrl, slugify, timeAgo, validSlug } from './format';
import { RecentSet, backoffMs } from './backoff';
import { LIST_OPS, describeCondition, isComplete, parseValue, valueToText } from './conditions';
import { mergeItems, patchItem, prependItem } from './feedCache';
import { auditLabel, toCsv } from './audit';
import { toUpdate } from './sources';
import type { FeedItem, FeedPage, Source } from '../api/types';
import { mergeSubscriptions } from './subscriptions';

const fi = (id: string): FeedItem => ({ id, cursor: 'c1_' + id, event_id: id, event_type: 't', source: 's', entities: [], title: id, body: '', severity: 'info', reason: '', created_at: '' });
const data = (...pages: string[][]) => ({ pages: pages.map((p): FeedPage => ({ items: p.map(fi), order: 'desc' })), pageParams: pages.map(() => '') });

describe('format', () => {
  it('timeAgo is human', () => {
    const now = Date.parse('2026-10-30T12:00:00Z');
    expect(timeAgo('2026-10-30T11:59:50Z', now)).toBe('just now');
    expect(timeAgo('2026-10-30T11:55:00Z', now)).toMatch(/5/);
    expect(timeAgo('2026-10-30T11:55:00Z', now)).toMatch(/ago/);
    expect(timeAgo(undefined, now)).toBe('');
    expect(timeAgo('garbage', now)).toBe('');
  });
  it('duration, plural, initials, hue and host', () => {
    expect(duration(300)).toBe('5 minutes'); expect(duration(3600)).toBe('1 hour'); expect(duration(86400)).toBe('1 day'); expect(duration(90)).toBe('90 seconds');
    expect(plural(1, 'alert')).toBe('1 alert'); expect(plural(2, 'alert')).toBe('2 alerts');
    expect(initials('jane.doe@example.com')).toBe('JD'); expect(initials('x@y.z')).toBe('X');
    expect(hueOf('cnbc')).toBe(hueOf('cnbc')); expect(hueOf('cnbc')).toBeGreaterThanOrEqual(0); expect(hueOf('cnbc')).toBeLessThan(360);
    expect(hostOf('https://www.cnbc.com/a')).toBe('cnbc.com'); expect(hostOf('nope')).toBe('');
  });
  it('safeUrl only allows http(s)', () => {
    expect(safeUrl('https://example.com/a')).toBe('https://example.com/a');
    expect(safeUrl('javascript:alert(1)')).toBeUndefined(); expect(safeUrl('data:text/html,x')).toBeUndefined(); expect(safeUrl('')).toBeUndefined(); expect(safeUrl(undefined)).toBeUndefined();
  });
  it('slugify matches the server rule', () => {
    expect(slugify('  CNBC Top Stories! ')).toBe('cnbc-top-stories');
    expect(validSlug('cnbc-top')).toBe(true); expect(validSlug('a')).toBe(false); expect(validSlug('-bad')).toBe(false); expect(validSlug('Bad')).toBe(false);
  });
});

describe('backoff', () => {
  it('grows, caps and jitters', () => {
    expect(backoffMs(0, () => 0)).toBe(500); expect(backoffMs(1, () => 1)).toBe(2000); expect(backoffMs(20, () => 1)).toBe(30_000);
    for (let i = 0; i < 20; i++) { const v = backoffMs(3); expect(v).toBeGreaterThanOrEqual(4000); expect(v).toBeLessThanOrEqual(8000); }
  });
  it('RecentSet dedupes and forgets the oldest', () => {
    const s = new RecentSet(2);
    expect(s.add('a')).toBe(true); expect(s.add('a')).toBe(false); s.add('b'); s.add('c');
    expect(s.has('a')).toBe(false); expect(s.has('c')).toBe(true);
  });
});

describe('conditions', () => {
  it('parses values by field kind and operator', () => {
    expect(parseValue('2.5', 'number', '>=')).toBe(2.5); expect(parseValue('x', 'number', '>=')).toBe('x'); expect(parseValue(' up ', 'string', '==')).toBe('up');
    expect(parseValue('true', 'bool', '==')).toBe(true); expect(parseValue('a, b ,', 'string', 'in')).toEqual(['a', 'b']); expect(parseValue('1,2', 'number', 'any_of')).toEqual([1, 2]);
    expect(LIST_OPS).toContain('in');
  });
  it('describes and validates', () => {
    expect(describeCondition({ field: 'abs_change_pct', op: '>=', value: 2 }, [{ name: 'abs_change_pct', kind: 'number' }])).toBe('abs change pct is at least 2');
    expect(valueToText(['a', 'b'])).toBe('a, b');
    expect(isComplete({ field: 'x', op: '==', value: 'y' })).toBe(true); expect(isComplete({ field: '', op: '==', value: 'y' })).toBe(false); expect(isComplete({ field: 'x', op: 'in', value: [] })).toBe(false);
  });
});

describe('feed cache', () => {
  it('prepends live items once and ignores duplicates', () => {
    const d = data(['b', 'a']);
    const out = prependItem(d, fi('c'))!;
    expect(out.pages[0]!.items.map((i) => i.id)).toEqual(['c', 'b', 'a']);
    expect(prependItem(out, fi('c'))).toBe(out);
    expect(prependItem(undefined, fi('x'))).toBeUndefined();
  });
  it('merges several, keeping order, skipping known ones', () => {
    const out = mergeItems(data(['b']), [fi('d'), fi('c'), fi('b')])!;
    expect(out.pages[0]!.items.map((i) => i.id)).toEqual(['d', 'c', 'b']);
  });
  it('patches one item across pages without touching others', () => {
    const d = data(['b'], ['a']);
    const out = patchItem(d, 'a', { feedback: 'useful' })!;
    expect(out.pages[1]!.items[0]!.feedback).toBe('useful'); expect(out.pages[0]!.items[0]!.feedback).toBeUndefined();
    expect(d.pages[1]!.items[0]!.feedback).toBeUndefined(); // immutable
  });
});

describe('audit helpers', () => {
  it('labels actions readably', () => {
    expect(auditLabel('source.create')).toBe('Source created'); expect(auditLabel('user.reset_password')).toBe('User password reset'); expect(auditLabel('weird.thing_happened')).toBe('Weird thing happened');
  });
  it('CSV quotes properly and neutralises spreadsheet formulas', () => {
    expect(toCsv([['a', 'b,c'], ['say "hi"', 'x\ny']])).toBe('a,"b,c"\r\n"say ""hi""","x\ny"');
    expect(toCsv([['=HYPERLINK("http://evil")', '+1', '-2', '@x', 'ok']])).toBe(`"'=HYPERLINK(""http://evil"")",'+1,'-2,'@x,ok`);
  });
});

describe('sources and subscriptions', () => {
  const src = { id: 's', name: 'n', type: 'rss', enabled: false, interval_seconds: 300, entities: ['a'], url: 'https://x.test/f', health: 'ok', consecutive_failures: 0, items_total: 0, created_at: '' } as Source;
  it('toUpdate resends everything PUT would otherwise erase', () => {
    expect(toUpdate(src, { enabled: true })).toEqual({ enabled: true, interval_seconds: 300, entities: ['a'], url: 'https://x.test/f' });
    expect(toUpdate({ ...src, type: 'webhook', url: undefined, webhook: { auth: 'hmac' }, interval_seconds: 0 })).toEqual({ enabled: false, interval_seconds: undefined, entities: ['a'], webhook: { auth: 'hmac' } });
  });
  it('mergeSubscriptions keeps advanced settings of entries that stay', () => {
    const existing = [{ kind: 'entity' as const, value: 'Oil', types: ['metric'], conditions: [{ field: 'v', op: '>' as const, value: 1 }] }];
    const out = mergeSubscriptions(existing, { source: ['cnbc'], entity: ['oil', 'rates'], keyword: [] });
    expect(out).toHaveLength(3);
    expect(out.find((s) => s.value === 'Oil')?.types).toEqual(['metric']);
    expect(out.find((s) => s.value === 'rates')).toEqual({ kind: 'entity', value: 'rates' });
  });
});
