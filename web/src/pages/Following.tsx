import { useEffect, useMemo, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/endpoints';
import { errorMessage } from '../api/client';
import type { Profile, ScopeKind } from '../api/types';
import { Banner, Button, Card, ChipInput, Skeleton, useToast } from '../ui';
import { useDocumentTitle } from '../lib/hooks';
import { SECTIONS, mergeSubscriptions } from '../lib/subscriptions';



export function FollowingPage() {
  useDocumentTitle('Following');
  const qc = useQueryClient();
  const toast = useToast();
  const profile = useQuery({ queryKey: ['profile'], queryFn: () => api.subscriptions.get() });
  const catalog = useQuery({ queryKey: ['catalog', 'sources'], queryFn: () => api.catalog.sources() });
  const [draft, setDraft] = useState<Record<ScopeKind, string[]>>({ source: [], entity: [], keyword: [] });
  const [muted, setMuted] = useState<string[]>([]);

  useEffect(() => {
    if (!profile.data) return;
    const subs = profile.data.subscriptions ?? [];
    setDraft({ source: subs.filter((s) => s.kind === 'source').map((s) => s.value), entity: subs.filter((s) => s.kind === 'entity').map((s) => s.value), keyword: subs.filter((s) => s.kind === 'keyword').map((s) => s.value) });
    setMuted(profile.data.muted ?? []);
  }, [profile.data]);

  const original = useMemo(() => JSON.stringify([...(profile.data?.subscriptions ?? []).map((s) => `${s.kind}:${s.value.toLowerCase()}`).sort(), [...(profile.data?.muted ?? [])].sort()]), [profile.data]);
  const current = JSON.stringify([...SECTIONS.flatMap((s) => draft[s.kind].map((v) => `${s.kind}:${v.toLowerCase()}`)).sort(), [...muted].sort()]);
  const dirty = !!profile.data && original !== current;

  const save = useMutation({
    mutationFn: () => api.subscriptions.put({ subscriptions: mergeSubscriptions(profile.data?.subscriptions ?? [], draft), muted } as Profile),
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ['profile'] }); void qc.invalidateQueries({ queryKey: ['feed'] }); toast.show('Saved. Your feed is updated.', 'success'); },
    onError: (e: unknown) => toast.show(errorMessage(e), 'danger'),
  });
  const suggestions: Record<ScopeKind, string[]> = { source: (catalog.data ?? []).map((s) => s.name), entity: [...new Set((catalog.data ?? []).flatMap((s) => s.entities))], keyword: [] };

  if (profile.isLoading) return <div className="page"><Skeleton h={200} /></div>;
  return (
    <div className="page">
      <div className="page-head"><div><h1>Following</h1><p>Decide what flows into your feed.</p></div><Button disabled={!dirty} loading={save.isPending} icon="check" onClick={() => save.mutate(undefined)}>Save changes</Button></div>
      {profile.isError && <Banner tone="danger">{errorMessage(profile.error)}</Banner>}
      <div className="stack">
        {SECTIONS.map((s) => (
          <Card key={s.kind} className="stack sm">
            <div><h3>{s.title}</h3><p className="muted small">{s.hint}</p></div>
            <ChipInput label={s.title} value={draft[s.kind]} onChange={(v) => setDraft((d) => ({ ...d, [s.kind]: v }))} suggestions={suggestions[s.kind]} placeholder={s.placeholder} max={200} />
          </Card>
        ))}
        <Card className="stack sm">
          <div><h3>Muted</h3><p className="muted small">Items mentioning these are hidden from your feed.</p></div>
          <ChipInput label="Muted" value={muted} onChange={setMuted} placeholder="Add a topic or word to mute" max={200} />
        </Card>
        {dirty && <Banner tone="warn" icon="info">You have unsaved changes.</Banner>}
      </div>
    </div>
  );
}
