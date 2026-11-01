import { NavLink, Link, Outlet } from 'react-router-dom';
import { useAuth } from '../auth/AuthContext';
import { LiveProvider, useLive } from '../live/LiveProvider';
import { Avatar, Button, Icon, useTheme } from '../ui';
import type { IconName } from '../ui';

const MAIN: { to: string; label: string; icon: IconName }[] = [
  { to: '/feed', label: 'Feed', icon: 'feed' },
  { to: '/alerts', label: 'Alerts', icon: 'bell' },
  { to: '/following', label: 'Following', icon: 'list' },
  { to: '/delivery', label: 'Delivery', icon: 'send' },
  { to: '/settings', label: 'Settings', icon: 'sliders' },
];
const ADMIN: { to: string; label: string; icon: IconName; end?: boolean }[] = [
  { to: '/admin', label: 'Overview', icon: 'activity', end: true },
  { to: '/admin/sources', label: 'Sources', icon: 'database' },
  { to: '/admin/invites', label: 'Invites', icon: 'mail' },
  { to: '/admin/members', label: 'Members', icon: 'users' },
  { to: '/admin/publish', label: 'Publish', icon: 'megaphone' },
  { to: '/admin/audit', label: 'Audit log', icon: 'shield' },
];

export function Brand() { return <Link to="/feed" className="brand"><span className="logo"><Icon name="activity" size={18} /></span>TradePulse</Link>; }

function LiveBadge() {
  const { status } = useLive();
  const label = status === 'live' ? 'Live' : status === 'connecting' ? 'Connecting' : 'Offline, retrying';
  return <span className={`live-dot ${status}`} role="status" aria-label={`Connection: ${label}`}><i />{label}</span>;
}

function Shell() {
  const { user, isAdmin } = useAuth();
  const [theme, setTheme] = useTheme();
  const dark = theme === 'dark' || (theme === 'system' && typeof matchMedia !== 'undefined' && matchMedia('(prefers-color-scheme: dark)').matches);
  return (
    <div className="shell">
      <header className="topbar">
        <span className="mobile-only"><Brand /></span>
        <span className="grow" />
        <LiveBadge />
        {isAdmin && <Link to="/admin" className="mobile-only btn ghost sm icon" aria-label="Operator console"><Icon name="shield" /></Link>}
        <Button variant="ghost" size="sm" iconOnly icon={dark ? 'sun' : 'moon'} aria-label={dark ? 'Switch to light theme' : 'Switch to dark theme'} onClick={() => setTheme(dark ? 'light' : 'dark')} />
        <Link to="/settings" aria-label="Account settings">{user && <Avatar email={user.email} />}</Link>
      </header>
      <nav className="nav" aria-label="Main">
        <span className="desktop-only"><Brand /></span>
        {MAIN.map((n) => <NavLink key={n.to} to={n.to} className={({ isActive }: { isActive: boolean }) => (isActive ? 'active' : '')}><Icon name={n.icon} size={20} /><span>{n.label}</span></NavLink>)}
        {isAdmin && <>
          <span className="nav-label">Operator</span>
          {ADMIN.map((n) => <NavLink key={n.to} to={n.to} end={n.end} className={({ isActive }: { isActive: boolean }) => (isActive ? 'active desktop-only' : 'desktop-only')}><Icon name={n.icon} size={20} /><span>{n.label}</span></NavLink>)}
        </>}
        <span className="grow-sep" />
      </nav>
      <main className="content" id="main"><Outlet /></main>
    </div>
  );
}

export function AppShell() { return <LiveProvider><Shell /></LiveProvider>; }
