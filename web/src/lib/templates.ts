import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
  type InfiniteData,
} from '@tanstack/react-query';
import { ApiError, api } from './api';

// Template categories mirror the Meta vocabulary because WhatsApp is the
// only provider we currently source templates from. Keep this list in
// sync with internal/domain/template/template.go until openapi-typescript
// codegen replaces the duplication.
export const TEMPLATE_CATEGORIES = ['MARKETING', 'UTILITY', 'AUTHENTICATION'] as const;
export type TemplateCategory = (typeof TEMPLATE_CATEGORIES)[number];

export const TEMPLATE_STATUSES = [
  'DRAFT',
  'PENDING',
  'APPROVED',
  'REJECTED',
  'PAUSED',
  'DISABLED',
] as const;
export type TemplateStatus = (typeof TEMPLATE_STATUSES)[number];

export type TemplateComponent = {
  type: string;
  format?: string;
  text?: string;
  example?: Record<string, unknown>;
  buttons?: Array<Record<string, unknown>>;
  cards?: Array<Record<string, unknown>>;
  extra?: Record<string, unknown>;
};

export type Template = {
  id: string;
  org_id: string;
  integration_id: string;
  provider_template_id?: string;
  name: string;
  language: string;
  category: TemplateCategory;
  status: TemplateStatus;
  components: TemplateComponent[];
  variables: Record<string, string>;
  last_synced_at?: string;
  created_at: string;
  updated_at: string;
};

export type TemplateListPage = {
  items: Template[];
  next_cursor?: string;
};

export type TemplateListFilter = {
  integration_id?: string;
  status?: TemplateStatus | '';
  limit?: number;
};

const TEMPLATES_KEY = ['templates'] as const;

function buildQueryString(filter: TemplateListFilter, cursor?: string): string {
  const params = new URLSearchParams();
  if (filter.integration_id !== undefined && filter.integration_id !== '') {
    params.set('integration_id', filter.integration_id);
  }
  if (filter.status !== undefined && filter.status !== '') {
    params.set('status', filter.status);
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

/** useTemplates paginates GET /api/v1/templates. */
export function useTemplates(filter: TemplateListFilter) {
  return useInfiniteQuery<TemplateListPage, ApiError, InfiniteData<TemplateListPage, string>, readonly unknown[], string>({
    queryKey: [...TEMPLATES_KEY, filter],
    initialPageParam: '',
    queryFn: async ({ pageParam }) => {
      const qs = buildQueryString(filter, pageParam);
      return api<TemplateListPage>(`/templates${qs}`);
    },
    getNextPageParam: (last) =>
      last.next_cursor !== undefined && last.next_cursor !== '' ? last.next_cursor : undefined,
    staleTime: 30_000,
    retry: (failureCount, err) => {
      if (err instanceof ApiError && err.status >= 400 && err.status < 500) return false;
      return failureCount < 2;
    },
  });
}

/** useTemplate fetches a single row. */
export function useTemplate(id: string | undefined) {
  return useQuery<Template, ApiError>({
    queryKey: [...TEMPLATES_KEY, 'detail', id],
    queryFn: () => api<Template>(`/templates/${id ?? ''}`),
    enabled: id !== undefined && id !== '',
    staleTime: 10_000,
  });
}

export type CreateTemplateInput = {
  integration_id: string;
  name: string;
  language: string;
  category: TemplateCategory;
  components: TemplateComponent[];
  submit?: boolean;
  allow_category_change?: boolean;
};

/** CreateTemplateSuccess is a bare Template.
 * CreateTemplateWithSubmitError is 201 shape returned when submit=true and
 * only the provider (Meta) rejected — the DRAFT is still persisted. */
export type CreateTemplateWithSubmitError = {
  template: Template;
  submit_error: string;
  provider_error_code?: string;
  provider_error_type?: string;
  provider_trace_id?: string;
};

export type CreateTemplateResult = Template | CreateTemplateWithSubmitError;

/** isCreateWithSubmitError narrows CreateTemplateResult to the failure shape. */
export function isCreateWithSubmitError(
  r: CreateTemplateResult,
): r is CreateTemplateWithSubmitError {
  return typeof r === 'object' && r !== null && 'submit_error' in r;
}

/** useCreateTemplate posts POST /api/v1/templates. */
export function useCreateTemplate() {
  const qc = useQueryClient();
  return useMutation<CreateTemplateResult, ApiError, CreateTemplateInput>({
    mutationFn: (body) =>
      api<CreateTemplateResult>('/templates', {
        method: 'POST',
        body,
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: TEMPLATES_KEY });
    },
  });
}

/** useSubmitTemplate POSTs /api/v1/templates/{id}/submit. */
export function useSubmitTemplate() {
  const qc = useQueryClient();
  return useMutation<Template, ApiError, string>({
    mutationFn: (id) =>
      api<Template>(`/templates/${id}/submit`, {
        method: 'POST',
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: TEMPLATES_KEY });
    },
  });
}

/** useSyncTemplates POSTs /api/v1/templates/sync. */
export function useSyncTemplates() {
  const qc = useQueryClient();
  return useMutation<{ fetched: number; upserted: number }, ApiError, { integration_id: string }>({
    mutationFn: (body) =>
      api<{ fetched: number; upserted: number }>(`/templates/sync`, {
        method: 'POST',
        body,
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: TEMPLATES_KEY });
    },
  });
}

export type UpdateTemplateInput = {
  id: string;
  category: TemplateCategory;
  components: TemplateComponent[];
};

/** useUpdateTemplate PUTs /api/v1/templates/{id}. Backend rejects with 409
 * not_editable when the row is not DRAFT. */
export function useUpdateTemplate() {
  const qc = useQueryClient();
  return useMutation<Template, ApiError, UpdateTemplateInput>({
    mutationFn: ({ id, ...body }) =>
      api<Template>(`/templates/${id}`, {
        method: 'PUT',
        body,
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: TEMPLATES_KEY });
    },
  });
}

/** useDeleteTemplate DELETEs /api/v1/templates/{id}. */
export function useDeleteTemplate() {
  const qc = useQueryClient();
  return useMutation<void, ApiError, string>({
    mutationFn: (id) =>
      api<void>(`/templates/${id}`, {
        method: 'DELETE',
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: TEMPLATES_KEY });
    },
  });
}
