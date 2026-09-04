import { useMutation } from '@tanstack/react-query';
import { ApiError, type ProblemDetail } from './api';
import { readCookie } from './cookie';

/** Largest file the composer will accept before showing a client-side error. Mirrors the backend cap. */
export const MAX_ATTACHMENT_BYTES = 16 * 1024 * 1024;

/** Server response for POST /api/v1/attachments. */
export type UploadResult = {
  attachment_id: string;
  media_url: string;
  /**
   * Meta-native handle returned by Media Upload API. Preferred over
   * media_url when present — Meta uses it directly instead of re-fetching
   * a URL. Empty when the server couldn't upload to the provider.
   */
  media_id?: string;
  provider?: string;
  size: number;
  content_type: string;
  filename?: string;
};

const CSRF_COOKIE = 'fullwa_csrf';

/** Map a MIME type to the canonical WhatsApp message type the send API accepts. */
export function mediaKindFromContentType(contentType: string): 'image' | 'video' | 'audio' | 'document' {
  const ct = contentType.toLowerCase();
  if (ct.startsWith('image/')) return 'image';
  if (ct.startsWith('video/')) return 'video';
  if (ct.startsWith('audio/')) return 'audio';
  return 'document';
}

/**
 * useUploadAttachment posts a File to /api/v1/attachments as multipart/form-data
 * with the standard CSRF header, returning the persisted `{attachment_id, media_url, ...}`.
 * Callers own progress UI — the promise resolves once the server responds.
 */
export function useUploadAttachment() {
  return useMutation<UploadResult, ApiError, File>({
    mutationFn: async (file: File) => {
      if (file.size > MAX_ATTACHMENT_BYTES) {
        throw new ApiError(413, {
          status: 413,
          title: 'attachment_too_large',
          detail: `attachment exceeds ${MAX_ATTACHMENT_BYTES} bytes`,
        });
      }

      const form = new FormData();
      form.append('file', file, file.name);

      const headers: Record<string, string> = {
        Accept: 'application/json, application/problem+json',
      };
      const csrf = readCookie(CSRF_COOKIE);
      if (csrf !== null) {
        headers['X-CSRF-Token'] = csrf;
      }

      const res = await fetch('/api/v1/attachments', {
        method: 'POST',
        credentials: 'include',
        headers,
        body: form,
      });

      const contentType = res.headers.get('content-type') ?? '';
      const isJson =
        contentType.includes('application/json') || contentType.includes('application/problem+json');

      if (!res.ok) {
        let problem: ProblemDetail = { status: res.status, title: res.statusText };
        if (isJson) {
          try {
            problem = (await res.json()) as ProblemDetail;
          } catch {
            // fall through to default problem
          }
        }
        throw new ApiError(res.status, problem);
      }

      return (await res.json()) as UploadResult;
    },
  });
}
