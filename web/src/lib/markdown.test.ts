import { describe, expect, it } from 'vitest';
import { outline, parseInline, parseMarkdown, plain, slugHeading } from './markdown';
import type { Inline } from './markdown';
import { searchTopics } from './helpSearch';

describe('markdown parser', () => {
  it('parses headings with stable anchors, paragraphs and lists', () => {
    const b = parseMarkdown('# Title\n\nSome **bold** and `code` text\nwrapped.\n\n## Why an alert gets paused\n\n- one\n- two\n  continued\n\n1. first\n2. second\n');
    expect(b[0]).toMatchObject({ t: 'h', level: 1, id: 'title' });
    expect(b[1]).toMatchObject({ t: 'p' });
    expect(plain((b[1] as unknown as { inline: Inline[] }).inline)).toBe('Some bold and code text wrapped.');
    expect(b[2]).toMatchObject({ t: 'h', level: 2, id: 'why-an-alert-gets-paused' });
    expect(b[3]).toMatchObject({ t: 'ul' });
    expect((b[3] as unknown as { items: unknown[][] }).items).toHaveLength(2);
    expect(plain((b[3] as unknown as { items: Inline[][] }).items[1]!)).toBe('two continued');
    expect(b[4]).toMatchObject({ t: 'ol' });
  });
  it('parses tables, callouts, rules and fenced code (verbatim)', () => {
    const b = parseMarkdown('> **Tip:** be kind\n\n| A | B |\n|---|---|\n| 1 | **2** |\n\n---\n\n```js\nconst a = "<b>x</b>"; // # not a heading\n```\n');
    expect(b[0]).toMatchObject({ t: 'quote' });
    expect(b[1]).toMatchObject({ t: 'table' });
    expect((b[1] as unknown as { rows: unknown[][] }).rows).toHaveLength(1);
    expect(b[2]).toMatchObject({ t: 'hr' });
    expect(b[3]).toMatchObject({ t: 'code', lang: 'js', v: 'const a = "<b>x</b>"; // # not a heading' });
  });
  it('keeps raw HTML as text and never creates elements for it', () => {
    const inl = parseInline('<script>alert(1)</script> and <img src=x onerror=alert(1)>');
    expect(inl.every((x) => x.t === 'text')).toBe(true);
    expect(plain(inl)).toContain('<script>alert(1)</script>');
  });
  it('keeps only safe link targets', () => {
    const l = (s: string) => parseInline(s)[0]!;
    expect(l('[ok](https://example.com/a)')).toMatchObject({ t: 'link', kind: 'external' });
    expect(l('[in](/alerts)')).toMatchObject({ t: 'link', kind: 'internal' });
    expect(l('[hash](#paused)')).toMatchObject({ t: 'link', kind: 'internal' });
    expect(l('[mail](mailto:help@example.com)')).toMatchObject({ t: 'link', kind: 'mail' });
    for (const bad of ['[x](javascript:alert(1))', '[x](data:text/html,hi)', '[x](//evil.example.com)', '[x](vbscript:x)', '[x](ftp://example.com)']) expect(l(bad)).toMatchObject({ t: 'text', v: 'x' });
  });
  it('supports explicit heading ids', () => {
    const b = parseMarkdown('## Views {#views}\n\ntext');
    expect(b[0]).toMatchObject({ t: 'h', id: 'views' });
    expect(plain((b[0] as unknown as { inline: Inline[] }).inline)).toBe('Views');
  });
  it('slugs headings and outlines a page', () => {
    expect(slugHeading('Why an alert gets paused!')).toBe('why-an-alert-gets-paused');
    const o = outline('# My page\n\nFirst paragraph here.\n\n## More\n\nOther text.');
    expect(o.title).toBe('My page'); expect(o.summary).toBe('First paragraph here.'); expect(o.text).toContain('Other text.');
  });
});

describe('help search', () => {
  const docs = [
    { slug: 'alerts', title: 'Alerts', summary: 'Get notified about what matters.', text: 'A cooldown keeps an alert quiet. Paused rules can be resumed.' },
    { slug: 'feed', title: 'Your feed', summary: 'Everything you follow.', text: 'Views narrow the feed. Rate items.' },
  ];
  it('requires every word, ranks title hits first and returns an excerpt', () => {
    expect(searchTopics(docs, 'cooldown')[0]!.doc.slug).toBe('alerts');
    expect(searchTopics(docs, 'alerts cooldown')).toHaveLength(1);
    expect(searchTopics(docs, 'feed')[0]!.doc.slug).toBe('feed');
    expect(searchTopics(docs, 'nonexistent')).toHaveLength(0);
    expect(searchTopics(docs, ' ')).toHaveLength(0);
    expect(searchTopics(docs, 'cooldown')[0]!.excerpt).toContain('cooldown');
  });
});
