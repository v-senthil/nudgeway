import { useState } from 'react';
import { Button } from '../../components/Button';
import { IntegrationStatusBadge } from './IntegrationStatusBadge';
import { DeleteConfirmModal } from './DeleteConfirmModal';
import {
  integrationPhoneNumberID,
  integrationWABAID,
  useDeleteIntegration,
  useTestIntegration,
} from '../../lib/integrations';
import type { Integration } from '../../lib/integrations';
import { ApiError } from '../../lib/api';

type Props = {
  items: Integration[];
  onOpenSettings?: (it: Integration) => void;
};

export function IntegrationList({ items, onOpenSettings }: Props) {
  const [pendingDelete, setPendingDelete] = useState<Integration | null>(null);
  const [testResult, setTestResult] = useState<{ id: string; ok: boolean; message: string } | null>(null);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  const test = useTestIntegration();
  const del = useDeleteIntegration();

  const runTest = async (id: string) => {
    setTestResult(null);
    try {
      const res = await test.mutateAsync(id);
      setTestResult({ id, ok: res.ok, message: res.message ?? (res.ok ? 'Connection successful' : 'Connection failed') });
    } catch (err) {
      const message = err instanceof ApiError ? err.problem.detail ?? err.problem.title ?? 'Test failed' : 'Test failed';
      setTestResult({ id, ok: false, message });
    }
    window.setTimeout(() => setTestResult((r) => (r?.id === id ? null : r)), 4000);
  };

  const confirmDelete = async () => {
    if (pendingDelete === null) return;
    setDeleteError(null);
    try {
      await del.mutateAsync(pendingDelete.id);
      setPendingDelete(null);
    } catch (err) {
      setDeleteError(err instanceof ApiError ? err.problem.detail ?? err.problem.title ?? 'Delete failed' : 'Delete failed');
    }
  };

  return (
    <div className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm">
      <table className="min-w-full divide-y divide-slate-200">
        <thead className="bg-slate-50">
          <tr>
            <th scope="col" className="px-4 py-2 text-left text-xs font-semibold uppercase tracking-wide text-slate-500">
              Name
            </th>
            <th scope="col" className="px-4 py-2 text-left text-xs font-semibold uppercase tracking-wide text-slate-500">
              Provider
            </th>
            <th scope="col" className="px-4 py-2 text-left text-xs font-semibold uppercase tracking-wide text-slate-500">
              Status
            </th>
            <th scope="col" className="px-4 py-2 text-right text-xs font-semibold uppercase tracking-wide text-slate-500">
              <span className="sr-only">Actions</span>
            </th>
          </tr>
        </thead>
        <tbody className="divide-y divide-slate-100">
          {items.map((it) => (
            <tr key={it.id} className="text-sm">
              <td className="px-4 py-3 font-medium text-slate-900">
                {it.name}
                {(() => {
                  const pnid = integrationPhoneNumberID(it);
                  const waba = integrationWABAID(it);
                  if (pnid === undefined && waba === undefined) return null;
                  return (
                    <div className="mt-1 flex flex-wrap gap-1.5">
                      {pnid !== undefined && (
                        <span
                          title={`Phone Number ID: ${pnid}`}
                          className="inline-flex max-w-[14rem] items-center gap-1 rounded-md bg-slate-50 px-1.5 py-0.5 font-mono text-[11px] text-slate-500 ring-1 ring-inset ring-slate-200"
                        >
                          <span className="font-sans font-medium text-slate-400">PHONE</span>
                          <span className="truncate">{pnid}</span>
                        </span>
                      )}
                      {waba !== undefined && (
                        <span
                          title={`WABA ID: ${waba}`}
                          className="inline-flex max-w-[14rem] items-center gap-1 rounded-md bg-slate-50 px-1.5 py-0.5 font-mono text-[11px] text-slate-500 ring-1 ring-inset ring-slate-200"
                        >
                          <span className="font-sans font-medium text-slate-400">WABA</span>
                          <span className="truncate">{waba}</span>
                        </span>
                      )}
                    </div>
                  );
                })()}
              </td>
              <td className="px-4 py-3 text-slate-600 capitalize">{it.provider}</td>
              <td className="px-4 py-3">
                <IntegrationStatusBadge status={it.status} />
                {testResult !== null && testResult.id === it.id && (
                  <p
                    role="status"
                    className={
                      'mt-1 text-xs ' + (testResult.ok ? 'text-emerald-700' : 'text-rose-700')
                    }
                  >
                    {testResult.message}
                  </p>
                )}
              </td>
              <td className="px-4 py-3 text-right">
                <div className="inline-flex items-center gap-2">
                  <Button
                    variant="secondary"
                    onClick={() => void runTest(it.id)}
                    loading={test.isPending && test.variables === it.id}
                    aria-label={`Test connection for ${it.name}`}
                  >
                    Test
                  </Button>
                  {onOpenSettings !== undefined && (
                    <button
                      type="button"
                      aria-label={`Open settings for ${it.name}`}
                      onClick={() => onOpenSettings(it)}
                      className="flex h-9 w-9 items-center justify-center rounded-full text-slate-500 hover:bg-slate-100 hover:text-slate-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-emerald-500 focus-visible:ring-offset-2"
                    >
                      <svg
                        aria-hidden="true"
                        viewBox="0 0 24 24"
                        fill="none"
                        stroke="currentColor"
                        strokeWidth="1.8"
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        className="h-5 w-5"
                      >
                        <path d="M12 15.5a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7Z" />
                        <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06A1.65 1.65 0 0 0 15 19.4a1.65 1.65 0 0 0-1 1.51V21a2 2 0 1 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.6 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 1 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.6a1.65 1.65 0 0 0 1-1.51V3a2 2 0 1 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 1 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1Z" />
                      </svg>
                    </button>
                  )}
                  <Button
                    variant="ghost"
                    onClick={() => setPendingDelete(it)}
                    aria-label={`Delete integration ${it.name}`}
                    className="text-rose-700 hover:bg-rose-50"
                  >
                    Delete
                  </Button>
                </div>
              </td>
            </tr>
          ))}
        </tbody>
      </table>

      <DeleteConfirmModal
        open={pendingDelete !== null}
        onClose={() => {
          setPendingDelete(null);
          setDeleteError(null);
        }}
        onConfirm={() => void confirmDelete()}
        loading={del.isPending}
        title={`Delete ${pendingDelete?.name ?? 'integration'}?`}
        description="This disconnects the channel and stops message delivery. You'll need to reconnect and re-register the Meta webhook to receive messages again."
        errorMessage={deleteError ?? undefined}
      />
    </div>
  );
}
