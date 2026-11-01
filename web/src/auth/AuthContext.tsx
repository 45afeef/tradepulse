import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import type { ReactNode } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { api } from '../api/endpoints';
import { http } from '../api/client';
import type { TokenPair, User } from '../api/types';

type Status = 'loading' | 'anon' | 'authed';
interface AuthApi {
  status: Status;
  user: User | null;
  isAdmin: boolean;
  login(email: string, password: string): Promise<User>;
  register(email: string, password: string, inviteCode: string): Promise<User>;
  logout(everywhere?: boolean): Promise<void>;
  changePassword(current: string, next: string): Promise<void>;
}
const Ctx = createContext<AuthApi | null>(null);
export function useAuth(): AuthApi {
  const v = useContext(Ctx);
  if (!v) throw new Error('useAuth must be used inside <AuthProvider>');
  return v;
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const qc = useQueryClient();
  const [status, setStatus] = useState<Status>(http.hasSession() ? 'loading' : 'anon');
  const [user, setUser] = useState<User | null>(null);

  const end = useCallback(() => { http.clearSession(); setUser(null); setStatus('anon'); qc.clear(); }, [qc]);
  const begin = useCallback((p: TokenPair) => { http.setSession(p); setUser(p.user); setStatus('authed'); return p.user; }, []);

  useEffect(() => {
    http.setLogoutHandler(end);
    if (!http.hasSession()) return;
    let alive = true;
    api.auth.me().then((u) => { if (alive) { setUser(u); setStatus('authed'); } }).catch(() => { if (alive) end(); });
    return () => { alive = false; };
  }, [end]);

  const value = useMemo<AuthApi>(() => ({
    status, user, isAdmin: user?.role === 'admin',
    login: async (email, password) => begin(await api.auth.login(email.trim(), password)),
    register: async (email, password, code) => begin(await api.auth.register(email.trim(), password, code.trim())),
    logout: async (everywhere) => {
      try {
        if (everywhere) await api.auth.logoutAll();
        else { const rt = localStorage.getItem('tp.refresh'); if (rt) await api.auth.logout(rt); }
      } catch { /* leaving anyway */ }
      end();
    },
    changePassword: async (cur, next) => { begin(await api.auth.changePassword(cur, next)); },
  }), [status, user, begin, end]);

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}
