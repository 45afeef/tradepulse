import { useMemo, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/endpoints';
import { errorMessage } from '../api/client';
import type { AlertFire, AlertRule, FieldDef, Rating } from '../api/types';
import { AlertEditor } from '../components/AlertEditor';
import { describeCondition } from '../lib/conditions';
import { duration, safeUrl, timeAgo } from '../lib/format';
import { useDocumentTitle, useTick } from '../lib/hooks';
import { Badge, Banner, Button, Card, Chip, Empty, Icon, SkeletonCards, Switch, Tabs, useConfirm, useToast } from '../ui';

const toSpec = (r: AlertRule, enabled = r.enabled) => ({ name: r.name, scope: r.scope, types: r.types, conditions: r.conditions, cooldown_seconds: r.cooldown_seconds, severity: r.severity, channels: r.channels, enabled });

export function AlertsPage() {
  useDocumentTitle('Alerts');
  const [params, setParams] = useSearchParams();
  const tab = params.get('tab') === 'history' ? 'history' : 'rules';
  const [editor, setEditor] = useState<{ rule?: AlertRule } | null>(null);
  return (
    <div className="page">
      <div className="page-head">
        <div><h1>Alerts</h1><p>The moments you cannot miss, pushed to you with the reason.</p></div>
        <Button icon="plus" onClick={() => setEditor({})}>New alert</Button>
      </div>
      <div style={{ marginBottom: '1rem' }}><Tabs label="Alerts" value={tab} onChange={(t: string) => setParams(t === 'rules' ? {} : { tab: t })} items={[{ id: 'rules', label: 'My alerts' }, { id: 'history', label: 'History' }]} /></div>
      {tab === 'rules' ? <Rules onEdit={(rule) => setEditor({ rule })} onNew={() => setEditor({})} /> : <History />}
      {editor && <AlertEditor rule={editor.rule} onClose={() => setEditor(null)} />}
    </div>
  );
}

function Rules({ onEdit, onNew }: { onEdit: (r: AlertRule) => void; onNew: () => void }) {
  const qc = useQueryClient();
  const toast = useToast();
  const confirm = useConfirm();
  const now = useTick();
  const rules = useQuery({ queryKey: ['alerts'], queryFn: () => api.alerts.list() });
  const types = useQuery({ queryKey: ['catalog', 'types'], queryFn: () => api.catalog.eventTypes() });
  const fields = useMemo<FieldDef[]>(() => [...(types.data?.builtin_fields ?? []), ...(types.data?.types ?? []).flatMap((t) => t.fields)], [types.data]);
  const toggle = useMutation({
    mutationFn: (v: { r: AlertRule; on: boolean }) => api.alerts.update(v.r.id, toSpec(v.r, v.on)),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ['alerts'] }),
    onError: (e: unknown) => toast.show(errorMessage(e), 'danger'),
  });
  const remove = async (r: AlertRule) => {
    if (!(await confirm({ title: `Delete “${r.name}”?`, message: 'It stops immediately. Past alerts stay in your history.', confirmLabel: 'Delete alert', danger: true }))) return;
    try { await api.alerts.remove(r.id); void qc.invalidateQueries({ queryKey: ['alerts'] }); toast.show('Alert deleted'); } catch (e) { toast.show(errorMessage(e), 'danger'); }
  };
  if (rules.isLoading) return <SkeletonCards n={2} />;
  if (rules.isError) return <Banner tone="danger">{errorMessage(rules.error)}</Banner>;
  if (!rules.data || rules.data.length === 0) return <Empty icon="bell" title="No alerts yet" action={<Button icon="plus" onClick={onNew}>Create your first alert</Button>}>Pick a template, see how often it would have fired, and save. Takes about a minute.</Empty>;
  return (
    <div className="stack">
      {rules.data.map((r: AlertRule) => (
        <Card key={r.id} className="stack">
          <div className="row top">
            <div className="grow stack sm">
              <div className="row wrap"><h3>{r.name}</h3><Badge tone={r.severity}>{r.severity}</Badge>{!r.enabled && <Badge tone="disabled">{r.paused_reason ? 'paused' : 'off'}</Badge>}</div>
              <div className="chips"><Badge tone="primary">{r.scope.kind === 'entity' ? 'topic' : r.scope.kind}</Badge>{r.scope.values.map((v) => <Chip key={v}>{v}</Chip>)}</div>
              {r.conditions.length > 0 && <p className="small muted">Only when {r.conditions.map((c) => describeCondition(c, fields)).join(' and ')}.</p>}
              <p className="small faint">At most one every {duration(r.cooldown_seconds)} · {r.last_fired_at ? `last fired ${timeAgo(r.last_fired_at, now)}` : 'has not fired yet'}{r.channels.length > 1 ? ' · webhooks on' : ''}</p>
            </div>
            <Switch checked={r.enabled} onChange={(on) => toggle.mutate({ r, on })} label={`${r.name} enabled`} />
          </div>
          {r.paused_reason && <Banner tone="warn" icon="alert" action={<Button size="sm" onClick={() => toggle.mutate({ r, on: true })}>Resume</Button>}><b>Paused automatically.</b> {r.paused_reason}</Banner>}
          <div className="row"><Button variant="ghost" size="sm" icon="edit" onClick={() => onEdit(r)}>Edit</Button><Button variant="ghost" size="sm" icon="trash" onClick={() => remove(r)}>Delete</Button></div>
        </Card>
      ))}
    </div>
  );
}

function History() {
  const qc = useQueryClient();
  const toast = useToast();
  const now = useTick();
  const fires = useInfiniteQuery({
    queryKey: ['fires'],
    queryFn: ({ pageParam }: { pageParam: unknown }) => api.alerts.fires({ before: (pageParam as string) || undefined }),
    initialPageParam: '',
    getNextPageParam: (l: { next_cursor?: string }) => l.next_cursor || undefined,
  });
  const rate = useMutation({
    mutationFn: (v: { id: string; rating: Rating | null }) => (v.rating ? api.alerts.rate(v.id, v.rating) : api.alerts.clearRating(v.id)),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ['fires'] }),
    onError: (e: unknown) => toast.show(errorMessage(e), 'danger'),
  });
  const list = fires.data?.pages.flatMap((p: { fires: AlertFire[] }) => p.fires) ?? [];
  if (fires.isLoading) return <SkeletonCards n={3} />;
  if (list.length === 0) return <Empty icon="clock" title="No alerts have fired yet">When one of your alerts fires it shows up here with the reason. Rate them: it keeps your alerts sharp.</Empty>;
  return (
    <div className="stack">
      {list.map((f) => {
        const url = safeUrl(f.url);
        return (
          <Card key={f.id} className="stack sm">
            <div className="row wrap small"><Badge tone={f.severity} icon="zap">{f.rule_name}</Badge><span className="faint">{timeAgo(f.fired_at, now)}</span></div>
            <h3>{url ? <a href={url} target="_blank" rel="noopener noreferrer">{f.title}</a> : f.title}</h3>
            {f.body && <p className="muted">{f.body}</p>}
            <p className="small faint">{f.reason}</p>
            <div className="feed-actions">
              <button type="button" className="rate up" aria-pressed={f.feedback === 'useful'} onClick={() => rate.mutate({ id: f.id, rating: f.feedback === 'useful' ? null : 'useful' })}><Icon name="up" size={15} />Useful</button>
              <button type="button" className="rate down" aria-pressed={f.feedback === 'not_useful'} onClick={() => rate.mutate({ id: f.id, rating: f.feedback === 'not_useful' ? null : 'not_useful' })}><Icon name="down" size={15} />Not useful</button>
            </div>
          </Card>
        );
      })}
      {fires.hasNextPage && <div className="row" style={{ justifyContent: 'center' }}><Button variant="secondary" loading={fires.isFetchingNextPage} onClick={() => void fires.fetchNextPage()}>Load more</Button></div>}
    </div>
  );
}
