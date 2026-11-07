import { describe, expect, it } from 'vitest';
import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { parseMarkdown, parseInline, plain } from '../lib/markdown';
import type { Block, Inline } from '../lib/markdown';

// Guards the help pages like code: every page is registered, every link goes somewhere real, nothing is left unfinished.
const src = join(process.cwd(), 'src');
const files = readdirSync(join(src, 'help')).filter((f) => f.endsWith('.md'));
const registry = readFileSync(join(src, 'help', 'index.ts'), 'utf8');
const slugs = [...registry.matchAll(/slug: '([^']+)'/g)].map((m) => m[1]!);
const appRoutes = [...readFileSync(join(src, 'App.tsx'), 'utf8').matchAll(/path="([^"*]+)"/g)].map((m) => m[1]!);
const read = (f: string) => readFileSync(join(src, 'help', f), 'utf8');
const idsOf = (md: string) => new Set(parseMarkdown(md).filter((b): b is Extract<Block, { t: 'h' }> => b.t === 'h').map((h) => h.id));

function linksOf(md: string): { text: string; href: string; kind: string }[] {
  const out: { text: string; href: string; kind: string }[] = [];
  const walk = (inl: Inline[]) => inl.forEach((x) => { if (x.t === 'link') out.push({ text: x.v, href: x.href, kind: x.kind }); });
  for (const b of parseMarkdown(md)) {
    if (b.t === 'p' || b.t === 'quote' || b.t === 'h') walk(b.inline);
    if (b.t === 'ul' || b.t === 'ol') b.items.forEach(walk);
    if (b.t === 'table') [b.head, ...b.rows].forEach((r) => r.forEach(walk));
  }
  return out;
}

describe('help pages', () => {
  it('every .md file is registered exactly once and every registered slug has a file', () => {
    for (const f of files) expect(registry).toContain(`./${f}?raw`);
    expect(new Set(slugs).size).toBe(slugs.length);
    expect(slugs.length).toBe(files.length);
    for (const s of slugs) expect(files).toContain(s + '.md');
  });

  it('each page has a title, an intro paragraph and no unfinished markers', () => {
    const titles = new Set<string>();
    for (const f of files) {
      const md = read(f);
      const blocks = parseMarkdown(md);
      const h1 = blocks.find((b) => b.t === 'h' && b.level === 1);
      expect(h1 ? plain((h1 as Extract<Block, { t: 'h' }>).inline) : '').not.toBe('');
      expect(blocks.filter((b) => b.t === 'h' && b.level === 1)).toHaveLength(1);
      expect(blocks.some((b) => b.t === 'p')).toBe(true);
      expect(/\b(TODO|TBD|FIXME|lorem ipsum)\b/i.test(md)).toBe(false);
      const t = plain((h1 as Extract<Block, { t: 'h' }>).inline);
      expect(titles.has(t)).toBe(false);
      titles.add(t);
    }
  });

  it('every internal link points at a real page, route or heading', () => {
    for (const f of files) {
      const own = idsOf(read(f));
      for (const l of linksOf(read(f))) {
        if (l.kind === 'external') { expect(l.href.startsWith('https://')).toBe(true); continue; }
        if (l.kind === 'mail') continue;
        const [path, hash] = l.href.split('#');
        if (!path) { expect(own.has(hash!)).toBe(true); continue; }
        if (path.startsWith('/help/')) {
          const slug = path.slice('/help/'.length);
          expect(slugs).toContain(slug);
          if (hash) expect(idsOf(read(slug + '.md')).has(hash)).toBe(true);
        } else {
          expect(appRoutes).toContain(path);
        }
      }
    }
  });

  it('in-app links to help anchors from the code resolve too', () => {
    const code = ['pages/Alerts.tsx', 'pages/Delivery.tsx', 'pages/Welcome.tsx', 'components/SourceWizard.tsx', 'pages/admin/Invites.tsx', 'pages/Settings.tsx'].map((f) => readFileSync(join(src, f), 'utf8')).join('\n');
    for (const m of code.matchAll(/to="\/help\/([a-z-]+)(?:#([a-z0-9-]+))?"/g)) {
      expect(slugs).toContain(m[1]!);
      if (m[2]) expect(idsOf(read(m[1] + '.md')).has(m[2])).toBe(true);
    }
  });

  it('operator pages are marked for operators only and member pages never link to them as required reading', () => {
    expect(registry).toContain("slug: 'operator-handbook', audience: 'operator'");
    expect(registry).toContain("slug: 'sources-guide', audience: 'operator'");
    for (const f of files.filter((x) => !/operator|sources-guide/.test(x))) for (const l of linksOf(read(f))) expect(l.href).not.toMatch(/operator-handbook|sources-guide/);
  });

  it('parseInline is the only path to links (no raw HTML survives)', () => {
    for (const f of files) expect(parseInline(read(f).split('\n').filter((x) => /<[a-z]/i.test(x) && !x.startsWith('```')).join(' ')).every((x) => x.t !== 'link' || x.kind !== 'external' || x.href.startsWith('https://'))).toBe(true);
  });
});
