import type { FeedItem, FeedPage } from '../api/types';

export interface InfiniteData<T> { pages: T[]; pageParams: unknown[] }

/** Put one live item at the top of the first page (no-op when it is already loaded). */
export function prependItem(data: InfiniteData<FeedPage> | undefined, item: FeedItem): InfiniteData<FeedPage> | undefined {
  return mergeItems(data, [item]);
}

/** Prepend several items (given newest first), skipping the ones already loaded. */
export function mergeItems(data: InfiniteData<FeedPage> | undefined, items: FeedItem[]): InfiniteData<FeedPage> | undefined {
  if (!data || data.pages.length === 0) return data;
  const have = new Set(data.pages.flatMap((p) => p.items.map((i) => i.id)));
  const fresh = items.filter((i) => !have.has(i.id));
  if (fresh.length === 0) return data;
  const [first, ...rest] = data.pages;
  return { ...data, pages: [{ ...first!, items: [...fresh, ...first!.items] }, ...rest] };
}

/** Change fields of one item wherever it is loaded (used for optimistic 👍/👎). */
export function patchItem(data: InfiniteData<FeedPage> | undefined, id: string, patch: Partial<FeedItem>): InfiniteData<FeedPage> | undefined {
  if (!data) return data;
  return { ...data, pages: data.pages.map((p) => ({ ...p, items: p.items.map((i) => (i.id === id ? { ...i, ...patch } : i)) })) };
}
