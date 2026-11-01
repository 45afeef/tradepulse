import { useState } from 'react';
import type { FormEvent } from 'react';
import { Link } from 'react-router-dom';
import { errorMessage } from '../api/client';
import { useAuth } from '../auth/AuthContext';
import { Avatar, Badge, Banner, Button, Card, Field, Icon, useConfirm, useTheme, useToast } from '../ui';
import type { ThemePref } from '../ui';
import { useDocumentTitle } from '../lib/hooks';

const THEMES: { id: ThemePref; label: string }[] = [{ id: 'system', label: 'System' }, { id: 'light', label: 'Light' }, { id: 'dark', label: 'Dark' }];

export function SettingsPage() {
  useDocumentTitle('Settings');
  const { user, isAdmin, logout, changePassword } = useAuth();
  const toast = useToast();
  const confirm = useConfirm();
  const [theme, setTheme] = useTheme();
  const [cur, setCur] = useState('');
  const [next, setNext] = useState('');
  const [again, setAgain] = useState('');
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const mismatch = again.length > 0 && again !== next;
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setErr(null); setBusy(true);
    try { await changePassword(cur, next); setCur(''); setNext(''); setAgain(''); toast.show('Password changed. Other devices were signed out.', 'success'); }
    catch (x) { setErr(errorMessage(x)); } finally { setBusy(false); }
  };
  return (
    <div className="page">
      <div className="page-head"><div><h1>Settings</h1></div></div>
      <div className="stack lg">
        <Card className="row">{user && <Avatar email={user.email} />}<div className="grow"><b>{user?.email}</b><p className="muted small">Member since {user ? new Date(user.created_at).toLocaleDateString() : ''}</p></div>{isAdmin && <Badge tone="primary" icon="shield">operator</Badge>}</Card>
        {isAdmin && <Card className="row"><Icon name="shield" /><div className="grow"><b>Operator console</b><p className="muted small">Sources, invites, members and health.</p></div><Link className="btn sm" to="/admin">Open</Link></Card>}
        <Card className="stack sm">
          <h3>Appearance</h3>
          <div className="tabs" role="radiogroup" aria-label="Theme" style={{ alignSelf: 'flex-start' }}>
            {THEMES.map((t) => <button key={t.id} type="button" role="radio" aria-checked={theme === t.id} className="tab" aria-selected={theme === t.id} onClick={() => setTheme(t.id)}>{t.label}</button>)}
          </div>
        </Card>
        <Card>
          <form className="stack" onSubmit={submit}>
            <h3>Change password</h3>
            {err && <Banner tone="danger" icon="alert">{err}</Banner>}
            <Field label="Current password" htmlFor="cur"><input id="cur" className="input" type="password" autoComplete="current-password" value={cur} onChange={(e) => setCur(e.target.value)} /></Field>
            <Field label="New password" htmlFor="new" help="At least 10 characters."><input id="new" className="input" type="password" autoComplete="new-password" minLength={10} value={next} onChange={(e) => setNext(e.target.value)} /></Field>
            <Field label="Repeat new password" htmlFor="again" error={mismatch ? 'The passwords do not match.' : null}><input id="again" className="input" type="password" autoComplete="new-password" aria-invalid={mismatch} value={again} onChange={(e) => setAgain(e.target.value)} /></Field>
            <div><Button type="submit" loading={busy} disabled={!cur || next.length < 10 || next !== again}>Change password</Button></div>
          </form>
        </Card>
        <Card className="stack sm">
          <h3>Sessions</h3>
          <div className="row wrap">
            <Button variant="secondary" icon="logout" onClick={() => void logout()}>Sign out</Button>
            <Button variant="ghost" onClick={async () => { if (await confirm({ title: 'Sign out everywhere?', message: 'Every device, including this one, is signed out immediately.', confirmLabel: 'Sign out everywhere', danger: true })) await logout(true); }}>Sign out everywhere</Button>
          </div>
        </Card>
      </div>
    </div>
  );
}
