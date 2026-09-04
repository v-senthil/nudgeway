import { useEffect, useState } from 'react';
import { Button } from '../../components/Button';
import {
  detectNgrokTunnel,
  useSetIntegrationWebhook,
  type Integration,
} from '../../lib/integrations';
import { ApiError } from '../../lib/api';

type Props = {
  open: boolean;
  integration: Integration | null;
  onClose: () => void;
};

/**
 * SetWebhookModal collects a public base URL and pushes it to the
 * provider (Meta) as the webhook callback override. It auto-detects a
 * running ngrok tunnel when opened so the operator usually clicks Set
 * without typing anything. The verify token is reused from the stored
 * integration secrets — the operator never handles it here.
 */
export function SetWebhookModal({ open, integration, onClose }: Props) {
  const [publicURL, setPublicURL] = useState('');
  const [detecting, setDetecting] = useState(false);
  const [detectResult, setDetectResult] = useState<
    { kind: 'none' } | { kind: 'found'; url: string } | { kind: 'missing' }
  >({ kind: 'none' });
  const [result, setResult] = useState<
    { ok: true; webhookURL: string } | { ok: false; message: string } | null
  >(null);

  const setWebhook = useSetIntegrationWebhook();

  useEffect(() => {
    if (!open) return;
    setPublicURL('');
    setResult(null);
    setDetectResult({ kind: 'none' });
    setDetecting(true);
    detectNgrokTunnel()
      .then((url) => {
        if (url !== '') {
          setPublicURL(url);
          setDetectResult({ kind: 'found', url });
        } else {
          setDetectResult({ kind: 'missing' });
        }
      })
      .finally(() => setDetecting(false));
  }, [open, integration?.id]);

  if (!open || integration === null) return null;

  const previewURL =
    publicURL.trim() === ''
      ? ''
      : `${publicURL.replace(/\/$/, '')}/webhooks/${integration.provider}/${integration.id}`;

  const submit = async () => {
    setResult(null);
    try {
      const res = await setWebhook.mutateAsync({
        id: integration.id,
        public_url: publicURL.trim(),
      });
      setResult({ ok: true, webhookURL: res.webhook_url });
    } catch (err) {
      const message =
        err instanceof ApiError
          ? err.problem.detail ?? err.problem.title ?? 'Set webhook failed'
          : err instanceof Error
            ? err.message
            : 'Set webhook failed';
      setResult({ ok: false, message });
    }
  };

  const redetect = async () => {
    setDetecting(true);
    setDetectResult({ kind: 'none' });
    const url = await detectNgrokTunnel();
    if (url !== '') {
      setPublicURL(url);
      setDetectResult({ kind: 'found', url });
    } else {
      setDetectResult({ kind: 'missing' });
    }
    setDetecting(false);
  };

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-label={`Set webhook for ${integration.name}`}
      className="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/50 p-4"
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div className="w-full max-w-lg overflow-hidden rounded-2xl bg-white shadow-xl">
        <div className="flex items-start justify-between border-b border-slate-200 px-5 py-4">
          <div>
            <h2 className="text-base font-semibold text-slate-900">Set webhook</h2>
            <p className="mt-0.5 text-xs text-slate-500">
              Push the callback URL to Meta for <span className="font-medium">{integration.name}</span>.
            </p>
          </div>
          <button
            type="button"
            aria-label="Close"
            onClick={onClose}
            className="rounded-md p-1 text-slate-400 hover:bg-slate-100 hover:text-slate-600"
          >
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" className="h-5 w-5">
              <path d="M6 6l12 12M18 6L6 18" strokeLinecap="round" />
            </svg>
          </button>
        </div>

        <div className="space-y-4 px-5 py-4">
          <div>
            <label htmlFor="public-url" className="block text-xs font-medium text-slate-600">
              Public base URL
            </label>
            <div className="mt-1 flex gap-2">
              <input
                id="public-url"
                type="url"
                value={publicURL}
                onChange={(e) => setPublicURL(e.target.value)}
                placeholder="https://a1b2.ngrok-free.app"
                className="flex-1 rounded-md border border-slate-300 px-3 py-1.5 text-sm shadow-sm placeholder:text-slate-400 focus:border-emerald-500 focus:outline-none focus:ring-1 focus:ring-emerald-500"
              />
              <Button variant="secondary" onClick={() => void redetect()} loading={detecting}>
                Detect ngrok
              </Button>
            </div>
            {detectResult.kind === 'found' && (
              <p className="mt-1 text-[11px] text-emerald-700">
                Detected ngrok tunnel — prefilled above.
              </p>
            )}
            {detectResult.kind === 'missing' && (
              <p className="mt-1 text-[11px] text-slate-500">
                No local ngrok tunnel detected. Paste your public URL or run{' '}
                <code className="rounded bg-slate-100 px-1 py-0.5 font-mono text-[11px]">
                  ngrok http 8080
                </code>
                .
              </p>
            )}
          </div>

          {previewURL !== '' && (
            <div className="rounded-lg border border-slate-200 bg-slate-50 px-3 py-2">
              <div className="text-[11px] uppercase tracking-wide text-slate-500">Full webhook URL</div>
              <div className="mt-0.5 break-all font-mono text-xs text-slate-800">{previewURL}</div>
            </div>
          )}

          <p className="text-[11px] text-slate-500">
            Nudgeway uses the verify token stored with this integration; you don't need to enter it again.
          </p>

          {result !== null && result.ok && (
            <div className="rounded-lg border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-800">
              Webhook pushed to Meta. Callback URL:
              <div className="mt-1 break-all font-mono text-xs">{result.webhookURL}</div>
            </div>
          )}
          {result !== null && !result.ok && (
            <div className="rounded-lg border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-800">
              {result.message}
            </div>
          )}
        </div>

        <div className="flex items-center justify-end gap-2 border-t border-slate-200 bg-slate-50 px-5 py-3">
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button onClick={() => void submit()} loading={setWebhook.isPending} disabled={publicURL.trim() === ''}>
            Set webhook
          </Button>
        </div>
      </div>
    </div>
  );
}
