import { useState } from 'react';
import { Button } from '../../components/Button';
import { IntegrationStatusBadge } from './IntegrationStatusBadge';
import { DeleteConfirmModal } from './DeleteConfirmModal';
import { useDeleteIntegration, useTestIntegration } from '../../lib/integrations';
import type { Integration } from '../../lib/integrations';
import { ApiError } from '../../lib/api';

type Props = {
  items: Integration[];
};

export function IntegrationList({ items }: Props) {
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
                {it.phone_number_id !== undefined && (
                  <p className="text-xs text-slate-500">Phone ID: {it.phone_number_id}</p>
                )}
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
                <div className="inline-flex gap-2">
                  <Button
                    variant="secondary"
                    onClick={() => void runTest(it.id)}
                    loading={test.isPending && test.variables === it.id}
                    aria-label={`Test connection for ${it.name}`}
                  >
                    Test
                  </Button>
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
