/** Exponential back-off with jitter: ~1s, 2s, 4s ... capped at 30s. */
export function backoffMs(attempt: number, rnd: () => number = Math.random): number {
  const base = Math.min(30_000, 1000 * 2 ** Math.max(0, attempt));
  return Math.round(base * (0.5 + rnd() * 0.5));
}

/** A bounded set that forgets the oldest entries (de-duplicates live items without growing forever). */
export class RecentSet {
  private s = new Set<string>();
  constructor(private max = 2000) {}
  /** returns true when the id was NOT seen before */
  add(id: string): boolean {
    if (this.s.has(id)) return false;
    this.s.add(id);
    if (this.s.size > this.max) this.s.delete(this.s.values().next().value as string);
    return true;
  }
  has(id: string) { return this.s.has(id); }
}
