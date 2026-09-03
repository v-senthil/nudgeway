import { readCookie } from './cookie';
export class ApiError extends Error {
    status;
    problem;
    constructor(status, problem) {
        super(problem.detail ?? problem.title ?? `HTTP ${status}`);
        this.name = 'ApiError';
        this.status = status;
        this.problem = problem;
    }
}
const CSRF_COOKIE = 'fullwa_csrf';
export async function api(path, opts = {}) {
    const method = opts.method ?? 'GET';
    const headers = {
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
    const init = {
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
        return undefined;
    }
    const contentType = res.headers.get('content-type') ?? '';
    const isJson = contentType.includes('application/json') || contentType.includes('application/problem+json');
    if (!res.ok) {
        let problem = { status: res.status, title: res.statusText };
        if (isJson) {
            try {
                problem = (await res.json());
            }
            catch {
                // ignore parse errors, fall back to default problem
            }
        }
        throw new ApiError(res.status, problem);
    }
    if (!isJson) {
        return undefined;
    }
    return (await res.json());
}
export async function ensureCsrf() {
    await fetch('/api/v1/auth/csrf', { credentials: 'include' });
}
