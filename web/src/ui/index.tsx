import { createContext, useCallback, useContext, useEffect, useId, useMemo, useRef, useState } from 'react';
import type { ButtonHTMLAttributes, CSSProperties, HTMLAttributes, KeyboardEvent, ReactNode } from 'react';
import { Icon } from './Icon';
import type { IconName } from './Icon';
import { hueOf, initials } from '../lib/format';

export { Icon };
export type { IconName };

export function cx(...p: (string | false | null | undefined)[]): string { return p.filter(Boolean).join(' '); }

// ---------- basic ----------
type BtnProps = ButtonHTMLAttributes<HTMLButtonElement> & { variant?: 'primary' | 'secondary' | 'ghost' | 'danger'; size?: 'md' | 'sm'; icon?: IconName; loading?: boolean; block?: boolean; iconOnly?: boolean };
export function Button({ variant = 'primary', size = 'md', icon, loading, block, iconOnly, className, children, disabled, type = 'button', ...rest }: BtnProps) {
  return (
    <button type={type} className={cx('btn', variant !== 'primary' && variant, size === 'sm' && 'sm', block && 'block', iconOnly && 'icon', className)} disabled={disabled || loading} aria-busy={loading || undefined} {...rest}>
      {loading ? <span className="spinner" aria-hidden="true" /> : icon ? <Icon name={icon} size={size === 'sm' ? 15 : 17} /> : null}
      {children}
    </button>
  );
}
export function Spinner({ label = 'Loading' }: { label?: string }) { return <span role="status" aria-label={label} className="spinner" />; }
export function Badge({ tone, children, icon }: { tone?: string; children: ReactNode; icon?: IconName }) { return <span className={cx('badge', tone)}>{icon && <Icon name={icon} size={12} />}{children}</span>; }
export function Chip({ children, onRemove, source }: { children: ReactNode; onRemove?: () => void; source?: string }) {
  return (
    <span className={cx('chip', source && 'source')} style={source ? ({ ['--h' as string]: hueOf(source) } as CSSProperties) : undefined}>
      {children}
      {onRemove && <button type="button" onClick={onRemove} aria-label="Remove"><Icon name="x" size={12} /></button>}
    </span>
  );
}
export function Card({ children, className, ...rest }: { children: ReactNode; className?: string } & HTMLAttributes<HTMLDivElement>) { return <div className={cx('card', className)} {...rest}>{children}</div>; }
export function Skeleton({ h = 14, w = '100%' }: { h?: number; w?: number | string }) { return <div className="skeleton" style={{ height: h, width: w }} aria-hidden="true" />; }
export function SkeletonCards({ n = 3 }: { n?: number }) {
  return <div className="stack" aria-busy="true" aria-label="Loading">{Array.from({ length: n }, (_, i) => (
    <Card key={i}><div className="stack sm"><Skeleton h={12} w="30%" /><Skeleton h={18} w="80%" /><Skeleton h={12} /><Skeleton h={12} w="60%" /></div></Card>))}</div>;
}
export function Empty({ icon = 'info', title, children, action }: { icon?: IconName; title: string; children?: ReactNode; action?: ReactNode }) {
  return <div className="empty"><div className="art"><Icon name={icon} size={26} /></div><h3>{title}</h3>{children && <p style={{ maxWidth: 420 }}>{children}</p>}{action}</div>;
}
export function Banner({ tone = 'info', icon = 'info', children, action }: { tone?: 'info' | 'warn' | 'danger' | 'success'; icon?: IconName; children: ReactNode; action?: ReactNode }) {
  return <div className={cx('banner', tone !== 'info' && tone)} role={tone === 'danger' ? 'alert' : 'status'}><Icon name={icon} size={18} /><div className="grow">{children}</div>{action}</div>;
}
export function Avatar({ email }: { email: string }) { return <span className="avatar" aria-hidden="true">{initials(email)}</span>; }
export function ProgressBar({ value, tone }: { value: number; tone?: 'warn' | 'danger' }) {
  const v = Math.max(0, Math.min(100, value));
  return <div className={cx('progress', tone)} role="progressbar" aria-valuenow={Math.round(v)} aria-valuemin={0} aria-valuemax={100}><div style={{ width: `${v}%` }} /></div>;
}
export function Stepper({ step, total }: { step: number; total: number }) { return <div className="stepper" aria-label={`Step ${step + 1} of ${total}`}>{Array.from({ length: total }, (_, i) => <i key={i} className={i <= step ? 'on' : ''} />)}</div>; }

export function CopyButton({ text, label = 'Copy' }: { text: string; label?: string }) {
  const [done, setDone] = useState(false);
  const toast = useToast();
  return (
    <Button variant="secondary" size="sm" icon={done ? 'check' : 'copy'} onClick={async () => {
      try { await navigator.clipboard.writeText(text); setDone(true); setTimeout(() => setDone(false), 1500); } catch { toast.show('Copy failed: select the text and copy it manually', 'danger'); }
    }}>{done ? 'Copied' : label}</Button>
  );
}

// ---------- forms ----------
export function Field({ label, help, error, children, htmlFor }: { label?: string; help?: ReactNode; error?: string | null; children: ReactNode; htmlFor?: string }) {
  return <div className="field">{label && <label htmlFor={htmlFor}>{label}</label>}{children}{error ? <span className="error-text" role="alert">{error}</span> : help ? <span className="help">{help}</span> : null}</div>;
}
export function Switch({ checked, onChange, label, disabled }: { checked: boolean; onChange: (v: boolean) => void; label: string; disabled?: boolean }) {
  return <label className="switch" title={label}><input type="checkbox" role="switch" checked={checked} disabled={disabled} aria-label={label} onChange={(e) => onChange(e.target.checked)} /><span /></label>;
}
export function Tabs<T extends string>({ value, onChange, items, label = 'Views' }: { value: T; onChange: (v: T) => void; items: { id: T; label: ReactNode }[]; label?: string }) {
  const onKey = (e: KeyboardEvent<HTMLDivElement>) => {
    const i = items.findIndex((x) => x.id === value);
    if (e.key === 'ArrowRight') onChange(items[(i + 1) % items.length]!.id);
    if (e.key === 'ArrowLeft') onChange(items[(i - 1 + items.length) % items.length]!.id);
  };
  return <div className="tabs" role="tablist" aria-label={label} onKeyDown={onKey}>{items.map((it) => <button key={it.id} role="tab" className="tab" aria-selected={it.id === value} tabIndex={it.id === value ? 0 : -1} onClick={() => onChange(it.id)}>{it.label}</button>)}</div>;
}

/** Tag input: Enter, comma or paste adds values; Backspace on empty removes the last. */
export function ChipInput({ value, onChange, placeholder, max = 20, suggestions, label }: { value: string[]; onChange: (v: string[]) => void; placeholder?: string; max?: number; suggestions?: string[]; label: string }) {
  const [text, setText] = useState('');
  const id = useId();
  const add = (raw: string) => {
    const parts = raw.split(/[,\n]/).map((s) => s.trim()).filter(Boolean);
    if (!parts.length) return;
    const next = [...value];
    for (const p of parts) if (!next.some((x) => x.toLowerCase() === p.toLowerCase()) && next.length < max) next.push(p);
    onChange(next); setText('');
  };
  const left = (suggestions ?? []).filter((s) => !value.some((v) => v.toLowerCase() === s.toLowerCase())).slice(0, 8);
  return (
    <div className="stack sm">
      <div className="chip-input" onClick={() => document.getElementById(id)?.focus()}>
        {value.map((v) => <Chip key={v} onRemove={() => onChange(value.filter((x) => x !== v))}>{v}</Chip>)}
        <input id={id} aria-label={label} value={text} placeholder={value.length ? '' : placeholder} onChange={(e) => (/[,\n]/.test(e.target.value) ? add(e.target.value) : setText(e.target.value))}
          onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); add(text); } else if (e.key === 'Backspace' && !text && value.length) onChange(value.slice(0, -1)); }} onBlur={() => add(text)} />
      </div>
      {left.length > 0 && <div className="chips" aria-label="Suggestions">{left.map((s) => <button type="button" key={s} className="chip" style={{ border: 0, cursor: 'pointer' }} onClick={() => add(s)}>+ {s}</button>)}</div>}
    </div>
  );
}

// ---------- overlays ----------
export function Modal({ title, onClose, children, footer, wide }: { title: string; onClose: () => void; children: ReactNode; footer?: ReactNode; wide?: boolean }) {
  const ref = useRef<HTMLDivElement>(null);
  const titleId = useId();
  useEffect(() => {
    const prev = document.activeElement as HTMLElement | null;
    const el = ref.current;
    el?.querySelector<HTMLElement>('input,select,textarea,button:not([data-close])')?.focus();
    document.body.style.overflow = 'hidden';
    return () => { document.body.style.overflow = ''; prev?.focus?.(); };
  }, []);
  const onKey = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.key === 'Escape') { e.stopPropagation(); onClose(); return; }
    if (e.key !== 'Tab' || !ref.current) return;
    const f = Array.from(ref.current.querySelectorAll<HTMLElement>('a[href],button:not([disabled]),input:not([disabled]),select,textarea,[tabindex]:not([tabindex="-1"])'));
    if (!f.length) return;
    const first = f[0]!, last = f[f.length - 1]!;
    if (e.shiftKey && document.activeElement === first) { e.preventDefault(); last.focus(); } else if (!e.shiftKey && document.activeElement === last) { e.preventDefault(); first.focus(); }
  };
  return (
    <div className="overlay" onMouseDown={(e) => { if (e.target === e.currentTarget) onClose(); }}>
      <div className={cx('modal', wide && 'wide')} role="dialog" aria-modal="true" aria-labelledby={titleId} ref={ref} onKeyDown={onKey}>
        <div className="modal-head"><h2 id={titleId}>{title}</h2><Button variant="ghost" size="sm" iconOnly icon="x" aria-label="Close" data-close onClick={onClose} /></div>
        {children}
        {footer && <div className="modal-foot">{footer}</div>}
      </div>
    </div>
  );
}

type Tone = 'default' | 'success' | 'danger' | 'alert';
interface ToastItem { id: number; msg: ReactNode; tone: Tone }
const ToastCtx = createContext<{ show: (msg: ReactNode, tone?: Tone, ms?: number) => void }>({ show: () => {} });
export const useToast = () => useContext(ToastCtx);

interface ConfirmOpts { title: string; message?: ReactNode; confirmLabel?: string; danger?: boolean }
const ConfirmCtx = createContext<(o: ConfirmOpts) => Promise<boolean>>(async () => false);
export const useConfirm = () => useContext(ConfirmCtx);

export function OverlayProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<ToastItem[]>([]);
  const [ask, setAsk] = useState<(ConfirmOpts & { resolve: (v: boolean) => void }) | null>(null);
  const seq = useRef(0);
  const show = useCallback((msg: ReactNode, tone: Tone = 'default', ms = 4500) => {
    const id = ++seq.current;
    setToasts((t) => [...t.slice(-3), { id, msg, tone }]);
    setTimeout(() => setToasts((t) => t.filter((x) => x.id !== id)), ms);
  }, []);
  const confirm = useCallback((o: ConfirmOpts) => new Promise<boolean>((resolve) => setAsk({ ...o, resolve })), []);
  const toastValue = useMemo(() => ({ show }), [show]);
  const close = (v: boolean) => { ask?.resolve(v); setAsk(null); };
  return (
    <ToastCtx.Provider value={toastValue}>
      <ConfirmCtx.Provider value={confirm}>
        {children}
        <div className="toasts" aria-live="polite" aria-atomic="false">{toasts.map((t) => <div key={t.id} className={cx('toast', t.tone !== 'default' && t.tone)} role={t.tone === 'danger' ? 'alert' : 'status'}>{t.msg}</div>)}</div>
        {ask && (
          <Modal title={ask.title} onClose={() => close(false)} footer={<><Button variant="secondary" onClick={() => close(false)}>Cancel</Button><Button variant={ask.danger ? 'danger' : 'primary'} onClick={() => close(true)}>{ask.confirmLabel ?? 'Confirm'}</Button></>}>
            {ask.message && <div className="muted">{ask.message}</div>}
          </Modal>
        )}
      </ConfirmCtx.Provider>
    </ToastCtx.Provider>
  );
}

// ---------- hooks ----------
export function useDebounced<T>(value: T, ms = 450): T {
  const [v, setV] = useState(value);
  useEffect(() => { const t = setTimeout(() => setV(value), ms); return () => clearTimeout(t); }, [value, ms]);
  return v;
}

export type ThemePref = 'system' | 'light' | 'dark';
export function useTheme(): [ThemePref, (t: ThemePref) => void] {
  const [t, setT] = useState<ThemePref>(() => { try { const s = localStorage.getItem('tp.theme'); return s === 'light' || s === 'dark' ? s : 'system'; } catch { return 'system'; } });
  const set = (n: ThemePref) => {
    setT(n);
    try { if (n === 'system') localStorage.removeItem('tp.theme'); else localStorage.setItem('tp.theme', n); } catch { /* storage blocked */ }
    if (n === 'system') delete document.documentElement.dataset.theme; else document.documentElement.dataset.theme = n;
  };
  return [t, set];
}
