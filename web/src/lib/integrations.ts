import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ApiError, api } from './api';

export type IntegrationStatus =
  | 'pending'
  | 'connected'
  | 'degraded'
  | 'auth_failed'
  | 'rate_limited'
  | 'disconnected'
  | 'disabled'
  | 'unknown';

export type Integration = {
  id: string;
  org_id: string;
  provider: string;
  name: string;
  status: IntegrationStatus;
  phone_number_id?: string;
  waba_id?: string;
  webhook_url?: string;
  verify_token?: string;
  created_at: string;
  updated_at?: string;
};

export type CreateIntegrationInput = {
  name: string;
  provider: 'whatsapp';
  phone_number_id: string;
  waba_id: string;
  access_token: string;
  app_secret: string;
  verify_token: string;
};

export type TestIntegrationResult = {
  ok: boolean;
  message?: string;
  checked_at: string;
};

const INTEGRATIONS_KEY = ['integrations'] as const;

type IntegrationsListResponse = { items: Integration[] } | Integration[];

function normalizeList(res: IntegrationsListResponse): Integration[] {
  if (Array.isArray(res)) return res;
  return res.items;
}

export function useIntegrations() {
  return useQuery<Integration[], ApiError>({
    queryKey: INTEGRATIONS_KEY,
    queryFn: async () => {
      const res = await api<IntegrationsListResponse>('/integrations');
      return normalizeList(res);
    },
    staleTime: 15_000,
    retry: (failureCount, err) => {
      if (err instanceof ApiError && err.status >= 400 && err.status < 500) return false;
      return failureCount < 2;
    },
  });
}

export function useCreateIntegration() {
  const qc = useQueryClient();
  return useMutation<Integration, ApiError, CreateIntegrationInput>({
    mutationFn: async (input) => {
      // Backend expects a nested shape: {type, provider, name, config, secrets}.
      // The form collects a flat object; we split it here so the UI stays
      // simple.
      const body = {
        type: 'channel',
        provider: input.provider,
        name: input.name,
        config: {
          phone_number_id: input.phone_number_id,
          waba_id: input.waba_id,
        },
        secrets: {
          access_token: input.access_token,
          app_secret: input.app_secret,
          verify_token: input.verify_token,
        },
      };
      return api<Integration>('/integrations', { method: 'POST', body });
    },
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: INTEGRATIONS_KEY });
    },
  });
}

export function useTestIntegration() {
  const qc = useQueryClient();
  return useMutation<TestIntegrationResult, ApiError, string>({
    mutationFn: async (id) => {
      return api<TestIntegrationResult>(`/integrations/${id}/test`, { method: 'POST' });
    },
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: INTEGRATIONS_KEY });
    },
  });
}

export function useDeleteIntegration() {
  const qc = useQueryClient();
  return useMutation<void, ApiError, string>({
    mutationFn: async (id) => {
      await api<void>(`/integrations/${id}`, { method: 'DELETE' });
    },
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: INTEGRATIONS_KEY });
    },
  });
}
