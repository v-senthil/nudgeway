import { useEffect, useState } from 'react';
import { createRoute, useNavigate } from '@tanstack/react-router';
import { rootRoute } from './__root';
import { useMe } from '../lib/auth';
import { Spinner } from '../components/Spinner';
import { KpiCard } from '../components/KpiCard';
import { Sparkline } from '../components/Sparkline';
import { Header } from '../features/inbox/Header';
import { MetaAnalyticsTab } from '../features/analytics/MetaAnalyticsTab';
import {
  defaultRange,
  useAnalyticsOverview,
  useAnalyticsSeries,
  useCallsSeries,
  type DateRange,
} from '../lib/analytics';
import { ApiError } from '../lib/api';

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

// TabId enumerates the top-level tabs on the Analytics page. Kept as a
// string union so switch/select is exhaustive at compile time.
type TabId = 'nudgeway' | 'meta';

// NudgewayTab wraps the original Nudgeway-side analytics view (local
// rollups, delivery rate, response time, calls). Split out of the page
// component so the top-level render stays a thin router between tabs.
function NudgewayTab() {
  const [range, setRange] = useState<DateRange>(() => defaultRange());
  const overview = useAnalyticsOverview(range);
  const messagesSeries = useAnalyticsSeries('messages_daily', range);
  const deliverySeries = useAnalyticsSeries('delivery_rate', range);
  const callsSeries = useCallsSeries(range);

  const ov = overview.data;
  const messagesPoints = messagesSeries.data?.points ?? [];
  const deliveryPoints = deliverySeries.data?.points ?? [];
  const callsPoints = callsSeries.data?.points ?? [];

  const formatAvgCallDuration = (seconds: number | undefined): string => {
    if (seconds === undefined || seconds <= 0) return '—';
    const total = Math.floor(seconds);
    const m = Math.floor(total / 60);
    const s = total % 60;
    return `${m}:${s.toString().padStart(2, '0')}`;
  };

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-end gap-2 text-sm text-slate-600">
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

      <AnalyticsErrorRow
        err={
          overview.error ??
          messagesSeries.error ??
          deliverySeries.error ??
          callsSeries.error ??
          null
        }
      />

      <section className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
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

      <section className="grid grid-cols-1 gap-4 sm:grid-cols-3">
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

      <section className="grid grid-cols-1 gap-4 lg:grid-cols-2">
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
        <div className="flex items-center gap-2 text-sm text-slate-500">
          <Spinner className="h-4 w-4" label="Loading analytics" /> Loading analytics…
        </div>
      ) : null}
    </div>
  );
}

function AnalyticsPage() {
  const me = useMe();
  const navigate = useNavigate();
  const [tab, setTab] = useState<TabId>('nudgeway');

  useEffect(() => {
    if (!me.isPending && (me.data === null || me.data === undefined)) {
      void navigate({ to: '/login' });
    }
  }, [me.isPending, me.data, navigate]);

  if (me.isPending) {
    return (
      <div className="flex min-h-screen items-center justify-center text-slate-500">
        <Spinner className="h-6 w-6" label="Loading session" />
      </div>
    );
  }
  if (me.data === null || me.data === undefined) return null;

  const tabs: Array<{ id: TabId; label: string }> = [
    { id: 'nudgeway', label: 'Nudgeway' },
    { id: 'meta', label: 'Meta Analytics' },
  ];

  return (
    <div className="min-h-screen bg-slate-50">
      <Header me={me.data} />
      <main className="mx-auto max-w-6xl px-6 py-8">
        <div className="mb-6 flex items-center justify-between">
          <h1 className="text-2xl font-semibold text-slate-900">Analytics</h1>
        </div>

        <div className="mb-6 border-b border-slate-200">
          <nav className="-mb-px flex gap-6" aria-label="Analytics tabs">
            {tabs.map((t) => {
              const selected = tab === t.id;
              return (
                <button
                  key={t.id}
                  type="button"
                  onClick={() => setTab(t.id)}
                  aria-current={selected ? 'page' : undefined}
                  className={`border-b-2 px-1 py-2 text-sm font-medium transition ${
                    selected
                      ? 'border-slate-900 text-slate-900'
                      : 'border-transparent text-slate-500 hover:border-slate-300 hover:text-slate-700'
                  }`}
                >
                  {t.label}
                </button>
              );
            })}
          </nav>
        </div>

        {tab === 'nudgeway' ? <NudgewayTab /> : <MetaAnalyticsTab active={tab === 'meta'} />}
      </main>
    </div>
  );
}

export const analyticsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/analytics',
  component: AnalyticsPage,
});
