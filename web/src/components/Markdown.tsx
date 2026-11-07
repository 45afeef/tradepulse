import { useMemo } from 'react';
import { Link } from 'react-router-dom';
import { parseMarkdown } from '../lib/markdown';
import type { Block, Inline } from '../lib/markdown';
import { Icon } from '../ui';

function Inl({ parts }: { parts: Inline[] }) {
  return (
    <>
      {parts.map((p, i) => {
        switch (p.t) {
          case 'b': return <strong key={i}>{p.v}</strong>;
          case 'i': return <em key={i}>{p.v}</em>;
          case 'code': return <code key={i}>{p.v}</code>;
          case 'link':
            if (p.kind === 'internal') return p.href.startsWith('#') ? <a key={i} href={p.href}>{p.v}</a> : <Link key={i} to={p.href}>{p.v}</Link>;
            if (p.kind === 'mail') return <a key={i} href={p.href}>{p.v}</a>;
            return <a key={i} href={p.href} target="_blank" rel="noopener noreferrer">{p.v}</a>;
          default: return <span key={i}>{p.v}</span>;
        }
      })}
    </>
  );
}

function Blk({ b, skipTitle }: { b: Block; skipTitle?: boolean }) {
  switch (b.t) {
    case 'h':
      if (b.level === 1) return skipTitle ? null : <h1 id={b.id}><Inl parts={b.inline} /></h1>;
      return b.level === 2 ? <h2 id={b.id}><Inl parts={b.inline} /></h2> : <h3 id={b.id}><Inl parts={b.inline} /></h3>;
    case 'p': return <p><Inl parts={b.inline} /></p>;
    case 'ul': return <ul>{b.items.map((it, i) => <li key={i}><Inl parts={it} /></li>)}</ul>;
    case 'ol': return <ol>{b.items.map((it, i) => <li key={i}><Inl parts={it} /></li>)}</ol>;
    case 'quote': return <div className="banner" role="note"><Icon name="info" size={18} /><div className="grow"><Inl parts={b.inline} /></div></div>;
    case 'code': return <pre className="code" tabIndex={0}><code>{b.v}</code></pre>;
    case 'table': return (
      <div className="table-wrap"><table className="table"><thead><tr>{b.head.map((c, i) => <th key={i}><Inl parts={c} /></th>)}</tr></thead>
        <tbody>{b.rows.map((r, i) => <tr key={i}>{r.map((c, j) => <td key={j}><Inl parts={c} /></td>)}</tr>)}</tbody></table></div>
    );
    case 'hr': return <hr className="divider" />;
  }
}

/** Renders the safe Markdown subset as React elements (no HTML is ever injected). */
export function Markdown({ source, skipTitle }: { source: string; skipTitle?: boolean }) {
  const blocks = useMemo(() => parseMarkdown(source), [source]);
  return <div className="prose">{blocks.map((b, i) => <Blk key={i} b={b} skipTitle={skipTitle} />)}</div>;
}
