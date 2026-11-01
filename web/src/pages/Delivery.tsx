import { useState } from 'react';
import type { FormEvent } from 'react';
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/endpoints';
import { errorMessage } from '../api/client';
import type { Channel, Delivery } from '../api/types';
import { Badge, Banner, Button, Card, CopyButton, Empty, Field, Icon, Modal, SkeletonCards, useConfirm, useToast } from '../ui';
import { shortDateTime, timeAgo } from '../lib/format';
import { useDocumentTitle, useTick } from '../lib/hooks';

function SecretModal({ channel, onClose }: { channel: Channel; onClose: () => void }) {
  return (
    <Modal title="Save your signing secret" onClose={onClose} footer={<Button onClick={onClose}>I saved it</Button>}>
      <div className="stack">
        <Banner tone="warn" icon="key">This secret is shown <b>once</b>. Store it in your receiving system; if you lose it, rotate it.</Banner>
        <div className="code" aria-label="Signing secret">{channel.secret}</div>
        <div><CopyButton text={channel.secret ?? ''} label="Copy secret" /></div>
        <p className="muted small">Each alert is sent as JSON with <span className="mono">X-TP-Timestamp</span> and <span className="mono">X-TP-Signature: sha256=…</span>, where the signature is HMAC-SHA256 of <span className="mono">timestamp + &quot;.&quot; + raw body</span> using this secret. Reject deliveries older than 5 minutes.</p>
      </div>
    </Modal>
  );
}

export function DeliveryPage() {
  useDocumentTitle('Delivery');
  const qc = useQueryClient();
  const toast = useToast();
  const confirm = useConfirm();
  const now = useTick();
  const [adding, setAdding] = useState(false);
  const [name, setName] = useState('');
  const [url, setUrl] = useState('');
  const [err, setErr] = useState<string | null>(null);
  const [secret, setSecret] = useState<Channel | null>(null);
  const channels = useQuery({ queryKey: ['channels'], queryFn: () => api.channels.list() });
  const deliveries = useInfiniteQuery({
    queryKey: ['deliveries'],
    queryFn: ({ pageParam }: { pageParam: unknown }) => api.channels.deliveries({ before: (pageParam as string) || undefined }),
    initialPageParam: '',
    getNextPageParam: (l: { next_cursor?: string }) => l.next_cursor || undefined,
  });
  const refresh = () => { void qc.invalidateQueries({ queryKey: ['channels'] }); void qc.invalidateQueries({ queryKey: ['deliveries'] }); };

  const create = useMutation({
    mutationFn: () => api.channels.create(name.trim(), url.trim()),
    onSuccess: (c: Channel) => { setAdding(false); setName(''); setUrl(''); setSecret(c); refresh(); },
    onError: (e: unknown) => setErr(errorMessage(e)),
  });
  const act = async (label: string, fn: () => Promise<unknown>, after?: () => void) => {
    try { await fn(); toast.show(label, 'success'); refresh(); after?.(); } catch (e) { toast.show(errorMessage(e), 'danger'); }
  };
  const rotate = async (c: Channel) => {
    if (!(await confirm({ title: `Rotate the secret for “${c.name}”?`, message: 'The old secret stops working immediately. Update your receiver right after.', confirmLabel: 'Rotate secret' }))) return;
    try { setSecret(await api.channels.rotate(c.id)); refresh(); } catch (e) { toast.show(errorMessage(e), 'danger'); }
  };
  const remove = async (c: Channel) => {
    if (!(await confirm({ title: `Delete “${c.name}”?`, message: 'Alerts that used it fall back to in-app delivery.', confirmLabel: 'Delete channel', danger: true }))) return;
    await act('Channel deleted', () => api.channels.remove(c.id));
  };
  const submit = (e: FormEvent) => { e.preventDefault(); setErr(null); create.mutate(undefined); };
  const list = deliveries.data?.pages.flatMap((p: { deliveries: Delivery[] }) => p.deliveries) ?? [];

  return (
    <div className="page">
      <div className="page-head"><div><h1>Delivery</h1><p>Where your alerts go besides this app.</p></div><Button icon="plus" onClick={() => setAdding(true)}>Add webhook</Button></div>
      <div className="stack lg">
        <Card className="row"><Icon name="bell" /><div className="grow"><b>In the app</b><p className="muted small">Always on. Alerts appear live and in your history.</p></div><Badge tone="ok">active</Badge></Card>
        {channels.isLoading && <SkeletonCards n={1} />}
        {(channels.data ?? []).map((c: Channel) => (
          <Card key={c.id} className="stack">
            <div className="row top"><div className="grow stack sm"><div className="row wrap"><h3>{c.name}</h3><Badge tone={c.enabled ? 'ok' : 'warn'}>{c.enabled ? 'active' : 'disabled'}</Badge></div><p className="small muted mono truncate">{c.url}</p>
              <p className="small faint">{c.last_delivery_at ? `Last delivery ${timeAgo(c.last_delivery_at, now)}` : 'No deliveries yet'}{c.consecutive_failures > 0 ? ` · ${c.consecutive_failures} failed in a row` : ''}</p></div></div>
            {!c.enabled && <Banner tone="warn" icon="alert" action={<Button size="sm" onClick={() => void act('Channel enabled', () => api.channels.enable(c.id))}>Re-enable</Button>}>{c.disabled_reason ?? 'This channel was disabled.'}</Banner>}
            <div className="row wrap">
              <Button variant="secondary" size="sm" icon="play" disabled={!c.enabled} onClick={() => void act('Test delivery queued: check the log below', () => api.channels.test(c.id), () => setTimeout(refresh, 2500))}>Send test</Button>
              <Button variant="ghost" size="sm" icon="key" onClick={() => void rotate(c)}>Rotate secret</Button>
              <Button variant="ghost" size="sm" icon="trash" onClick={() => void remove(c)}>Delete</Button>
            </div>
          </Card>
        ))}
        {channels.data && channels.data.length === 0 && <Empty icon="send" title="No webhooks yet" action={<Button onClick={() => setAdding(true)}>Add a webhook</Button>}>Send alerts to your own system (n8n, Zapier, your servers). Deliveries are signed and retried automatically.</Empty>}

        <div className="stack"><h2>Delivery log</h2>
          {list.length === 0 ? <p className="muted">Nothing delivered yet.</p> : (
            <div className="table-wrap"><table className="table"><thead><tr><th>When</th><th>Result</th><th>Attempts</th><th>Detail</th></tr></thead><tbody>
              {list.map((d) => <tr key={d.id}><td>{shortDateTime(d.at)}</td><td><Badge tone={d.ok ? 'ok' : 'danger'}>{d.ok ? `OK ${d.status ?? ''}` : 'Failed'}</Badge></td><td>{d.attempts}</td><td className="small muted">{d.error ?? ''}</td></tr>)}
            </tbody></table></div>)}
          {deliveries.hasNextPage && <div><Button variant="secondary" loading={deliveries.isFetchingNextPage} onClick={() => void deliveries.fetchNextPage()}>Load more</Button></div>}
        </div>
      </div>

      {adding && (
        <Modal title="Add a webhook" onClose={() => setAdding(false)} footer={<><Button variant="secondary" onClick={() => setAdding(false)}>Cancel</Button><Button loading={create.isPending} disabled={!name.trim() || !url.trim()} onClick={() => { setErr(null); create.mutate(undefined); }}>Create webhook</Button></>}>
          <form className="stack" onSubmit={submit}>
            <Field label="Name" htmlFor="ch-name"><input id="ch-name" className="input" value={name} maxLength={60} placeholder="e.g. n8n" onChange={(e) => setName(e.target.value)} /></Field>
            <Field label="HTTPS endpoint" htmlFor="ch-url" help="Must be a public https address (port 443). Private and internal addresses are refused."><input id="ch-url" className="input" type="url" inputMode="url" value={url} placeholder="https://example.com/hooks/tradepulse" onChange={(e) => setUrl(e.target.value)} /></Field>
            {err && <Banner tone="danger" icon="alert">{err}</Banner>}
            <button type="submit" hidden />
          </form>
        </Modal>
      )}
      {secret && <SecretModal channel={secret} onClose={() => setSecret(null)} />}
    </div>
  );
}
