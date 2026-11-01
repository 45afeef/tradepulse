import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../api/endpoints';
import { errorMessage } from '../../api/client';
import type { Health, Source } from '../../api/types';
import { SourceWizard, WebhookSecret } from '../../components/SourceWizard';
import { duration, plural, shortDateTime, timeAgo } from '../../lib/format';
import { useDocumentTitle, useTick } from '../../lib/hooks';
import { INTERVALS, TYPE_INFO, toUpdate } from '../../lib/sources';
import { Badge, Banner, Button, Card, ChipInput, Empty, Field, Icon, Modal, SkeletonCards, Switch, Tabs, useConfirm, useToast } from '../../ui';

const ATTENTION: Health[] = ['failing', 'overdue', 'degraded'];

function SourceDetail({ id, onClose }: { id: string; onClose: () => void }) {
  const qc = useQueryClient();
  const toast = useToast();
  const confirm = useConfirm();
  const now = useTick();
  const q = useQuery({ queryKey: ['admin', 'source', id], queryFn: () => api.admin.source(id), refetchInterval: 15_000 });
  const [entities, setEntities] = useState<string[] | null>(null);
  const [interval, setIntervalS] = useState<number | null>(null);
  const [secret, setSecret] = useState<Source | null>(null);
  const refresh = () => { void qc.invalidateQueries({ queryKey: ['admin', 'source', id] }); void qc.invalidateQueries({ queryKey: ['admin', 'sources'] }); };
  const s = q.data?.source;
  const act = async (label: string, fn: () => Promise<unknown>) => { try { await fn(); toast.show(label, 'success'); refresh(); } catch (e) { toast.show(errorMessage(e), 'danger'); } };
  const save = useMutation({
    mutationFn: () => api.admin.updateSource(id, toUpdate(s!, { entities: entities ?? s!.entities, interval_seconds: interval ?? (s!.interval_seconds || undefined) })),
    onSuccess: () => { toast.show('Saved', 'success'); setEntities(null); setIntervalS(null); refresh(); },
    onError: (e: unknown) => toast.show(errorMessage(e), 'danger'),
  });
  const del = async () => {
    if (!s || !(await confirm({ title: `Delete “${s.name}”?`, message: 'Members subscribed to it will stop receiving items from it. This cannot be undone.', confirmLabel: 'Delete source', danger: true }))) return;
    await act('Source deleted', () => api.admin.deleteSource(id)); onClose();
  };
  const dirty = (entities !== null) || (interval !== null);
  return (
    <Modal wide title={s ? s.name : 'Source'} onClose={onClose}>
      {q.isLoading && <SkeletonCards n={2} />}
      {q.isError && <Banner tone="danger">{errorMessage(q.error)}</Banner>}
      {s && (
        <div className="stack">
          <div className="row wrap"><Badge tone={s.health}>{s.health}</Badge><Badge tone="info">{TYPE_INFO[s.type]?.title ?? s.type}</Badge><span className="grow" />
            <span className="small muted">{s.enabled ? 'Running' : 'Stopped'}</span><Switch checked={s.enabled} onChange={(on) => void act(on ? 'Source started' : 'Source stopped', () => api.admin.updateSource(id, toUpdate(s, { enabled: on })))} label={`${s.name} enabled`} /></div>
          {s.last_error && <Banner tone="danger" icon="alert"><b>Last error:</b> {s.last_error}{s.consecutive_failures > 1 ? ` (${s.consecutive_failures} in a row)` : ''}</Banner>}
          <div className="grid cols-3">
            <Card className="stat flat"><span className="n">{s.items_total}</span><span className="l">items collected</span></Card>
            <Card className="stat flat"><span className="n">{s.last_success_at ? timeAgo(s.last_success_at, now) : 'never'}</span><span className="l">last success</span></Card>
            <Card className="stat flat"><span className="n">{s.next_run_at ? shortDateTime(s.next_run_at) : 'n/a'}</span><span className="l">next check</span></Card>
          </div>
          {s.url && <p className="small muted mono" style={{ wordBreak: 'break-all' }}>{s.url}</p>}
          {s.type === 'webhook' && <Card className="flat stack sm"><b>Webhook</b><p className="small mono">{s.webhook_path}</p><div className="row wrap"><Button variant="secondary" size="sm" icon="key" onClick={async () => { if (await confirm({ title: 'Rotate the signing secret?', message: 'The old secret stops working at once; update the sender right after.', confirmLabel: 'Rotate' })) { try { setSecret(await api.admin.webhookSecret(id)); refresh(); } catch (e) { toast.show(errorMessage(e), 'danger'); } } }}>Rotate secret</Button></div></Card>}
          <div className="grid cols-2">
            {s.type !== 'webhook' && s.type !== 'manual' && <Field label="Check" htmlFor="sd-int"><select id="sd-int" className="select" value={interval ?? s.interval_seconds} onChange={(e) => setIntervalS(Number(e.target.value))}>{[...new Set([...INTERVALS.map(([x]) => x), s.interval_seconds])].sort((a, b) => a - b).map((x) => <option key={x} value={x}>{INTERVALS.find(([v]) => v === x)?.[1] ?? `Every ${duration(x)}`}</option>)}</select></Field>}
            <Field label="Topics"><ChipInput label="Topics" value={entities ?? s.entities} onChange={setEntities} placeholder="Add a topic" /></Field>
          </div>
          <div className="row wrap">
            <Button size="sm" loading={save.isPending} disabled={!dirty} onClick={() => save.mutate(undefined)}>Save changes</Button>
            {(s.type === 'rss' || s.type === 'webmonitor' || s.type === 'x') && <Button variant="secondary" size="sm" icon="refresh" onClick={() => void act('Fetching now…', async () => { await api.admin.runSource(id); setTimeout(refresh, 2500); })}>Run now</Button>}
            <span className="grow" /><Button variant="ghost" size="sm" icon="trash" onClick={() => void del()}>Delete</Button>
          </div>
          <h3>Recent runs</h3>
          {q.data!.recent_runs.length === 0 ? <p className="muted small">No runs yet.</p> : (
            <div className="table-wrap"><table className="table"><thead><tr><th>When</th><th>Result</th><th>New</th><th>Took</th><th>Detail</th></tr></thead><tbody>
              {q.data!.recent_runs.slice(0, 10).map((r, i) => <tr key={i}><td>{shortDateTime(r.started_at)}</td><td><Badge tone={r.ok ? 'ok' : 'danger'}>{r.ok ? 'OK' : `Failed${r.http_status ? ' ' + r.http_status : ''}`}</Badge></td><td>{r.items_new}/{r.items_seen}</td><td>{r.duration_ms} ms</td><td className="small muted">{r.error ?? ''}</td></tr>)}
            </tbody></table></div>)}
        </div>
      )}
      {secret && <Modal title="Save the new secret" onClose={() => setSecret(null)} footer={<Button onClick={() => setSecret(null)}>I saved it</Button>}><WebhookSecret source={secret} /></Modal>}
    </Modal>
  );
}

export function AdminSources() {
  useDocumentTitle('Sources');
  const now = useTick();
  const [tab, setTab] = useState<'all' | 'attention' | 'off'>('all');
  const [adding, setAdding] = useState(false);
  const [open, setOpen] = useState<string | null>(null);
  const q = useQuery({ queryKey: ['admin', 'sources'], queryFn: () => api.admin.sources(), refetchInterval: 20_000 });
  const all = q.data ?? [];
  const attention = all.filter((s) => ATTENTION.includes(s.health));
  const shown = tab === 'attention' ? attention : tab === 'off' ? all.filter((s) => !s.enabled) : all;
  return (
    <div className="page wide">
      <div className="page-head"><div><h1>Sources</h1><p>Where your information comes from. {all.length > 0 && plural(all.filter((s) => s.enabled).length, 'source') + ' running.'}</p></div><Button icon="plus" onClick={() => setAdding(true)}>Add source</Button></div>
      <div style={{ marginBottom: '1rem' }}><Tabs label="Source filter" value={tab} onChange={setTab} items={[{ id: 'all', label: `All (${all.length})` }, { id: 'attention', label: `Needs attention (${attention.length})` }, { id: 'off', label: 'Stopped' }]} /></div>
      {q.isLoading && <SkeletonCards n={3} />}
      {q.isError && <Banner tone="danger">{errorMessage(q.error)}</Banner>}
      {q.data && all.length === 0 && <Empty icon="database" title="No sources yet" action={<Button icon="plus" onClick={() => setAdding(true)}>Add your first source</Button>}>Start with an RSS feed: paste a URL, test it, and members start receiving items within minutes.</Empty>}
      {shown.length > 0 && (
        <div className="table-wrap"><table className="table"><thead><tr><th>Source</th><th>Health</th><th>Checks</th><th>Last item</th><th>Items</th></tr></thead><tbody>
          {shown.map((s) => (
            <tr key={s.id} style={{ cursor: 'pointer' }} onClick={() => setOpen(s.id)}>
              <td><button type="button" className="tab" style={{ padding: 0, fontWeight: 700, color: 'var(--text)' }} onClick={(e) => { e.stopPropagation(); setOpen(s.id); }}>{s.name}</button><div className="small faint">{TYPE_INFO[s.type]?.title ?? s.type}</div></td>
              <td><Badge tone={s.health}>{s.health}</Badge>{s.last_error && <div className="small faint truncate" style={{ maxWidth: 260 }}>{s.last_error}</div>}</td>
              <td className="small">{s.type === 'webhook' || s.type === 'manual' ? 'push' : duration(s.interval_seconds)}</td>
              <td className="small">{s.last_item_at ? timeAgo(s.last_item_at, now) : '–'}</td>
              <td>{s.items_total}</td>
            </tr>))}
        </tbody></table></div>)}
      {tab !== 'all' && shown.length === 0 && all.length > 0 && <Empty icon="check" title="Nothing here">{tab === 'attention' ? 'Every running source is healthy.' : 'Every source is running.'}</Empty>}
      {adding && <SourceWizard onClose={() => setAdding(false)} />}
      {open && <SourceDetail id={open} onClose={() => setOpen(null)} />}
    </div>
  );
}
