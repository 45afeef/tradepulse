import { Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { api } from '../../api/endpoints';
import type { AuditEntry, Health, Source } from '../../api/types';
import { auditLabel } from '../../lib/audit';
import { money, pct, timeAgo } from '../../lib/format';
import { useDocumentTitle, useTick } from '../../lib/hooks';
import { Badge, Banner, Card, Icon, ProgressBar, SkeletonCards } from '../../ui';
import type { IconName } from '../../ui';

const ATTENTION: Health[] = ['failing', 'overdue', 'degraded'];

function Step({ done, title, to, cta, icon }: { done: boolean; title: string; to: string; cta: string; icon: IconName }) {
  return (
    <div className="row" style={{ padding: '.35rem 0' }}>
      <span className={`badge ${done ? 'ok' : ''}`} style={{ width: 26, height: 26, justifyContent: 'center', padding: 0 }}><Icon name={done ? 'check' : icon} size={14} /></span>
      <span className={`grow ${done ? 'muted' : ''}`} style={done ? { textDecoration: 'line-through' } : undefined}>{title}</span>
      {!done && <Link className="btn sm secondary" to={to}>{cta}</Link>}
    </div>
  );
}

export function AdminOverview() {
  useDocumentTitle('Operator overview');
  const now = useTick();
  const users = useQuery({ queryKey: ['admin', 'users'], queryFn: () => api.admin.users() });
  const invites = useQuery({ queryKey: ['admin', 'invites'], queryFn: () => api.admin.invites() });
  const sources = useQuery({ queryKey: ['admin', 'sources'], queryFn: () => api.admin.sources(), refetchInterval: 30_000 });
  const usage = useQuery({ queryKey: ['admin', 'usage'], queryFn: () => api.admin.usage() });
  const fb = useQuery({ queryKey: ['admin', 'feedback', 30], queryFn: () => api.admin.feedback(30) });
  const audit = useQuery({ queryKey: ['admin', 'audit', 'recent'], queryFn: () => api.admin.audit({ limit: 6 }) });

  const u = users.data ?? [];
  const inv = invites.data ?? [];
  const src = sources.data ?? [];
  const bad = src.filter((s: Source) => s.enabled && ATTENTION.includes(s.health));
  const ratedAlerts = (fb.data?.alert_fires.useful ?? 0) + (fb.data?.alert_fires.not_useful ?? 0);
  const precision = fb.data?.alert_fires.precision;
  const noisy = (fb.data?.groups ?? []).filter((g) => g.useful + g.not_useful >= 3 && (g.precision ?? 1) < 0.5);
  const steps = [
    { done: src.length > 0, title: 'Add your first source', to: '/admin/sources', cta: 'Add source', icon: 'database' as IconName },
    { done: src.some((s: Source) => s.enabled), title: 'Start it', to: '/admin/sources', cta: 'Open sources', icon: 'play' as IconName },
    { done: src.some((s: Source) => s.items_total > 0), title: 'See items flowing in', to: '/admin/sources', cta: 'Check health', icon: 'activity' as IconName },
    { done: inv.length > 0 || u.length > 1, title: 'Invite your first members', to: '/admin/invites', cta: 'Invite', icon: 'mail' as IconName },
  ];
  const doneCount = steps.filter((s) => s.done).length;
  const loading = users.isLoading || sources.isLoading;

  return (
    <div className="page wide">
      <div className="page-head"><div><h1>Operator overview</h1><p>How your workspace is doing.</p></div></div>
      {loading ? <SkeletonCards n={2} /> : (
        <div className="stack lg">
          {doneCount < steps.length && (
            <Card className="accent stack sm">
              <div className="row between"><h3>Get your workspace running</h3><span className="small muted">{doneCount} of {steps.length}</span></div>
              <ProgressBar value={(doneCount / steps.length) * 100} />
              {steps.map((s) => <Step key={s.title} {...s} />)}
            </Card>
          )}
          {bad.length > 0 && <Banner tone="danger" icon="alert" action={<Link className="btn sm" to="/admin/sources">Review</Link>}><b>{bad.length} {bad.length === 1 ? 'source needs' : 'sources need'} attention:</b> {bad.slice(0, 3).map((s: Source) => `${s.name} (${s.health})`).join(', ')}{bad.length > 3 ? '…' : ''}</Banner>}
          {usage.data?.x_configured && usage.data.x_budget_usd > 0 && usage.data.x_spent_usd / usage.data.x_budget_usd > 0.8 && <Banner tone="warn" icon="alert">X API spend is at {pct(usage.data.x_spent_usd / usage.data.x_budget_usd)} of this month&rsquo;s budget; collection pauses at the cap.</Banner>}
          {noisy.length > 0 && <Banner tone="warn" icon="down">Members rate these as mostly not useful: {noisy.slice(0, 3).map((g) => `${g.label} (${pct(g.precision)})`).join(', ')}. Consider tightening or retiring them.</Banner>}

          <div className="grid cols-3">
            <Link to="/admin/members" style={{ textDecoration: 'none', color: 'inherit' }}><Card className="stat interactive"><span className="n">{u.filter((x) => x.status === 'active').length}</span><span className="l">active members{u.some((x) => x.status !== 'active') ? ` · ${u.filter((x) => x.status !== 'active').length} suspended` : ''}</span></Card></Link>
            <Link to="/admin/invites" style={{ textDecoration: 'none', color: 'inherit' }}><Card className="stat interactive"><span className="n">{inv.filter((i) => i.status === 'pending').length}</span><span className="l">invites waiting · {inv.filter((i) => i.status === 'used').length} accepted</span></Card></Link>
            <Link to="/admin/sources" style={{ textDecoration: 'none', color: 'inherit' }}><Card className="stat interactive"><span className="n">{src.filter((s: Source) => s.enabled).length}<span className="muted" style={{ fontSize: '1rem' }}> / {src.length}</span></span><span className="l">sources running{bad.length ? ` · ${bad.length} need attention` : ''}</span></Card></Link>
            <Card className="stat"><span className="n">{ratedAlerts >= 10 ? pct(precision) : 'n/a'}</span><span className="l">{ratedAlerts >= 10 ? `alert precision (target 70%) from ${ratedAlerts} ratings` : `alert precision: ${ratedAlerts} of 10 ratings so far`}</span></Card>
            {usage.data?.x_configured && <Card className="stat"><span className="n">{money(usage.data.x_spent_usd)}</span><span className="l">of {money(usage.data.x_budget_usd)} X budget this month</span><ProgressBar value={usage.data.x_budget_usd ? (usage.data.x_spent_usd / usage.data.x_budget_usd) * 100 : 0} tone={usage.data.x_spent_usd / (usage.data.x_budget_usd || 1) > 0.8 ? 'warn' : undefined} /></Card>}
          </div>

          <div className="grid cols-2">
            <Card className="stack sm">
              <div className="row between"><h3>Recent activity</h3><Link to="/admin/audit" className="small">All activity</Link></div>
              {(audit.data?.entries ?? []).length === 0 && <p className="muted small">Nothing yet.</p>}
              {(audit.data?.entries ?? []).map((e: AuditEntry) => <div key={e.id} className="row small"><Badge>{auditLabel(e.action)}</Badge><span className="grow muted truncate">{e.detail ?? e.target_id ?? ''}</span><span className="faint">{timeAgo(e.at, now)}</span></div>)}
            </Card>
            <Card className="stack sm">
              <h3>Quick actions</h3>
              <div className="row wrap"><Link className="btn secondary sm" to="/admin/invites"><Icon name="mail" size={15} />Invite someone</Link><Link className="btn secondary sm" to="/admin/sources"><Icon name="database" size={15} />Add a source</Link><Link className="btn secondary sm" to="/admin/publish"><Icon name="megaphone" size={15} />Publish an announcement</Link></div>
              <p className="small muted">Health, invites and members refresh on their own; open a page for details.</p>
            </Card>
          </div>
        </div>
      )}
    </div>
  );
}
