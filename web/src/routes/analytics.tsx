import { useEffect, useMemo, useState } from 'react';
import { createRoute, useNavigate } from '@tanstack/react-router';
import { rootRoute } from './__root';
import { useMe } from '../lib/auth';
import { Spinner } from '../components/Spinner';
import { Header } from '../features/inbox/Header';
import {
  defaultRange,
  useAnalyticsOverview,
  useAnalyticsSeries,
  useCallsSeries,
  type AnalyticsPoint,
  type DateRange,
} from '../lib/analytics';
import { ApiError } from '../lib/api';

// KpiCard renders one dashboard KPI: a title, a big number, and an
// optional footnote (used for the p50 units).
function KpiCard({
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

// Sparkline renders a simple SVG polyline. Kept dependency-free so we
// don't inflate the bundle with a full charting library. The Y axis is
// auto-scaled to the max Value in the series with a small headroom
// factor so the top of the line doesn't touch the frame.
function Sparkline({
  points,
  color,
  label,
  ariaLabel,
}: {
  points: AnalyticsPoint[];
  color: string;
  label: string;
  ariaLabel: string;
}) {
  const width = 640;
  const height = 140;
  const pad = 8;

  const geometry = useMemo(() => {
    if (points.length === 0) {
      return {
        path: '',
        area: '',
        maxY: 0,
        ticks: [] as Array<{ x: number; day: string }>,
        dots: [] as Array<{ x: number; y: number; day: string }>,
      };
    }
    const values = points.map((p) => p.value);
    const maxRaw = Math.max(...values, 1);
    // Add ~15% headroom so the line doesn't hug the top edge.
    const maxY = Math.max(1, Math.ceil(maxRaw * 1.15));
    const innerW = width - pad * 2;
    const stepX = points.length > 1 ? innerW / (points.length - 1) : 0;
    const innerH = height - pad * 2;
    const coords = points.map((p, i) => {
      // Center a single point in the chart so the operator sees where
      // the lone value sits along the time axis.
      const x = points.length > 1 ? pad + stepX * i : pad + innerW / 2;
      const y = pad + innerH - (p.value / maxY) * innerH;
      return { x, y, day: p.day };
    });
    const path = coords.map((c, i) => `${i === 0 ? 'M' : 'L'} ${c.x.toFixed(1)} ${c.y.toFixed(1)}`).join(' ');
    const area =
      coords.length > 1
        ? `${path} L ${coords[coords.length - 1]!.x.toFixed(1)} ${(height - pad).toFixed(1)} L ${coords[0]!.x.toFixed(1)} ${(height - pad).toFixed(1)} Z`
        : '';
    // Show at most 6 X labels — every Nth point.
    const stride = Math.max(1, Math.ceil(coords.length / 6));
    const ticks = coords.filter((_, i) => i % stride === 0).map((c) => ({ x: c.x, day: c.day }));
    // Render explicit dots on every point so single-point series (and
    // sparse days between multi-point series) stay visible — SVG will
    // not stroke a polyline with fewer than two vertices.
    const dots = coords.map((c) => ({ x: c.x, y: c.y, day: c.day }));
    return { path, area, maxY, ticks, dots };
  }, [points]);

  return (
    <div className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm">
      <div className="mb-2 flex items-baseline justify-between">
        <div className="text-sm font-medium text-slate-700">{label}</div>
        <div className="text-xs text-slate-500">max {geometry.maxY.toLocaleString()}</div>
      </div>
      {points.length === 0 ? (
        <div className="flex h-[140px] items-center justify-center text-sm text-slate-400">
          No data in the selected range.
        </div>
      ) : (
        <svg
          viewBox={`0 0 ${width} ${height}`}
          role="img"
          aria-label={ariaLabel}
          className="w-full"
        >
          <path d={geometry.area} fill={color} opacity={0.15} />
          <path d={geometry.path} fill="none" stroke={color} strokeWidth={2} />
          {geometry.dots.map((d) => (
            <circle key={`dot-${d.day}`} cx={d.x} cy={d.y} r={2.5} fill={color} />
          ))}
          {geometry.ticks.map((t) => (
            <text
              key={t.day}
              x={t.x}
              y={height - 2}
              textAnchor="middle"
              className="fill-slate-400 text-[9px]"
            >
              {t.day.slice(5)}
            </text>
          ))}
        </svg>
      )}
    </div>
  );
}

function toDateInput(iso: string): string {
  return iso;
}

function fromDateInput(local: string): string {
  return local;
}

function AnalyticsErrorRow({ err }: { err: ApiError | Error | null }) {
  if (err === null) return null;
  const detail = err instanceof ApiError ? (err.problem.detail ?? err.message) : err.message;
  return (
    <div className="rounded-xl border border-rose-200 bg-rose-50 p-3 text-sm text-rose-700">
      Failed to load analytics: {detail}
    </div>
  );
}

function AnalyticsPage() {
  const me = useMe();
  const navigate = useNavigate();
  const [range, setRange] = useState<DateRange>(() => defaultRange());

  useEffect(() => {
    if (!me.isPending && (me.data === null || me.data === undefined)) {
      void navigate({ to: '/login' });
    }
  }, [me.isPending, me.data, navigate]);

  const overview = useAnalyticsOverview(range);
  const messagesSeries = useAnalyticsSeries('messages_daily', range);
  const deliverySeries = useAnalyticsSeries('delivery_rate', range);
  const callsSeries = useCallsSeries(range);

  if (me.isPending) {
    return (
      <div className="flex min-h-screen items-center justify-center text-slate-500">
        <Spinner className="h-6 w-6" label="Loading session" />
      </div>
    );
  }
  if (me.data === null || me.data === undefined) return null;

  const ov = overview.data;
  const messagesPoints = messagesSeries.data?.points ?? [];
  const deliveryPoints = deliverySeries.data?.points ?? [];
  const callsPoints = callsSeries.data?.points ?? [];

  // formatAvgCallDuration renders the "Avg call duration" KPI as m:ss.
  // Zero and undefined both fall back to "—" to match the other KPI
  // cards.
  const formatAvgCallDuration = (seconds: number | undefined): string => {
    if (seconds === undefined || seconds <= 0) return '—';
    const total = Math.floor(seconds);
    const m = Math.floor(total / 60);
    const s = total % 60;
    return `${m}:${s.toString().padStart(2, '0')}`;
  };

  return (
    <div className="min-h-screen bg-slate-50">
      <Header me={me.data} />
      <main className="mx-auto max-w-6xl px-6 py-8">
        <div className="mb-6 flex items-center justify-between">
          <h1 className="text-2xl font-semibold text-slate-900">Analytics</h1>
          <div className="flex items-center gap-2 text-sm text-slate-600">
            <label className="flex items-center gap-1">
              From
              <input
                type="date"
                value={toDateInput(range.from)}
                onChange={(e) => setRange((r) => ({ ...r, from: fromDateInput(e.target.value) }))}
                className="rounded-md border border-slate-300 bg-white px-2 py-1 text-sm"
              />
            </label>
            <label className="flex items-center gap-1">
              To
              <input
                type="date"
                value={toDateInput(range.to)}
                onChange={(e) => setRange((r) => ({ ...r, to: fromDateInput(e.target.value) }))}
                className="rounded-md border border-slate-300 bg-white px-2 py-1 text-sm"
              />
            </label>
          </div>
        </div>

        <AnalyticsErrorRow
          err={
            overview.error ??
            messagesSeries.error ??
            deliverySeries.error ??
            callsSeries.error ??
            null
          }
        />

        <section className="mt-4 grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <KpiCard title="Messages" value={ov ? ov.messages_total.toLocaleString() : '—'} />
          <KpiCard
            title="Delivery rate"
            value={ov ? `${ov.delivery_rate_pct}%` : '—'}
            footnote="delivered / sent"
          />
          <KpiCard
            title="Response time"
            value={ov ? `${ov.response_time_seconds_p50}s` : '—'}
            footnote="p50 of daily averages"
          />
          <KpiCard
            title="Conversations opened"
            value={ov ? ov.conversations_opened.toLocaleString() : '—'}
          />
        </section>

        <section className="mt-4 grid grid-cols-1 gap-4 sm:grid-cols-3">
          <KpiCard
            title="Calls total"
            value={ov?.calls_total !== undefined ? ov.calls_total.toLocaleString() : '—'}
          />
          <KpiCard
            title="Calls answered"
            value={ov?.calls_answered !== undefined ? ov.calls_answered.toLocaleString() : '—'}
            footnote="answered / connected"
          />
          <KpiCard
            title="Avg call duration"
            value={formatAvgCallDuration(ov?.calls_avg_duration_seconds)}
            footnote="answered calls only"
          />
        </section>

        <section className="mt-6 grid grid-cols-1 gap-4 lg:grid-cols-2">
          <Sparkline
            points={messagesPoints}
            color="#059669"
            label="Messages per day"
            ariaLabel="Messages per day sparkline"
          />
          <Sparkline
            points={deliveryPoints}
            color="#4f46e5"
            label="Delivery rate (%)"
            ariaLabel="Delivery rate percentage sparkline"
          />
          <Sparkline
            points={callsPoints}
            color="#0ea5e9"
            label="Calls per day"
            ariaLabel="Calls per day sparkline"
          />
        </section>

        {(overview.isPending ||
          messagesSeries.isPending ||
          deliverySeries.isPending ||
          callsSeries.isPending) &&
        ov === undefined ? (
          <div className="mt-4 flex items-center gap-2 text-sm text-slate-500">
            <Spinner className="h-4 w-4" label="Loading analytics" /> Loading analytics…
          </div>
        ) : null}
      </main>
    </div>
  );
}

export const analyticsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/analytics',
  component: AnalyticsPage,
});
