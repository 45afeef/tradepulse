import { describe, expect, it, vi } from 'vitest';
import type { FeedItem } from '../api/types';
import { LiveClient, buildWsUrl } from './LiveClient';
import type { LiveStatus, SocketLike } from './LiveClient';

const item = (id: string, cursor = 'c1_' + id): FeedItem => ({ id, cursor, event_id: id, event_type: 't', source: 's', entities: [], title: id, body: '', severity: 'info', reason: 'r', created_at: '2026-01-01T00:00:00Z' });
const flush = () => new Promise((r) => setTimeout(r, 0));

class FakeSocket implements SocketLike {
  onopen: (() => void) | null = null; onmessage: ((e: { data: string }) => void) | null = null; onclose: ((e: { code: number }) => void) | null = null; onerror: (() => void) | null = null;
  closed = false;
  close() { this.closed = true; }
  open() { this.onopen?.(); }
  send(o: unknown) { this.onmessage?.({ data: typeof o === 'string' ? o : JSON.stringify(o) }); }
  drop(code = 1006) { this.onclose?.({ code }); }
}

function setup(over: { getTicket?: () => Promise<string>; fetchAfter?: (c: string) => Promise<{ items: FeedItem[]; next?: string }> } = {}) {
  const sockets: FakeSocket[] = [];
  const timers: { fn: () => void; ms: number; cancelled?: boolean }[] = [];
  const statuses: LiveStatus[] = [];
  const items: [FeedItem, boolean][] = [];
  const alerts: unknown[] = [];
  const notices: [string, unknown][] = [];
  const fetchAfter = vi.fn(over.fetchAfter ?? (async () => ({ items: [] })));
  const client = new LiveClient(
    { onItem: (i, via) => items.push([i, via]), onAlert: (a) => alerts.push(a), onNotice: (k, d) => notices.push([k, d]), onStatus: (s) => statuses.push(s) },
    {
      getTicket: over.getTicket ?? (async () => 'tkt'), wsUrl: (t) => `ws://x/ws?ticket=${t}`, fetchAfter,
      createSocket: () => { const s = new FakeSocket(); sockets.push(s); return s; },
      schedule: (fn, ms) => { const t = { fn, ms }; timers.push(t); return t; }, cancel: (h) => { (h as { cancelled?: boolean }).cancelled = true; }, random: () => 1,
    },
  );
  return { client, sockets, timers, statuses, items, alerts, notices, fetchAfter };
}

describe('LiveClient', () => {
  it('connects, reports status and de-duplicates live items by id', async () => {
    const t = setup();
    t.client.start(); await flush();
    expect(t.statuses).toEqual(['connecting']);
    t.sockets[0]!.open();
    expect(t.statuses).toEqual(['connecting', 'live']);
    t.sockets[0]!.send({ type: 'feed_item', data: item('a') });
    t.sockets[0]!.send({ type: 'feed_item', data: item('a') });
    t.sockets[0]!.send({ type: 'feed_item', data: item('b') });
    expect(t.items.map(([i]) => i.id)).toEqual(['a', 'b']);
    expect(t.items.every(([, via]) => via === false)).toBe(true);
  });

  it('routes alerts and notices and ignores unknown or malformed messages', async () => {
    const t = setup();
    t.client.start(); await flush(); t.sockets[0]!.open();
    t.sockets[0]!.send({ type: 'connected' });
    t.sockets[0]!.send({ type: 'something_new', data: {} });
    t.sockets[0]!.send('not json');
    t.sockets[0]!.send({ type: 'alert', data: { id: 'f1', title: 'T' } });
    t.sockets[0]!.send({ type: 'alert_paused', data: { name: 'r' } });
    t.sockets[0]!.send({ type: 'channel_disabled', data: { name: 'c' } });
    expect(t.alerts).toHaveLength(1);
    expect(t.notices.map(([k]) => k)).toEqual(['alert_paused', 'channel_disabled']);
    expect(t.items).toHaveLength(0);
  });

  it('reconnects with a fresh ticket and catches up after the last cursor', async () => {
    const t = setup({ fetchAfter: async (c) => (c === 'c1_a' ? { items: [item('b'), item('c')] } : { items: [] }) });
    t.client.start(); await flush(); t.sockets[0]!.open();
    expect(t.fetchAfter).not.toHaveBeenCalled(); // nothing to catch up on the very first connection
    t.sockets[0]!.send({ type: 'feed_item', data: item('a') });
    t.sockets[0]!.drop();
    expect(t.statuses[t.statuses.length - 1]).toBe('offline');
    expect(t.timers).toHaveLength(1);
    t.timers[0]!.fn(); await flush();
    expect(t.sockets).toHaveLength(2);
    t.sockets[1]!.open(); await flush();
    expect(t.fetchAfter).toHaveBeenCalledWith('c1_a');
    expect(t.items.map(([i, via]) => `${i.id}:${via}`)).toEqual(['a:false', 'b:true', 'c:true']);
  });

  it('does not deliver a catch-up item that was already delivered live', async () => {
    const t = setup({ fetchAfter: async () => ({ items: [item('a'), item('b')] }) });
    t.client.start(); await flush(); t.sockets[0]!.open();
    t.sockets[0]!.send({ type: 'feed_item', data: item('a') });
    t.sockets[0]!.drop(); t.timers[0]!.fn(); await flush(); t.sockets[1]!.open(); await flush();
    expect(t.items.map(([i]) => i.id)).toEqual(['a', 'b']);
  });

  it('follows catch-up pages until there is no next cursor', async () => {
    const pages: Record<string, { items: FeedItem[]; next?: string }> = { c1_s: { items: [item('x1')], next: 'c1_x1' }, c1_x1: { items: [item('x2')], next: 'c1_x2' }, c1_x2: { items: [item('x3')] } };
    const t = setup({ fetchAfter: async (c) => pages[c] ?? { items: [] } });
    t.client.seed('c1_s');
    t.client.start(); await flush(); t.sockets[0]!.open(); await flush();
    expect(t.items.map(([i]) => i.id)).toEqual(['x1', 'x2', 'x3']);
  });

  it('seed only sets the cursor once', async () => {
    const t = setup({ fetchAfter: async () => ({ items: [] }) });
    t.client.seed('c1_first'); t.client.seed('c1_second');
    t.client.start(); await flush(); t.sockets[0]!.open(); await flush();
    expect(t.fetchAfter).toHaveBeenCalledWith('c1_first');
  });

  it('backs off for a minute after a policy close (1008) instead of looping', async () => {
    const t = setup();
    t.client.start(); await flush(); t.sockets[0]!.open();
    t.sockets[0]!.drop(1008);
    expect(t.timers[0]!.ms).toBe(60_000);
  });

  it('retries when the ticket request fails and stops cleanly', async () => {
    let fail = true;
    const t = setup({ getTicket: async () => { if (fail) throw new Error('no'); return 'tkt'; } });
    t.client.start(); await flush();
    expect(t.sockets).toHaveLength(0);
    expect(t.timers).toHaveLength(1);
    fail = false; t.timers[0]!.fn(); await flush();
    expect(t.sockets).toHaveLength(1);
    t.client.stop();
    expect(t.sockets[0]!.closed).toBe(true);
    t.sockets[0]!.drop(); // late close events after stop() must not schedule anything
    expect(t.timers).toHaveLength(1);
    expect(t.client.currentStatus()).toBe('offline');
  });
});

describe('buildWsUrl', () => {
  it('uses the page origin by default (wss on https)', () => {
    expect(buildWsUrl('t 1', undefined, { protocol: 'https:', host: 'app.example.com' })).toBe('wss://app.example.com/ws?ticket=t%201');
    expect(buildWsUrl('t', undefined, { protocol: 'http:', host: 'localhost:5173' })).toBe('ws://localhost:5173/ws?ticket=t');
  });
  it('derives the socket from the API base when one is configured', () => {
    expect(buildWsUrl('t', 'https://api.example.com/', { protocol: 'https:', host: 'x' })).toBe('wss://api.example.com/ws?ticket=t');
  });
});
