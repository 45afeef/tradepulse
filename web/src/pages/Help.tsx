import { useEffect, useMemo, useState } from 'react';
import { Link, useLocation, useParams } from 'react-router-dom';
import { useAuth } from '../auth/AuthContext';
import { Markdown } from '../components/Markdown';
import { TOPICS, topicBySlug } from '../help';
import type { Topic } from '../help';
import { searchTopics } from '../lib/helpSearch';
import { useDocumentTitle } from '../lib/hooks';
import { Card, Empty, Icon } from '../ui';

function Contact() {
  const email = import.meta.env.VITE_SUPPORT_EMAIL as string | undefined;
  return (
    <Card className="row"><Icon name="mail" /><div className="grow"><b>Still stuck?</b>
      <p className="muted small">{email ? <>Write to <a href={`mailto:${email}`}>{email}</a>.</> : 'Ask the person who invited you or runs this workspace.'} If you saw an error, include its reference id.</p></div></Card>
  );
}

function TopicCard({ t }: { t: Topic }) {
  return (
    <Link to={`/help/${t.slug}`} style={{ textDecoration: 'none', color: 'inherit' }}>
      <Card className="interactive row top"><span className="avatar" style={{ borderRadius: 12 }}><Icon name={t.icon} size={18} /></span><div className="grow"><h3>{t.title}</h3><p className="muted small">{t.summary}</p></div></Card>
    </Link>
  );
}

function HelpIndex({ isAdmin }: { isAdmin: boolean }) {
  const [q, setQ] = useState('');
  const visible = useMemo(() => TOPICS.filter((t) => isAdmin || t.audience === 'member'), [isAdmin]);
  const results = useMemo(() => searchTopics(visible, q), [visible, q]);
  const members = visible.filter((t) => t.audience === 'member');
  const ops = visible.filter((t) => t.audience === 'operator');
  return (
    <div className="stack lg">
      <div className="page-head"><div><h1>Help</h1><p>Guides for getting the most out of TradePulse.</p></div></div>
      <div className="field"><label htmlFor="help-q" className="sr-only">Search help</label><div className="row"><Icon name="search" /><input id="help-q" className="input" type="search" placeholder="Search help, for example “paused” or “webhook”" value={q} onChange={(e) => setQ(e.target.value)} /></div></div>
      {q.trim() ? (
        results.length === 0
          ? <Empty icon="search" title="No matches">Try a different word, or browse the topics.</Empty>
          : <div className="stack">{results.map((r) => <Link key={r.doc.slug} to={`/help/${r.doc.slug}`} style={{ textDecoration: 'none', color: 'inherit' }}><Card className="interactive"><h3>{r.doc.title}</h3><p className="muted small">{r.excerpt}</p></Card></Link>)}</div>
      ) : (
        <>
          <div className="stack"><h2>For members</h2><div className="grid cols-2">{members.map((t) => <TopicCard key={t.slug} t={t} />)}</div></div>
          {isAdmin && ops.length > 0 && <div className="stack"><h2>For operators</h2><div className="grid cols-2">{ops.map((t) => <TopicCard key={t.slug} t={t} />)}</div></div>}
        </>
      )}
      <Contact />
    </div>
  );
}

function HelpTopic({ topic, isAdmin }: { topic: Topic; isAdmin: boolean }) {
  useDocumentTitle(topic.title);
  const { hash } = useLocation();
  useEffect(() => {
    if (hash) document.getElementById(decodeURIComponent(hash.slice(1)))?.scrollIntoView({ block: 'start' });
    else window.scrollTo({ top: 0 });
  }, [hash, topic.slug]);
  const same = TOPICS.filter((t) => t.audience === topic.audience);
  const i = same.findIndex((t) => t.slug === topic.slug);
  const prev = same[i - 1], next = same[i + 1];
  if (topic.audience === 'operator' && !isAdmin) return <Empty icon="shield" title="This page is for operators" action={<Link className="btn" to="/help">Back to help</Link>}>Your operator can answer questions about how this workspace is run.</Empty>;
  return (
    <div className="stack lg">
      <nav aria-label="Breadcrumb" className="small"><Link to="/help">Help</Link> <span className="faint">/</span> {topic.audience === 'operator' ? 'Operators' : 'Members'}</nav>
      <article className="stack">
        <h1>{topic.title}</h1>
        <Markdown source={topic.md} skipTitle />
      </article>
      <div className="row between wrap">
        {prev ? <Link className="btn secondary sm" to={`/help/${prev.slug}`}>← {prev.title}</Link> : <span />}
        {next ? <Link className="btn secondary sm" to={`/help/${next.slug}`}>{next.title} →</Link> : <span />}
      </div>
      <Contact />
    </div>
  );
}

export function HelpPage() {
  useDocumentTitle('Help');
  const { slug } = useParams();
  const { isAdmin } = useAuth();
  const topic = topicBySlug(slug);
  return (
    <div className="page">
      {slug && !topic ? <Empty icon="search" title="That page does not exist" action={<Link className="btn" to="/help">Back to help</Link>}>It may have moved. Search the help instead.</Empty>
        : topic ? <HelpTopic topic={topic} isAdmin={isAdmin} /> : <HelpIndex isAdmin={isAdmin} />}
    </div>
  );
}
