import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ApiError, api, ensureCsrf } from './api';

export type Me = {
  user_id: string;
  org_id: string;
  email: string;
  display_name: string;
  org_name: string;
  permissions: string[];
};

export type LoginResponse = {
  user_id: string;
  org_id: string;
  permissions: string[];
};

const ME_KEY = ['auth', 'me'] as const;

export function useMe() {
  return useQuery<Me | null>({
    queryKey: ME_KEY,
    queryFn: async () => {
      try {
        return await api<Me>('/auth/me');
      } catch (err) {
        if (err instanceof ApiError && err.status === 401) {
          return null;
        }
        throw err;
      }
    },
    staleTime: 30_000,
    retry: false,
  });
}

export function useLogin() {
  const qc = useQueryClient();
  return useMutation<LoginResponse, ApiError, { email: string; password: string }>({
    mutationFn: async (creds) => {
      await ensureCsrf();
      return api<LoginResponse>('/auth/login', { method: 'POST', body: creds });
    },
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ME_KEY });
    },
  });
}

export function useLogout() {
  const qc = useQueryClient();
  return useMutation<void, ApiError, void>({
    mutationFn: async () => {
      await api<void>('/auth/logout', { method: 'POST' });
    },
    onSuccess: async () => {
      qc.setQueryData(ME_KEY, null);
      await qc.invalidateQueries({ queryKey: ME_KEY });
    },
  });
}
