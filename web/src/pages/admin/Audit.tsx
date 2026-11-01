import { useMemo, useState } from 'react';
import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { api } from '../../api/endpoints';
import { errorMessage } from '../../api/client';
import type { AuditEntry } from '../../api/types';
import { auditLabel, toCsv } from '../../lib/audit';
import { shortDateTime } from '../../lib/format';
import { useDocumentTitle } from '../../lib/hooks';
import { Badge, Banner, Button, Empty, SkeletonCards } from '../../ui';

export function AdminAudit() {
  useDocumentTitle('Audit log');
  const [filter, setFilter] = useState('');
  const users = useQuery({ queryKey: ['admin', 'users'], queryFn: () => api.admin.users() });
  const q = useInfiniteQuery({
    queryKey: ['admin', 'audit'],
    queryFn: ({ pageParam }: { pageParam: unknown }) => api.admin.audit({ before: (pageParam as string) || undefined }),
    initialPageParam: '',
    getNextPageParam: (l: { next_cursor?: string }) => l.next_cursor || undefined,
  });
  const who = useMemo(() => new Map((users.data ?? []).map((u) => [u.id, u.email])), [users.data]);
  const all = q.data?.pages.flatMap((p: { entries: AuditEntry[] }) => p.entries) ?? [];
  const f = filter.trim().toLowerCase();
  const rows = f ? all.filter((e) => `${auditLabel(e.action)} ${e.action} ${who.get(e.actor_id) ?? ''} ${e.detail ?? ''} ${e.target_id ?? ''}`.toLowerCase().includes(f)) : all;
  const download = () => {
    const csv = toCsv([['time', 'actor', 'action', 'target_type', 'target_id', 'detail'], ...rows.map((e) => [e.at, who.get(e.actor_id) ?? e.actor_id, e.action, e.target_type ?? '', e.target_id ?? '', e.detail ?? ''])]);
    const url = URL.createObjectURL(new Blob([csv], { type: 'text/csv;charset=utf-8' }));
    const a = document.createElement('a'); a.href = url; a.download = 'tradepulse-audit.csv'; a.click(); URL.revokeObjectURL(url);
  };
  return (
    <div className="page wide">
      <div className="page-head"><div><h1>Audit log</h1><p>Every operator action, newest first. Kept for a year.</p></div>
        <div className="row"><input className="input" style={{ maxWidth: 240 }} type="search" placeholder="Filter" aria-label="Filter audit log" value={filter} onChange={(e) => setFilter(e.target.value)} /><Button variant="secondary" size="sm" icon="copy" disabled={rows.length === 0} onClick={download}>CSV</Button></div></div>
      {q.isLoading && <SkeletonCards n={3} />}
      {q.isError && <Banner tone="danger">{errorMessage(q.error)}</Banner>}
      {q.data && rows.length === 0 && <Empty icon="shield" title={f ? 'Nothing matches' : 'No activity yet'}>{f ? 'Try a different filter, or load more entries.' : 'Operator actions will be recorded here.'}</Empty>}
      {rows.length > 0 && <div className="table-wrap"><table className="table"><thead><tr><th>When</th><th>Who</th><th>What</th><th>Detail</th></tr></thead><tbody>
        {rows.map((e) => <tr key={e.id}><td className="small" style={{ whiteSpace: 'nowrap' }}>{shortDateTime(e.at)}</td><td className="small">{who.get(e.actor_id) ?? <span className="mono faint">{e.actor_id.slice(0, 8)}</span>}</td><td><Badge>{auditLabel(e.action)}</Badge></td><td className="small muted">{e.detail ?? e.target_id ?? ''}</td></tr>)}
      </tbody></table></div>}
      {q.hasNextPage && <div className="row" style={{ justifyContent: 'center', marginTop: '1rem' }}><Button variant="secondary" loading={q.isFetchingNextPage} onClick={() => void q.fetchNextPage()}>Load older entries</Button></div>}
    </div>
  );
}
