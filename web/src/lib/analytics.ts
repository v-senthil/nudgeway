import { useQuery } from '@tanstack/react-query';
import { ApiError, api } from './api';

// AnalyticsOverview mirrors the JSON body of GET /api/v1/analytics/overview.
export type AnalyticsOverview = {
  messages_total: number;
  delivery_rate_pct: number;
  response_time_seconds_p50: number;
  conversations_opened: number;
  // Call KPIs — sibling counts of messages_total. Marked optional so
  // older backend deploys that haven't rolled the calls block still
  // render (cards fall back to "—").
  calls_total?: number;
  calls_answered?: number;
  calls_avg_duration_seconds?: number;
};

// SeriesKind mirrors internal/domain/analytics.SeriesKind.
export type SeriesKind =
  | 'messages_daily'
  | 'delivery_rate'
  | 'conversations_opened'
  | 'calls_daily';

// AnalyticsPoint is one day's value in a time series.
export type AnalyticsPoint = {
  day: string; // YYYY-MM-DD, UTC.
  value: number;
};

// AnalyticsSeries is the JSON body of GET /api/v1/analytics/series.
export type AnalyticsSeries = {
  name: string;
  points: AnalyticsPoint[];
};

// DateRange is the shared ?from=&to= tuple used by every analytics
// endpoint. The frontend always renders in UTC so calendars align with
// the rollup worker.
export type DateRange = {
  from: string; // YYYY-MM-DD.
  to: string; // YYYY-MM-DD.
};

const ANALYTICS_KEY = ['analytics'] as const;

function retryPolicy(failureCount: number, err: unknown): boolean {
  if (err instanceof ApiError && err.status >= 400 && err.status < 500) return false;
  return failureCount < 2;
}

// useAnalyticsOverview drives the four KPI cards on the dashboard.
export function useAnalyticsOverview(range: DateRange) {
  return useQuery<AnalyticsOverview, ApiError>({
    queryKey: [...ANALYTICS_KEY, 'overview', range],
    queryFn: async () => {
      const qs = new URLSearchParams({ from: range.from, to: range.to }).toString();
      return api<AnalyticsOverview>(`/analytics/overview?${qs}`);
    },
    staleTime: 60_000,
    retry: retryPolicy,
  });
}

// useAnalyticsSeries drives the sparkline charts. Provider is
// optional — omit to select the pan-provider "all" aggregate row.
export function useAnalyticsSeries(
  kind: SeriesKind,
  range: DateRange,
  provider?: string,
) {
  return useQuery<AnalyticsSeries, ApiError>({
    queryKey: [...ANALYTICS_KEY, 'series', kind, range, provider ?? ''],
    queryFn: async () => {
      const params = new URLSearchParams({ kind, from: range.from, to: range.to });
      if (provider !== undefined && provider !== '') params.set('provider', provider);
      return api<AnalyticsSeries>(`/analytics/series?${params.toString()}`);
    },
    staleTime: 60_000,
    retry: retryPolicy,
  });
}

// useCallsSeries is a convenience wrapper around useAnalyticsSeries for
// the "calls_daily" kind — keeps the analytics dashboard tidy without
// re-typing the string literal at every call site.
export function useCallsSeries(range: DateRange, provider?: string) {
  return useAnalyticsSeries('calls_daily', range, provider);
}

// defaultRange returns the last 14 UTC days ending today, formatted
// for the analytics query params. Kept as a helper so multiple call
// sites stay in sync.
export function defaultRange(): DateRange {
  const today = new Date();
  const from = new Date(today);
  from.setUTCDate(from.getUTCDate() - 13);
  return { from: toISODate(from), to: toISODate(today) };
}

function toISODate(d: Date): string {
  const y = d.getUTCFullYear();
  const m = String(d.getUTCMonth() + 1).padStart(2, '0');
  const day = String(d.getUTCDate()).padStart(2, '0');
  return `${y}-${m}-${day}`;
}
