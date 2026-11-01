// Hand-written mirror of the API contract (docs/API.md). `npm run contract` verifies that every
// path the client calls exists in openapi.json; replace with generated types when convenient.

export type Role = 'member' | 'admin';
export interface User { id: string; email: string; role: Role; created_at: string }
export interface TokenPair { token: string; refresh_token: string; expires_in: number; user: User }

export type Op = '>' | '>=' | '<' | '<=' | '==' | '!=' | 'in' | 'contains' | 'any_of';
export interface Condition { field: string; op: Op; value: unknown }
export type ScopeKind = 'entity' | 'keyword' | 'source';

export interface Subscription { kind: ScopeKind; value: string; types?: string[]; conditions?: Condition[] }
export interface Profile { subscriptions?: Subscription[]; muted?: string[] }

export type Rating = 'useful' | 'not_useful';
export type Severity = 'info' | 'notable' | 'high';

export interface FeedItem {
  id: string; cursor: string; event_id: string; event_type: string; source: string; entities: string[];
  title: string; body: string; url?: string; severity: Severity; reason: string; created_at: string; feedback?: Rating;
}
export interface Page<K extends string, T> { next_cursor?: string; order: 'asc' | 'desc'; items?: T[] }
export interface FeedPage { items: FeedItem[]; next_cursor?: string; order: 'asc' | 'desc' }

export interface FieldDef { name: string; kind: 'number' | 'string' | 'bool'; doc?: string }
export interface TypeDef { id: string; description?: string; fields: FieldDef[]; title_template: string; body_template: string; severity: string }
export interface EventTypes { types: TypeDef[]; builtin_fields: FieldDef[] }
export interface CatalogSource { name: string; type: string; entities: string[] }

export interface AlertSpec {
  name: string; scope: { kind: ScopeKind; values: string[] }; types?: string[]; conditions?: Condition[];
  cooldown_seconds?: number; severity?: Severity; channels?: string[]; enabled: boolean;
}
export interface AlertRule extends AlertSpec {
  id: string; types: string[]; conditions: Condition[]; cooldown_seconds: number; severity: Severity; channels: string[];
  paused_reason?: string; created_at: string; updated_at: string; last_fired_at?: string;
}
export interface AlertTemplate {
  id: string; name: string; description: string; scope_kind: ScopeKind; values_hint: string; types?: string[];
  conditions?: Condition[]; cooldown_seconds: number; severity: Severity;
}
export interface AlertPreview {
  examined: number; would_fire: number; would_match: number; would_pause: boolean; examples: { at: string; title: string }[];
}
export interface AlertFire {
  id: string; rule_id: string; event_id: string; rule_name: string; severity: Severity; title: string; body: string; url?: string;
  source: string; entities: string[]; reason: string; fired_at: string; cursor: string; feedback?: Rating;
}

export interface Channel {
  id: string; type: 'webhook'; name: string; url: string; enabled: boolean; consecutive_failures: number;
  disabled_reason?: string; last_delivery_at?: string; created_at: string; secret?: string;
}
export interface Delivery { id: string; channel_id: string; fire_id: string; at: string; ok: boolean; status?: number; attempts: number; error?: string; cursor: string }

export interface ViewFilter { sources?: string[]; entities?: string[]; keywords?: string[]; types?: string[]; within_hours?: number }
export interface SavedView { id: string; name: string; filter: ViewFilter; created_at: string; updated_at: string }

// ---- admin ----
export type SourceType = 'rss' | 'webmonitor' | 'manual' | 'webhook' | 'x';
export type Health = 'ok' | 'pending' | 'degraded' | 'failing' | 'overdue' | 'disabled';
export interface WebConfig {
  item_selector: string; title_selector?: string; link_selector?: string; summary_selector?: string; max_items?: number;
  ignore_robots?: boolean; ignore_robots_reason?: string;
}
export interface SourceDraft {
  name: string; type: SourceType; url?: string; enabled?: boolean; interval_seconds?: number; entities?: string[];
  web?: WebConfig; webhook?: { auth: 'hmac' | 'token' }; x?: { handle: string; user_id?: string };
}
export interface SourceUpdate { url?: string; enabled: boolean; interval_seconds?: number; entities?: string[]; web?: WebConfig; webhook?: { auth: 'hmac' | 'token' }; x?: { handle: string; user_id?: string } }
export interface Source extends SourceDraft {
  id: string; enabled: boolean; interval_seconds: number; entities: string[]; webhook_path?: string; webhook_secret?: string;
  health: Health; last_run_at?: string; last_success_at?: string; next_run_at?: string; last_item_at?: string; last_error?: string;
  consecutive_failures: number; items_total: number; created_at: string;
}
export interface SourceRun { started_at: string; duration_ms: number; ok: boolean; http_status?: number; items_seen: number; items_new: number; items_published: number; error?: string }
export interface SourceTest { ok: boolean; error?: string; count: number; preview: { title: string; url?: string; summary?: string; published_at?: string }[] }
export interface AdminUser { id: string; email: string; role: Role; status: 'active' | 'suspended' | string; created_at: string }
export type InviteStatus = 'pending' | 'used' | 'expired' | 'revoked';
export interface Invite { id: string; email?: string; note?: string; created_by: string; created_at: string; expires_at: string; status: InviteStatus; used_by?: string; used_at?: string }
export interface InviteCreated { id: string; code: string; email?: string; expires_at: string }
export interface AuditEntry { id: string; at: string; actor_id: string; action: string; target_type?: string; target_id?: string; detail?: string; cursor: string }
export interface Usage { month: string; x_configured: boolean; x_budget_usd: number; x_spent_usd: number; x_remaining_usd: number; x_cost_per_resource_usd: number }
export interface Totals { useful: number; not_useful: number; precision?: number }
export interface FeedbackSummary {
  days: number; feed_items: Totals; alert_fires: Totals;
  groups: { target_type: 'feed_item' | 'alert_fire'; group: string; label: string; useful: number; not_useful: number; precision?: number }[];
}

export interface ProblemFieldError { location?: string; message?: string; value?: unknown }
