import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../api/endpoints';
import { errorMessage } from '../../api/client';
import type { Invite, InviteCreated, InviteStatus } from '../../api/types';
import { plural, shortDateTime } from '../../lib/format';
import { useDocumentTitle } from '../../lib/hooks';
import { Badge, Banner, Button, Card, CopyButton, Empty, Field, Modal, SkeletonCards, Tabs, useConfirm, useToast } from '../../ui';

const EXPIRY: [number, string][] = [[24, '1 day'], [72, '3 days'], [168, '7 days'], [720, '30 days']];

export function AdminInvites() {
  useDocumentTitle('Invites');
  const qc = useQueryClient();
  const toast = useToast();
  const confirm = useConfirm();
  const [open, setOpen] = useState(false);
  const [email, setEmail] = useState('');
  const [note, setNote] = useState('');
  const [hours, setHours] = useState(168);
  const [err, setErr] = useState<string | null>(null);
  const [made, setMade] = useState<InviteCreated | null>(null);
  const [tab, setTab] = useState<'all' | InviteStatus>('all');
  const q = useQuery({ queryKey: ['admin', 'invites'], queryFn: () => api.admin.invites() });
  const all = q.data ?? [];
  const count = (s: InviteStatus) => all.filter((i: Invite) => i.status === s).length;
  const shown = tab === 'all' ? all : all.filter((i: Invite) => i.status === tab);

  const create = useMutation({
    mutationFn: () => api.admin.createInvite({ email: email.trim() || undefined, note: note.trim() || undefined, expires_in_hours: hours }),
    onSuccess: (r: InviteCreated) => { setOpen(false); setEmail(''); setNote(''); setMade(r); void qc.invalidateQueries({ queryKey: ['admin', 'invites'] }); },
    onError: (e: unknown) => setErr(errorMessage(e)),
  });
  const revoke = async (i: Invite) => {
    if (!(await confirm({ title: 'Revoke this invite?', message: 'The code stops working immediately.', confirmLabel: 'Revoke', danger: true }))) return;
    try { await api.admin.revokeInvite(i.id); void qc.invalidateQueries({ queryKey: ['admin', 'invites'] }); toast.show('Invite revoked'); } catch (e) { toast.show(errorMessage(e), 'danger'); }
  };
  const link = made ? `${location.origin}/register?invite=${encodeURIComponent(made.code)}` : '';
  const canShare = typeof navigator !== 'undefined' && 'share' in navigator;

  return (
    <div className="page wide">
      <div className="page-head"><div><h1>Invites</h1><p>TradePulse is invite-only. Each code works once.</p></div><Button icon="plus" onClick={() => setOpen(true)}>New invite</Button></div>
      <div className="grid cols-3" style={{ marginBottom: '1rem' }}>
        <Card className="stat"><span className="n">{count('pending')}</span><span className="l">waiting to be used</span></Card>
        <Card className="stat"><span className="n">{count('used')}</span><span className="l">accepted</span></Card>
        <Card className="stat"><span className="n">{count('expired') + count('revoked')}</span><span className="l">expired or revoked</span></Card>
      </div>
      <div style={{ marginBottom: '1rem' }}><Tabs label="Invite status" value={tab} onChange={setTab} items={[{ id: 'all', label: 'All' }, { id: 'pending', label: 'Pending' }, { id: 'used', label: 'Accepted' }, { id: 'expired', label: 'Expired' }, { id: 'revoked', label: 'Revoked' }]} /></div>
      {q.isLoading && <SkeletonCards n={2} />}
      {q.isError && <Banner tone="danger">{errorMessage(q.error)}</Banner>}
      {q.data && all.length === 0 && <Empty icon="mail" title="Invite your first members" action={<Button icon="plus" onClick={() => setOpen(true)}>Create an invite</Button>}>Create a code, send the link, and they are in. Start with a handful of people who will give you honest feedback.</Empty>}
      {shown.length > 0 && <div className="table-wrap"><table className="table"><thead><tr><th>For</th><th>Status</th><th>Created</th><th>Expires</th><th /></tr></thead><tbody>
        {shown.map((i: Invite) => <tr key={i.id}><td>{i.email || <span className="muted">anyone with the link</span>}{i.note && <div className="small faint">{i.note}</div>}</td><td><Badge tone={i.status}>{i.status === 'used' ? 'accepted' : i.status}</Badge></td><td className="small">{shortDateTime(i.created_at)}</td><td className="small">{shortDateTime(i.expires_at)}</td><td>{i.status === 'pending' && <Button variant="ghost" size="sm" onClick={() => void revoke(i)}>Revoke</Button>}</td></tr>)}
      </tbody></table></div>}

      {open && (
        <Modal title="New invite" onClose={() => setOpen(false)} footer={<><Button variant="secondary" onClick={() => setOpen(false)}>Cancel</Button><Button loading={create.isPending} onClick={() => { setErr(null); create.mutate(undefined); }}>Create invite</Button></>}>
          <div className="stack">
            <Field label="Email (optional)" htmlFor="inv-email" help="If set, only this address can use the invite."><input id="inv-email" className="input" type="email" value={email} onChange={(e) => setEmail(e.target.value)} placeholder="name@example.com" /></Field>
            <Field label="Note (optional)" htmlFor="inv-note" help="Only you see this: who it is for, why."><input id="inv-note" className="input" maxLength={200} value={note} onChange={(e) => setNote(e.target.value)} /></Field>
            <Field label="Valid for" htmlFor="inv-exp"><select id="inv-exp" className="select" value={hours} onChange={(e) => setHours(Number(e.target.value))}>{EXPIRY.map(([h, l]) => <option key={h} value={h}>{l}</option>)}</select></Field>
            {err && <Banner tone="danger" icon="alert">{err}</Banner>}
          </div>
        </Modal>
      )}
      {made && (
        <Modal title="Invite ready" onClose={() => setMade(null)} footer={<Button onClick={() => setMade(null)}>Done</Button>}>
          <div className="stack">
            <Banner tone="success" icon="check">Valid for {plural(Math.round((Date.parse(made.expires_at) - Date.now()) / 3_600_000), 'hour')}. The code is shown <b>once</b>.</Banner>
            <Field label="Signup link"><div className="code">{link}</div></Field>
            <div className="row wrap"><CopyButton text={link} label="Copy link" />{canShare && <Button variant="secondary" size="sm" icon="send" onClick={() => void navigator.share({ title: 'Your TradePulse invite', url: link }).catch(() => {})}>Share…</Button>}<CopyButton text={made.code} label="Copy code only" /></div>
          </div>
        </Modal>
      )}
    </div>
  );
}
