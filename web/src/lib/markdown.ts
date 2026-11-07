// A tiny, safe Markdown subset for the in-app help. It never produces HTML: the renderer builds React elements,
// raw HTML in the source stays visible text, and only http(s)/mailto/relative links are kept.
import { safeUrl } from './format';

export type Inline =
  | { t: 'text'; v: string }
  | { t: 'b' | 'i' | 'code'; v: string }
  | { t: 'link'; v: string; href: string; kind: 'internal' | 'external' | 'mail' };

export type Block =
  | { t: 'h'; level: 1 | 2 | 3; inline: Inline[]; id: string }
  | { t: 'p'; inline: Inline[] }
  | { t: 'ul' | 'ol'; items: Inline[][] }
  | { t: 'quote'; inline: Inline[] }
  | { t: 'code'; v: string; lang: string }
  | { t: 'table'; head: Inline[][]; rows: Inline[][][] }
  | { t: 'hr' };

export function slugHeading(s: string): string {
  return s.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '');
}

function classifyLink(href: string): 'internal' | 'external' | 'mail' | null {
  if (href.startsWith('#') || (href.startsWith('/') && !href.startsWith('//'))) return 'internal';
  if (href.startsWith('mailto:') && /^mailto:[^\s@]+@[^\s@]+$/.test(href)) return 'mail';
  return safeUrl(href) ? 'external' : null;
}

const TOKEN = /(`[^`\n]+`)|(\*\*[^*\n]+\*\*)|(\*[^*\n]+\*)|(\[[^\]\n]+\]\([^)\s]+\))/;

export function parseInline(src: string): Inline[] {
  const out: Inline[] = [];
  let rest = src;
  while (rest) {
    const m = TOKEN.exec(rest);
    if (!m) { out.push({ t: 'text', v: rest }); break; }
    if (m.index > 0) out.push({ t: 'text', v: rest.slice(0, m.index) });
    const tok = m[0];
    if (m[1]) out.push({ t: 'code', v: tok.slice(1, -1) });
    else if (m[2]) out.push({ t: 'b', v: tok.slice(2, -2) });
    else if (m[3]) out.push({ t: 'i', v: tok.slice(1, -1) });
    else {
      const lm = /^\[([^\]]+)\]\(([^)]+)\)$/.exec(tok)!;
      const kind = classifyLink(lm[2]!);
      out.push(kind ? { t: 'link', v: lm[1]!, href: lm[2]!, kind } : { t: 'text', v: lm[1]! }); // unsafe targets degrade to plain text
    }
    rest = rest.slice(m.index + tok.length);
  }
  return out;
}

const cells = (line: string) => line.trim().replace(/^\|/, '').replace(/\|$/, '').split('|').map((c) => c.trim());
const isSep = (line: string) => /^\s*\|?\s*:?-{3,}:?\s*(\|\s*:?-{3,}:?\s*)*\|?\s*$/.test(line) && line.includes('-');

export function parseMarkdown(src: string): Block[] {
  const lines = src.replace(/\r\n?/g, '\n').split('\n');
  const blocks: Block[] = [];
  let para: string[] = [];
  const flush = () => { if (para.length) { blocks.push({ t: 'p', inline: parseInline(para.join(' ')) }); para = []; } };
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]!;
    const fence = /^```(\w*)\s*$/.exec(line);
    if (fence) {
      flush();
      const body: string[] = [];
      for (i++; i < lines.length && !/^```\s*$/.test(lines[i]!); i++) body.push(lines[i]!);
      blocks.push({ t: 'code', v: body.join('\n'), lang: fence[1] ?? '' });
      continue;
    }
    if (!line.trim()) { flush(); continue; }
    const h = /^(#{1,3})\s+(.*\S)\s*$/.exec(line);
    if (h) { flush(); const raw = h[2]!; const custom = /\s*\{#([a-z0-9-]+)\}\s*$/.exec(raw); const text = custom ? raw.slice(0, custom.index) : raw; blocks.push({ t: 'h', level: h[1]!.length as 1 | 2 | 3, inline: parseInline(text), id: custom ? custom[1]! : slugHeading(text) }); continue; }
    if (/^-{3,}\s*$/.test(line)) { flush(); blocks.push({ t: 'hr' }); continue; }
    if (line.startsWith('>')) {
      flush();
      const q: string[] = [line.replace(/^>\s?/, '')];
      while (i + 1 < lines.length && lines[i + 1]!.startsWith('>')) q.push(lines[++i]!.replace(/^>\s?/, ''));
      blocks.push({ t: 'quote', inline: parseInline(q.join(' ')) });
      continue;
    }
    if (line.trim().startsWith('|') && i + 1 < lines.length && isSep(lines[i + 1]!)) {
      flush();
      const head = cells(line).map(parseInline);
      const rows: Inline[][][] = [];
      for (i += 2; i < lines.length && lines[i]!.trim().startsWith('|'); i++) rows.push(cells(lines[i]!).map(parseInline));
      i--;
      blocks.push({ t: 'table', head, rows });
      continue;
    }
    const li = /^(\s*)([-*]|\d+\.)\s+(.*)$/.exec(line);
    if (li) {
      flush();
      const ordered = /\d/.test(li[2]!);
      const items: string[] = [li[3]!];
      while (i + 1 < lines.length) {
        const nx = lines[i + 1]!;
        const nm = /^(\s*)([-*]|\d+\.)\s+(.*)$/.exec(nx);
        if (nm && /\d/.test(nm[2]!) === ordered) { items.push(nm[3]!); i++; }
        else if (/^\s{2,}\S/.test(nx) && !nm) { items[items.length - 1] += ' ' + nx.trim(); i++; }
        else break;
      }
      blocks.push({ t: ordered ? 'ol' : 'ul', items: items.map(parseInline) });
      continue;
    }
    para.push(line.trim());
  }
  flush();
  return blocks;
}

export const plain = (inl: Inline[]): string => inl.map((x) => x.v).join('');

/** Title (first h1) and the first paragraph, for topic cards. */
export function outline(src: string): { title: string; summary: string; text: string } {
  const blocks = parseMarkdown(src);
  const h1 = blocks.find((b) => b.t === 'h' && b.level === 1);
  const p = blocks.find((b) => b.t === 'p');
  const text = blocks.map((b) => (b.t === 'h' || b.t === 'p' || b.t === 'quote' ? plain(b.inline) : b.t === 'ul' || b.t === 'ol' ? b.items.map(plain).join(' ') : b.t === 'table' ? b.rows.map((r) => r.map(plain).join(' ')).join(' ') : b.t === 'code' ? '' : '')).join(' ');
  return { title: h1 && h1.t === 'h' ? plain(h1.inline) : '', summary: p && p.t === 'p' ? plain(p.inline) : '', text };
}
