import { describe, expect, it, vi } from 'vitest';
import { ApiError, Http, errorMessage } from './client';
import type { TokenPair } from './types';

const user = { id: 'u1', email: 'a@example.com', role: 'member' as const, created_at: '2026-01-01T00:00:00Z' };
const pair = (token: string, refresh: string, expires = 900): TokenPair => ({ token, refresh_token: refresh, expires_in: expires, user });
const json = (status: number, body?: unknown, headers: Record<string, string> = {}) => new Response(body === undefined ? null : JSON.stringify(body), { status, headers });
const memStore = (initial: string | null = null) => { let rt = initial; return { getRefresh: () => rt, setRefresh: (v: string | null) => { rt = v; } }; };
const authOf = (call: unknown[]) => ((call[1] as { headers: Record<string, string> }).headers.Authorization);

describe('Http', () => {
  it('sends the bearer token and stores the refresh token from a session', async () => {
    const fetchFn = vi.fn(async () => json(200, { ok: true }));
    const store = memStore();
    const http = new Http('', store, () => 0, fetchFn as unknown as typeof fetch);
    http.setSession(pair('a1', 'r1'));
    await http.request('GET', '/api/v1/me');
    expect(authOf(fetchFn.mock.calls[0] as unknown[])).toBe('Bearer a1');
    expect(store.getRefresh()).toBe('r1');
  });

  it('shares ONE refresh between concurrent requests (refresh tokens are single use)', async () => {
    let refreshes = 0;
    const fetchFn = vi.fn(async (url: string) => {
      if (String(url).includes('/auth/refresh')) { refreshes++; return json(200, pair('a2', 'r2')); }
      return json(200, { ok: true });
    });
    const store = memStore();
    const http = new Http('', store, () => 0, fetchFn as unknown as typeof fetch);
    http.setSession(pair('a1', 'r1', 5)); // expires in 5s: inside the 20s safety margin
    await Promise.all([http.request('GET', '/api/v1/me'), http.request('GET', '/api/v1/me/feed'), http.request('GET', '/api/v1/me/alerts')]);
    expect(refreshes).toBe(1);
    expect(store.getRefresh()).toBe('r2');
    const apiCalls = fetchFn.mock.calls.filter((c: unknown[]) => !String(c[0]).includes('/auth/refresh'));
    expect(apiCalls.every((c: unknown[]) => authOf(c) === 'Bearer a2')).toBe(true);
  });

  it('refreshes after a 401 and retries the request once', async () => {
    let n = 0;
    const fetchFn = vi.fn(async (url: string) => {
      if (String(url).includes('/auth/refresh')) return json(200, pair('a2', 'r2'));
      return n++ === 0 ? json(401, { title: 'Unauthorized', status: 401, detail: 'expired' }) : json(200, { ok: true });
    });
    const http = new Http('', memStore(), () => 0, fetchFn as unknown as typeof fetch);
    http.setSession(pair('a1', 'r1'));
    await expect(http.request('GET', '/api/v1/me')).resolves.toEqual({ ok: true });
    expect(fetchFn).toHaveBeenCalledTimes(3);
  });

  it('ends the session when the refresh token is rejected', async () => {
    const fetchFn = vi.fn(async (url: string) => (String(url).includes('/auth/refresh') ? json(401, { title: 'Unauthorized', status: 401, detail: 'no' }) : json(401, { title: 'Unauthorized', status: 401, detail: 'expired' })));
    const store = memStore();
    const http = new Http('', store, () => 0, fetchFn as unknown as typeof fetch);
    const out = vi.fn();
    http.setLogoutHandler(out);
    http.setSession(pair('a1', 'r1'));
    await expect(http.request('GET', '/api/v1/me')).rejects.toMatchObject({ status: 401 });
    expect(out).toHaveBeenCalled();
    expect(store.getRefresh()).toBeNull();
  });

  it('does not log out when the network is down during a refresh', async () => {
    const fetchFn = vi.fn(async () => { throw new TypeError('offline'); });
    const store = memStore('r1');
    const http = new Http('', store, () => 0, fetchFn as unknown as typeof fetch);
    const out = vi.fn();
    http.setLogoutHandler(out);
    await expect(http.refresh()).resolves.toBe(false);
    expect(out).not.toHaveBeenCalled();
    expect(store.getRefresh()).toBe('r1');
  });

  it('turns problem+json into an ApiError with the request id and field errors', async () => {
    const fetchFn = vi.fn(async () => json(422, { title: 'Unprocessable Entity', status: 422, detail: 'validation failed', errors: [{ message: 'name is required' }] }, { 'X-Request-ID': 'req-7' }));
    const http = new Http('', memStore(), () => 0, fetchFn as unknown as typeof fetch);
    const e = await http.request('POST', '/api/v1/x', { body: {}, auth: false }).catch((x: unknown) => x);
    expect(e).toBeInstanceOf(ApiError);
    expect(e).toMatchObject({ status: 422, detail: 'validation failed', requestId: 'req-7' });
    expect(errorMessage(e)).toContain('name is required');
  });

  it('maps network failures to status 0 and never leaks internals for 5xx', async () => {
    const down = new Http('', memStore(), () => 0, (async () => { throw new TypeError('x'); }) as unknown as typeof fetch);
    const e = await down.request('GET', '/api/v1/me', { auth: false }).catch((x: unknown) => x);
    expect(e).toMatchObject({ status: 0 });
    expect(errorMessage(e)).toMatch(/cannot reach/i);
    expect(errorMessage(new ApiError(500, 'Internal', 'stack trace here', 'rid-1'))).not.toContain('stack trace');
    expect(errorMessage(new ApiError(429, 'Too Many', 'x'))).toMatch(/too many/i);
  });

  it('returns undefined for 204 and skips auth when auth is false', async () => {
    const fetchFn = vi.fn(async () => json(204));
    const http = new Http('', memStore('r1'), () => 0, fetchFn as unknown as typeof fetch);
    await expect(http.request('POST', '/api/v1/auth/login', { body: {}, auth: false })).resolves.toBeUndefined();
    expect(fetchFn).toHaveBeenCalledTimes(1); // no refresh attempted for unauthenticated calls
    expect(((fetchFn.mock.calls[0] as unknown[])[1] as { headers: Record<string, string> }).headers.Authorization).toBeUndefined();
  });
});
