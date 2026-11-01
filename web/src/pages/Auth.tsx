import { useState } from 'react';
import type { FormEvent, ReactNode } from 'react';
import { Link, useLocation, useNavigate, useSearchParams } from 'react-router-dom';
import { errorMessage } from '../api/client';
import { useAuth } from '../auth/AuthContext';
import { Banner, Button, Field, Icon } from '../ui';
import { useDocumentTitle } from '../lib/hooks';

function AuthLayout({ title, subtitle, children, footer }: { title: string; subtitle: string; children: ReactNode; footer: ReactNode }) {
  return (
    <div className="auth-wrap">
      <div className="auth-card stack lg">
        <div className="hero">
          <div className="logo"><Icon name="activity" size={26} /></div>
          <h1>{title}</h1>
          <p className="muted" style={{ marginTop: '.3rem' }}>{subtitle}</p>
        </div>
        <div className="card pad-lg">{children}</div>
        <p className="muted small" style={{ textAlign: 'center' }}>{footer}</p>
      </div>
    </div>
  );
}

export function LoginPage() {
  useDocumentTitle('Sign in');
  const { login } = useAuth();
  const nav = useNavigate();
  const loc = useLocation();
  const [email, setEmail] = useState('');
  const [pw, setPw] = useState('');
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setErr(null); setBusy(true);
    try { await login(email, pw); nav((loc.state as { from?: string } | null)?.from ?? '/feed', { replace: true }); }
    catch (x) { setErr(errorMessage(x)); } finally { setBusy(false); }
  };
  return (
    <AuthLayout title="Welcome back" subtitle="Sign in to your live feed." footer={<>Have an invite? <Link to="/register">Create your account</Link></>}>
      <form className="stack" onSubmit={submit} noValidate>
        {err && <Banner tone="danger" icon="alert">{err}</Banner>}
        <Field label="Email" htmlFor="email"><input id="email" className="input" type="email" autoComplete="username" autoFocus required value={email} onChange={(e) => setEmail(e.target.value)} /></Field>
        <Field label="Password" htmlFor="pw"><input id="pw" className="input" type="password" autoComplete="current-password" required value={pw} onChange={(e) => setPw(e.target.value)} /></Field>
        <Button type="submit" block loading={busy} disabled={!email || !pw}>Sign in</Button>
      </form>
    </AuthLayout>
  );
}

export function RegisterPage() {
  useDocumentTitle('Create account');
  const { register } = useAuth();
  const nav = useNavigate();
  const [params] = useSearchParams();
  const [code, setCode] = useState(params.get('invite') ?? '');
  const [email, setEmail] = useState('');
  const [pw, setPw] = useState('');
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const short = pw.length > 0 && pw.length < 10;
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setErr(null); setBusy(true);
    try { await register(email, pw, code); nav('/welcome', { replace: true }); }
    catch (x) { setErr(errorMessage(x)); } finally { setBusy(false); }
  };
  return (
    <AuthLayout title="Join TradePulse" subtitle="One feed for everything you follow, and alerts you can trust." footer={<>Already a member? <Link to="/login">Sign in</Link></>}>
      <form className="stack" onSubmit={submit} noValidate>
        {err && <Banner tone="danger" icon="alert">{err}</Banner>}
        <Field label="Invite code" htmlFor="code" help="TradePulse is invite-only. Your invite came from the person who runs this workspace."><input id="code" className="input mono" autoComplete="off" spellCheck={false} required value={code} onChange={(e) => setCode(e.target.value)} /></Field>
        <Field label="Email" htmlFor="email"><input id="email" className="input" type="email" autoComplete="username" required value={email} onChange={(e) => setEmail(e.target.value)} /></Field>
        <Field label="Password" htmlFor="pw" help="At least 10 characters. A few random words make a strong password." error={short ? 'Use at least 10 characters.' : null}>
          <input id="pw" className="input" type="password" autoComplete="new-password" required minLength={10} aria-invalid={short} value={pw} onChange={(e) => setPw(e.target.value)} />
        </Field>
        <Button type="submit" block loading={busy} disabled={!code || !email || pw.length < 10}>Create account</Button>
      </form>
    </AuthLayout>
  );
}
