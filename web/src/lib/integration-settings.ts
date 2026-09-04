import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ApiError, api } from './api';

// BusinessProfile mirrors the JSON shape returned by
// GET /api/v1/integrations/{id}/business-profile. Empty strings are used
// for missing fields so <input value={...}> stays a controlled input.
export type BusinessProfile = {
  about?: string;
  address?: string;
  description?: string;
  email?: string;
  profile_picture_url?: string;
  vertical?: string;
  websites?: string[];
};

export type WeeklyHours = {
  day_of_week: string; // MONDAY..SUNDAY
  open_time: string; // "HHMM"
  close_time: string; // "HHMM"
};

export type CallHours = {
  status?: string; // ENABLED | DISABLED
  timezone_id?: string;
  weekly_operating_hours?: WeeklyHours[];
};

export type CallSettings = {
  status?: string; // ENABLED | DISABLED
  call_icon_visibility?: string; // DEFAULT | DISABLE_ALL
  call_hours?: CallHours | null;
  callback_permission_status?: string; // ENABLED | DISABLED
};

// OBAStatus values: PENDING | APPROVED | REJECTED | CANCELLED | NOT_APPLIED.
export type OBAStatus = {
  oba_status?: string;
  status_message?: string;
};

// Username mirrors GET /api/v1/integrations/{id}/username. Empty username
// means the phone number has no adopted handle yet — the UI renders the
// adopt form in that case, not an error.
export type Username = {
  username?: string;
  status?: 'approved' | 'reserved' | '';
};

// PhoneNumber mirrors GET /api/v1/integrations/{id}/phone-number.
// All fields are optional — Meta omits fields the account tier does not
// unlock (e.g. throughput on legacy phones). An empty object is a valid
// steady state: the configured phone_number_id was not present in the
// WABA's phone-number list at fetch time.
export type PhoneNumber = {
  id?: string;
  display_phone_number?: string;
  verified_name?: string;
  status?: string;
  quality_rating?: string;
  country_code?: string;
  country_dial_code?: string;
  code_verification_status?: string;
  account_mode?: string;
  host_platform?: string;
  messaging_limit_tier?: string;
  is_official_business_account?: boolean;
};

// SetUsernameInput is the PUT body — transfer_action defaults to "none"
// when omitted, or "force_transfer" to reclaim a handle from another of
// the operator's phone numbers (Meta error 147005 path).
export type SetUsernameInput = {
  username: string;
  transfer_action?: 'none' | 'force_transfer';
};

// Query keys are functions so the drawer can pin them to a specific
// integration id without stringly-typed cache collisions.
export const businessProfileKey = (id: string) => ['integration-settings', id, 'business-profile'] as const;
export const callSettingsKey = (id: string) => ['integration-settings', id, 'call-settings'] as const;
export const obaStatusKey = (id: string) => ['integration-settings', id, 'oba-status'] as const;
export const usernameKey = (id: string) => ['integration-username', id] as const;
export const usernameSuggestionsKey = (id: string) => ['integration-username-suggestions', id] as const;
export const phoneNumberKey = (id: string) => ['integration-settings', id, 'phone-number'] as const;

// usePhoneNumber fetches the Meta phone-number record for the
// integration. Enabled when a non-null integration id is supplied;
// cached for 60s since the underlying record (quality rating,
// messaging tier, etc.) changes on the order of hours, not seconds.
export function usePhoneNumber(integrationID: string | null) {
  return useQuery<PhoneNumber, ApiError>({
    queryKey: phoneNumberKey(integrationID ?? ''),
    queryFn: async () => api<PhoneNumber>(`/integrations/${integrationID ?? ''}/phone-number`),
    enabled: integrationID !== null && integrationID.length > 0,
    staleTime: 60_000,
    retry: (failureCount, err) => {
      if (err instanceof ApiError && err.status >= 400 && err.status < 500) return false;
      return failureCount < 2;
    },
  });
}

export function useBusinessProfile(integrationID: string, enabled = true) {
  return useQuery<BusinessProfile, ApiError>({
    queryKey: businessProfileKey(integrationID),
    queryFn: async () => api<BusinessProfile>(`/integrations/${integrationID}/business-profile`),
    enabled: enabled && integrationID.length > 0,
    staleTime: 15_000,
    retry: (failureCount, err) => {
      if (err instanceof ApiError && err.status >= 400 && err.status < 500) return false;
      return failureCount < 2;
    },
  });
}

export function useUpdateBusinessProfile(integrationID: string) {
  const qc = useQueryClient();
  return useMutation<BusinessProfile, ApiError, BusinessProfile>({
    mutationFn: (body) => api<BusinessProfile>(`/integrations/${integrationID}/business-profile`, { method: 'PUT', body }),
    onSuccess: (data) => {
      qc.setQueryData(businessProfileKey(integrationID), data);
    },
  });
}

export function useCallSettings(integrationID: string, enabled = true) {
  return useQuery<CallSettings, ApiError>({
    queryKey: callSettingsKey(integrationID),
    queryFn: async () => api<CallSettings>(`/integrations/${integrationID}/call-settings`),
    enabled: enabled && integrationID.length > 0,
    staleTime: 15_000,
    retry: (failureCount, err) => {
      if (err instanceof ApiError && err.status >= 400 && err.status < 500) return false;
      return failureCount < 2;
    },
  });
}

export function useUpdateCallSettings(integrationID: string) {
  const qc = useQueryClient();
  return useMutation<CallSettings, ApiError, CallSettings>({
    mutationFn: (body) => api<CallSettings>(`/integrations/${integrationID}/call-settings`, { method: 'PUT', body }),
    onSuccess: (data) => {
      qc.setQueryData(callSettingsKey(integrationID), data);
    },
  });
}

export function useOBAStatus(integrationID: string, enabled = true) {
  return useQuery<OBAStatus, ApiError>({
    queryKey: obaStatusKey(integrationID),
    queryFn: async () => api<OBAStatus>(`/integrations/${integrationID}/oba-status`),
    enabled: enabled && integrationID.length > 0,
    staleTime: 15_000,
    retry: (failureCount, err) => {
      if (err instanceof ApiError && err.status >= 400 && err.status < 500) return false;
      return failureCount < 2;
    },
  });
}

export function useApplyOBA(integrationID: string) {
  const qc = useQueryClient();
  return useMutation<OBAStatus, ApiError, void>({
    mutationFn: () => api<OBAStatus>(`/integrations/${integrationID}/oba-status/apply`, { method: 'POST' }),
    onSuccess: (data) => {
      qc.setQueryData(obaStatusKey(integrationID), data);
    },
  });
}

export function useWithdrawOBA(integrationID: string) {
  const qc = useQueryClient();
  return useMutation<OBAStatus, ApiError, void>({
    mutationFn: () => api<OBAStatus>(`/integrations/${integrationID}/oba-status/withdraw`, { method: 'POST' }),
    onSuccess: (data) => {
      qc.setQueryData(obaStatusKey(integrationID), data);
    },
  });
}

// useUsername fetches the current business-scoped username for the
// integration. A 200 with an empty {} response is valid — the phone
// number simply has no handle adopted yet.
export function useUsername(integrationID: string, enabled = true) {
  return useQuery<Username, ApiError>({
    queryKey: usernameKey(integrationID),
    queryFn: async () => api<Username>(`/integrations/${integrationID}/username`),
    enabled: enabled && integrationID.length > 0,
    staleTime: 15_000,
    retry: (failureCount, err) => {
      if (err instanceof ApiError && err.status >= 400 && err.status < 500) return false;
      return failureCount < 2;
    },
  });
}

// useSetUsername adopts or changes the business username. On success the
// username query is invalidated so the reserved/approved state re-renders.
export function useSetUsername(integrationID: string) {
  const qc = useQueryClient();
  return useMutation<Username, ApiError, SetUsernameInput>({
    mutationFn: (body) => api<Username>(`/integrations/${integrationID}/username`, { method: 'PUT', body }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: usernameKey(integrationID) });
    },
  });
}

// useDeleteUsername releases the current handle.
export function useDeleteUsername(integrationID: string) {
  const qc = useQueryClient();
  return useMutation<{ success: boolean }, ApiError, void>({
    mutationFn: () => api<{ success: boolean }>(`/integrations/${integrationID}/username`, { method: 'DELETE' }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: usernameKey(integrationID) });
    },
  });
}

// CallPermission mirrors GET /api/v1/integrations/{id}/call-permission.
// status is the free-form Meta enum ("temporary" | "permanent" |
// "no_permission" | ...); expiration_time is unix seconds and only
// present when status === "temporary".
export type CallPermission = {
  status?: string;
  expiration_time?: number;
};

// useCallPermission polls the backend for the recipient's current
// call-permission state. Disabled until both integrationID and a
// well-formed `to` are supplied — the endpoint fans out to Meta and we
// want to avoid noisy 400s while the operator types the E.164.
export function useCallPermission(params: {
  integrationID: string;
  to: string;
  enabled?: boolean;
}) {
  const { integrationID, to } = params;
  const enabled = params.enabled ?? true;
  return useQuery<CallPermission, ApiError>({
    queryKey: ['integration-call-permission', integrationID, to],
    queryFn: async () =>
      api<CallPermission>(
        `/integrations/${integrationID}/call-permission?to=${encodeURIComponent(to)}`,
      ),
    enabled: enabled && integrationID.length > 0 && to.length >= 6,
    staleTime: 15_000,
    retry: (failureCount, err) => {
      if (err instanceof ApiError && err.status >= 400 && err.status < 500) return false;
      return failureCount < 1;
    },
  });
}

// SendPermissionRequestInput is the POST body for
// /api/v1/calls/permission-request.
export type SendPermissionRequestInput = {
  integration_id: string;
  to: string;
  prompt?: string;
};

// useSendPermissionRequest fires the interactive call_permission_request
// message. On success the call-permission query for the same (integration,
// to) is invalidated so the chip re-renders.
export function useSendPermissionRequest() {
  const qc = useQueryClient();
  return useMutation<{ wamid: string }, ApiError, SendPermissionRequestInput>({
    mutationFn: (body) =>
      api<{ wamid: string }>(`/calls/permission-request`, { method: 'POST', body }),
    onSuccess: (_data, variables) => {
      void qc.invalidateQueries({
        queryKey: ['integration-call-permission', variables.integration_id, variables.to],
      });
    },
  });
}

// useUsernameSuggestions is opt-in via the `enabled` flag — Meta throttles
// this endpoint, so we only fire it once the operator clicks "Show
// suggestions" (never on drawer open).
export function useUsernameSuggestions(integrationID: string, enabled: boolean) {
  return useQuery<{ suggestions: string[] }, ApiError>({
    queryKey: usernameSuggestionsKey(integrationID),
    queryFn: async () => api<{ suggestions: string[] }>(`/integrations/${integrationID}/username/suggestions`),
    enabled: enabled && integrationID.length > 0,
    staleTime: 60_000,
    retry: (failureCount, err) => {
      if (err instanceof ApiError && err.status >= 400 && err.status < 500) return false;
      return failureCount < 1;
    },
  });
}

// USERNAME_FORBIDDEN_SUFFIXES is the domain-tail blocklist Meta enforces
// server-side. We mirror it client-side to fail fast — the list is not
// exhaustive (Meta's rule is "any TLD"); the server has final say.
const USERNAME_FORBIDDEN_SUFFIXES = [
  '.com',
  '.org',
  '.net',
  '.edu',
  '.gov',
  '.io',
  '.co',
  '.info',
  '.biz',
  '.us',
  '.uk',
  '.in',
];

// validateUsername returns null when the value satisfies Meta's format
// rules, or a single human-readable error describing the first failed
// rule. See ~/Documents/whatsapp_doc_tracker/docs/business-scoped-user-ids.md.
export function validateUsername(raw: string): string | null {
  if (raw.length === 0) return 'Enter a username';
  if (raw.length < 3) return 'Must be at least 3 characters';
  if (raw.length > 35) return 'Must be at most 35 characters';
  if (!/^[a-z0-9._]+$/.test(raw)) return 'Only lowercase letters, digits, "." and "_" are allowed';
  if (!/[a-z]/.test(raw)) return 'Must contain at least one letter';
  if (raw.startsWith('.') || raw.endsWith('.')) return 'Cannot start or end with "."';
  if (raw.includes('..')) return 'Cannot contain ".."';
  if (raw.startsWith('www.')) return 'Cannot start with "www."';
  for (const suffix of USERNAME_FORBIDDEN_SUFFIXES) {
    if (raw.endsWith(suffix)) return `Cannot end with a domain suffix like "${suffix}"`;
  }
  return null;
}

// The canonical Meta vertical enum. Order matches Meta's docs; the
// dropdown consumes this directly.
export const VERTICALS = [
  'AUTO',
  'BEAUTY',
  'APPAREL',
  'EDU',
  'ENTERTAIN',
  'EVENT_PLAN',
  'FINANCE',
  'GROCERY',
  'GOVT',
  'HOTEL',
  'HEALTH',
  'NONPROFIT',
  'PROF_SERVICES',
  'RETAIL',
  'TRAVEL',
  'RESTAURANT',
  'NOT_A_BIZ',
  'OTHER',
] as const;

// Curated timezone list — Meta accepts IANA identifiers; we surface a
// small useful subset in the dropdown to keep the form scannable.
export const TIMEZONES = [
  'UTC',
  'America/New_York',
  'America/Los_Angeles',
  'America/Sao_Paulo',
  'America/Manaus',
  'Europe/London',
  'Europe/Berlin',
  'Europe/Paris',
  'Asia/Kolkata',
  'Asia/Jakarta',
  'Asia/Singapore',
  'Asia/Tokyo',
  'Australia/Sydney',
] as const;

export const DAYS_OF_WEEK = [
  'MONDAY',
  'TUESDAY',
  'WEDNESDAY',
  'THURSDAY',
  'FRIDAY',
  'SATURDAY',
  'SUNDAY',
] as const;

// hhmmToDisplay converts Meta's "HHMM" ("0900") into "HH:MM" ("09:00").
export function hhmmToDisplay(v: string | undefined): string {
  if (v === undefined || v.length < 3) return '';
  const raw = v.length === 3 ? `0${v}` : v;
  return `${raw.slice(0, 2)}:${raw.slice(2, 4)}`;
}

// displayToHHMM converts "HH:MM" back into Meta's "HHMM" wire format.
export function displayToHHMM(v: string): string {
  return v.replace(':', '').padStart(4, '0').slice(0, 4);
}
