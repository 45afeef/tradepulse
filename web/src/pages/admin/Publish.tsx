import { useState } from 'react';
import type { FormEvent } from 'react';
import { Link } from 'react-router-dom';
import { useMutation, useQuery } from '@tanstack/react-query';
import { api } from '../../api/endpoints';
import { errorMessage } from '../../api/client';
import { useDocumentTitle } from '../../lib/hooks';
import { Banner, Button, Card, ChipInput, Empty, Field, SkeletonCards, useToast } from '../../ui';

export function AdminPublish() {
  useDocumentTitle('Publish');
  const toast = useToast();
  const sources = useQuery({ queryKey: ['admin', 'sources'], queryFn: () => api.admin.sources() });
  const manual = (sources.data ?? []).filter((s) => s.type === 'manual' && s.enabled);
  const [source, setSource] = useState('');
  const [title, setTitle] = useState('');
  const [summary, setSummary] = useState('');
  const [url, setUrl] = useState('');
  const [topics, setTopics] = useState<string[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const chosen = source || manual[0]?.name || '';
  const post = useMutation({
    mutationFn: () => api.admin.postItem({ source: chosen, title: title.trim(), summary: summary.trim() || undefined, url: url.trim() || undefined, entities: topics.length ? topics : undefined }),
    onSuccess: () => { toast.show('Published. Members following it see it now.', 'success'); setTitle(''); setSummary(''); setUrl(''); setTopics([]); setErr(null); },
    onError: (e: unknown) => setErr(errorMessage(e)),
  });
  const submit = (e: FormEvent) => { e.preventDefault(); setErr(null); post.mutate(undefined); };
  return (
    <div className="page">
      <div className="page-head"><div><h1>Publish an announcement</h1><p>Post to your members through a manual source. It is matched and alerted like any other item.</p></div></div>
      {sources.isLoading && <SkeletonCards n={1} />}
      {sources.data && manual.length === 0 && <Empty icon="megaphone" title="You need a manual source first" action={<Link className="btn" to="/admin/sources">Add a manual source</Link>}>Create a source of type &ldquo;Manual posts&rdquo; and start it. Then you can publish here.</Empty>}
      {manual.length > 0 && (
        <Card>
          <form className="stack" onSubmit={submit}>
            {err && <Banner tone="danger" icon="alert">{err}</Banner>}
            {manual.length > 1 && <Field label="Source" htmlFor="pub-src"><select id="pub-src" className="select" value={chosen} onChange={(e) => setSource(e.target.value)}>{manual.map((s) => <option key={s.id} value={s.name}>{s.name}</option>)}</select></Field>}
            <Field label="Title" htmlFor="pub-title"><input id="pub-title" className="input" maxLength={300} value={title} onChange={(e) => setTitle(e.target.value)} /></Field>
            <Field label="Details (optional)" htmlFor="pub-sum"><textarea id="pub-sum" className="textarea" maxLength={1000} value={summary} onChange={(e) => setSummary(e.target.value)} /></Field>
            <Field label="Link (optional)" htmlFor="pub-url"><input id="pub-url" className="input" type="url" placeholder="https://" value={url} onChange={(e) => setUrl(e.target.value)} /></Field>
            <Field label="Extra topics (optional)" help={`Added to the source's own topics${manual.find((s) => s.name === chosen)?.entities.length ? ': ' + manual.find((s) => s.name === chosen)!.entities.join(', ') : ''}.`}><ChipInput label="Topics" value={topics} onChange={setTopics} placeholder="e.g. maintenance" max={10} /></Field>
            <div><Button type="submit" icon="megaphone" loading={post.isPending} disabled={!title.trim() || !chosen}>Publish</Button></div>
          </form>
        </Card>
      )}
    </div>
  );
}
