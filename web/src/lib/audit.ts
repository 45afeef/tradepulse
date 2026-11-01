/** "source.create" -> "Source created"; unknown actions are shown readably instead of hidden. */
const VERBS: Record<string, string> = {
  create: 'created', update: 'updated', delete: 'deleted', run: 'run', suspend: 'suspended', reactivate: 'reactivated', reset_password: 'password reset', revoke: 'revoked',
  rotate_secret: 'secret rotated', logout_all: 'signed out everywhere', change_password: 'password changed', post: 'posted', login: 'signed in',
};
export function auditLabel(action: string): string {
  const [area = '', ...rest] = action.split('.');
  const verb = rest.join('.');
  const noun = area.replace(/_/g, ' ');
  const v = VERBS[verb] ?? verb.replace(/_/g, ' ');
  const text = v ? `${noun} ${v}` : noun;
  return text.charAt(0).toUpperCase() + text.slice(1);
}

/** RFC 4180 CSV with spreadsheet-formula neutralisation (a leading = + - @ is prefixed with an apostrophe). */
export function toCsv(rows: string[][]): string {
  const cell = (v: string) => { const safe = /^[=+\-@\t\r]/.test(v) ? `'${v}` : v; return /[",\n\r]/.test(safe) ? `"${safe.replace(/"/g, '""')}"` : safe; };
  return rows.map((r) => r.map(cell).join(',')).join('\r\n');
}
