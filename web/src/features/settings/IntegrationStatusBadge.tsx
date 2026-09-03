import type { IntegrationStatus } from '../../lib/integrations';

const styles: Record<IntegrationStatus, { label: string; className: string }> = {
  connected: { label: 'Connected', className: 'bg-emerald-50 text-emerald-700 ring-emerald-200' },
  pending: { label: 'Pending', className: 'bg-amber-50 text-amber-700 ring-amber-200' },
  degraded: { label: 'Degraded', className: 'bg-amber-50 text-amber-800 ring-amber-300' },
  auth_failed: { label: 'Auth failed', className: 'bg-rose-50 text-rose-700 ring-rose-200' },
  disabled: { label: 'Disabled', className: 'bg-slate-100 text-slate-600 ring-slate-200' },
  unknown: { label: 'Unknown', className: 'bg-slate-100 text-slate-600 ring-slate-200' },
};

type Props = { status: IntegrationStatus };

export function IntegrationStatusBadge({ status }: Props) {
  const s = styles[status] ?? styles.unknown;
  return (
    <span
      className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium ring-1 ring-inset ${s.className}`}
      aria-label={`Integration status: ${s.label}`}
    >
      <span
        aria-hidden="true"
        className={
          'h-1.5 w-1.5 rounded-full ' +
          (status === 'connected'
            ? 'bg-emerald-500'
            : status === 'auth_failed'
              ? 'bg-rose-500'
              : status === 'degraded' || status === 'pending'
                ? 'bg-amber-500'
                : 'bg-slate-400')
        }
      />
      {s.label}
    </span>
  );
}
