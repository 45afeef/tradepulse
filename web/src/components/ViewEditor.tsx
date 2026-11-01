import { useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/endpoints';
import { errorMessage } from '../api/client';
import type { SavedView, ViewFilter } from '../api/types';
import { Button, ChipInput, Field, Modal, useToast } from '../ui';

const WINDOWS: [number, string][] = [[0, 'Any time'], [1, 'Last hour'], [6, 'Last 6 hours'], [24, 'Last 24 hours'], [72, 'Last 3 days'], [168, 'Last 7 days']];

export function ViewEditor({ view, sources, topics, onClose, onSaved }: { view?: SavedView; sources: string[]; topics: string[]; onClose: () => void; onSaved: (v: SavedView) => void }) {
  const qc = useQueryClient();
  const toast = useToast();
  const [name, setName] = useState(view?.name ?? '');
  const [f, setF] = useState<ViewFilter>(view?.filter ?? {});
  const [err, setErr] = useState<string | null>(null);
  const save = useMutation({
    mutationFn: () => (view ? api.views.update(view.id, name, f) : api.views.create(name, f)),
    onSuccess: (v: SavedView) => { void qc.invalidateQueries({ queryKey: ['views'] }); toast.show(view ? 'View updated' : 'View saved', 'success'); onSaved(v); },
    onError: (e: unknown) => setErr(errorMessage(e)),
  });
  const set = (patch: Partial<ViewFilter>) => setF((p) => ({ ...p, ...patch }));
  return (
    <Modal title={view ? 'Edit view' : 'New view'} onClose={onClose} footer={<><Button variant="secondary" onClick={onClose}>Cancel</Button><Button loading={save.isPending} disabled={!name.trim()} onClick={() => { setErr(null); save.mutate(undefined); }}>{view ? 'Save changes' : 'Save view'}</Button></>}>
      <div className="stack">
        <p className="muted small">A view narrows your feed. Items must match every filter you set; within a filter any value matches.</p>
        <Field label="Name"><input className="input" value={name} maxLength={60} placeholder="e.g. Oil and rates" onChange={(e) => setName(e.target.value)} /></Field>
        <Field label="Sources"><ChipInput label="Sources" value={f.sources ?? []} onChange={(v) => set({ sources: v })} suggestions={sources} placeholder="Add a source" /></Field>
        <Field label="Topics"><ChipInput label="Topics" value={f.entities ?? []} onChange={(v) => set({ entities: v })} suggestions={topics} placeholder="Add a topic" /></Field>
        <Field label="Keywords" help="Whole words or phrases in the title or text."><ChipInput label="Keywords" value={f.keywords ?? []} onChange={(v) => set({ keywords: v })} placeholder="Add a word or phrase" /></Field>
        <Field label="Time window"><select className="select" value={f.within_hours ?? 0} onChange={(e) => set({ within_hours: Number(e.target.value) })}>{WINDOWS.map(([h, l]) => <option key={h} value={h}>{l}</option>)}</select></Field>
        {err && <p className="error-text" role="alert">{err}</p>}
      </div>
    </Modal>
  );
}
