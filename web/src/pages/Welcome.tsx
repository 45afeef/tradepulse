import { useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/endpoints';
import { errorMessage } from '../api/client';
import type { AlertTemplate, ScopeKind, Subscription } from '../api/types';
import { Badge, Banner, Button, Card, ChipInput, Empty, Icon, Skeleton, Stepper, useToast } from '../ui';
import { useDocumentTitle } from '../lib/hooks';
import { plural } from '../lib/format';

/** First-run wizard: follow something, get one alert, done. Aim: a first useful alert in under ten minutes. */
export function WelcomePage() {
  useDocumentTitle('Get started');
  const nav = useNavigate();
  const qc = useQueryClient();
  const toast = useToast();
  const [step, setStep] = useState(0);
  const [sources, setSources] = useState<string[]>([]);
  const [topics, setTopics] = useState<string[]>([]);
  const [words, setWords] = useState<string[]>([]);
  const [picked, setPicked] = useState<Record<string, string[]>>({});
  const catalog = useQuery({ queryKey: ['catalog', 'sources'], queryFn: () => api.catalog.sources() });
  const templates = useQuery({ queryKey: ['templates'], queryFn: () => api.alerts.templates() });
  const profile = useQuery({ queryKey: ['profile'], queryFn: () => api.subscriptions.get() });
  const allTopics = useMemo(() => [...new Set((catalog.data ?? []).flatMap((s) => s.entities))], [catalog.data]);

  const toggle = (name: string) => setSources((s) => (s.includes(name) ? s.filter((x) => x !== name) : [...s, name]));
  const followCount = sources.length + topics.length + words.length;

  const defaults = (t: AlertTemplate): string[] => (t.scope_kind === 'keyword' ? words : t.scope_kind === 'entity' ? topics : sources);
  const valuesFor = (t: AlertTemplate) => picked[t.id] ?? defaults(t);
  const chosen = (templates.data ?? []).filter((t) => t.id in picked);

  const finish = useMutation({
    mutationFn: async () => {
      const existing = profile.data?.subscriptions ?? [];
      const add: Subscription[] = [
        ...sources.map((v) => ({ kind: 'source' as ScopeKind, value: v })), ...topics.map((v) => ({ kind: 'entity' as ScopeKind, value: v })), ...words.map((v) => ({ kind: 'keyword' as ScopeKind, value: v })),
      ].filter((n) => !existing.some((e) => e.kind === n.kind && e.value.toLowerCase() === n.value.toLowerCase()));
      await api.subscriptions.put({ ...(profile.data ?? {}), subscriptions: [...existing, ...add] });
      for (const t of chosen) { const v = valuesFor(t); if (v.length) await api.alerts.fromTemplate(t.id, v); }
      return chosen.length;
    },
    onSuccess: (n: number) => {
      for (const k of ['profile', 'alerts', 'feed']) void qc.invalidateQueries({ queryKey: [k] });
      toast.show(n ? `You are set: following ${followCount} and ${plural(n, 'alert')} active` : 'You are set. Your feed is live.', 'success');
      nav('/feed', { replace: true });
    },
    onError: (e: unknown) => toast.show(errorMessage(e), 'danger'),
  });

  return (
    <div className="page">
      <div className="stack lg">
        <div><h1>Let&rsquo;s set you up</h1><p className="muted">About two minutes. You can change everything later.</p></div>
        <Stepper step={step} total={3} />

        {step === 0 && (
          <div className="stack">
            <h2>What do you want to follow?</h2>
            {catalog.isLoading && <Skeleton h={90} />}
            {catalog.data && catalog.data.length === 0 && <Banner>No sources have been added yet. You can still follow topics and keywords; items will show up as soon as sources are connected.</Banner>}
            <div className="grid cols-2">
              {(catalog.data ?? []).map((s) => (
                <button type="button" key={s.name} className={`pick ${sources.includes(s.name) ? 'on' : ''}`} aria-pressed={sources.includes(s.name)} onClick={() => toggle(s.name)}>
                  <Icon name={sources.includes(s.name) ? 'check' : 'database'} />
                  <span className="stack sm" style={{ textAlign: 'left' }}><b>{s.name}</b><span className="row wrap small">{s.entities.slice(0, 4).map((e) => <Badge key={e}>{e}</Badge>)}<Badge tone="info">{s.type}</Badge></span></span>
                </button>
              ))}
            </div>
            <div className="field"><span className="label">Topics</span><ChipInput label="Topics" value={topics} onChange={setTopics} suggestions={allTopics} placeholder="e.g. markets, rates, oil" /></div>
            <div className="field"><span className="label">Keywords</span><ChipInput label="Keywords" value={words} onChange={setWords} placeholder="Words or phrases you care about" /><span className="help">Matches whole words or phrases in titles and text.</span></div>
            <div className="row between"><Button variant="ghost" onClick={() => nav('/feed')}>Skip for now</Button><Button disabled={followCount === 0} onClick={() => setStep(1)}>Continue</Button></div>
          </div>
        )}

        {step === 1 && (
          <div className="stack">
            <h2>Which moments should interrupt you?</h2>
            <p className="muted">Alerts are for what you cannot miss. Pick any you like; each shows how often it would have fired recently.</p>
            {(templates.data ?? []).filter((t) => defaults(t).length > 0 || t.id in picked).map((t) => (
              <TemplateChoice key={t.id} t={t} values={valuesFor(t)} on={t.id in picked} onToggle={(on) => setPicked((p) => { const n = { ...p }; if (on) n[t.id] = defaults(t); else delete n[t.id]; return n; })} onValues={(v) => setPicked((p) => ({ ...p, [t.id]: v }))} />
            ))}
            {(templates.data ?? []).every((t) => defaults(t).length === 0) && <Empty icon="bell" title="Nothing to alert on yet">Follow a source, topic or keyword first, or skip this step and add alerts later.</Empty>}
            <div className="row between"><Button variant="ghost" onClick={() => setStep(0)}>Back</Button><Button onClick={() => setStep(2)}>Continue</Button></div>
          </div>
        )}

        {step === 2 && (
          <div className="stack">
            <h2>Ready</h2>
            <Card className="stack">
              <div className="row"><Icon name="list" /><span>Following <b>{followCount}</b> {followCount === 1 ? 'thing' : 'things'}: {[...sources, ...topics, ...words].slice(0, 6).join(', ')}{followCount > 6 ? '…' : ''}</span></div>
              <div className="row"><Icon name="bell" /><span>{chosen.length ? <><b>{plural(chosen.length, 'alert')}</b> will notify you here, live.</> : 'No alerts yet. You can add them any time under Alerts.'}</span></div>
              <div className="row"><Icon name="send" /><span className="muted">Want alerts in your own tools? Add a webhook under Delivery.</span></div>
            </Card>
            <div className="row between"><Button variant="ghost" onClick={() => setStep(1)}>Back</Button><Button loading={finish.isPending} icon="check" onClick={() => finish.mutate(undefined)}>Start my feed</Button></div>
          </div>
        )}
      </div>
    </div>
  );
}

function TemplateChoice({ t, values, on, onToggle, onValues }: { t: AlertTemplate; values: string[]; on: boolean; onToggle: (on: boolean) => void; onValues: (v: string[]) => void }) {
  const spec = { name: t.name, scope: { kind: t.scope_kind, values }, types: t.types, conditions: t.conditions, cooldown_seconds: t.cooldown_seconds, severity: t.severity, enabled: true };
  const preview = useQuery({ queryKey: ['preview', t.id, values.join('|')], queryFn: () => api.alerts.preview(spec), enabled: on && values.length > 0, staleTime: 60_000 });
  return (
    <Card className={on ? 'card accent stack' : 'card stack'}>
      <label className="check"><input type="checkbox" checked={on} onChange={(e) => onToggle(e.target.checked)} /><span><b>{t.name}</b><span className="muted"> · {t.description}</span></span></label>
      {on && <>
        <ChipInput label={t.values_hint} value={values} onChange={onValues} placeholder={t.values_hint} />
        {preview.data && <p className="small muted"><Icon name="clock" size={14} /> In the last day this would have fired <b>{preview.data.would_fire}</b> {preview.data.would_fire === 1 ? 'time' : 'times'}{preview.data.would_pause ? ', which is noisy: it would be paused automatically' : ''}.</p>}
      </>}
    </Card>
  );
}
