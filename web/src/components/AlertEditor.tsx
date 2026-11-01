import { useEffect, useMemo, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/endpoints';
import { errorMessage } from '../api/client';
import type { AlertRule, AlertSpec, AlertTemplate, Condition, FieldDef, ScopeKind, Severity } from '../api/types';
import { LIST_OPS, OPS_BY_KIND, isComplete, parseValue, valueToText } from '../lib/conditions';
import { Badge, Banner, Button, Card, ChipInput, Field, Icon, Modal, Skeleton, Switch, useDebounced, useToast } from '../ui';
import { duration } from '../lib/format';

const COOLDOWNS = [300, 900, 3600, 21600, 86400];
const KINDS: { id: ScopeKind; label: string; hint: string }[] = [
  { id: 'entity', label: 'Topic', hint: 'Items tagged with these topics' },
  { id: 'keyword', label: 'Keyword', hint: 'Whole words or phrases in the title or text' },
  { id: 'source', label: 'Source', hint: 'Every new item from these sources' },
];

function ConditionsEditor({ value, onChange, fields }: { value: Condition[]; onChange: (v: Condition[]) => void; fields: FieldDef[] }) {
  const set = (i: number, c: Condition) => onChange(value.map((x, j) => (j === i ? c : x)));
  return (
    <div className="stack sm">
      {value.map((c, i) => {
        const f = fields.find((x) => x.name === c.field);
        const kind = f?.kind ?? 'string';
        const ops = OPS_BY_KIND[kind];
        return (
          <div className="row wrap" key={i}>
            <select className="select" style={{ flex: '1 1 150px' }} aria-label="Field" value={c.field} onChange={(e) => { const nf = fields.find((x) => x.name === e.target.value); set(i, { field: e.target.value, op: OPS_BY_KIND[nf?.kind ?? 'string'][0]!.op, value: '' }); }}>
              <option value="">Choose a field…</option>{fields.map((x) => <option key={x.name} value={x.name}>{x.name.replace(/_/g, ' ')}</option>)}
            </select>
            <select className="select" style={{ flex: '1 1 130px' }} aria-label="Operator" value={c.op} onChange={(e) => set(i, { ...c, op: e.target.value as Condition['op'], value: '' })}>
              {ops.map((o) => <option key={o.op} value={o.op}>{o.label}</option>)}
            </select>
            {kind === 'bool'
              ? <select className="select" style={{ flex: '1 1 100px' }} aria-label="Value" value={String(c.value ?? '')} onChange={(e) => set(i, { ...c, value: e.target.value === 'true' })}><option value="">…</option><option value="true">true</option><option value="false">false</option></select>
              : <input className="input" style={{ flex: '1 1 120px' }} aria-label="Value" type={kind === 'number' && !LIST_OPS.includes(c.op) ? 'number' : 'text'} step="any" placeholder={LIST_OPS.includes(c.op) ? 'a, b, c' : 'value'} value={valueToText(c.value)} onChange={(e) => set(i, { ...c, value: parseValue(e.target.value, kind, c.op) })} />}
            <Button variant="ghost" size="sm" iconOnly icon="x" aria-label="Remove condition" onClick={() => onChange(value.filter((_, j) => j !== i))} />
          </div>
        );
      })}
      <div><Button variant="secondary" size="sm" icon="plus" onClick={() => onChange([...value, { field: '', op: '==', value: '' }])}>Add condition</Button></div>
    </div>
  );
}

export function AlertEditor({ rule, startTemplate, onClose }: { rule?: AlertRule; startTemplate?: string; onClose: () => void }) {
  const qc = useQueryClient();
  const toast = useToast();
  const templates = useQuery({ queryKey: ['templates'], queryFn: () => api.alerts.templates() });
  const types = useQuery({ queryKey: ['catalog', 'types'], queryFn: () => api.catalog.eventTypes() });
  const catalog = useQuery({ queryKey: ['catalog', 'sources'], queryFn: () => api.catalog.sources() });
  const channels = useQuery({ queryKey: ['channels'], queryFn: () => api.channels.list() });

  const [name, setName] = useState(rule?.name ?? '');
  const [kind, setKind] = useState<ScopeKind>(rule?.scope.kind ?? 'keyword');
  const [values, setValues] = useState<string[]>(rule?.scope.values ?? []);
  const [evTypes, setEvTypes] = useState<string[]>(rule?.types ?? []);
  const [conds, setConds] = useState<Condition[]>(rule?.conditions ?? []);
  const [cooldown, setCooldown] = useState(rule?.cooldown_seconds ?? 300);
  const [severity, setSeverity] = useState<Severity>(rule?.severity ?? 'notable');
  const [chans, setChans] = useState<string[]>(rule?.channels ?? ['in_app']);
  const [enabled, setEnabled] = useState(rule?.enabled ?? true);
  const [tpl, setTpl] = useState<string>('');
  const [advanced, setAdvanced] = useState(!!rule && (rule.types.length > 0 || rule.conditions.length > 0));
  const [err, setErr] = useState<string | null>(null);

  const applyTemplate = (t: AlertTemplate | null) => {
    setTpl(t?.id ?? 'custom');
    if (!t) return;
    setName((n) => n || t.name); setKind(t.scope_kind); setEvTypes(t.types ?? []); setConds(t.conditions ?? []); setCooldown(t.cooldown_seconds); setSeverity(t.severity);
    setAdvanced(!!(t.types?.length || t.conditions?.length));
  };
  useEffect(() => { // deep link from the empty state: ?new=<template id>
    if (!rule && startTemplate && templates.data && !tpl) applyTemplate(templates.data.find((t) => t.id === startTemplate) ?? null);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [templates.data]);

  const fields = useMemo(() => {
    const all: FieldDef[] = [...(types.data?.builtin_fields ?? [])];
    for (const t of types.data?.types ?? []) if (evTypes.length === 0 || evTypes.includes(t.id)) for (const f of t.fields) if (!all.some((x) => x.name === f.name)) all.push(f);
    return all;
  }, [types.data, evTypes]);

  const spec: AlertSpec = { name: name.trim(), scope: { kind, values }, types: evTypes, conditions: conds.filter(isComplete), cooldown_seconds: cooldown, severity, channels: chans, enabled };
  const previewSpec = JSON.stringify({ ...spec, name: spec.name || 'Preview', enabled: true });
  const debounced = useDebounced(previewSpec, 600);
  const preview = useQuery({ queryKey: ['alert-preview', debounced], queryFn: () => api.alerts.preview(JSON.parse(debounced) as AlertSpec), enabled: values.length > 0, retry: false, staleTime: 30_000 });

  const save = useMutation({
    mutationFn: () => (rule ? api.alerts.update(rule.id, spec) : api.alerts.create(spec)),
    onSuccess: () => { void qc.invalidateQueries({ queryKey: ['alerts'] }); toast.show(rule ? 'Alert updated' : 'Alert created', 'success'); onClose(); },
    onError: (e: unknown) => setErr(errorMessage(e)),
  });
  const kindMeta = KINDS.find((k) => k.id === kind)!;
  const suggestions = kind === 'source' ? (catalog.data ?? []).map((s) => s.name) : kind === 'entity' ? [...new Set((catalog.data ?? []).flatMap((s) => s.entities))] : [];
  const p = preview.data;

  return (
    <Modal wide title={rule ? 'Edit alert' : 'New alert'} onClose={onClose}
      footer={<><Button variant="secondary" onClick={onClose}>Cancel</Button><Button loading={save.isPending} disabled={!name.trim() || values.length === 0} onClick={() => { setErr(null); save.mutate(undefined); }}>{rule ? 'Save changes' : 'Create alert'}</Button></>}>
      <div className="stack">
        {!rule && (
          <div className="stack sm">
            <span className="label">Start from</span>
            {templates.isLoading ? <Skeleton h={60} /> : (
              <div className="grid cols-3">
                {(templates.data ?? []).map((t) => (
                  <button key={t.id} type="button" className={`pick ${tpl === t.id ? 'on' : ''}`} aria-pressed={tpl === t.id} onClick={() => applyTemplate(t)}><span className="stack sm" style={{ textAlign: 'left' }}><b>{t.name}</b><span className="small muted">{t.description}</span></span></button>
                ))}
                <button type="button" className={`pick ${tpl === 'custom' ? 'on' : ''}`} aria-pressed={tpl === 'custom'} onClick={() => applyTemplate(null)}><span className="stack sm" style={{ textAlign: 'left' }}><b>Custom</b><span className="small muted">Build your own rule</span></span></button>
              </div>
            )}
          </div>
        )}
        <Field label="Name" htmlFor="alert-name"><input id="alert-name" className="input" value={name} maxLength={80} placeholder="e.g. Rate decisions" onChange={(e) => setName(e.target.value)} /></Field>
        <div className="field">
          <span className="label">Alert me about</span>
          <div className="row wrap">{KINDS.map((k) => <button key={k.id} type="button" className={`pick ${kind === k.id ? 'on' : ''}`} style={{ padding: '.45rem .8rem' }} aria-pressed={kind === k.id} onClick={() => { setKind(k.id); setValues([]); }}>{k.label}</button>)}</div>
          <ChipInput label={kindMeta.label + ' values'} value={values} onChange={setValues} suggestions={suggestions} placeholder={kindMeta.hint} />
        </div>
        <div>
          <button type="button" className="rate" aria-expanded={advanced} onClick={() => setAdvanced((a) => !a)}><Icon name="chevron" size={14} />{advanced ? 'Hide' : 'Show'} advanced filters (event types, conditions)</button>
          {advanced && (
            <div className="stack" style={{ marginTop: '.6rem' }}>
              <Field label="Only these event types" help="Leave empty for all types.">
                <div className="row wrap">{(types.data?.types ?? []).map((t) => (
                  <label key={t.id} className="check small"><input type="checkbox" checked={evTypes.includes(t.id)} onChange={(e) => setEvTypes((v) => (e.target.checked ? [...v, t.id] : v.filter((x) => x !== t.id)))} />{t.id.replace(/_/g, ' ')}</label>))}</div>
              </Field>
              <Field label="Only when" help="All conditions must hold."><ConditionsEditor value={conds} onChange={setConds} fields={fields} /></Field>
            </div>
          )}
        </div>
        <div className="grid cols-2">
          <Field label="At most one alert every" htmlFor="alert-cd"><select id="alert-cd" className="select" value={cooldown} onChange={(e) => setCooldown(Number(e.target.value))}>{[...new Set([...COOLDOWNS, cooldown])].sort((a, b) => a - b).map((c) => <option key={c} value={c}>{duration(c)}</option>)}</select></Field>
          <Field label="Importance" htmlFor="alert-sev"><select id="alert-sev" className="select" value={severity} onChange={(e) => setSeverity(e.target.value as Severity)}><option value="info">Info</option><option value="notable">Notable</option><option value="high">High</option></select></Field>
        </div>
        <div className="field">
          <span className="label">Deliver to</span>
          <label className="check"><input type="checkbox" checked={chans.includes('in_app')} onChange={(e) => setChans((c) => (e.target.checked ? [...c, 'in_app'] : c.filter((x) => x !== 'in_app')))} />In the app (live)</label>
          {(channels.data ?? []).map((c) => <label key={c.id} className="check"><input type="checkbox" checked={chans.includes(c.id)} onChange={(e) => setChans((v) => (e.target.checked ? [...v, c.id] : v.filter((x) => x !== c.id)))} />Webhook: {c.name}{!c.enabled && <Badge tone="warn">disabled</Badge>}</label>)}
        </div>
        {rule && <div className="row"><Switch checked={enabled} onChange={setEnabled} label="Enabled" /><span>{enabled ? 'Enabled' : 'Paused'}</span></div>}
        <Card className="flat stack sm" aria-live="polite">
          <div className="row"><Icon name="eye" /><b>Preview</b>{preview.isFetching && <span className="spinner" aria-hidden="true" />}</div>
          {values.length === 0 && <p className="muted small">Add at least one value to see how often this would have fired.</p>}
          {preview.isError && <p className="muted small">{errorMessage(preview.error)}</p>}
          {p && <>
            <p>In the last day <b>{p.examined}</b> items were checked: this alert would have fired <b>{p.would_fire}</b> {p.would_fire === 1 ? 'time' : 'times'}.</p>
            {p.would_pause && <Banner tone="warn" icon="alert">This is too noisy: it would be paused automatically. Tighten the filters or increase the interval.</Banner>}
            {p.examples.length > 0 && <ul className="stack sm" style={{ margin: 0, paddingLeft: '1.1rem' }}>{p.examples.map((e, i) => <li key={i} className="small">{e.title}</li>)}</ul>}
            {p.examined < 20 && <p className="faint small">Based on recent items seen since the server started; it improves as more items arrive.</p>}
          </>}
        </Card>
        {err && <Banner tone="danger" icon="alert">{err}</Banner>}
      </div>
    </Modal>
  );
}
