import { useState } from 'react';
import { Modal } from '../../components/Modal';
import { Button } from '../../components/Button';

type Props = {
  open: boolean;
  onClose: () => void;
  /** Full plaintext token — shown once, never fetchable again. */
  plaintext: string;
  /** Human name so the operator knows which token they just minted. */
  name: string;
};

/** APITokenCreatedModal displays the plaintext token exactly once.
 * The backend cannot re-emit it, so the modal has to make the "copy
 * now or lose it" contract obvious. */
export function APITokenCreatedModal({ open, onClose, plaintext, name }: Props) {
  const [copied, setCopied] = useState(false);
  const [copyError, setCopyError] = useState<string | null>(null);

  const copy = async () => {
    setCopyError(null);
    try {
      await navigator.clipboard.writeText(plaintext);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    } catch {
      setCopyError('Could not access the clipboard. Copy the value manually.');
    }
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="Token created"
      footer={
        <Button variant="primary" onClick={onClose}>
          Done
        </Button>
      }
    >
      <div className="space-y-4">
        <div
          role="alert"
          className="flex items-start gap-2 rounded-xl bg-amber-50 px-3 py-2.5 text-xs text-amber-900 ring-1 ring-inset ring-amber-200"
        >
          <svg
            aria-hidden="true"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.75"
            strokeLinecap="round"
            strokeLinejoin="round"
            className="mt-0.5 h-4 w-4 flex-shrink-0"
          >
            <path d="M12 9v4" />
            <path d="M12 17h.01" />
            <path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0Z" />
          </svg>
          <p>
            <span className="font-semibold">Copy this token now.</span> It won&rsquo;t be shown
            again — if you lose it you&rsquo;ll have to revoke and mint a new one.
          </p>
        </div>

        <div>
          <p className="text-xs font-medium text-slate-700">
            {name === '' ? 'Token' : name}
          </p>
          <div className="mt-1 flex items-center gap-2 rounded-xl border border-slate-200 bg-slate-50 px-3 py-2">
            <code className="flex-1 overflow-x-auto whitespace-nowrap font-mono text-xs text-slate-800">
              {plaintext}
            </code>
            <button
              type="button"
              onClick={() => void copy()}
              aria-label="Copy token to clipboard"
              className="rounded-lg bg-white px-2 py-1 text-xs font-medium text-emerald-700 shadow-sm ring-1 ring-inset ring-slate-200 hover:bg-emerald-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-emerald-500"
            >
              {copied ? 'Copied' : 'Copy'}
            </button>
          </div>
          {copyError !== null && (
            <p role="alert" className="mt-2 text-xs text-rose-700">
              {copyError}
            </p>
          )}
        </div>

        <p className="text-xs text-slate-500">
          Send it as a Bearer header:{' '}
          <code className="rounded bg-slate-100 px-1">Authorization: Bearer &lt;token&gt;</code>
        </p>
      </div>
    </Modal>
  );
}
