import { useState } from 'react';
import { Button } from '../../components/Button';
import { ConfirmDialog } from '../../components/ConfirmDialog';
import { Input } from '../../components/Input';
import { Spinner } from '../../components/Spinner';
import { ApiError } from '../../lib/api';
import {
  useDeleteUsername,
  useSetUsername,
  useUsername,
  useUsernameSuggestions,
  validateUsername,
} from '../../lib/integration-settings';

// UsernameTab renders the Business-Scoped Username surface for a
// WhatsApp integration. Two panes stacked vertically:
//   1) "Current" — a big @handle + status pill + Delete affordance (only
//      rendered when a username is adopted).
//   2) "Adopt / change" — a live-validated form with an optional
//      force-transfer checkbox for the "already held on another PNID"
//      path (Meta error 147005).
//   3) Reserved suggestions — collapsed by default; the network call is
//      fired lazily to preserve Meta's suggestions quota.
export function UsernameTab({ integrationID, active }: { integrationID: string; active: boolean }) {
  const query = useUsername(integrationID, active);
  const setMut = useSetUsername(integrationID);
  const delMut = useDeleteUsername(integrationID);

  const [draft, setDraft] = useState('');
  const [forceTransfer, setForceTransfer] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [showSuggestions, setShowSuggestions] = useState(false);

  const suggestionsQuery = useUsernameSuggestions(integrationID, active && showSuggestions);

  if (query.isPending) {
    return (
      <div className="flex items-center justify-center py-12">
        <Spinner className="h-6 w-6 text-slate-500" label="Loading username" />
      </div>
    );
  }
  if (query.isError) {
    const detail = query.error.problem.detail ?? query.error.problem.title ?? 'Failed to load username';
    return (
      <div role="alert" className="rounded-lg border border-rose-200 bg-rose-50 p-3 text-sm text-rose-800">
        {detail}
      </div>
    );
  }

  const current = query.data.username ?? '';
  const currentStatus = (query.data.status ?? '').toLowerCase();
  const hasUsername = current.length > 0;

  const draftNormalized = draft.trim().toLowerCase();
  const draftError = draftNormalized.length === 0 ? null : validateUsername(draftNormalized);
  const canSubmit = draftNormalized.length > 0 && draftError === null && !setMut.isPending;

  const mutErr =
    (setMut.error instanceof ApiError && setMut.error) ||
    (delMut.error instanceof ApiError && delMut.error) ||
    null;
  const mutErrText = mutErr !== null
    ? mutErr.problem.detail ?? mutErr.problem.title ?? 'Request failed'
    : null;

  const onSubmit = (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (!canSubmit) return;
    setMut.mutate(
      { username: draftNormalized, transfer_action: forceTransfer ? 'force_transfer' : 'none' },
      {
        onSuccess: () => {
          setDraft('');
          setForceTransfer(false);
        },
      },
    );
  };

  const onDelete = () => {
    delMut.mutate(undefined, {
      onSuccess: () => setConfirmDelete(false),
    });
  };

  return (
    <div className="space-y-6">
      {hasUsername && (
        <section className="space-y-3 rounded-lg border border-slate-200 bg-slate-50 p-4">
          <div className="flex items-center justify-between gap-3">
            <div>
              <p className="text-xs uppercase tracking-wide text-slate-500">Current username</p>
              <p className="mt-1 font-mono text-xl font-semibold text-slate-900">@{current}</p>
            </div>
            <span className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-semibold ${statusPill(currentStatus)}`}>
              {currentStatus || 'unknown'}
            </span>
          </div>
          <div className="flex justify-end">
            <Button
              variant="ghost"
              onClick={() => setConfirmDelete(true)}
              disabled={delMut.isPending}
            >
              Remove username
            </Button>
          </div>
        </section>
      )}

      <section className="space-y-3">
        <header>
          <h3 className="text-sm font-semibold text-slate-900">
            {hasUsername ? 'Change username' : 'Adopt a username'}
          </h3>
          <p className="mt-1 text-xs text-slate-500">
            Lowercase letters, digits, "." and "_". 3–35 characters. Must contain at least one letter,
            cannot start or end with ".", cannot contain "..", and cannot look like a domain.
          </p>
        </header>

        <form className="space-y-3" onSubmit={onSubmit}>
          <Input
            label="New username"
            placeholder="acme_support"
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            error={draftError ?? undefined}
            autoComplete="off"
            spellCheck={false}
          />

          <label className="flex items-start gap-2 text-sm text-slate-700">
            <input
              type="checkbox"
              className="mt-0.5 h-4 w-4 rounded border-slate-300 text-emerald-600 focus:ring-emerald-500"
              checked={forceTransfer}
              onChange={(e) => setForceTransfer(e.target.checked)}
            />
            <span>
              This username is on another of my phone numbers — force transfer
              <span className="ml-1 text-xs text-slate-500">
                (needed when Meta returns error 147005)
              </span>
            </span>
          </label>

          {mutErrText !== null && (
            <div role="alert" className="rounded-lg border border-rose-200 bg-rose-50 p-3 text-sm text-rose-800">
              {mutErrText}
            </div>
          )}

          <div className="flex justify-end">
            <Button
              type="submit"
              variant="primary"
              loading={setMut.isPending}
              disabled={!canSubmit}
            >
              {hasUsername ? 'Change username' : 'Adopt username'}
            </Button>
          </div>
        </form>
      </section>

      <section className="space-y-3">
        <button
          type="button"
          className="text-sm font-medium text-emerald-700 hover:underline"
          onClick={() => setShowSuggestions((v) => !v)}
        >
          {showSuggestions ? 'Hide reserved suggestions' : 'Show reserved suggestions'}
        </button>

        {showSuggestions && (
          <div className="rounded-lg border border-slate-200 bg-white p-3">
            {suggestionsQuery.isPending && (
              <div className="flex items-center gap-2 text-sm text-slate-500">
                <Spinner className="h-4 w-4" label="Loading suggestions" />
                <span>Loading suggestions…</span>
              </div>
            )}
            {suggestionsQuery.isError && (
              <div role="alert" className="text-sm text-rose-700">
                {suggestionsQuery.error.problem.detail ?? suggestionsQuery.error.problem.title ?? 'Failed to load suggestions'}
              </div>
            )}
            {suggestionsQuery.isSuccess && (
              suggestionsQuery.data.suggestions.length === 0 ? (
                <p className="text-sm text-slate-500">Meta returned no suggestions for this phone number.</p>
              ) : (
                <ul className="flex flex-wrap gap-2">
                  {suggestionsQuery.data.suggestions.map((s) => (
                    <li key={s}>
                      <button
                        type="button"
                        className="rounded-full border border-slate-200 bg-slate-50 px-3 py-1 text-xs font-mono text-slate-700 hover:bg-emerald-50 hover:text-emerald-800 hover:border-emerald-200"
                        onClick={() => setDraft(s)}
                      >
                        @{s}
                      </button>
                    </li>
                  ))}
                </ul>
              )
            )}
          </div>
        )}
      </section>

      <ConfirmDialog
        open={confirmDelete}
        title="Remove username"
        message={`Release @${current}? The phone number will have no business username until you adopt a new one.`}
        confirmLabel="Remove username"
        tone="danger"
        loading={delMut.isPending}
        onConfirm={onDelete}
        onCancel={() => setConfirmDelete(false)}
      />
    </div>
  );
}

function statusPill(status: string): string {
  switch (status) {
    case 'approved':
      return 'bg-emerald-100 text-emerald-800';
    case 'reserved':
      return 'bg-amber-100 text-amber-800';
    default:
      return 'bg-slate-200 text-slate-700';
  }
}
