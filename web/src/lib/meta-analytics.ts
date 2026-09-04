import { useQuery } from '@tanstack/react-query';
import { ApiError, api } from './api';

// Meta's analytics API returns UNIX epoch seconds for every start/end
// bucket. The Nudgeway proxy passes the response through verbatim; we
// mirror the JSON shapes here so the UI can narrow without `any`.

export type MetaGranularity = 'HALF_HOUR' | 'DAILY' | 'MONTHLY';

/** MetaRange is the ?since=&until= tuple the proxy forwards to Meta.
 * Values are YYYY-MM-DD in the operator's local timezone — the backend
 * converts them to UNIX seconds before calling Graph. */
export type MetaRange = {
  since: string;
  until: string;
};

// ---- Messaging analytics ------------------------------------------------

export type MetaMessagingPoint = {
  start: number;
  end: number;
  sent: number;
  delivered: number;
};

export type MetaMessagingResponse = {
  analytics: {
    phone_numbers?: string[];
    country_codes?: string[];
    granularity: string;
    data_points: MetaMessagingPoint[];
  };
  id?: string;
};

// ---- Conversation analytics --------------------------------------------

export type MetaConversationDirection =
  | 'BUSINESS_INITIATED'
  | 'USER_INITIATED'
  | 'UNKNOWN';

export type MetaConversationType =
  | 'REGULAR'
  | 'FREE_TIER'
  | 'FREE_ENTRY_POINT';

export type MetaConversationCategory =
  | 'AUTHENTICATION'
  | 'MARKETING'
  | 'SERVICE'
  | 'UTILITY';

export type MetaConversationPoint = {
  start: number;
  end: number;
  conversation?: number;
  cost?: number;
  currency?: string;
  phone_number?: string;
  country?: string;
  conversation_direction?: MetaConversationDirection;
  conversation_type?: MetaConversationType;
  conversation_category?: MetaConversationCategory;
};

export type MetaConversationSeries = {
  data_points: MetaConversationPoint[];
  conversation_direction?: MetaConversationDirection;
  conversation_type?: MetaConversationType;
  conversation_category?: MetaConversationCategory;
};

export type MetaConversationResponse = {
  conversation_analytics: {
    data: MetaConversationSeries[];
  };
  id?: string;
};

// ---- Pricing analytics -------------------------------------------------

export type MetaPricingPoint = {
  start: number;
  end: number;
  cost?: number;
  currency?: string;
  volume?: number;
  country?: string;
  phone_number?: string;
  tier?: string;
  pricing_type?: string;
  pricing_category?: string;
};

export type MetaPricingSeries = {
  data_points: MetaPricingPoint[];
  pricing_category?: string;
  pricing_type?: string;
  tier?: string;
};

export type MetaPricingResponse = {
  pricing_analytics: {
    data: MetaPricingSeries[];
  };
  id?: string;
};

// ---- Call analytics ----------------------------------------------------

export type MetaCallDirection = 'BUSINESS_INITIATED' | 'USER_INITIATED';

export type MetaCallPoint = {
  start: number;
  end: number;
  cost?: number;
  currency?: string;
  count?: number;
  average_duration?: number;
  direction?: MetaCallDirection;
};

export type MetaCallResponse = {
  call_analytics: {
    granularity: string;
    directions?: string;
    data_points: MetaCallPoint[];
  };
  id?: string;
};

// ---- Template analytics ------------------------------------------------

export type MetaTemplateCost = {
  type: string;
  value: number;
};

export type MetaTemplateClick = {
  type: string;
  button_content?: string;
  count: number;
};

export type MetaTemplatePoint = {
  template_id: string;
  template_name?: string;
  start: number;
  end: number;
  sent?: number;
  delivered?: number;
  read?: number;
  cost?: MetaTemplateCost[];
  clicked?: MetaTemplateClick[];
};

export type MetaTemplateSeries = {
  granularity?: string;
  product_type?: string;
  data_points: MetaTemplatePoint[];
};

export type MetaTemplateResponse = {
  data: MetaTemplateSeries[];
  paging?: {
    cursors?: { before?: string; after?: string };
  };
};

// ---- Query hooks -------------------------------------------------------

const META_KEY = ['meta-analytics'] as const;

function retryPolicy(failureCount: number, err: unknown): boolean {
  if (err instanceof ApiError && err.status >= 400 && err.status < 500) return false;
  return failureCount < 2;
}

function buildQuery(range: MetaRange, granularity: MetaGranularity): string {
  return new URLSearchParams({
    since: range.since,
    until: range.until,
    granularity,
  }).toString();
}

/** useMetaMessaging fetches the WABA-level messaging analytics via the
 * Nudgeway proxy. `enabled=false` short-circuits the fetch so hidden
 * tabs don't burn API quota. */
export function useMetaMessaging(
  integrationID: string,
  range: MetaRange,
  granularity: MetaGranularity,
  enabled: boolean,
) {
  return useQuery<MetaMessagingResponse, ApiError>({
    queryKey: [...META_KEY, 'messaging', integrationID, range, granularity],
    queryFn: async () => {
      const qs = buildQuery(range, granularity);
      return api<MetaMessagingResponse>(
        `/integrations/${integrationID}/meta-analytics/messaging?${qs}`,
      );
    },
    enabled: enabled && integrationID !== '',
    staleTime: 60_000,
    retry: retryPolicy,
  });
}

/** useMetaConversations fetches per-conversation cost + count buckets. */
export function useMetaConversations(
  integrationID: string,
  range: MetaRange,
  granularity: MetaGranularity,
  enabled: boolean,
) {
  return useQuery<MetaConversationResponse, ApiError>({
    queryKey: [...META_KEY, 'conversations', integrationID, range, granularity],
    queryFn: async () => {
      const qs = buildQuery(range, granularity);
      return api<MetaConversationResponse>(
        `/integrations/${integrationID}/meta-analytics/conversations?${qs}`,
      );
    },
    enabled: enabled && integrationID !== '',
    staleTime: 60_000,
    retry: retryPolicy,
  });
}

/** useMetaPricing fetches billing tier + pricing category breakdowns. */
export function useMetaPricing(
  integrationID: string,
  range: MetaRange,
  granularity: MetaGranularity,
  enabled: boolean,
) {
  return useQuery<MetaPricingResponse, ApiError>({
    queryKey: [...META_KEY, 'pricing', integrationID, range, granularity],
    queryFn: async () => {
      const qs = buildQuery(range, granularity);
      return api<MetaPricingResponse>(
        `/integrations/${integrationID}/meta-analytics/pricing?${qs}`,
      );
    },
    enabled: enabled && integrationID !== '',
    staleTime: 60_000,
    retry: retryPolicy,
  });
}

/** useMetaCalls fetches WABA voice call counts + costs. */
export function useMetaCalls(
  integrationID: string,
  range: MetaRange,
  granularity: MetaGranularity,
  enabled: boolean,
) {
  return useQuery<MetaCallResponse, ApiError>({
    queryKey: [...META_KEY, 'calls', integrationID, range, granularity],
    queryFn: async () => {
      const qs = buildQuery(range, granularity);
      return api<MetaCallResponse>(
        `/integrations/${integrationID}/meta-analytics/calls?${qs}`,
      );
    },
    enabled: enabled && integrationID !== '',
    staleTime: 60_000,
    retry: retryPolicy,
  });
}

/** useMetaTemplates fetches per-template send/delivery/read/click stats. */
export function useMetaTemplates(
  integrationID: string,
  range: MetaRange,
  granularity: MetaGranularity,
  enabled: boolean,
) {
  return useQuery<MetaTemplateResponse, ApiError>({
    queryKey: [...META_KEY, 'templates', integrationID, range, granularity],
    queryFn: async () => {
      const qs = buildQuery(range, granularity);
      return api<MetaTemplateResponse>(
        `/integrations/${integrationID}/meta-analytics/templates?${qs}`,
      );
    },
    enabled: enabled && integrationID !== '',
    staleTime: 60_000,
    retry: retryPolicy,
  });
}

// ---- Helpers -----------------------------------------------------------

/** defaultMetaRange returns the last 7 days ending today, in YYYY-MM-DD. */
export function defaultMetaRange(): MetaRange {
  const today = new Date();
  const from = new Date(today);
  from.setDate(from.getDate() - 6);
  return { since: toISODate(from), until: toISODate(today) };
}

/** rangeDaysAgo returns a MetaRange ending today, starting `days-1` days ago
 * (so `rangeDaysAgo(7)` yields a 7-day inclusive window). */
export function rangeDaysAgo(days: number): MetaRange {
  const today = new Date();
  const from = new Date(today);
  from.setDate(from.getDate() - Math.max(0, days - 1));
  return { since: toISODate(from), until: toISODate(today) };
}

function toISODate(d: Date): string {
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, '0');
  const day = String(d.getDate()).padStart(2, '0');
  return `${y}-${m}-${day}`;
}

/** epochToISODay converts a Meta UNIX-seconds bucket-start to YYYY-MM-DD
 * in the browser's local timezone. Used to align Meta buckets with the
 * existing Sparkline component. */
export function epochToISODay(seconds: number): string {
  const d = new Date(seconds * 1000);
  return toISODate(d);
}

/** formatCurrency renders a numeric cost using the given ISO-4217 code
 * (falling back to USD). Meta returns currency codes on some endpoints
 * and omits them on others, so callers pass whatever they have. */
export function formatCurrency(value: number, currency: string | undefined): string {
  const code = currency !== undefined && currency !== '' ? currency : 'USD';
  try {
    return new Intl.NumberFormat(undefined, {
      style: 'currency',
      currency: code,
      maximumFractionDigits: 4,
    }).format(value);
  } catch {
    return `${value.toFixed(2)} ${code}`;
  }
}
