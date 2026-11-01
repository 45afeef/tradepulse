import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from 'react';
import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { api } from '../api/endpoints';
import type { AlertFire, FeedItem } from '../api/types';
import { useToast } from '../ui';
import { LiveClient, buildWsUrl } from './LiveClient';
import type { LiveStatus } from './LiveClient';

interface LiveApi {
  status: LiveStatus;
  /** Subscribe to live feed items; returns the unsubscribe function. */
  onItem(cb: (it: FeedItem, viaCatchUp: boolean) => void): () => void;
  seed(cursor: string): void;
}
const Ctx = createContext<LiveApi>({ status: 'offline', onItem: () => () => {}, seed: () => {} });
export const useLive = () => useContext(Ctx);

export function LiveProvider({ children }: { children: ReactNode }) {
  const toast = useToast();
  const [status, setStatus] = useState<LiveStatus>('connecting');
  const listeners = useRef(new Set<(it: FeedItem, via: boolean) => void>());
  const client = useRef<LiveClient | null>(null);

  useEffect(() => {
    const c = new LiveClient(
      {
        onItem: (it, via) => listeners.current.forEach((l) => l(it, via)),
        onAlert: (a: AlertFire) => toast.show(<span><b>{a.rule_name}</b>: {a.title} <Link to="/alerts?tab=history">View</Link></span>, 'alert', 9000),
        onNotice: (kind, data) => toast.show(kind === 'alert_paused'
          ? <span>Alert &ldquo;{String(data.name)}&rdquo; was paused: it fired too often. <Link to="/alerts">Review it</Link></span>
          : <span>Channel &ldquo;{String(data.name)}&rdquo; was disabled after repeated failures. <Link to="/delivery">Fix it</Link></span>, 'danger', 12000),
        onStatus: setStatus,
      },
      {
        getTicket: async () => (await api.live.ticket()).ticket,
        wsUrl: (t) => buildWsUrl(t, import.meta.env.VITE_API_BASE, location),
        fetchAfter: async (cursor) => {
          if (!cursor) { const r = await api.feed.list({ limit: 30 }); return { items: [...r.items].reverse() }; }
          const r = await api.feed.list({ after: cursor, limit: 100 });
          return { items: r.items, next: r.next_cursor };
        },
      },
    );
    client.current = c;
    c.start();
    return () => { c.stop(); client.current = null; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const onItem = useCallback((cb: (it: FeedItem, via: boolean) => void) => { listeners.current.add(cb); return () => { listeners.current.delete(cb); }; }, []);
  const seed = useCallback((c: string) => client.current?.seed(c), []);
  const value = useMemo(() => ({ status, onItem, seed }), [status, onItem, seed]);
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}
