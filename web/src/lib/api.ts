import { readCookie } from './cookie';

export type ProblemDetail = {
  type?: string;
  title?: string;
  status?: number;
  detail?: string;
  request_id?: string;
  errors?: Array<{ field?: string; message?: string }>;
};

export class ApiError extends Error {
  readonly status: number;
  readonly problem: ProblemDetail;

  constructor(status: number, problem: ProblemDetail) {
    super(problem.detail ?? problem.title ?? `HTTP ${status}`);
    this.name = 'ApiError';
    this.status = status;
    this.problem = problem;
  }
}

type HttpMethod = 'GET' | 'HEAD' | 'POST' | 'PUT' | 'PATCH' | 'DELETE';

type ApiOptions = {
  method?: HttpMethod;
  body?: unknown;
  headers?: Record<string, string>;
  signal?: AbortSignal;
};

const CSRF_COOKIE = 'fullwa_csrf';

export async function api<T = unknown>(path: string, opts: ApiOptions = {}): Promise<T> {
  const method: HttpMethod = opts.method ?? 'GET';
  const headers: Record<string, string> = {
    Accept: 'application/json, application/problem+json',
    ...(opts.headers ?? {}),
  };

  if (opts.body !== undefined) {
    headers['Content-Type'] = 'application/json';
  }

  if (method !== 'GET' && method !== 'HEAD') {
    const csrf = readCookie(CSRF_COOKIE);
    if (csrf !== null) {
      headers['X-CSRF-Token'] = csrf;
    }
  }

  const init: RequestInit = {
    method,
    credentials: 'include',
    headers,
  };
  if (opts.body !== undefined) {
    init.body = JSON.stringify(opts.body);
  }
  if (opts.signal !== undefined) {
    init.signal = opts.signal;
  }

  const res = await fetch(`/api/v1${path}`, init);

  if (res.status === 204) {
    return undefined as T;
  }

  const contentType = res.headers.get('content-type') ?? '';
  const isJson = contentType.includes('application/json') || contentType.includes('application/problem+json');

  if (!res.ok) {
    let problem: ProblemDetail = { status: res.status, title: res.statusText };
    if (isJson) {
      try {
        problem = (await res.json()) as ProblemDetail;
      } catch {
        // ignore parse errors, fall back to default problem
      }
    }
    throw new ApiError(res.status, problem);
  }

  if (!isJson) {
    return undefined as T;
  }

  return (await res.json()) as T;
}

export async function ensureCsrf(): Promise<void> {
  await fetch('/api/v1/auth/csrf', { credentials: 'include' });
}
