// KpiCard renders one dashboard KPI: a small-caps title, a big number,
// and an optional footnote. Shared between the Nudgeway and Meta
// analytics tabs so both tabs stay visually identical.
export function KpiCard({
  title,
  value,
  footnote,
}: {
  title: string;
  value: string;
  footnote?: string;
}) {
  return (
    <div className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm">
      <div className="text-xs font-medium uppercase tracking-wide text-slate-500">{title}</div>
      <div className="mt-2 text-3xl font-semibold tabular-nums text-slate-900">{value}</div>
      {footnote !== undefined && footnote !== '' ? (
        <div className="mt-1 text-xs text-slate-500">{footnote}</div>
      ) : null}
    </div>
  );
}
