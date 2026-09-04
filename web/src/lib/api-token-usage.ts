import { useInfiniteQuery, useQuery, type InfiniteData } from '@tanstack/react-query';
import { ApiError, api } from './api';

// UsageEntry is one row from GET /api/v1/api-tokens/{id}/usage.
// Bodies may be truncated / JSON-redacted server-side.
export type UsageEntry = {
  id: string;
  occurred_at: string;
  request_id: string;
  method: string;
  path: string;
  status_code: number;
  latency_ms: number;
  remote_ip: string;
  user_agent?: string;
  request_body?: string;
  response_body?: string;
  error_message?: string;
};

export type UsagePage = {
  items: UsageEntry[];
  next_cursor?: string;
};

export type UsageFilter = {
  since?: string;
  until?: string;
  status_min?: number;
  status_max?: number;
  limit?: number;
};

export type MetricsRange = {
  from: string;
  to: string;
};

export type MetricsByDay = {
  day: string;
  requests: number;
  errors: number;
};

export type MetricsTopPath = {
  path: string;
  count: number;
};

export type TokenMetrics = {
  total_requests: number;
  error_count: number;
  avg_latency_ms: number;
  bytes_in: number;
  bytes_out: number;
  by_day: MetricsByDay[];
  by_status: Record<string, number>;
  top_paths: MetricsTopPath[];
};

const API_TOKEN_METRICS_KEY = ['api-token-metrics'] as const;
const API_TOKEN_USAGE_KEY = ['api-token-usage'] as const;

function retryPolicy(failureCount: number, err: unknown): boolean {
  if (err instanceof ApiError && err.status >= 400 && err.status < 500) return false;
  return failureCount < 2;
}

function toIsoStart(day: string): string {
  return `${day}T00:00:00Z`;
}

function toIsoEnd(day: string): string {
  return `${day}T23:59:59Z`;
}

// useAPITokenMetrics loads aggregate metrics for one token over a
// YYYY-MM-DD range. The hook translates the range into ISO instants
// covering the full days on the wire.
export function useAPITokenMetrics(id: string | null, range: MetricsRange) {
  return useQuery<TokenMetrics, ApiError>({
    queryKey: [...API_TOKEN_METRICS_KEY, id, range],
    enabled: id !== null && id !== '',
    queryFn: async () => {
      const params = new URLSearchParams({
        since: toIsoStart(range.from),
        until: toIsoEnd(range.to),
      }).toString();
      return api<TokenMetrics>(`/api-tokens/${id ?? ''}/metrics?${params}`);
    },
    staleTime: 30_000,
    retry: retryPolicy,
  });
}

function buildUsageQuery(filter: UsageFilter, cursor: string): string {
  const params = new URLSearchParams();
  if (filter.since !== undefined && filter.since !== '') params.set('since', filter.since);
  if (filter.until !== undefined && filter.until !== '') params.set('until', filter.until);
  if (filter.status_min !== undefined) params.set('status_min', String(filter.status_min));
  if (filter.status_max !== undefined) params.set('status_max', String(filter.status_max));
  if (filter.limit !== undefined) params.set('limit', String(filter.limit));
  if (cursor !== '') params.set('cursor', cursor);
  const q = params.toString();
  return q === '' ? '' : `?${q}`;
}

// useAPITokenUsage is a cursor-paginated infinite query over the raw
// execution log. Callers concatenate pages themselves.
export function useAPITokenUsage(id: string | null, filter: UsageFilter) {
  return useInfiniteQuery<
    UsagePage,
    ApiError,
    InfiniteData<UsagePage, string>,
    readonly unknown[],
    string
  >({
    queryKey: [...API_TOKEN_USAGE_KEY, id, filter],
    enabled: id !== null && id !== '',
    initialPageParam: '',
    queryFn: async ({ pageParam }) => {
      const qs = buildUsageQuery(filter, pageParam);
      return api<UsagePage>(`/api-tokens/${id ?? ''}/usage${qs}`);
    },
    getNextPageParam: (last) =>
      last.next_cursor !== undefined && last.next_cursor !== '' ? last.next_cursor : undefined,
    staleTime: 15_000,
    retry: retryPolicy,
  });
}

// defaultMetricsRange returns the last 14 days ending today as YYYY-MM-DD.
export function defaultMetricsRange(): MetricsRange {
  const today = new Date();
  const from = new Date(today);
  from.setDate(from.getDate() - 13);
  return { from: toISODate(from), to: toISODate(today) };
}

function toISODate(d: Date): string {
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, '0');
  const day = String(d.getDate()).padStart(2, '0');
  return `${y}-${m}-${day}`;
}
