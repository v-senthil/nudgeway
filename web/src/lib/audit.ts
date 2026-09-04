import { useInfiniteQuery, type InfiniteData } from '@tanstack/react-query';
import { ApiError, api } from './api';

export type AuditLog = {
  id: string;
  org_id: string;
  actor_user_id?: string;
  action: string;
  resource_type: string;
  resource_id?: string;
  ip?: string;
  metadata?: Record<string, unknown>;
  occurred_at: string;
};

export type AuditLogPage = {
  items: AuditLog[];
  next_cursor?: string;
};

export type AuditLogFilter = {
  resource_type?: string;
  resource_id?: string;
  action?: string;
  actor_user_id?: string;
  since?: string;
  until?: string;
  limit?: number;
};

const AUDIT_KEY = ['audit-logs'] as const;

function buildQueryString(filter: AuditLogFilter, cursor?: string): string {
  const params = new URLSearchParams();
  if (filter.resource_type !== undefined && filter.resource_type !== '') {
    params.set('resource_type', filter.resource_type);
  }
  if (filter.resource_id !== undefined && filter.resource_id !== '') {
    params.set('resource_id', filter.resource_id);
  }
  if (filter.action !== undefined && filter.action !== '') {
    params.set('action', filter.action);
  }
  if (filter.actor_user_id !== undefined && filter.actor_user_id !== '') {
    params.set('actor_user_id', filter.actor_user_id);
  }
  if (filter.since !== undefined && filter.since !== '') {
    params.set('since', filter.since);
  }
  if (filter.until !== undefined && filter.until !== '') {
    params.set('until', filter.until);
  }
  if (filter.limit !== undefined) {
    params.set('limit', String(filter.limit));
  }
  if (cursor !== undefined && cursor !== '') {
    params.set('cursor', cursor);
  }
  const q = params.toString();
  return q === '' ? '' : `?${q}`;
}

// AUDIT_ACTIONS mirrors the constants declared in
// internal/domain/audit/entry.go. Keep the two lists in sync; a future
// codegen pass off openapi.yaml will remove this duplication.
export const AUDIT_ACTIONS = [
  'integration.created',
  'integration.deleted',
  'integration.tested',
  'message.sent',
  'message.marked_read',
  'conversation.marked_read',
  'attachment.uploaded',
  'user.logged_in',
  'user.logged_out',
] as const;

export const AUDIT_RESOURCE_TYPES = [
  'integration',
  'message',
  'conversation',
  'attachment',
  'session',
] as const;

export function useAuditLogs(filter: AuditLogFilter) {
  return useInfiniteQuery<AuditLogPage, ApiError, InfiniteData<AuditLogPage, string>, readonly unknown[], string>({
    queryKey: [...AUDIT_KEY, filter],
    initialPageParam: '',
    queryFn: async ({ pageParam }) => {
      const qs = buildQueryString(filter, pageParam);
      return api<AuditLogPage>(`/audit-logs${qs}`);
    },
    getNextPageParam: (last) =>
      last.next_cursor !== undefined && last.next_cursor !== '' ? last.next_cursor : undefined,
    staleTime: 30_000,
    retry: (failureCount, err) => {
      if (err instanceof ApiError && err.status >= 400 && err.status < 500) return false;
      return failureCount < 2;
    },
  });
}
