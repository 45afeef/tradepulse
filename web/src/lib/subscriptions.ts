import type { ScopeKind, Subscription } from '../api/types';

export const SECTIONS: { kind: ScopeKind; title: string; hint: string; placeholder: string }[] = [
  { kind: 'source', title: 'Sources', hint: 'Everything these sources publish.', placeholder: 'Add a source' },
  { kind: 'entity', title: 'Topics', hint: 'Items tagged with these topics.', placeholder: 'Add a topic' },
  { kind: 'keyword', title: 'Keywords', hint: 'Whole words or phrases in titles and text.', placeholder: 'Add a word or phrase' },
];
/** Build the subscription list from the three editors, keeping advanced settings (types, conditions) of entries that stay. */
export function mergeSubscriptions(existing: Subscription[], byKind: Record<ScopeKind, string[]>): Subscription[] {
  const out: Subscription[] = [];
  for (const { kind } of SECTIONS) {
    for (const v of byKind[kind]) {
      const keep = existing.find((s) => s.kind === kind && s.value.toLowerCase() === v.toLowerCase());
      out.push(keep ?? { kind, value: v });
    }
  }
  return out;
}

