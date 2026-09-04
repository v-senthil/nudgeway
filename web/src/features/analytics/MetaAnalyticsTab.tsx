import { useMemo, useState } from 'react';
import { Spinner } from '../../components/Spinner';
import { KpiCard } from '../../components/KpiCard';
import { Sparkline, type SparklinePoint } from '../../components/Sparkline';
import { ApiError } from '../../lib/api';
import {
  useIntegrations,
  type Integration,
} from '../../lib/integrations';
import {
  defaultMetaRange,
  epochToISODay,
  formatCurrency,
  rangeDaysAgo,
  useMetaCalls,
  useMetaConversations,
  useMetaMessaging,
  useMetaPricing,
  useMetaTemplates,
  type MetaCallPoint,
  type MetaConversationPoint,
  type MetaGranularity,
  type MetaMessagingPoint,
  type MetaPricingPoint,
  type MetaRange,
  type MetaTemplatePoint,
} from '../../lib/meta-analytics';

// SectionError renders a red banner with the RFC 7807 detail or a
// generic error message. Kept identical to the Nudgeway tab's
// AnalyticsErrorRow so operators see one consistent failure shape.
function SectionError({ err }: { err: ApiError | Error | null | undefined }) {
  if (err === null || err === undefined) return null;
  const detail = err instanceof ApiError ? (err.problem.detail ?? err.message) : err.message;
  return (
    <div className="rounded-xl border border-rose-200 bg-rose-50 p-3 text-sm text-rose-700">
      Failed to load: {detail}
    </div>
  );
}

// SectionSpinner is a tiny inline "loading …" row shown while a section
// still has no cached data.
function SectionSpinner({ label }: { label: string }) {
  return (
    <div className="flex items-center gap-2 text-sm text-slate-500">
      <Spinner className="h-4 w-4" label={label} /> {label}…
    </div>
  );
}

function formatInt(n: number | undefined): string {
  if (n === undefined || !Number.isFinite(n)) return '—';
  return Math.round(n).toLocaleString();
}

function formatPct(numerator: number, denominator: number): string {
  if (denominator <= 0) return '—';
  return `${((numerator / denominator) * 100).toFixed(1)}%`;
}

function formatDurationSeconds(seconds: number | undefined): string {
  if (seconds === undefined || seconds <= 0) return '—';
  const total = Math.floor(seconds);
  const m = Math.floor(total / 60);
  const s = total % 60;
  return `${m}:${s.toString().padStart(2, '0')}`;
}

// pickWhatsAppIntegrations filters the org's integrations to the ones
// that speak the WhatsApp Graph API. Meta analytics is currently only
// wired for that provider.
function pickWhatsAppIntegrations(all: Integration[]): Integration[] {
  return all.filter((i) => i.provider === 'whatsapp');
}

// ---- Messaging section --------------------------------------------------

function MessagingSection({
  integrationID,
  range,
  granularity,
  active,
}: {
  integrationID: string;
  range: MetaRange;
  granularity: MetaGranularity;
  active: boolean;
}) {
  const q = useMetaMessaging(integrationID, range, granularity, active);
  const points: MetaMessagingPoint[] = q.data?.analytics.data_points ?? [];

  const totals = useMemo(() => {
    let sent = 0;
    let delivered = 0;
    for (const p of points) {
      sent += p.sent;
      delivered += p.delivered;
    }
    return { sent, delivered };
  }, [points]);

  const sparkPoints: SparklinePoint[] = useMemo(
    () =>
      points
        .slice()
        .sort((a, b) => a.start - b.start)
        .map((p) => ({ day: epochToISODay(p.start), value: p.sent })),
    [points],
  );

  return (
    <section className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-slate-900">Messaging</h2>
        {q.isFetching ? <Spinner className="h-4 w-4 text-slate-400" label="Refreshing" /> : null}
      </div>
      <SectionError err={q.error} />
      {q.isPending && q.data === undefined ? (
        <SectionSpinner label="Loading messaging" />
      ) : (
        <>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
            <KpiCard title="Sent" value={formatInt(totals.sent)} />
            <KpiCard title="Delivered" value={formatInt(totals.delivered)} />
            <KpiCard
              title="Delivery rate"
              value={formatPct(totals.delivered, totals.sent)}
              footnote="delivered / sent"
            />
          </div>
          <Sparkline
            points={sparkPoints}
            color="#059669"
            label="Messages sent per bucket"
            ariaLabel="Messages sent per bucket sparkline"
          />
        </>
      )}
    </section>
  );
}

// ---- Conversations section ---------------------------------------------

function ConversationsSection({
  integrationID,
  range,
  granularity,
  active,
}: {
  integrationID: string;
  range: MetaRange;
  granularity: MetaGranularity;
  active: boolean;
}) {
  const q = useMetaConversations(integrationID, range, granularity, active);
  const series = q.data?.conversation_analytics.data ?? [];
  const points: MetaConversationPoint[] = useMemo(
    () => series.flatMap((s) => s.data_points),
    [series],
  );

  const totals = useMemo(() => {
    let conv = 0;
    let cost = 0;
    let currency: string | undefined;
    for (const p of points) {
      conv += p.conversation ?? 0;
      cost += p.cost ?? 0;
      if (currency === undefined && p.currency !== undefined) currency = p.currency;
    }
    return { conv, cost, currency };
  }, [points]);

  // Aggregate rows for the breakdown table by (direction, type,
  // category). Meta returns a row per bucket so we roll them up client
  // side.
  const breakdown = useMemo(() => {
    const map = new Map<
      string,
      { direction: string; type: string; category: string; conv: number; cost: number; currency?: string }
    >();
    for (const p of points) {
      const direction = p.conversation_direction ?? '—';
      const type = p.conversation_type ?? '—';
      const category = p.conversation_category ?? '—';
      const key = `${direction}|${type}|${category}`;
      const existing = map.get(key);
      if (existing === undefined) {
        map.set(key, {
          direction,
          type,
          category,
          conv: p.conversation ?? 0,
          cost: p.cost ?? 0,
          ...(p.currency !== undefined ? { currency: p.currency } : {}),
        });
      } else {
        existing.conv += p.conversation ?? 0;
        existing.cost += p.cost ?? 0;
        if (existing.currency === undefined && p.currency !== undefined) {
          existing.currency = p.currency;
        }
      }
    }
    return Array.from(map.values()).sort((a, b) => b.conv - a.conv);
  }, [points]);

  const avgCost = totals.conv > 0 ? totals.cost / totals.conv : 0;

  return (
    <section className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-slate-900">Conversations</h2>
        {q.isFetching ? <Spinner className="h-4 w-4 text-slate-400" label="Refreshing" /> : null}
      </div>
      <SectionError err={q.error} />
      {q.isPending && q.data === undefined ? (
        <SectionSpinner label="Loading conversations" />
      ) : (
        <>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
            <KpiCard title="Conversations" value={formatInt(totals.conv)} />
            <KpiCard title="Total cost" value={formatCurrency(totals.cost, totals.currency)} />
            <KpiCard
              title="Avg cost / conv"
              value={totals.conv > 0 ? formatCurrency(avgCost, totals.currency) : '—'}
            />
          </div>
          <div className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm">
            <table className="min-w-full divide-y divide-slate-200 text-sm">
              <thead className="bg-slate-50 text-xs uppercase tracking-wide text-slate-500">
                <tr>
                  <th className="px-4 py-2 text-left font-medium">Direction</th>
                  <th className="px-4 py-2 text-left font-medium">Type</th>
                  <th className="px-4 py-2 text-left font-medium">Category</th>
                  <th className="px-4 py-2 text-right font-medium">Conversations</th>
                  <th className="px-4 py-2 text-right font-medium">Cost</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100">
                {breakdown.length === 0 ? (
                  <tr>
                    <td colSpan={5} className="px-4 py-6 text-center text-slate-400">
                      No data in the selected range.
                    </td>
                  </tr>
                ) : (
                  breakdown.map((row) => (
                    <tr key={`${row.direction}-${row.type}-${row.category}`}>
                      <td className="px-4 py-2 text-slate-700">{row.direction}</td>
                      <td className="px-4 py-2 text-slate-700">{row.type}</td>
                      <td className="px-4 py-2 text-slate-700">{row.category}</td>
                      <td className="px-4 py-2 text-right tabular-nums text-slate-900">
                        {formatInt(row.conv)}
                      </td>
                      <td className="px-4 py-2 text-right tabular-nums text-slate-900">
                        {formatCurrency(row.cost, row.currency)}
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </>
      )}
    </section>
  );
}

// ---- Calls section ------------------------------------------------------

function CallsSection({
  integrationID,
  range,
  granularity,
  active,
}: {
  integrationID: string;
  range: MetaRange;
  granularity: MetaGranularity;
  active: boolean;
}) {
  const q = useMetaCalls(integrationID, range, granularity, active);
  const points: MetaCallPoint[] = q.data?.call_analytics.data_points ?? [];

  const totals = useMemo(() => {
    let count = 0;
    let cost = 0;
    let weightedDurSum = 0;
    let weightedDurWeight = 0;
    let currency: string | undefined;
    for (const p of points) {
      const c = p.count ?? 0;
      count += c;
      cost += p.cost ?? 0;
      if (p.average_duration !== undefined && c > 0) {
        weightedDurSum += p.average_duration * c;
        weightedDurWeight += c;
      }
      if (currency === undefined && p.currency !== undefined) currency = p.currency;
    }
    const avgDuration = weightedDurWeight > 0 ? weightedDurSum / weightedDurWeight : undefined;
    return { count, cost, avgDuration, currency };
  }, [points]);

  const sparkPoints: SparklinePoint[] = useMemo(
    () =>
      points
        .slice()
        .sort((a, b) => a.start - b.start)
        .map((p) => ({ day: epochToISODay(p.start), value: p.count ?? 0 })),
    [points],
  );

  const byDirection = useMemo(() => {
    const map = new Map<string, { direction: string; count: number; cost: number; currency?: string }>();
    for (const p of points) {
      const direction = p.direction ?? '—';
      const existing = map.get(direction);
      if (existing === undefined) {
        map.set(direction, {
          direction,
          count: p.count ?? 0,
          cost: p.cost ?? 0,
          ...(p.currency !== undefined ? { currency: p.currency } : {}),
        });
      } else {
        existing.count += p.count ?? 0;
        existing.cost += p.cost ?? 0;
      }
    }
    return Array.from(map.values()).sort((a, b) => b.count - a.count);
  }, [points]);

  return (
    <section className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-slate-900">Calls</h2>
        {q.isFetching ? <Spinner className="h-4 w-4 text-slate-400" label="Refreshing" /> : null}
      </div>
      <SectionError err={q.error} />
      {q.isPending && q.data === undefined ? (
        <SectionSpinner label="Loading calls" />
      ) : (
        <>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
            <KpiCard title="Calls" value={formatInt(totals.count)} />
            <KpiCard title="Total cost" value={formatCurrency(totals.cost, totals.currency)} />
            <KpiCard
              title="Avg duration"
              value={formatDurationSeconds(totals.avgDuration)}
              footnote="weighted by count"
            />
          </div>
          <Sparkline
            points={sparkPoints}
            color="#0ea5e9"
            label="Calls per bucket"
            ariaLabel="Calls per bucket sparkline"
          />
          <div className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm">
            <table className="min-w-full divide-y divide-slate-200 text-sm">
              <thead className="bg-slate-50 text-xs uppercase tracking-wide text-slate-500">
                <tr>
                  <th className="px-4 py-2 text-left font-medium">Direction</th>
                  <th className="px-4 py-2 text-right font-medium">Count</th>
                  <th className="px-4 py-2 text-right font-medium">Cost</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100">
                {byDirection.length === 0 ? (
                  <tr>
                    <td colSpan={3} className="px-4 py-6 text-center text-slate-400">
                      No data in the selected range.
                    </td>
                  </tr>
                ) : (
                  byDirection.map((row) => (
                    <tr key={row.direction}>
                      <td className="px-4 py-2 text-slate-700">{row.direction}</td>
                      <td className="px-4 py-2 text-right tabular-nums text-slate-900">
                        {formatInt(row.count)}
                      </td>
                      <td className="px-4 py-2 text-right tabular-nums text-slate-900">
                        {formatCurrency(row.cost, row.currency)}
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </>
      )}
    </section>
  );
}

// ---- Pricing section ----------------------------------------------------

function PricingSection({
  integrationID,
  range,
  granularity,
  active,
}: {
  integrationID: string;
  range: MetaRange;
  granularity: MetaGranularity;
  active: boolean;
}) {
  const q = useMetaPricing(integrationID, range, granularity, active);
  const series = q.data?.pricing_analytics.data ?? [];
  const points: MetaPricingPoint[] = useMemo(
    () => series.flatMap((s) => s.data_points),
    [series],
  );

  // Meta returns two shapes: the newer minimal one is just
  // {start, end, volume, cost}; the classic breakdown adds
  // category/type/tier. Detect which by scanning the sample.
  const hasBreakdown = useMemo(
    () =>
      points.some(
        (p) =>
          (p.pricing_category !== undefined && p.pricing_category !== '') ||
          (p.pricing_type !== undefined && p.pricing_type !== '') ||
          (p.tier !== undefined && p.tier !== ''),
      ),
    [points],
  );

  const currency =
    points.find((p) => p.currency !== undefined && p.currency !== '')?.currency ?? 'USD';

  const totals = useMemo(() => {
    let volume = 0;
    let cost = 0;
    for (const p of points) {
      volume += p.volume ?? 0;
      cost += p.cost ?? 0;
    }
    const perMessage = volume > 0 ? cost / volume : 0;
    return { volume, cost, perMessage };
  }, [points]);

  // Per-day roll-up for the fallback view (used when Meta returns the
  // minimal shape). Groups by start-of-bucket day in local time.
  const perDayRows = useMemo(() => {
    const map = new Map<string, { day: string; volume: number; cost: number }>();
    for (const p of points) {
      const day = epochToISODay(p.start);
      const existing = map.get(day);
      if (existing === undefined) {
        map.set(day, { day, volume: p.volume ?? 0, cost: p.cost ?? 0 });
      } else {
        existing.volume += p.volume ?? 0;
        existing.cost += p.cost ?? 0;
      }
    }
    return Array.from(map.values()).sort((a, b) => (a.day < b.day ? -1 : 1));
  }, [points]);

  // Breakdown roll-up (Meta's richer shape).
  const breakdownRows = useMemo(() => {
    const map = new Map<
      string,
      { pricing_category: string; pricing_type: string; tier: string; volume: number; cost: number }
    >();
    for (const p of points) {
      const category = p.pricing_category ?? '—';
      const type = p.pricing_type ?? '—';
      const tier = p.tier ?? '—';
      const key = `${category}|${type}|${tier}`;
      const existing = map.get(key);
      if (existing === undefined) {
        map.set(key, {
          pricing_category: category,
          pricing_type: type,
          tier,
          volume: p.volume ?? 0,
          cost: p.cost ?? 0,
        });
      } else {
        existing.volume += p.volume ?? 0;
        existing.cost += p.cost ?? 0;
      }
    }
    return Array.from(map.values()).sort((a, b) => b.cost - a.cost);
  }, [points]);

  return (
    <section className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-slate-900">Pricing</h2>
        {q.isFetching ? <Spinner className="h-4 w-4 text-slate-400" label="Refreshing" /> : null}
      </div>
      <SectionError err={q.error} />
      {q.isPending && q.data === undefined ? (
        <SectionSpinner label="Loading pricing" />
      ) : points.length === 0 ? (
        <div className="rounded-xl border border-slate-200 bg-white px-4 py-6 text-center text-sm text-slate-400 shadow-sm">
          No data in the selected range.
        </div>
      ) : (
        <div className="space-y-3">
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
            <KpiCard title="Total volume" value={formatInt(totals.volume)} footnote="billable messages" />
            <KpiCard
              title="Total cost"
              value={formatCurrency(totals.cost, currency)}
            />
            <KpiCard
              title="Cost per message"
              value={formatCurrency(totals.perMessage, currency)}
              footnote="cost ÷ volume"
            />
          </div>

          {hasBreakdown ? (
            <div className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm">
              <table className="min-w-full divide-y divide-slate-200 text-sm">
                <thead className="bg-slate-50 text-xs uppercase tracking-wide text-slate-500">
                  <tr>
                    <th className="px-4 py-2 text-left font-medium">Category</th>
                    <th className="px-4 py-2 text-left font-medium">Type</th>
                    <th className="px-4 py-2 text-left font-medium">Tier</th>
                    <th className="px-4 py-2 text-right font-medium">Volume</th>
                    <th className="px-4 py-2 text-right font-medium">Cost</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-100">
                  {breakdownRows.map((row) => (
                    <tr key={`${row.pricing_category}-${row.pricing_type}-${row.tier}`}>
                      <td className="px-4 py-2 text-slate-700">{row.pricing_category}</td>
                      <td className="px-4 py-2 text-slate-700">{row.pricing_type}</td>
                      <td className="px-4 py-2 text-slate-700">{row.tier}</td>
                      <td className="px-4 py-2 text-right tabular-nums text-slate-900">
                        {formatInt(row.volume)}
                      </td>
                      <td className="px-4 py-2 text-right tabular-nums text-slate-900">
                        {formatCurrency(row.cost, currency)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <div className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm">
              <table className="min-w-full divide-y divide-slate-200 text-sm">
                <thead className="bg-slate-50 text-xs uppercase tracking-wide text-slate-500">
                  <tr>
                    <th className="px-4 py-2 text-left font-medium">Day</th>
                    <th className="px-4 py-2 text-right font-medium">Volume</th>
                    <th className="px-4 py-2 text-right font-medium">Cost</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-100">
                  {perDayRows.map((row) => (
                    <tr key={row.day}>
                      <td className="px-4 py-2 text-slate-700">{row.day}</td>
                      <td className="px-4 py-2 text-right tabular-nums text-slate-900">
                        {formatInt(row.volume)}
                      </td>
                      <td className="px-4 py-2 text-right tabular-nums text-slate-900">
                        {formatCurrency(row.cost, currency)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}
    </section>
  );
}

// ---- Templates section --------------------------------------------------

// pickCost extracts the `amount_spent` value from Meta's cost array
// (falling back to the first entry). Templates return cost as an array
// of {type, value} objects rather than a single scalar.
function pickCost(entries: MetaTemplatePoint['cost']): number {
  if (entries === undefined || entries.length === 0) return 0;
  const amount = entries.find((e) => e.type === 'amount_spent');
  return amount !== undefined ? amount.value : (entries[0]?.value ?? 0);
}

function sumClicks(entries: MetaTemplatePoint['clicked']): number {
  if (entries === undefined || entries.length === 0) return 0;
  return entries.reduce((acc, e) => acc + e.count, 0);
}

function TemplatesSection({
  integrationID,
  range,
  granularity,
  active,
}: {
  integrationID: string;
  range: MetaRange;
  granularity: MetaGranularity;
  active: boolean;
}) {
  const q = useMetaTemplates(integrationID, range, granularity, active);
  const points: MetaTemplatePoint[] = useMemo(
    () => (q.data?.data ?? []).flatMap((s) => s.data_points),
    [q.data],
  );

  const rows = useMemo(() => {
    const map = new Map<
      string,
      {
        template_id: string;
        template_name: string;
        sent: number;
        delivered: number;
        read: number;
        clicked: number;
        cost: number;
      }
    >();
    for (const p of points) {
      const key = p.template_id;
      const existing = map.get(key);
      if (existing === undefined) {
        map.set(key, {
          template_id: p.template_id,
          template_name: p.template_name ?? p.template_id,
          sent: p.sent ?? 0,
          delivered: p.delivered ?? 0,
          read: p.read ?? 0,
          clicked: sumClicks(p.clicked),
          cost: pickCost(p.cost),
        });
      } else {
        existing.sent += p.sent ?? 0;
        existing.delivered += p.delivered ?? 0;
        existing.read += p.read ?? 0;
        existing.clicked += sumClicks(p.clicked);
        existing.cost += pickCost(p.cost);
      }
    }
    return Array.from(map.values()).sort((a, b) => b.sent - a.sent);
  }, [points]);

  // Suppress the section entirely when the integration has no templates
  // in this range — matches the requested "rendered only when the
  // integration has templates" behaviour.
  if (!q.isPending && q.error === null && rows.length === 0) {
    return null;
  }

  return (
    <section className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold text-slate-900">Templates</h2>
        {q.isFetching ? <Spinner className="h-4 w-4 text-slate-400" label="Refreshing" /> : null}
      </div>
      <SectionError err={q.error} />
      {q.isPending && q.data === undefined ? (
        <SectionSpinner label="Loading templates" />
      ) : (
        <div className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm">
          <table className="min-w-full divide-y divide-slate-200 text-sm">
            <thead className="bg-slate-50 text-xs uppercase tracking-wide text-slate-500">
              <tr>
                <th className="px-4 py-2 text-left font-medium">Template</th>
                <th className="px-4 py-2 text-right font-medium">Sent</th>
                <th className="px-4 py-2 text-right font-medium">Delivered</th>
                <th className="px-4 py-2 text-right font-medium">Read</th>
                <th className="px-4 py-2 text-right font-medium">Clicked</th>
                <th className="px-4 py-2 text-right font-medium">Cost</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100">
              {rows.map((row) => (
                <tr key={row.template_id}>
                  <td className="px-4 py-2 text-slate-700">
                    <div className="font-medium text-slate-900">{row.template_name}</div>
                    <div className="text-xs text-slate-500">{row.template_id}</div>
                  </td>
                  <td className="px-4 py-2 text-right tabular-nums text-slate-900">
                    {formatInt(row.sent)}
                  </td>
                  <td className="px-4 py-2 text-right tabular-nums text-slate-900">
                    {formatInt(row.delivered)}
                  </td>
                  <td className="px-4 py-2 text-right tabular-nums text-slate-900">
                    {formatInt(row.read)}
                  </td>
                  <td className="px-4 py-2 text-right tabular-nums text-slate-900">
                    {formatInt(row.clicked)}
                  </td>
                  <td className="px-4 py-2 text-right tabular-nums text-slate-900">
                    {formatCurrency(row.cost, undefined)}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

// ---- Root tab -----------------------------------------------------------

const QUICK_RANGES: Array<{ id: string; label: string; days: number }> = [
  { id: '7d', label: 'Last 7 days', days: 7 },
  { id: '30d', label: 'Last 30 days', days: 30 },
  { id: '90d', label: 'Last 90 days', days: 90 },
];

// MetaAnalyticsTab is the "Meta Analytics" tab body. Rendered inside
// analytics.tsx when the top-level tab bar selects it. Data fetching is
// gated on `active` so switching back to the Nudgeway tab pauses
// polling.
export function MetaAnalyticsTab({ active }: { active: boolean }) {
  const integrations = useIntegrations();
  const waIntegrations = useMemo(
    () => pickWhatsAppIntegrations(integrations.data ?? []),
    [integrations.data],
  );

  const [integrationID, setIntegrationID] = useState<string>('');
  const [rangeChoice, setRangeChoice] = useState<string>('7d');
  const [customRange, setCustomRange] = useState<MetaRange>(() => defaultMetaRange());
  const [granularity, setGranularity] = useState<MetaGranularity>('DAILY');

  // Auto-select the first WhatsApp integration on first load so the
  // operator sees data immediately.
  const effectiveID = useMemo(() => {
    if (integrationID !== '') return integrationID;
    return waIntegrations[0]?.id ?? '';
  }, [integrationID, waIntegrations]);

  const range: MetaRange = useMemo(() => {
    if (rangeChoice === 'custom') return customRange;
    const quick = QUICK_RANGES.find((r) => r.id === rangeChoice);
    return quick !== undefined ? rangeDaysAgo(quick.days) : defaultMetaRange();
  }, [rangeChoice, customRange]);

  if (integrations.isPending) {
    return (
      <div className="flex items-center gap-2 text-sm text-slate-500">
        <Spinner className="h-4 w-4" label="Loading integrations" /> Loading integrations…
      </div>
    );
  }

  if (waIntegrations.length === 0) {
    return (
      <div className="rounded-xl border border-slate-200 bg-white p-6 text-center text-sm text-slate-500 shadow-sm">
        Add a WhatsApp integration in Settings to view Meta analytics.
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-end gap-3 rounded-xl border border-slate-200 bg-white p-4 shadow-sm">
        <label className="flex flex-col gap-1 text-xs font-medium uppercase tracking-wide text-slate-500">
          Integration
          <select
            value={effectiveID}
            onChange={(e) => setIntegrationID(e.target.value)}
            className="rounded-md border border-slate-300 bg-white px-2 py-1 text-sm normal-case text-slate-900"
          >
            {waIntegrations.map((i) => (
              <option key={i.id} value={i.id}>
                {i.name} ({i.status})
              </option>
            ))}
          </select>
        </label>

        <div className="flex flex-col gap-1 text-xs font-medium uppercase tracking-wide text-slate-500">
          Range
          <div className="flex gap-1">
            {QUICK_RANGES.map((r) => (
              <button
                key={r.id}
                type="button"
                onClick={() => setRangeChoice(r.id)}
                className={`rounded-md border px-2 py-1 text-sm normal-case ${
                  rangeChoice === r.id
                    ? 'border-slate-900 bg-slate-900 text-white'
                    : 'border-slate-300 bg-white text-slate-700 hover:bg-slate-50'
                }`}
              >
                {r.label}
              </button>
            ))}
            <button
              type="button"
              onClick={() => setRangeChoice('custom')}
              className={`rounded-md border px-2 py-1 text-sm normal-case ${
                rangeChoice === 'custom'
                  ? 'border-slate-900 bg-slate-900 text-white'
                  : 'border-slate-300 bg-white text-slate-700 hover:bg-slate-50'
              }`}
            >
              Custom
            </button>
          </div>
        </div>

        {rangeChoice === 'custom' ? (
          <div className="flex items-end gap-2 text-sm text-slate-600">
            <label className="flex flex-col gap-1 text-xs font-medium uppercase tracking-wide text-slate-500">
              From
              <input
                type="date"
                value={customRange.since}
                onChange={(e) =>
                  setCustomRange((r) => ({ ...r, since: e.target.value }))
                }
                className="rounded-md border border-slate-300 bg-white px-2 py-1 text-sm normal-case text-slate-900"
              />
            </label>
            <label className="flex flex-col gap-1 text-xs font-medium uppercase tracking-wide text-slate-500">
              To
              <input
                type="date"
                value={customRange.until}
                onChange={(e) =>
                  setCustomRange((r) => ({ ...r, until: e.target.value }))
                }
                className="rounded-md border border-slate-300 bg-white px-2 py-1 text-sm normal-case text-slate-900"
              />
            </label>
          </div>
        ) : null}

        <label className="flex flex-col gap-1 text-xs font-medium uppercase tracking-wide text-slate-500">
          Granularity
          <select
            value={granularity}
            onChange={(e) => setGranularity(e.target.value as MetaGranularity)}
            className="rounded-md border border-slate-300 bg-white px-2 py-1 text-sm normal-case text-slate-900"
          >
            <option value="DAILY">Daily</option>
            <option value="HALF_HOUR">Half hour</option>
            <option value="MONTHLY">Monthly</option>
          </select>
        </label>
      </div>

      {effectiveID === '' ? (
        <div className="rounded-xl border border-slate-200 bg-white p-6 text-center text-sm text-slate-500 shadow-sm">
          Choose an integration to view Meta analytics.
        </div>
      ) : (
        <>
          <MessagingSection
            integrationID={effectiveID}
            range={range}
            granularity={granularity}
            active={active}
          />
          <ConversationsSection
            integrationID={effectiveID}
            range={range}
            granularity={granularity}
            active={active}
          />
          <CallsSection
            integrationID={effectiveID}
            range={range}
            granularity={granularity}
            active={active}
          />
          <PricingSection
            integrationID={effectiveID}
            range={range}
            granularity={granularity}
            active={active}
          />
          <TemplatesSection
            integrationID={effectiveID}
            range={range}
            granularity={granularity}
            active={active}
          />
        </>
      )}
    </div>
  );
}
