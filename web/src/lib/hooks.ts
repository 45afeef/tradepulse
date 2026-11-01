import { useEffect, useState } from 'react';

/** Re-render every `ms` (keeps "5 min ago" honest without a timer per card). */
export function useTick(ms = 30_000): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => { const t = setInterval(() => setNow(Date.now()), ms); return () => clearInterval(t); }, [ms]);
  return now;
}

export function useDocumentTitle(title: string) {
  useEffect(() => { const prev = document.title; document.title = `${title} · TradePulse`; return () => { document.title = prev; }; }, [title]);
}
