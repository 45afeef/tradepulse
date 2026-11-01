import { useEffect, useMemo, useRef, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/endpoints';
import { errorMessage } from '../api/client';
import type { FeedItem, FeedPage, Rating, SavedView } from '../api/types';
import { FeedCard } from '../components/FeedCard';
import { ViewEditor } from '../components/ViewEditor';
import { useLive } from '../live/LiveProvider';
import { mergeItems, patchItem, prependItem } from '../lib/feedCache';
import type { InfiniteData } from '../lib/feedCache';
import { useDocumentTitle, useTick } from '../lib/hooks';
import { Banner, Button, Chip, Empty, SkeletonCards, Spinner, Tabs, useConfirm, useToast } from '../ui';

export function FeedPage() {
  useDocumentTitle('Feed');
  const [params, setParams] = useSearchParams();
  const view = params.get('view') ?? 'all';
  const qc = useQueryClient();
  const live = useLive();
  const toast = useToast();
  const confirm = useConfirm();
  const now = useTick();
  const [editor, setEditor] = useState<{ view?: SavedView } | null>(null);
  const [pending, setPending] = useState<FeedItem[]>([]);
  const [fresh, setFresh] = useState(0);

  const profile = useQuery({ queryKey: ['profile'], queryFn: () => api.subscriptions.get() });
  const views = useQuery({ queryKey: ['views'], queryFn: () => api.views.list() });
  const catalog = useQuery({ queryKey: ['catalog', 'sources'], queryFn: () => api.catalog.sources() });
  const feed = useInfiniteQuery({
    queryKey: ['feed', view],
    queryFn: ({ pageParam }: { pageParam: unknown }) => api.feed.list({ before: (pageParam as string) || undefined, view: view === 'all' ? undefined : view }),
    initialPageParam: '',
    getNextPageParam: (last: FeedPage) => last.next_cursor || undefined,
  });
  const items = useMemo(() => feed.data?.pages.flatMap((p: FeedPage) => p.items) ?? [], [feed.data]);
  const current = (views.data ?? []).find((v: SavedView) => v.id === view);

  // a view that no longer exists (deleted elsewhere) falls back to All
  useEffect(() => { if (view !== 'all' && views.data && !current) setParams({}); }, [view, views.data, current, setParams]);
  useEffect(() => { setPending([]); setFresh(0); }, [view]);
  useEffect(() => { const c = items[0]?.cursor; if (c && view === 'all') live.seed(c); }, [items, view, live]);

  // live items: merge straight in when the reader is at the top, otherwise offer a "new items" pill
  useEffect(() => live.onItem((it) => {
    if (view === 'all') {
      if (window.scrollY < 120) qc.setQueryData(['feed', 'all'], (d: InfiniteData<FeedPage> | undefined) => prependItem(d, it));
      else setPending((p) => (p.some((x) => x.id === it.id) ? p : [it, ...p]));
    } else setFresh((n) => n + 1);
  }), [live, view, qc]);

  const sentinel = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const el = sentinel.current;
    if (!el || !feed.hasNextPage || typeof IntersectionObserver === 'undefined') return;
    const io = new IntersectionObserver((es) => { if (es[0]?.isIntersecting && !feed.isFetchingNextPage) void feed.fetchNextPage(); }, { rootMargin: '700px' });
    io.observe(el);
    return () => io.disconnect();
  }, [feed.hasNextPage, feed.isFetchingNextPage, feed.fetchNextPage]);

  const rate = useMutation({
    mutationFn: (v: { id: string; rating: Rating | null }) => (v.rating ? api.feed.rate(v.id, v.rating) : api.feed.clearRating(v.id)),
    onMutate: (v: { id: string; rating: Rating | null }) => { qc.setQueriesData({ queryKey: ['feed'] }, (d: InfiniteData<FeedPage> | undefined) => patchItem(d, v.id, { feedback: v.rating ?? undefined })); },
    onError: (e: unknown) => { toast.show(errorMessage(e), 'danger'); void qc.invalidateQueries({ queryKey: ['feed'] }); },
  });

  const showNew = () => {
    if (view === 'all') { qc.setQueryData(['feed', 'all'], (d: InfiniteData<FeedPage> | undefined) => mergeItems(d, pending)); setPending([]); }
    else { void feed.refetch(); setFresh(0); }
    window.scrollTo({ top: 0, behavior: 'smooth' });
  };
  const newCount = pending.length + fresh;
  const hasSubs = (profile.data?.subscriptions ?? []).length > 0;
  const tabs = [{ id: 'all', label: 'All' }, ...(views.data ?? []).map((v: SavedView) => ({ id: v.id, label: v.name }))];

  const removeView = async () => {
    if (!current) return;
    if (!(await confirm({ title: `Delete “${current.name}”?`, message: 'The view is removed. Your feed items are not affected.', confirmLabel: 'Delete view', danger: true }))) return;
    try { await api.views.remove(current.id); void qc.invalidateQueries({ queryKey: ['views'] }); setParams({}); toast.show('View deleted'); } catch (e) { toast.show(errorMessage(e), 'danger'); }
  };

  return (
    <div className="page">
      <div className="page-head"><div><h1>Your feed</h1><p>Everything you follow, in one place, live.</p></div></div>
      <div className="row" style={{ marginBottom: '1rem' }}>
        <div className="grow" style={{ overflow: 'hidden' }}><Tabs label="Feed views" value={view} onChange={(id: string) => (id === 'all' ? setParams({}) : setParams({ view: id }))} items={tabs} /></div>
        <Button variant="secondary" size="sm" icon="plus" onClick={() => setEditor({})}>View</Button>
      </div>
      {current && (
        <div className="row wrap small" style={{ marginBottom: '.8rem' }}>
          {[...(current.filter.sources ?? []).map((s: string) => `source: ${s}`), ...(current.filter.entities ?? []).map((s: string) => `topic: ${s}`), ...(current.filter.keywords ?? []).map((s: string) => `“${s}”`), ...(current.filter.within_hours ? [`last ${current.filter.within_hours}h`] : [])].map((t: string) => <Chip key={t}>{t}</Chip>)}
          <span className="grow" />
          <Button variant="ghost" size="sm" icon="edit" onClick={() => setEditor({ view: current })}>Edit</Button>
          <Button variant="ghost" size="sm" icon="trash" onClick={removeView}>Delete</Button>
        </div>
      )}
      {newCount > 0 && <div className="new-pill"><Button size="sm" icon="refresh" onClick={showNew}>{newCount} new {newCount === 1 ? 'item' : 'items'}</Button></div>}
      {profile.data && !hasSubs && <Banner action={<Link className="btn sm" to="/welcome">Set up</Link>}><b>Nothing followed yet.</b> Pick sources and topics and get your first alert in about two minutes.</Banner>}
      <div className="stack" style={{ marginTop: '.8rem' }}>
        {feed.isLoading && <SkeletonCards n={4} />}
        {feed.isError && <Banner tone="danger" action={<Button size="sm" variant="secondary" onClick={() => void feed.refetch()}>Retry</Button>}>{errorMessage(feed.error)}</Banner>}
        {!feed.isLoading && !feed.isError && items.length === 0 && (
          view === 'all'
            ? <Empty icon="feed" title="Waiting for the first item" action={hasSubs ? undefined : <Link className="btn" to="/welcome">Choose what to follow</Link>}>New items appear here the moment they arrive. {(catalog.data ?? []).length > 0 ? `${(catalog.data ?? []).length} sources are being watched for you.` : ''}</Empty>
            : <Empty icon="filter" title="No items match this view yet">Items from your feed that match the filters will appear here. Try widening the time window.</Empty>
        )}
        {items.map((it) => <FeedCard key={it.id} item={it} now={now} onRate={(id, rating) => rate.mutate({ id, rating })} />)}
        <div ref={sentinel} />
        {feed.isFetchingNextPage && <div className="row" style={{ justifyContent: 'center' }}><Spinner /></div>}
        {!feed.hasNextPage && items.length > 10 && <p className="faint small" style={{ textAlign: 'center' }}>You are all caught up.</p>}
      </div>
      {editor && <ViewEditor view={editor.view} sources={(catalog.data ?? []).map((s) => s.name)} topics={[...new Set((catalog.data ?? []).flatMap((s) => s.entities))]} onClose={() => setEditor(null)} onSaved={(v) => { setEditor(null); setParams({ view: v.id }); }} />}
    </div>
  );
}
