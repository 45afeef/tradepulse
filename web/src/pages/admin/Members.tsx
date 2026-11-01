import { useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../../api/endpoints';
import { errorMessage } from '../../api/client';
import type { AdminUser } from '../../api/types';
import { useAuth } from '../../auth/AuthContext';
import { shortDateTime } from '../../lib/format';
import { useDocumentTitle } from '../../lib/hooks';
import { Avatar, Badge, Banner, Button, CopyButton, Empty, Modal, SkeletonCards, useConfirm, useToast } from '../../ui';

export function AdminMembers() {
  useDocumentTitle('Members');
  const qc = useQueryClient();
  const toast = useToast();
  const confirm = useConfirm();
  const { user: me } = useAuth();
  const [filter, setFilter] = useState('');
  const [temp, setTemp] = useState<{ email: string; password: string } | null>(null);
  const q = useQuery({ queryKey: ['admin', 'users'], queryFn: () => api.admin.users() });
  const rows = (q.data ?? []).filter((u: AdminUser) => u.email.toLowerCase().includes(filter.trim().toLowerCase()));
  const done = (m: string) => { toast.show(m, 'success'); void qc.invalidateQueries({ queryKey: ['admin', 'users'] }); };

  const suspend = async (u: AdminUser) => {
    if (!(await confirm({ title: `Suspend ${u.email}?`, message: 'They are signed out and cannot sign in or receive alerts until you reactivate them.', confirmLabel: 'Suspend', danger: true }))) return;
    try { await api.admin.suspend(u.id); done('Member suspended'); } catch (e) { toast.show(errorMessage(e), 'danger'); }
  };
  const reactivate = async (u: AdminUser) => { try { await api.admin.reactivate(u.id); done('Member reactivated'); } catch (e) { toast.show(errorMessage(e), 'danger'); } };
  const reset = async (u: AdminUser) => {
    if (!(await confirm({ title: `Reset the password for ${u.email}?`, message: 'A temporary password is generated and shown once. They are signed out everywhere.', confirmLabel: 'Reset password' }))) return;
    try { const r = await api.admin.resetPassword(u.id); setTemp({ email: u.email, password: r.temporary_password }); void qc.invalidateQueries({ queryKey: ['admin', 'audit'] }); } catch (e) { toast.show(errorMessage(e), 'danger'); }
  };

  return (
    <div className="page wide">
      <div className="page-head"><div><h1>Members</h1><p>{q.data ? `${q.data.length} in total` : ' '}</p></div><input className="input" style={{ maxWidth: 260 }} type="search" placeholder="Search by email" aria-label="Search members" value={filter} onChange={(e) => setFilter(e.target.value)} /></div>
      {q.isLoading && <SkeletonCards n={3} />}
      {q.isError && <Banner tone="danger">{errorMessage(q.error)}</Banner>}
      {q.data && rows.length === 0 && <Empty icon="users" title={filter ? 'No one matches' : 'No members yet'}>{filter ? 'Try a different search.' : 'Invite people from the Invites page.'}</Empty>}
      {rows.length > 0 && <div className="table-wrap"><table className="table"><thead><tr><th>Member</th><th>Role</th><th>Status</th><th>Joined</th><th /></tr></thead><tbody>
        {rows.map((u: AdminUser) => {
          const self = u.id === me?.id;
          return (
            <tr key={u.id}>
              <td><div className="row"><Avatar email={u.email} /><span>{u.email}{self && <span className="faint"> (you)</span>}</span></div></td>
              <td><Badge tone={u.role === 'admin' ? 'primary' : undefined}>{u.role === 'admin' ? 'operator' : 'member'}</Badge></td>
              <td><Badge tone={u.status === 'active' ? 'ok' : 'danger'}>{u.status}</Badge></td>
              <td className="small">{shortDateTime(u.created_at)}</td>
              <td><div className="row wrap" style={{ justifyContent: 'flex-end' }}>
                <Button variant="ghost" size="sm" icon="key" disabled={self} onClick={() => void reset(u)}>Reset password</Button>
                {u.status === 'active' ? <Button variant="ghost" size="sm" disabled={self} onClick={() => void suspend(u)}>Suspend</Button> : <Button variant="secondary" size="sm" onClick={() => void reactivate(u)}>Reactivate</Button>}
              </div></td>
            </tr>);
        })}
      </tbody></table></div>}
      {temp && (
        <Modal title="Temporary password" onClose={() => setTemp(null)} footer={<Button onClick={() => setTemp(null)}>Done</Button>}>
          <div className="stack"><Banner tone="warn" icon="key">Shown <b>once</b>. Give it to {temp.email} through a secure channel and ask them to change it under Settings.</Banner><div className="code">{temp.password}</div><div><CopyButton text={temp.password} label="Copy password" /></div></div>
        </Modal>
      )}
    </div>
  );
}
