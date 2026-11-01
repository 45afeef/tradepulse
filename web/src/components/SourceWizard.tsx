import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/endpoints';
import { errorMessage } from '../api/client';
import type { Source, SourceDraft, SourceTest, SourceType, WebConfig } from '../api/types';
import { INTERVALS, TYPE_INFO } from '../lib/sources';
import { slugify, validSlug } from '../lib/format';
import { Badge, Banner, Button, Card, ChipInput, CopyButton, Field, Icon, Modal, Stepper, useToast } from '../ui';

const TYPES: SourceType[] = ['rss', 'webmonitor', 'webhook', 'manual', 'x'];

export function WebhookSecret({ source }: { source: Source }) {
  const path = source.webhook_path ?? `/api/v1/hooks/${source.name}`;
  const base = (import.meta.env.VITE_API_BASE as string | undefined) || location.origin;
  const curl = `BODY='{"title":"Hello","summary":"from my system","entities":["demo"]}'; TS=$(date +%s)\nSIG=$(printf '%s.%s' "$TS" "$BODY" | openssl dgst -sha256 -hmac '${source.webhook_secret ?? '<secret>'}' -hex | sed 's/^.* //')\ncurl -X POST ${base}${path} -H "X-TP-Timestamp: $TS" -H "X-TP-Signature: sha256=$SIG" -d "$BODY"`;
  return (
    <div className="stack">
      <Banner tone="warn" icon="key">The signing secret is shown <b>once</b>. Store it in the sending system; you can rotate it later.</Banner>
      <div className="code">{source.webhook_secret}</div>
      <div className="row wrap"><CopyButton text={source.webhook_secret ?? ''} label="Copy secret" /><CopyButton text={`${base}${path}`} label="Copy URL" /></div>
      <details><summary className="small muted" style={{ cursor: 'pointer' }}>Example request</summary><pre className="code small" style={{ whiteSpace: 'pre-wrap' }}>{curl}</pre></details>
    </div>
  );
}

export function SourceWizard({ onClose }: { onClose: () => void }) {
  const qc = useQueryClient();
  const toast = useToast();
  const [step, setStep] = useState(0);
  const [type, setType] = useState<SourceType>('rss');
  const [name, setName] = useState('');
  const [url, setUrl] = useState('');
  const [interval, setIntervalS] = useState(300);
  const [entities, setEntities] = useState<string[]>([]);
  const [web, setWeb] = useState<WebConfig>({ item_selector: '', title_selector: '', link_selector: 'a', summary_selector: '' });
  const [auth, setAuth] = useState<'hmac' | 'token'>('hmac');
  const [handle, setHandle] = useState('');
  const [enableNow, setEnableNow] = useState(true);
  const [tested, setTested] = useState<SourceTest | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [created, setCreated] = useState<Source | null>(null);

  const usage = useQuery({ queryKey: ['admin', 'usage'], queryFn: () => api.admin.usage(), enabled: type === 'x' });
  const polled = type === 'rss' || type === 'webmonitor' || type === 'x';

  const draft = (): SourceDraft => {
    const d: SourceDraft = { name, type, entities };
    if (type === 'rss' || type === 'webmonitor') { d.url = url.trim(); d.interval_seconds = interval; }
    if (type === 'x') { d.x = { handle: handle.replace(/^@/, '').trim() }; d.interval_seconds = interval; }
    if (type === 'webmonitor') d.web = { ...web, title_selector: web.title_selector || undefined, summary_selector: web.summary_selector || undefined, link_selector: web.link_selector || undefined };
    if (type === 'webhook') d.webhook = { auth };
    return d;
  };
  const test = useMutation({
    mutationFn: () => api.admin.testSource(draft()),
    onSuccess: (r: SourceTest) => { setTested(r); setErr(null); },
    onError: (e: unknown) => { setTested(null); setErr(errorMessage(e)); },
  });
  const create = useMutation({
    mutationFn: async () => {
      const s = await api.admin.createSource(draft());
      if (!enableNow) return s;
      const on = await api.admin.updateSource(s.id, { enabled: true, url: s.url || undefined, interval_seconds: s.interval_seconds || undefined, entities: s.entities, web: s.web, webhook: s.webhook, x: s.x });
      return { ...on, webhook_secret: s.webhook_secret, webhook_path: s.webhook_path };
    },
    onSuccess: (s: Source) => { void qc.invalidateQueries({ queryKey: ['admin', 'sources'] }); toast.show(`Source “${s.name}” created`, 'success'); if (s.type === 'webhook') { setCreated(s); setStep(3); } else onClose(); },
    onError: (e: unknown) => setErr(errorMessage(e)),
  });

  const nameOk = validSlug(name);
  const configOk = nameOk && (type === 'manual' || type === 'webhook' || (type === 'x' ? handle.trim().length > 0 : url.trim().length > 0 && (type !== 'webmonitor' || web.item_selector.trim().length > 0)));

  return (
    <Modal wide title="Add a source" onClose={onClose}>
      <div className="stack">
        {step < 3 && <Stepper step={step} total={3} />}
        {step === 0 && (
          <div className="stack">
            <div className="grid cols-2">{TYPES.map((t) => (
              <button key={t} type="button" className={`pick ${type === t ? 'on' : ''}`} aria-pressed={type === t} onClick={() => { setType(t); setTested(null); }}>
                <Icon name={TYPE_INFO[t]!.icon} /><span className="stack sm" style={{ textAlign: 'left' }}><b>{TYPE_INFO[t]!.title}</b><span className="small muted">{TYPE_INFO[t]!.blurb}</span></span></button>))}</div>
            <div className="modal-foot"><Button onClick={() => setStep(1)}>Continue</Button></div>
          </div>
        )}
        {step === 1 && (
          <div className="stack">
            <Field label="Name" htmlFor="src-name" help="Lower-case letters, digits, - and _. Permanent: members subscribe to it by this name." error={name && !nameOk ? 'Use 2 to 40 characters: a-z, 0-9, - or _.' : null}>
              <input id="src-name" className="input mono" value={name} placeholder="e.g. cnbc-top" onChange={(e) => setName(slugify(e.target.value))} />
            </Field>
            {(type === 'rss' || type === 'webmonitor') && <Field label={type === 'rss' ? 'Feed URL' : 'Page URL'} htmlFor="src-url" help="Public https address. Private and internal addresses are refused."><input id="src-url" className="input" type="url" value={url} placeholder="https://example.com/feed.xml" onChange={(e) => { setUrl(e.target.value); setTested(null); }} /></Field>}
            {type === 'webmonitor' && (
              <Card className="flat stack sm">
                <Banner icon="info">Check the site&rsquo;s terms and robots.txt first. The monitor reads plain HTML (no JavaScript) and respects robots.txt.</Banner>
                <Field label="Item selector" htmlFor="sel-item" help="CSS selector that matches each headline block, e.g. article.story"><input id="sel-item" className="input mono" value={web.item_selector} onChange={(e) => { setWeb({ ...web, item_selector: e.target.value }); setTested(null); }} /></Field>
                <div className="grid cols-2">
                  <Field label="Title selector" htmlFor="sel-title" help="Inside the item; empty = the item text"><input id="sel-title" className="input mono" value={web.title_selector ?? ''} onChange={(e) => setWeb({ ...web, title_selector: e.target.value })} /></Field>
                  <Field label="Link selector" htmlFor="sel-link"><input id="sel-link" className="input mono" value={web.link_selector ?? ''} onChange={(e) => setWeb({ ...web, link_selector: e.target.value })} /></Field>
                </div>
                <Field label="Summary selector (optional)" htmlFor="sel-sum"><input id="sel-sum" className="input mono" value={web.summary_selector ?? ''} onChange={(e) => setWeb({ ...web, summary_selector: e.target.value })} /></Field>
              </Card>
            )}
            {type === 'x' && <>
              {usage.data && !usage.data.x_configured && <Banner tone="warn" icon="alert">X is not configured on this server (no API token). The source can be saved, but nothing will be fetched until it is.</Banner>}
              <Field label="Account handle" htmlFor="src-x"><input id="src-x" className="input" value={handle} placeholder="@username" onChange={(e) => { setHandle(e.target.value); setTested(null); }} /></Field>
            </>}
            {type === 'webhook' && <Field label="Authentication" htmlFor="src-auth"><select id="src-auth" className="select" value={auth} onChange={(e) => setAuth(e.target.value as 'hmac' | 'token')}><option value="hmac">Signed requests (HMAC, recommended)</option><option value="token">Bearer token (for tools that cannot sign)</option></select></Field>}
            {polled && <Field label="Check" htmlFor="src-int"><select id="src-int" className="select" value={interval} onChange={(e) => setIntervalS(Number(e.target.value))}>{INTERVALS.map(([s, l]) => <option key={s} value={s}>{l}</option>)}</select></Field>}
            <Field label="Topics" help="Tags put on every item from this source; members can follow them."><ChipInput label="Topics" value={entities} onChange={setEntities} placeholder="e.g. markets, india" /></Field>
            <div className="modal-foot"><Button variant="secondary" onClick={() => setStep(0)}>Back</Button><Button disabled={!configOk} onClick={() => setStep(2)}>Continue</Button></div>
          </div>
        )}
        {step === 2 && (
          <div className="stack">
            {polled ? (
              <>
                <p className="muted">Fetch it once and see what would be read. Nothing is saved.</p>
                <div><Button variant="secondary" icon="play" loading={test.isPending} onClick={() => { setErr(null); test.mutate(undefined); }}>Test fetch</Button></div>
                {tested && (tested.ok
                  ? <Card className="flat stack sm"><div className="row"><Badge tone="ok" icon="check">Works</Badge><span>{tested.count} items found. First few:</span></div>{tested.preview.slice(0, 5).map((p, i) => <div key={i} className="small"><b>{p.title}</b>{p.summary && <span className="muted"> · {p.summary.slice(0, 100)}</span>}</div>)}</Card>
                  : <Banner tone="danger" icon="alert"><b>Could not read it.</b> {tested.error}</Banner>)}
              </>
            ) : <p className="muted">{type === 'webhook' ? 'After saving you get a signing secret and a ready-to-run example.' : 'Manual sources need no setup. Publish items from the Publish page.'}</p>}
            <label className="check"><input type="checkbox" checked={enableNow} onChange={(e) => setEnableNow(e.target.checked)} />Start immediately {polled && !tested?.ok ? '(untested)' : ''}</label>
            {err && <Banner tone="danger" icon="alert">{err}</Banner>}
            <div className="modal-foot"><Button variant="secondary" onClick={() => setStep(1)}>Back</Button><Button loading={create.isPending} icon="check" onClick={() => { setErr(null); create.mutate(undefined); }}>Create source</Button></div>
          </div>
        )}
        {step === 3 && created && (
          <div className="stack"><Banner tone="success" icon="check">Source <b>{created.name}</b> is ready.</Banner><WebhookSecret source={created} /><div className="modal-foot"><Button onClick={onClose}>Done</Button></div></div>
        )}
      </div>
    </Modal>
  );
}
