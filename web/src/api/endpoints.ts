import { http } from './client';
import type {
  AdminUser, AlertFire, AlertPreview, AlertRule, AlertSpec, AlertTemplate, AuditEntry, CatalogSource, Channel, Delivery, EventTypes,
  FeedbackSummary, FeedItem, FeedPage, Invite, InviteCreated, Profile, Rating, SavedView, Source, SourceDraft, SourceRun, SourceTest,
  SourceUpdate, TokenPair, Usage, User, ViewFilter,
} from './types';

// Every call goes through these four helpers so `npm run contract` can check each path against openapi.json.
const get = <T>(path: string, query?: Record<string, string | number | undefined>) => http.request<T>('GET', path, { query });
const post = <T>(path: string, body?: unknown, auth = true) => http.request<T>('POST', path, { body: body ?? {}, auth });
const put = <T>(path: string, body: unknown) => http.request<T>('PUT', path, { body });
const del = <T = void>(path: string) => http.request<T>('DELETE', path);

export interface PageQuery { limit?: number; before?: string; after?: string }

export const api = {
  auth: {
    login: (email: string, password: string) => post<TokenPair>('/api/v1/auth/login', { email, password }, false),
    register: (email: string, password: string, invite_code: string) => post<TokenPair>('/api/v1/auth/register', { email, password, invite_code }, false),
    logout: (refresh_token: string) => post<void>('/api/v1/auth/logout', { refresh_token }, false),
    logoutAll: () => post<void>('/api/v1/me/logout-all'),
    changePassword: (current_password: string, new_password: string) => post<TokenPair>('/api/v1/me/password', { current_password, new_password }),
    me: () => get<User>('/api/v1/me'),
  },
  catalog: {
    eventTypes: () => get<EventTypes>('/api/v1/event-types'),
    sources: () => get<{ sources: CatalogSource[] }>('/api/v1/sources').then((r) => r.sources),
  },
  feed: {
    list: (q: PageQuery & { view?: string } = {}) => get<FeedPage>('/api/v1/me/feed', { limit: q.limit ?? 30, before: q.before, after: q.after, view: q.view }),
    rate: (id: string, rating: Rating, note?: string) => post<unknown>(`/api/v1/me/feed/${id}/feedback`, { rating, note }),
    clearRating: (id: string) => del(`/api/v1/me/feed/${id}/feedback`),
  },
  views: {
    list: () => get<{ views: SavedView[] }>('/api/v1/me/views').then((r) => r.views),
    create: (name: string, filter: ViewFilter) => post<SavedView>('/api/v1/me/views', { name, filter }),
    update: (id: string, name: string, filter: ViewFilter) => put<SavedView>(`/api/v1/me/views/${id}`, { name, filter }),
    remove: (id: string) => del(`/api/v1/me/views/${id}`),
  },
  subscriptions: {
    get: () => get<Profile>('/api/v1/me/subscriptions'),
    put: (p: Profile) => put<Profile>('/api/v1/me/subscriptions', p),
  },
  alerts: {
    templates: () => get<{ templates: AlertTemplate[] }>('/api/v1/alert-templates').then((r) => r.templates),
    list: () => get<{ rules: AlertRule[] }>('/api/v1/me/alerts').then((r) => r.rules),
    create: (s: AlertSpec) => post<AlertRule>('/api/v1/me/alerts', s),
    fromTemplate: (template_id: string, values: string[], name?: string, channels?: string[]) => post<AlertRule>('/api/v1/me/alerts/from-template', { template_id, values, name, channels }),
    update: (id: string, s: AlertSpec) => put<AlertRule>(`/api/v1/me/alerts/${id}`, s),
    remove: (id: string) => del(`/api/v1/me/alerts/${id}`),
    preview: (s: AlertSpec) => post<AlertPreview>('/api/v1/me/alerts/preview', s),
    fires: (q: PageQuery = {}) => get<{ fires: AlertFire[]; next_cursor?: string }>('/api/v1/me/alerts/fires', { limit: q.limit ?? 30, before: q.before, after: q.after }),
    rate: (id: string, rating: Rating, note?: string) => post<unknown>(`/api/v1/me/alerts/fires/${id}/feedback`, { rating, note }),
    clearRating: (id: string) => del(`/api/v1/me/alerts/fires/${id}/feedback`),
  },
  channels: {
    list: () => get<{ channels: Channel[] }>('/api/v1/me/channels').then((r) => r.channels),
    create: (name: string, url: string) => post<Channel>('/api/v1/me/channels', { type: 'webhook', name, url }),
    remove: (id: string) => del(`/api/v1/me/channels/${id}`),
    rotate: (id: string) => post<Channel>(`/api/v1/me/channels/${id}/rotate-secret`),
    enable: (id: string) => post<Channel>(`/api/v1/me/channels/${id}/enable`),
    test: (id: string) => post<{ queued: boolean }>(`/api/v1/me/channels/${id}/test`),
    deliveries: (q: PageQuery = {}) => get<{ deliveries: Delivery[]; next_cursor?: string }>('/api/v1/me/channels/deliveries', { limit: q.limit ?? 30, before: q.before }),
  },
  live: { ticket: () => post<{ ticket: string; expires_in: number }>('/api/v1/ws-ticket') },
  admin: {
    users: () => get<{ users: AdminUser[] }>('/api/v1/admin/users').then((r) => r.users),
    suspend: (id: string) => post<unknown>(`/api/v1/admin/users/${id}/suspend`),
    reactivate: (id: string) => post<unknown>(`/api/v1/admin/users/${id}/reactivate`),
    resetPassword: (id: string) => post<{ temporary_password: string }>(`/api/v1/admin/users/${id}/reset-password`),
    invites: () => get<{ invites: Invite[] }>('/api/v1/admin/invites').then((r) => r.invites),
    createInvite: (b: { email?: string; expires_in_hours?: number; note?: string }) => post<InviteCreated>('/api/v1/admin/invites', b),
    revokeInvite: (id: string) => del(`/api/v1/admin/invites/${id}`),
    sources: () => get<{ sources: Source[] }>('/api/v1/admin/sources').then((r) => r.sources),
    source: (id: string) => get<{ source: Source; recent_runs: SourceRun[] }>(`/api/v1/admin/sources/${id}`),
    testSource: (d: SourceDraft) => post<SourceTest>('/api/v1/admin/sources/test', d),
    createSource: (d: SourceDraft) => post<Source>('/api/v1/admin/sources', d),
    updateSource: (id: string, d: SourceUpdate) => put<Source>(`/api/v1/admin/sources/${id}`, d),
    deleteSource: (id: string) => del(`/api/v1/admin/sources/${id}`),
    runSource: (id: string) => post<unknown>(`/api/v1/admin/sources/${id}/run`),
    webhookSecret: (id: string) => post<Source>(`/api/v1/admin/sources/${id}/webhook-secret`),
    postItem: (b: { source: string; title: string; summary?: string; url?: string; entities?: string[] }) => post<{ event_id: string }>('/api/v1/admin/items', b),
    audit: (q: PageQuery = {}) => get<{ entries: AuditEntry[]; next_cursor?: string }>('/api/v1/admin/audit', { limit: q.limit ?? 50, before: q.before }),
    usage: () => get<Usage>('/api/v1/admin/usage'),
    feedback: (days = 30) => get<FeedbackSummary>('/api/v1/admin/feedback/summary', { days }),
  },
};

export type { FeedItem };
