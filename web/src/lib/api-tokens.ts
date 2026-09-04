import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ApiError, api } from './api';

/** APIToken describes a personal-access token as the list endpoint
 * returns it. The plaintext value is NEVER present here — it is only
 * emitted once by the create endpoint (see CreateAPITokenResponse). */
export type APIToken = {
  id: string;
  name: string;
  prefix: string;
  last_used_at?: string | null;
  expires_at?: string | null;
  created_at: string;
  revoked_at?: string | null;
};

export type CreateAPITokenInput = {
  name: string;
  /** Optional lifetime in days. Omit for a never-expiring token. */
  expires_in_days?: number;
};

export type CreateAPITokenResponse = {
  id: string;
  name: string;
  prefix: string;
  /** Full secret. Shown once; store immediately or discard. */
  plaintext: string;
  created_at: string;
  expires_at?: string | null;
};

const API_TOKENS_KEY = ['api-tokens'] as const;

type APITokensListResponse = { items: APIToken[] } | APIToken[];

function normalizeList(res: APITokensListResponse): APIToken[] {
  if (Array.isArray(res)) return res;
  return res.items;
}

export function useAPITokens() {
  return useQuery<APIToken[], ApiError>({
    queryKey: API_TOKENS_KEY,
    queryFn: async () => {
      const res = await api<APITokensListResponse>('/api-tokens');
      return normalizeList(res);
    },
    staleTime: 15_000,
    retry: (failureCount, err) => {
      if (err instanceof ApiError && err.status >= 400 && err.status < 500) return false;
      return failureCount < 2;
    },
  });
}

export function useCreateAPIToken() {
  const qc = useQueryClient();
  return useMutation<CreateAPITokenResponse, ApiError, CreateAPITokenInput>({
    mutationFn: async (input) => {
      const body: CreateAPITokenInput = { name: input.name };
      if (input.expires_in_days !== undefined) {
        body.expires_in_days = input.expires_in_days;
      }
      return api<CreateAPITokenResponse>('/api-tokens', { method: 'POST', body });
    },
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: API_TOKENS_KEY });
    },
  });
}

export function useRevokeAPIToken() {
  const qc = useQueryClient();
  return useMutation<void, ApiError, string>({
    mutationFn: async (id) => {
      await api<void>(`/api-tokens/${id}`, { method: 'DELETE' });
    },
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: API_TOKENS_KEY });
    },
  });
}
