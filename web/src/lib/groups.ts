import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api, ApiError } from './api';

// ---------- Wire types ----------

export type Group = {
  id: string;
  org_id: string;
  integration_id: string;
  provider_group_id: string;
  subject: string;
  description?: string;
  size: number;
  is_admin: boolean;
  metadata?: Record<string, unknown>;
  created_at: string;
  updated_at: string;
};

export type GroupListPage = {
  items: Group[];
  next_cursor?: string;
};

export type GroupMember = {
  id: number;
  group_id: string;
  contact_id?: string;
  wa_id?: string;
  bsuid?: string;
  role: 'member' | 'admin' | 'superadmin';
  joined_at: string;
  left_at?: string;
};

export type GroupMemberList = {
  items: GroupMember[];
};

export type GroupListFilter = {
  integration_id?: string;
  q?: string;
  limit?: number;
};

export type SyncGroupsRequest = {
  integration_id: string;
};

export type CreateGroupRequest = {
  integration_id: string;
  subject: string;
  description?: string;
  join_approval_mode?: 'auto_approve' | 'approval_required';
};

export type SyncGroupsResponse = {
  groups_upserted: number;
  members_upserted: number;
};

export type SendGroupMessagePayload = {
  type: 'text' | 'template' | 'image' | 'video' | 'audio' | 'document' | 'sticker';
  text?: unknown;
  template?: unknown;
  media?: unknown;
  idempotency_key?: string;
};

export type SendGroupMessageResponse = {
  message_id: string;
  status: string;
};

// ---------- Query keys ----------

const GROUPS_KEY = ['groups'] as const;
const groupKey = (id: string) => [...GROUPS_KEY, id] as const;
const groupMembersKey = (id: string) => [...GROUPS_KEY, id, 'members'] as const;

// ---------- Helpers ----------

function buildQueryString(filter: GroupListFilter): string {
  const params = new URLSearchParams();
  if (filter.integration_id !== undefined && filter.integration_id !== '') {
    params.set('integration_id', filter.integration_id);
  }
  if (filter.q !== undefined && filter.q !== '') {
    params.set('q', filter.q);
  }
  if (filter.limit !== undefined) {
    params.set('limit', String(filter.limit));
  }
  const q = params.toString();
  return q === '' ? '' : `?${q}`;
}

// ---------- Hooks ----------

/** useGroups fetches the org's persisted group list. */
export function useGroups(filter: GroupListFilter = {}) {
  return useQuery<GroupListPage, ApiError>({
    queryKey: [...GROUPS_KEY, filter],
    queryFn: async () => api<GroupListPage>(`/groups${buildQueryString(filter)}`),
    staleTime: 30_000,
    retry: (failureCount, err) => {
      if (err instanceof ApiError && err.status >= 400 && err.status < 500) return false;
      return failureCount < 2;
    },
  });
}

/** useGroup fetches a single persisted group by id. */
export function useGroup(id: string | undefined) {
  return useQuery<Group, ApiError>({
    queryKey: id === undefined ? [...GROUPS_KEY, 'disabled'] : groupKey(id),
    queryFn: async () => {
      if (id === undefined || id === '') {
        throw new ApiError(400, { title: 'group id required' });
      }
      return api<Group>(`/groups/${id}`);
    },
    enabled: id !== undefined && id !== '',
    staleTime: 30_000,
  });
}

/** useGroupMembers fetches the roster of a group. */
export function useGroupMembers(id: string | undefined) {
  return useQuery<GroupMemberList, ApiError>({
    queryKey: id === undefined ? [...GROUPS_KEY, 'members-disabled'] : groupMembersKey(id),
    queryFn: async () => {
      if (id === undefined || id === '') {
        throw new ApiError(400, { title: 'group id required' });
      }
      return api<GroupMemberList>(`/groups/${id}/members`);
    },
    enabled: id !== undefined && id !== '',
    staleTime: 30_000,
  });
}

/** useSyncGroups triggers a provider-side pull. */
export function useSyncGroups() {
  const qc = useQueryClient();
  return useMutation<SyncGroupsResponse, ApiError, SyncGroupsRequest>({
    mutationFn: async (req) =>
      api<SyncGroupsResponse>('/groups/sync', { method: 'POST', body: req }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: GROUPS_KEY });
    },
  });
}

/** useCreateGroup provisions a brand-new group on the provider. */
export function useCreateGroup() {
  const qc = useQueryClient();
  return useMutation<Group, ApiError, CreateGroupRequest>({
    mutationFn: async (req) =>
      api<Group>('/groups', { method: 'POST', body: req }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: GROUPS_KEY });
    },
  });
}

/** useSendGroupMessage dispatches a message to a group. */
export function useSendGroupMessage(groupID: string) {
  return useMutation<SendGroupMessageResponse, ApiError, SendGroupMessagePayload>({
    mutationFn: async (payload) =>
      api<SendGroupMessageResponse>(`/groups/${groupID}/messages`, {
        method: 'POST',
        body: payload,
      }),
  });
}
