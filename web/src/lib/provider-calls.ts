import { useInfiniteQuery, type InfiniteData } from '@tanstack/react-query';
import { ApiError, api } from './api';

export type ProviderCall = {
  id: number;
  org_id: string;
  integration_id?: string;
  provider: string;
  operation: string;
  direction: string;
  method: string;
  url: string;
  status_code: number;
  latency_ms: number;
  request_body?: string; // base64
  response_body?: string; // base64
  request_body_text?: string;
  response_body_text?: string;
  error_class?: string;
  error_message?: string;
  trace_id?: string;
  correlation_id?: string;
  occurred_at: string;
};

export type ProviderCallPage = {
  items: ProviderCall[];
  next_cursor?: string;
};

export type ProviderCallFilter = {
  integration_id?: string;
  operation?: string;
  status_min?: number;
  status_max?: number;
  since?: string;
  until?: string;
  limit?: number;
};

// PROVIDER_CALL_OPERATIONS mirrors the Op* constants in
// internal/domain/providercall/entry.go. Keep in sync until we generate
// clients from openapi.yaml.
export const PROVIDER_CALL_OPERATIONS = [
  'send_message',
  'mark_as_read',
  'get_media_url',
  'download_media',
  'list_templates',
  'create_template',
  'get_template_status',
  'upload_media',
] as const;

export type ProviderCallStatusPreset = 'all' | '2xx' | '4xx' | '5xx';

// statusRangeFromPreset maps a UI preset chip to the (min, max) pair the
// backend expects. `all` means no filter — both bounds omitted.
export function statusRangeFromPreset(
  preset: ProviderCallStatusPreset,
): { status_min?: number; status_max?: number } {
  switch (preset) {
    case '2xx':
      return { status_min: 200, status_max: 299 };
    case '4xx':
      return { status_min: 400, status_max: 499 };
    case '5xx':
      return { status_min: 500, status_max: 599 };
    default:
      return {};
  }
}

const PROVIDER_CALLS_KEY = ['provider-calls'] as const;

function buildQueryString(filter: ProviderCallFilter, cursor?: string): string {
  const params = new URLSearchParams();
  if (filter.integration_id !== undefined && filter.integration_id !== '') {
    params.set('integration_id', filter.integration_id);
  }
  if (filter.operation !== undefined && filter.operation !== '') {
    params.set('operation', filter.operation);
  }
  if (filter.status_min !== undefined) {
    params.set('status_min', String(filter.status_min));
  }
  if (filter.status_max !== undefined) {
    params.set('status_max', String(filter.status_max));
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

export function useProviderCalls(filter: ProviderCallFilter) {
  return useInfiniteQuery<
    ProviderCallPage,
    ApiError,
    InfiniteData<ProviderCallPage, string>,
    readonly unknown[],
    string
  >({
    queryKey: [...PROVIDER_CALLS_KEY, filter],
    initialPageParam: '',
    queryFn: async ({ pageParam }) => {
      const qs = buildQueryString(filter, pageParam);
      return api<ProviderCallPage>(`/provider-calls${qs}`);
    },
    getNextPageParam: (last) =>
      last.next_cursor !== undefined && last.next_cursor !== '' ? last.next_cursor : undefined,
    staleTime: 15_000,
    retry: (failureCount, err) => {
      if (err instanceof ApiError && err.status >= 400 && err.status < 500) return false;
      return failureCount < 2;
    },
  });
}
