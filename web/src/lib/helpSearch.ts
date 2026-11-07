export interface SearchDoc { slug: string; title: string; summary: string; text: string }

/** Rank topics for a query: every word must appear somewhere; title hits count most. Returns the best match first. */
export function searchTopics<T extends SearchDoc>(docs: T[], query: string): { doc: T; score: number; excerpt: string }[] {
  const words = query.toLowerCase().split(/\s+/).filter((w) => w.length > 1);
  if (words.length === 0) return [];
  const out: { doc: T; score: number; excerpt: string }[] = [];
  for (const doc of docs) {
    const title = doc.title.toLowerCase(), summary = doc.summary.toLowerCase(), text = doc.text.toLowerCase();
    let score = 0, ok = true;
    for (const w of words) {
      const s = (title.includes(w) ? 5 : 0) + (summary.includes(w) ? 2 : 0) + (text.includes(w) ? 1 : 0);
      if (s === 0) { ok = false; break; }
      score += s;
    }
    if (!ok) continue;
    const at = text.indexOf(words[0]!);
    const excerpt = at < 0 ? doc.summary : (at > 40 ? '…' : '') + doc.text.slice(Math.max(0, at - 40), at + 110).trim() + '…';
    out.push({ doc, score, excerpt });
  }
  return out.sort((a, b) => b.score - a.score || a.doc.title.localeCompare(b.doc.title));
}
