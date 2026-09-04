import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
  type InfiniteData,
} from '@tanstack/react-query';
import { ApiError, api } from './api';

// Call mirrors the JSON shape returned by internal/api/rest/v1/calls.go.
// Kept in sync manually until openapi-typescript codegen lands.
export type Call = {
  id: string;
  org_id: string;
  integration_id?: string;
  business_endpoint_id?: string;
  contact_id?: string;
  session_id?: string;
  conversation_id?: string;
  provider: string;
  provider_call_id: string;
  direction: 'inbound' | 'outbound';
  status:
    | 'queued'
    | 'ringing'
    | 'answered'
    | 'in_progress'
    | 'completed'
    | 'missed'
    | 'failed'
    | 'declined'
    | 'no_answer';
  from?: string;
  to?: string;
  from_user_id?: string;
  to_user_id?: string;
  // Enriched identity fields sourced from the backend join against the
  // contact record. Any of these may be absent when the enrichment hasn't
  // resolved yet — callers fall back through name → BSUID → phone.
  contact_name?: string;
  bsuid?: string;
  phone?: string;
  started_at?: string;
  answered_at?: string;
  ended_at?: string;
  duration_seconds: number;
  hangup_reason?: string;
  recording_url?: string;
  transcription_ref?: string;
  metadata?: Record<string, unknown>;
  created_at: string;
  updated_at?: string;
};

export type CallPage = {
  items: Call[];
  next_cursor?: string;
};

export type CallFilter = {
  status?: Call['status'] | '';
  direction?: Call['direction'] | '';
  contact_id?: string;
  since?: string;
  until?: string;
  limit?: number;
};

export type InitiateCallRequest = {
  integration_id: string;
  to?: string;
  to_user_id?: string;
  contact_id?: string;
  idempotency_key?: string;
  recording?: {
    enabled: boolean;
    purpose?: string;
    announcement_language?: string;
  };
  transcription?: {
    enabled: boolean;
    purpose?: string;
    announcement_language?: string;
  };
};

export type InitiateCallAccepted = {
  call_id: string;
  status: string;
};

export const CALL_STATUSES: Array<Call['status']> = [
  'queued',
  'ringing',
  'answered',
  'in_progress',
  'completed',
  'missed',
  'failed',
  'declined',
  'no_answer',
];

const CALLS_KEY = ['calls'] as const;

function buildQueryString(filter: CallFilter, cursor?: string): string {
  const params = new URLSearchParams();
  if (filter.status !== undefined && filter.status !== '') {
    params.set('status', filter.status);
  }
  if (filter.direction !== undefined && filter.direction !== '') {
    params.set('direction', filter.direction);
  }
  if (filter.contact_id !== undefined && filter.contact_id !== '') {
    params.set('contact_id', filter.contact_id);
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

export function useCalls(filter: CallFilter) {
  return useInfiniteQuery<
    CallPage,
    ApiError,
    InfiniteData<CallPage, string>,
    readonly unknown[],
    string
  >({
    queryKey: [...CALLS_KEY, filter],
    initialPageParam: '',
    queryFn: async ({ pageParam }) => {
      const qs = buildQueryString(filter, pageParam);
      return api<CallPage>(`/calls${qs}`);
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

/**
 * useConversationCalls fetches the recent calls attached to a single
 * conversation, newest-first. Disabled when conversationID is null.
 */
export function useConversationCalls(conversationID: string | null) {
  return useQuery<CallPage, ApiError>({
    queryKey: [...CALLS_KEY, 'by-conversation', conversationID],
    enabled: conversationID !== null && conversationID !== '',
    queryFn: async () =>
      api<CallPage>(`/calls?conversation_id=${encodeURIComponent(conversationID ?? '')}&limit=100`),
    staleTime: 10_000,
  });
}

export function useCall(id: string | null) {
  return useQuery<Call, ApiError>({
    queryKey: [...CALLS_KEY, 'detail', id],
    enabled: id !== null && id !== '',
    queryFn: async () => api<Call>(`/calls/${id ?? ''}`),
    staleTime: 5_000,
  });
}

export function useInitiateCall() {
  const qc = useQueryClient();
  return useMutation<InitiateCallAccepted, ApiError, InitiateCallRequest>({
    mutationFn: (body) => api<InitiateCallAccepted>('/calls', { method: 'POST', body }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: CALLS_KEY });
    },
  });
}

// CallSession is the GET /api/v1/calls/{id}/session response — the SDP
// offer that Meta shipped on the `connect` webhook, forwarded verbatim.
export type CallSession = {
  sdp_type: string;
  sdp: string;
};

// useCallSession fetches the SDP offer stored for an inbound call.
// Disabled until callID is non-empty. Retries only on 5xx — a 404 is a
// terminal signal (the call has no stored offer, e.g. business-initiated).
export function useCallSession(callID: string | null) {
  return useQuery<CallSession, ApiError>({
    queryKey: [...CALLS_KEY, 'session', callID],
    enabled: callID !== null && callID !== '',
    queryFn: async () => api<CallSession>(`/calls/${callID ?? ''}/session`),
    staleTime: 60_000,
    retry: (failureCount, err) => {
      if (err instanceof ApiError && err.status >= 400 && err.status < 500) return false;
      return failureCount < 2;
    },
  });
}

// AnswerCallInput carries the browser-side WebRTC answer + recording /
// transcription toggles. All fields are optional; an empty object performs
// the legacy bare-accept.
export type AnswerCallInput = {
  id: string;
  sdp?: string;
  recording?: {
    enabled: boolean;
    purpose?: string;
    announcement_language?: string;
  };
  transcription?: {
    enabled: boolean;
    purpose?: string;
    announcement_language?: string;
  };
};

export function useAnswerCall() {
  const qc = useQueryClient();
  return useMutation<void, ApiError, AnswerCallInput>({
    mutationFn: ({ id, sdp, recording, transcription }) => {
      const body: Record<string, unknown> = {};
      if (sdp !== undefined && sdp !== '') body['sdp'] = sdp;
      if (recording !== undefined) body['recording'] = recording;
      if (transcription !== undefined) body['transcription'] = transcription;
      const hasBody = Object.keys(body).length > 0;
      return api<void>(`/calls/${id}/answer`, {
        method: 'POST',
        ...(hasBody ? { body } : {}),
      });
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: CALLS_KEY });
    },
  });
}

export function useRejectCall() {
  const qc = useQueryClient();
  return useMutation<void, ApiError, { id: string; reason?: string }>({
    mutationFn: ({ id, reason }) =>
      api<void>(`/calls/${id}/reject`, {
        method: 'POST',
        body: reason !== undefined ? { reason } : undefined,
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: CALLS_KEY });
    },
  });
}

export function useEndCall() {
  const qc = useQueryClient();
  return useMutation<void, ApiError, string>({
    mutationFn: (id) => api<void>(`/calls/${id}/end`, { method: 'POST' }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: CALLS_KEY });
    },
  });
}

// recordingURL returns the proxy URL for a call's recording. The provider
// short-lived URL is never surfaced to the browser — the backend streams
// bytes through this endpoint (auth-gated).
export function recordingURL(id: string): string {
  return `/api/v1/calls/${id}/recording`;
}

// transcriptURL returns the proxy URL for a call's transcript JSON.
export function transcriptURL(id: string): string {
  return `/api/v1/calls/${id}/transcript`;
}

// CallTranscriptSegment mirrors one entry in Meta's transcript.segments[].
// Fields are optional because Meta may omit any of them on a partial
// transcript.
export type CallTranscriptSegment = {
  speaker?: string;
  channel?: string;
  start?: number;
  end?: number;
  text?: string;
  confidence?: number;
  words?: Array<{
    word?: string;
    start?: number;
    end?: number;
    confidence?: number;
  }>;
};

// CallTranscript mirrors Meta's transcript document shape. Top-level
// metadata (call id, participants) is preserved as an unknown object so
// the UI can render it if needed without importing every Meta field.
export type CallTranscript = {
  metadata?: Record<string, unknown>;
  transcript?: {
    text?: string;
    language?: string;
    duration?: number;
    confidence?: number;
    segments?: CallTranscriptSegment[];
  };
};

// useCallTranscript fetches the transcript JSON for a call. Only fires
// when both callID is non-empty AND the call already has a
// transcription_ref stamped — otherwise the backend would return 409.
export function useCallTranscript(callID: string | null, transcriptionRef: string | undefined) {
  return useQuery<CallTranscript, ApiError>({
    queryKey: [...CALLS_KEY, 'transcript', callID],
    enabled:
      callID !== null &&
      callID !== '' &&
      transcriptionRef !== undefined &&
      transcriptionRef !== '',
    queryFn: async () => api<CallTranscript>(`/calls/${callID ?? ''}/transcript`),
    staleTime: 60_000,
    retry: (failureCount, err) => {
      if (err instanceof ApiError && err.status >= 400 && err.status < 500) return false;
      return failureCount < 2;
    },
  });
}

// formatDuration renders a duration_seconds int as MM:SS. Zero renders as "—".
export function formatDuration(seconds: number): string {
  if (seconds <= 0) return '—';
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  return `${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`;
}
