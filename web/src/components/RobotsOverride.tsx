import { useState } from 'react';
import { REASON_MAX, REASON_MIN, reasonOk } from '../lib/sources';
import { Banner, Button, Field, Modal } from '../ui';

/**
 * A deliberate, recorded decision: read a page even though the site's robots.txt asks automated tools not to.
 * Requires an explicit acknowledgement and a reason; the server stores both in the audit log.
 */
export function RobotsOverrideModal({ initialReason = '', onApply, onClose, busy }: { initialReason?: string; onApply: (reason: string) => void; onClose: () => void; busy?: boolean }) {
  const [reason, setReason] = useState(initialReason);
  const [agreed, setAgreed] = useState(false);
  const ok = agreed && reasonOk(reason);
  return (
    <Modal title="Override robots.txt?" onClose={onClose}
      footer={<><Button variant="secondary" onClick={onClose}>Cancel</Button><Button variant="danger" loading={busy} disabled={!ok} onClick={() => onApply(reason.trim())}>Override for this source</Button></>}>
      <div className="stack">
        <Banner tone="warn" icon="alert"><b>The site asks automated tools not to read this page.</b> If you override, TradePulse reads it anyway. The site&rsquo;s terms of use still apply, and the site may block or complain.</Banner>
        <p className="muted small">Prefer, in this order: an official feed or API, asking the site for permission, a different source. Override only when you have permission or the terms clearly allow it.</p>
        <Field label="Why is this acceptable?" htmlFor="robots-reason" help={`Recorded in the audit log with your name and the time. ${REASON_MIN} to ${REASON_MAX} characters.`}>
          <textarea id="robots-reason" className="textarea" maxLength={REASON_MAX} value={reason} placeholder="e.g. The publisher confirmed by email on 30 Oct that automated reading of this page is fine." onChange={(e) => setReason(e.target.value)} />
        </Field>
        <label className="check"><input type="checkbox" checked={agreed} onChange={(e) => setAgreed(e.target.checked)} />I have checked the site&rsquo;s terms of use and I am entitled to read this page automatically.</label>
      </div>
    </Modal>
  );
}
