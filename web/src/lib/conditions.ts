import type { Condition, FieldDef, Op } from '../api/types';

export const OPS_BY_KIND: Record<FieldDef['kind'], { op: Op; label: string }[]> = {
  number: [{ op: '>', label: 'is above' }, { op: '>=', label: 'is at least' }, { op: '<', label: 'is below' }, { op: '<=', label: 'is at most' }, { op: '==', label: 'equals' }, { op: '!=', label: 'differs from' }],
  string: [{ op: '==', label: 'is' }, { op: '!=', label: 'is not' }, { op: 'contains', label: 'contains' }, { op: 'in', label: 'is one of' }],
  bool: [{ op: '==', label: 'is' }, { op: '!=', label: 'is not' }],
};

export const LIST_OPS: Op[] = ['in', 'any_of'];

/** Turn what the user typed into the JSON value the API expects for this field kind and operator. */
export function parseValue(raw: string, kind: FieldDef['kind'], op: Op): unknown {
  const one = (s: string): unknown => (kind === 'number' ? (s.trim() === '' || Number.isNaN(Number(s)) ? s.trim() : Number(s)) : kind === 'bool' ? s === 'true' : s.trim());
  if (LIST_OPS.includes(op)) return raw.split(',').map((x) => x.trim()).filter(Boolean).map(one);
  return one(raw);
}

export function valueToText(v: unknown): string {
  if (Array.isArray(v)) return v.join(', ');
  return v === undefined || v === null ? '' : String(v);
}

export function describeCondition(c: Condition, fields: FieldDef[]): string {
  const f = fields.find((x) => x.name === c.field);
  const op = (f ? OPS_BY_KIND[f.kind] : OPS_BY_KIND.string).find((o) => o.op === c.op)?.label ?? c.op;
  return `${c.field.replace(/_/g, ' ')} ${op} ${valueToText(c.value)}`;
}

/** A condition row is complete when it names a field and has a non-empty value. */
export function isComplete(c: Condition): boolean {
  return !!c.field && !(c.value === '' || c.value === undefined || (Array.isArray(c.value) && c.value.length === 0));
}
