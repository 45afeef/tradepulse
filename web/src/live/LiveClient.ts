import type { AlertFire, FeedItem } from '../api/types';
import { RecentSet, backoffMs } from '../lib/backoff';

export type LiveStatus = 'connecting' | 'live' | 'offline';

export interface LiveHandlers {
  onItem(it: FeedItem, viaCatchUp: boolean): void;
  onAlert(a: AlertFire): void;
  onNotice(kind: 'alert_paused' | 'channel_disabled', data: Record<string, unknown>): void;
  onStatus(s: LiveStatus): void;
}

export interface SocketLike {
  onopen: (() => void) | null;
  onmessage: ((e: { data: string }) => void) | null;
  onclose: ((e: { code: number }) => void) | null;
  onerror: (() => void) | null;
  close(): void;
}

export interface LiveDeps {
  getTicket(): Promise<string>;
  wsUrl(ticket: string): string;
  /** Items after a cursor, OLDEST first. cursor '' means "the newest page" (also oldest first). */
  fetchAfter(cursor: string): Promise<{ items: FeedItem[]; next?: string }>;
  createSocket?: (url: string) => SocketLike;
  schedule?: (fn: () => void, ms: number) => unknown;
  cancel?: (h: unknown) => void;
  random?: () => number;
}

/**
 * Live connection following docs/WS.md: one-time ticket, back-off with jitter, catch-up with cursors after every
 * reconnect, de-duplication by id, no tight reconnect loop on policy closes (1008).
 */
export class LiveClient {
  private ws: SocketLike | null = null;
  private stopped = true;
  private attempt = 0;
  private timer: unknown = null;
  private lastCursor = '';
  private connectedOnce = false;
  private seen = new RecentSet();
  private status: LiveStatus = 'offline';

  constructor(private h: LiveHandlers, private d: LiveDeps) {}

  start() { if (!this.stopped) return; this.stopped = false; void this.connect(); }
  stop() {
    this.stopped = true;
    if (this.timer) (this.d.cancel ?? clearTimeout)(this.timer as never);
    this.timer = null;
    const ws = this.ws; this.ws = null;
    if (ws) { ws.onclose = null; ws.close(); }
    this.setStatus('offline');
  }
  /** Remember where the feed currently ends (first item's cursor) so a reconnect can fill the gap. */
  seed(cursor: string) { if (!this.lastCursor && cursor) this.lastCursor = cursor; }
  currentStatus() { return this.status; }

  private setStatus(s: LiveStatus) { if (s !== this.status) { this.status = s; this.h.onStatus(s); } }

  private async connect() {
    if (this.stopped) return;
    this.setStatus('connecting');
    let ticket: string;
    try { ticket = await this.d.getTicket(); } catch { this.retry(); return; }
    if (this.stopped) return;
    const ws = (this.d.createSocket ?? ((u: string) => new WebSocket(u) as unknown as SocketLike))(this.d.wsUrl(ticket));
    this.ws = ws;
    ws.onopen = () => {
      this.attempt = 0;
      this.setStatus('live');
      if (this.connectedOnce || this.lastCursor) void this.catchUp();
      this.connectedOnce = true;
    };
    ws.onmessage = (e) => this.onMessage(e.data);
    ws.onerror = () => { /* close follows */ };
    ws.onclose = (e) => {
      if (this.stopped) return;
      this.ws = null;
      this.setStatus('offline');
      this.retry(e.code === 1008 ? 60_000 : undefined);
    };
  }

  private retry(forceMs?: number) {
    if (this.stopped) return;
    this.attempt++;
    const ms = forceMs ?? backoffMs(this.attempt, this.d.random);
    this.timer = (this.d.schedule ?? ((fn, t) => setTimeout(fn, t)))(() => { this.timer = null; void this.connect(); }, ms);
  }

  private onMessage(raw: string) {
    let m: { type?: string; data?: unknown };
    try { m = JSON.parse(raw); } catch { return; }
    switch (m.type) {
      case 'feed_item': this.deliver(m.data as FeedItem, false); break;
      case 'alert': this.h.onAlert(m.data as AlertFire); break;
      case 'alert_paused': case 'channel_disabled': this.h.onNotice(m.type, (m.data ?? {}) as Record<string, unknown>); break;
      default: break; // "connected" and unknown kinds are ignored on purpose (forward compatible)
    }
  }

  private deliver(it: FeedItem, viaCatchUp: boolean) {
    if (!it || !it.id || !this.seen.add(it.id)) return;
    if (it.cursor) this.lastCursor = it.cursor;
    this.h.onItem(it, viaCatchUp);
  }

  private async catchUp() {
    let cursor = this.lastCursor;
    for (let guard = 0; guard < 20 && !this.stopped; guard++) {
      let page: { items: FeedItem[]; next?: string };
      try { page = await this.d.fetchAfter(cursor); } catch { return; }
      for (const it of page.items) this.deliver(it, true);
      if (!page.next || page.items.length === 0) return;
      cursor = page.next;
    }
  }
}

export function buildWsUrl(ticket: string, base: string | undefined, loc: { protocol: string; host: string }): string {
  if (base) return `${base.replace(/^http/, 'ws').replace(/\/$/, '')}/ws?ticket=${encodeURIComponent(ticket)}`;
  return `${loc.protocol === 'https:' ? 'wss' : 'ws'}://${loc.host}/ws?ticket=${encodeURIComponent(ticket)}`;
}
