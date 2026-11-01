import type { TokenPair } from './types';

export class ApiError extends Error {
  status: number;
  title: string;
  detail: string;
  requestId?: string;
  fields: { location?: string; message?: string }[];
  constructor(status: number, title: string, detail: string, requestId?: string, fields: { location?: string; message?: string }[] = []) {
    super(detail || title);
    this.name = 'ApiError';
    this.status = status;
    this.title = title;
    this.detail = detail;
    this.requestId = requestId;
    this.fields = fields;
  }
}

/** Human-friendly text for any error (never leaks internals; the server's detail is safe to show). */
export function errorMessage(e: unknown): string {
  if (e instanceof ApiError) {
    if (e.status === 0) return 'Cannot reach the server. Check your connection and try again.';
    if (e.status === 429) return 'Too many attempts. Please wait a moment and try again.';
    if (e.status >= 500) return 'Something went wrong on our side. Please try again.' + (e.requestId ? ` (ref ${e.requestId})` : '');
    const extra = e.fields.map((f) => f.message).filter(Boolean).join('; ');
    return extra && !e.detail.includes(extra) ? `${e.detail}${e.detail ? ': ' : ''}${extra}` : e.detail || e.title;
  }
  return e instanceof Error ? e.message : 'Unexpected error';
}

const REFRESH_KEY = 'tp.refresh';

export interface TokenStore {
  getRefresh(): string | null;
  setRefresh(v: string | null): void;
}
export const browserStore: TokenStore = {
  getRefresh: () => { try { return localStorage.getItem(REFRESH_KEY); } catch { return null; } },
  setRefresh: (v) => { try { if (v) localStorage.setItem(REFRESH_KEY, v); else localStorage.removeItem(REFRESH_KEY); } catch { /* storage blocked */ } },
};

type Query = Record<string, string | number | boolean | undefined | null>;
export interface RequestOptions { body?: unknown; query?: Query; auth?: boolean; signal?: AbortSignal }

/**
 * Small fetch wrapper: bearer token in memory, single-flight refresh (the refresh token is single use,
 * so concurrent refreshes would revoke the session), problem+json errors with request ids.
 */
export class Http {
  private access: string | null = null;
  private expiresAt = 0;
  private refreshing: Promise<boolean> | null = null;
  private onLogout: () => void = () => {};
  constructor(private base = '', private store: TokenStore = browserStore, private now: () => number = Date.now, private fetchFn: typeof fetch = (...a) => fetch(...a)) {}

  setLogoutHandler(fn: () => void) { this.onLogout = fn; }
  hasSession() { return !!this.store.getRefresh() || !!this.access; }
  accessToken() { return this.access; }

  setSession(p: TokenPair) {
    this.access = p.token;
    this.expiresAt = this.now() + p.expires_in * 1000;
    this.store.setRefresh(p.refresh_token);
  }
  clearSession() { this.access = null; this.expiresAt = 0; this.store.setRefresh(null); }

  /** Exchange the stored refresh token. Returns false when the session is over. Concurrent callers share one request. */
  refresh(): Promise<boolean> {
    if (this.refreshing) return this.refreshing;
    const rt = this.store.getRefresh();
    if (!rt) return Promise.resolve(false);
    this.refreshing = (async () => {
      try {
        const res = await this.fetchFn(this.base + '/api/v1/auth/refresh', {
          method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ refresh_token: rt }),
        });
        if (!res.ok) { this.clearSession(); this.onLogout(); return false; }
        this.setSession((await res.json()) as TokenPair);
        return true;
      } catch {
        return false; // network trouble is not a logout: keep the refresh token and try again later
      } finally {
        this.refreshing = null;
      }
    })();
    return this.refreshing;
  }

  async request<T>(method: string, path: string, opts: RequestOptions = {}, retried = false): Promise<T> {
    const auth = opts.auth !== false;
    if (auth && this.store.getRefresh() && (!this.access || this.expiresAt - this.now() < 20_000)) await this.refresh();
    const url = new URL(this.base + path, typeof location !== 'undefined' ? location.origin : 'http://localhost');
    for (const [k, v] of Object.entries(opts.query ?? {})) if (v !== undefined && v !== null && v !== '') url.searchParams.set(k, String(v));
    const headers: Record<string, string> = { Accept: 'application/json' };
    if (opts.body !== undefined) headers['Content-Type'] = 'application/json';
    if (auth && this.access) headers.Authorization = `Bearer ${this.access}`;
    let res: Response;
    try {
      res = await this.fetchFn(url.toString(), { method, headers, body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined, signal: opts.signal });
    } catch (e) {
      if ((e as Error)?.name === 'AbortError') throw e;
      throw new ApiError(0, 'Network error', 'Cannot reach the server');
    }
    if (res.status === 401 && auth && !retried && this.store.getRefresh()) {
      if (await this.refresh()) return this.request<T>(method, path, opts, true);
    }
    if (res.status === 401 && auth && !retried) this.onLogout();
    if (res.status === 204) return undefined as T;
    const requestId = res.headers.get('X-Request-ID') ?? undefined;
    const text = await res.text();
    let data: unknown = undefined;
    if (text) { try { data = JSON.parse(text); } catch { data = undefined; } }
    if (!res.ok) {
      const p = (data ?? {}) as { title?: string; detail?: string; errors?: { location?: string; message?: string }[] };
      throw new ApiError(res.status, p.title ?? res.statusText, p.detail ?? '', requestId, p.errors ?? []);
    }
    return data as T;
  }
}

export const http = new Http(import.meta.env.VITE_API_BASE ?? '');
