import { Button } from '../../components/Button';
import { Spinner } from '../../components/Spinner';
import { ApiError } from '../../lib/api';
import { useApplyOBA, useOBAStatus, useWithdrawOBA } from '../../lib/integration-settings';

// OBATab renders the Official Business Account section. State
// transitions Meta allows drive which button surfaces:
//   NOT_APPLIED | REJECTED | CANCELLED → Apply
//   PENDING                             → Withdraw
//   APPROVED                            → informational only
export function OBATab({ integrationID, active }: { integrationID: string; active: boolean }) {
  const query = useOBAStatus(integrationID, active);
  const apply = useApplyOBA(integrationID);
  const withdraw = useWithdrawOBA(integrationID);

  if (query.isPending) {
    return (
      <div className="flex items-center justify-center py-12">
        <Spinner className="h-6 w-6 text-slate-500" label="Loading OBA status" />
      </div>
    );
  }
  if (query.isError) {
    const detail = query.error.problem.detail ?? query.error.problem.title ?? 'Failed to load OBA status';
    return (
      <div role="alert" className="rounded-lg border border-rose-200 bg-rose-50 p-3 text-sm text-rose-800">
        {detail}
      </div>
    );
  }

  const status = (query.data.oba_status ?? 'NOT_APPLIED').toUpperCase();
  const message = query.data.status_message ?? '';

  const mutationErr = (apply.error instanceof ApiError && apply.error) || (withdraw.error instanceof ApiError && withdraw.error) || null;
  const mutationErrText = mutationErr !== null
    ? mutationErr.problem.detail ?? mutationErr.problem.title ?? 'Request failed'
    : null;

  const pillClass = pillClasses(status);
  const canApply = ['NOT_APPLIED', 'REJECTED', 'CANCELLED', ''].includes(status);
  const canWithdraw = status === 'PENDING';

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-3">
        <span className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-semibold ${pillClass}`}>
          {status || 'UNKNOWN'}
        </span>
        {message !== '' && <span className="text-sm text-slate-600">{message}</span>}
      </div>

      <div className="rounded-lg border border-slate-200 bg-slate-50 p-3 text-xs text-slate-600">
        <p className="font-medium text-slate-700">Eligibility</p>
        <p className="mt-1">
          Official Business Account status is granted by Meta based on brand recognition + policy
          compliance. See <span className="font-mono text-[11px]">docs/providers/whatsapp.md</span> for
          the current criteria.
        </p>
      </div>

      {mutationErrText !== null && (
        <div role="alert" className="rounded-lg border border-rose-200 bg-rose-50 p-3 text-sm text-rose-800">
          {mutationErrText}
        </div>
      )}

      <div className="flex justify-end gap-2">
        {canApply && (
          <Button
            variant="primary"
            loading={apply.isPending}
            onClick={() => {
              apply.mutate();
            }}
          >
            Apply for OBA
          </Button>
        )}
        {canWithdraw && (
          <Button
            variant="secondary"
            loading={withdraw.isPending}
            onClick={() => {
              withdraw.mutate();
            }}
          >
            Withdraw application
          </Button>
        )}
        {status === 'APPROVED' && (
          <span className="text-sm text-emerald-700">This account is approved as an OBA.</span>
        )}
      </div>
    </div>
  );
}

function pillClasses(status: string): string {
  switch (status) {
    case 'APPROVED':
      return 'bg-emerald-100 text-emerald-800';
    case 'PENDING':
      return 'bg-amber-100 text-amber-800';
    case 'REJECTED':
      return 'bg-rose-100 text-rose-800';
    case 'CANCELLED':
    case 'NOT_APPLIED':
    default:
      return 'bg-slate-200 text-slate-700';
  }
}
