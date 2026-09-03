import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ApiError, api, ensureCsrf } from './api';
const ME_KEY = ['auth', 'me'];
export function useMe() {
    return useQuery({
        queryKey: ME_KEY,
        queryFn: async () => {
            try {
                return await api('/auth/me');
            }
            catch (err) {
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
    return useMutation({
        mutationFn: async (creds) => {
            await ensureCsrf();
            return api('/auth/login', { method: 'POST', body: creds });
        },
        onSuccess: async () => {
            await qc.invalidateQueries({ queryKey: ME_KEY });
        },
    });
}
export function useLogout() {
    const qc = useQueryClient();
    return useMutation({
        mutationFn: async () => {
            await api('/auth/logout', { method: 'POST' });
        },
        onSuccess: async () => {
            qc.setQueryData(ME_KEY, null);
            await qc.invalidateQueries({ queryKey: ME_KEY });
        },
    });
}
