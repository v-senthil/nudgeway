import { Fragment, useEffect, useMemo, useState } from 'react';
import { createPortal } from 'react-dom';
import { Spinner } from '../../components/Spinner';
import { ApiError } from '../../lib/api';
import type { APIToken } from '../../lib/api-tokens';
import {
  defaultMetricsRange,
  useAPITokenMetrics,
  useAPITokenUsage,
  type MetricsByDay,
  type MetricsRange,
  type TokenMetrics,
  type UsageEntry,
  type UsageFilter,
} from '../../lib/api-token-usage';

type Tab = 'overview' | 'log';
type MethodFilter = '' | 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE';
type StatusBucket = '' | '2xx' | '3xx' | '4xx' | '5xx';

function isExpired(token: APIToken): boolean {
  if (token.expires_at === null || token.expires_at === undefined || token.expires_at === '') {
    return false;
  }
  const t = new Date(token.expires_at).getTime();
  if (Number.isNaN(t)) return false;
  return t <= Date.now();
}

function TokenStatusChip({ token }: { token: APIToken }) {
  if (token.revoked_at !== null && token.revoked_at !== undefined && token.revoked_at !== '') {
    return (
      <span className="inline-flex items-center rounded-md bg-slate-100 px-2 py-0.5 text-xs font-medium text-slate-600 ring-1 ring-inset ring-slate-200">
        Revoked
      </span>
    );
  }
  if (isExpired(token)) {
    return (
      <span className="inline-flex items-center rounded-md bg-amber-50 px-2 py-0.5 text-xs font-medium text-amber-700 ring-1 ring-inset ring-amber-200">
        Expired
      </span>
    );
  }
  return (
    <span className="inline-flex items-center rounded-md bg-emerald-50 px-2 py-0.5 text-xs font-medium text-emerald-700 ring-1 ring-inset ring-emerald-200">
      Active
    </span>
  );
}

function CloseIcon() {
  return (
    <svg
      aria-hidden="true"
      viewBox="0 0 20 20"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.75"
      strokeLinecap="round"
      strokeLinejoin="round"
      className="h-5 w-5"
    >
      <path d="M5 5l10 10M15 5L5 15" />
    </svg>
  );
}

function CopyIcon() {
  return (
    <svg
      aria-hidden="true"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.75"
      strokeLinecap="round"
      strokeLinejoin="round"
      className="h-3.5 w-3.5"
    >
      <rect x="9" y="9" width="11" height="11" rx="2" />
      <path d="M5 15V6a2 2 0 0 1 2-2h9" />
    </svg>
  );
}

function ChevronIcon({ open }: { open: boolean }) {
  return (
    <svg
      aria-hidden="true"
      viewBox="0 0 20 20"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.75"
      strokeLinecap="round"
      strokeLinejoin="round"
      className={`h-4 w-4 transition-transform ${open ? 'rotate-90' : ''}`}
    >
      <path d="M7 5l6 5-6 5" />
    </svg>
  );
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(2)} MB`;
}

function formatRelative(iso: string): string {
  const d = new Date(iso).getTime();
  if (Number.isNaN(d)) return iso;
  const now = Date.now();
  const diff = Math.max(0, now - d);
  const s = Math.floor(diff / 1000);
  if (s < 60) return `${s}s ago`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m ago`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h ago`;
  const days = Math.floor(h / 24);
  if (days < 7) return `${days}d ago`;
  return new Date(iso).toLocaleDateString();
}

function KpiCard({ title, value, footnote }: { title: string; value: string; footnote?: string }) {
  return (
    <div className="rounded-xl border border-slate-200 bg-white p-3 shadow-sm">
      <div className="text-[10px] font-medium uppercase tracking-wide text-slate-500">{title}</div>
      <div className="mt-1 text-xl font-semibold tabular-nums text-slate-900">{value}</div>
      {footnote !== undefined && footnote !== '' ? (
        <div className="mt-0.5 text-[11px] text-slate-500">{footnote}</div>
      ) : null}
    </div>
  );
}

function ByDaySparkline({ points }: { points: MetricsByDay[] }) {
  const width = 600;
  const height = 120;
  const pad = 8;

  const geometry = useMemo(() => {
    if (points.length === 0) {
      return { path: '', area: '', maxY: 0, ticks: [] as Array<{ x: number; day: string }>, dots: [] as Array<{ x: number; y: number; day: string }> };
    }
    const values = points.map((p) => p.requests);
    const maxRaw = Math.max(...values, 1);
    const maxY = Math.max(1, Math.ceil(maxRaw * 1.15));
    const innerW = width - pad * 2;
    const innerH = height - pad * 2;
    const stepX = points.length > 1 ? innerW / (points.length - 1) : 0;
    const coords = points.map((p, i) => {
      const x = points.length > 1 ? pad + stepX * i : pad + innerW / 2;
      const y = pad + innerH - (p.requests / maxY) * innerH;
      return { x, y, day: p.day };
    });
    const path = coords
      .map((c, i) => `${i === 0 ? 'M' : 'L'} ${c.x.toFixed(1)} ${c.y.toFixed(1)}`)
      .join(' ');
    const area =
      coords.length > 1
        ? `${path} L ${coords[coords.length - 1]!.x.toFixed(1)} ${(height - pad).toFixed(1)} L ${coords[0]!.x.toFixed(1)} ${(height - pad).toFixed(1)} Z`
        : '';
    const stride = Math.max(1, Math.ceil(coords.length / 6));
    const ticks = coords.filter((_, i) => i % stride === 0).map((c) => ({ x: c.x, day: c.day }));
    const dots = coords.map((c) => ({ x: c.x, y: c.y, day: c.day }));
    return { path, area, maxY, ticks, dots };
  }, [points]);

  return (
    <div className="rounded-xl border border-slate-200 bg-white p-3 shadow-sm">
      <div className="mb-1 flex items-baseline justify-between">
        <div className="text-sm font-medium text-slate-700">Requests per day</div>
        <div className="text-xs text-slate-500">max {geometry.maxY.toLocaleString()}</div>
      </div>
      {points.length === 0 ? (
        <div className="flex h-[120px] items-center justify-center text-sm text-slate-400">
          No data in the selected range.
        </div>
      ) : (
        <svg
          viewBox={`0 0 ${width} ${height}`}
          role="img"
          aria-label="Requests per day sparkline"
          className="w-full"
        >
          <path d={geometry.area} fill="#059669" opacity={0.15} />
          <path d={geometry.path} fill="none" stroke="#059669" strokeWidth={2} />
          {geometry.dots.map((d) => (
            <circle key={`d-${d.day}`} cx={d.x} cy={d.y} r={2.5} fill="#059669" />
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

function StatusChip({ code }: { code: number }) {
  let cls = 'bg-slate-100 text-slate-700 ring-slate-200';
  if (code >= 200 && code < 300) cls = 'bg-emerald-50 text-emerald-700 ring-emerald-200';
  else if (code >= 300 && code < 400) cls = 'bg-amber-50 text-amber-700 ring-amber-200';
  else if (code >= 400) cls = 'bg-rose-50 text-rose-700 ring-rose-200';
  return (
    <span
      className={`inline-flex items-center rounded-md px-1.5 py-0.5 font-mono text-[11px] font-medium ring-1 ring-inset ${cls}`}
    >
      {code}
    </span>
  );
}

function MethodChip({ method }: { method: string }) {
  const upper = method.toUpperCase();
  let cls = 'bg-slate-100 text-slate-700 ring-slate-200';
  if (upper === 'GET') cls = 'bg-sky-50 text-sky-700 ring-sky-200';
  else if (upper === 'POST') cls = 'bg-emerald-50 text-emerald-700 ring-emerald-200';
  else if (upper === 'PUT' || upper === 'PATCH') cls = 'bg-amber-50 text-amber-700 ring-amber-200';
  else if (upper === 'DELETE') cls = 'bg-rose-50 text-rose-700 ring-rose-200';
  return (
    <span
      className={`inline-flex items-center rounded-md px-1.5 py-0.5 font-mono text-[10px] font-semibold uppercase ring-1 ring-inset ${cls}`}
    >
      {upper}
    </span>
  );
}

function OverviewTab({
  metrics,
  isPending,
  isError,
  error,
  range,
  onRangeChange,
}: {
  metrics: TokenMetrics | undefined;
  isPending: boolean;
  isError: boolean;
  error: ApiError | null;
  range: MetricsRange;
  onRangeChange: (r: MetricsRange) => void;
}) {
  const totalRequests = metrics?.total_requests ?? 0;
  const errorRatePct =
    metrics !== undefined && totalRequests > 0
      ? ((metrics.error_count / totalRequests) * 100).toFixed(1)
      : '0.0';
  const avgLatency = metrics?.avg_latency_ms ?? 0;
  const bytesTotal = (metrics?.bytes_in ?? 0) + (metrics?.bytes_out ?? 0);

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2 text-xs text-slate-600">
        <label className="flex items-center gap-1">
          From
          <input
            type="date"
            value={range.from}
            onChange={(e) => onRangeChange({ ...range, from: e.target.value })}
            className="rounded-md border border-slate-300 bg-white px-2 py-1 text-xs"
          />
        </label>
        <label className="flex items-center gap-1">
          To
          <input
            type="date"
            value={range.to}
            onChange={(e) => onRangeChange({ ...range, to: e.target.value })}
            className="rounded-md border border-slate-300 bg-white px-2 py-1 text-xs"
          />
        </label>
      </div>

      {isError && (
        <div className="rounded-lg border border-rose-200 bg-rose-50 p-3 text-xs text-rose-700">
          Failed to load metrics: {error?.problem.detail ?? error?.message ?? 'unknown error'}
        </div>
      )}

      <section className="grid grid-cols-2 gap-3">
        <KpiCard title="Total requests" value={totalRequests.toLocaleString()} />
        <KpiCard title="Error rate" value={`${errorRatePct}%`} footnote={`${(metrics?.error_count ?? 0).toLocaleString()} errors`} />
        <KpiCard title="Avg latency" value={`${Math.round(avgLatency)} ms`} />
        <KpiCard title="Data transferred" value={formatBytes(bytesTotal)} footnote="in + out" />
      </section>

      <ByDaySparkline points={metrics?.by_day ?? []} />

      <section className="rounded-xl border border-slate-200 bg-white shadow-sm">
        <div className="border-b border-slate-200 px-3 py-2 text-sm font-medium text-slate-700">
          Top paths
        </div>
        {isPending && metrics === undefined ? (
          <div className="flex items-center justify-center py-6">
            <Spinner className="h-4 w-4 text-slate-500" label="Loading metrics" />
          </div>
        ) : (metrics?.top_paths ?? []).length === 0 ? (
          <div className="px-3 py-4 text-xs text-slate-500">No requests in this range yet.</div>
        ) : (
          <table className="min-w-full divide-y divide-slate-100">
            <thead className="bg-slate-50 text-left text-[10px] font-semibold uppercase tracking-wide text-slate-500">
              <tr>
                <th className="px-3 py-1.5">Path</th>
                <th className="px-3 py-1.5 text-right">Hits</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 text-xs">
              {(metrics?.top_paths ?? []).map((p, i) => (
                <tr key={`${p.path}-${i}`}>
                  <td className="px-3 py-1.5">
                    <code className="font-mono text-[11px] text-slate-700">{p.path}</code>
                  </td>
                  <td className="px-3 py-1.5 text-right tabular-nums text-slate-700">
                    {(p.count ?? 0).toLocaleString()}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>

      {Object.keys(metrics?.by_status ?? {}).length > 0 && (
        <section className="rounded-xl border border-slate-200 bg-white p-3 shadow-sm">
          <div className="mb-2 text-sm font-medium text-slate-700">By status</div>
          <div className="flex flex-wrap gap-2">
            {Object.entries(metrics?.by_status ?? {}).map(([code, count]) => (
              <div
                key={code}
                className="flex items-center gap-1.5 rounded-md bg-slate-50 px-2 py-1 ring-1 ring-inset ring-slate-200"
              >
                <StatusChip code={Number(code)} />
                <span className="text-xs tabular-nums text-slate-600">{count.toLocaleString()}</span>
              </div>
            ))}
          </div>
        </section>
      )}
    </div>
  );
}

function formatBody(body: string, raw: boolean): string {
  if (raw) return body;
  try {
    return JSON.stringify(JSON.parse(body), null, 2);
  } catch {
    return body;
  }
}

function LogRowDetail({ entry }: { entry: UsageEntry }) {
  const [rawReq, setRawReq] = useState(false);
  const [rawRes, setRawRes] = useState(false);
  const [copied, setCopied] = useState(false);

  const copyRequestId = async () => {
    try {
      await navigator.clipboard.writeText(entry.request_id);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      // clipboard unavailable — ignore
    }
  };

  return (
    <div className="space-y-3 bg-slate-50 p-3 text-xs">
      <div className="flex flex-wrap items-center gap-2 text-slate-600">
        <span className="font-medium text-slate-700">Request ID:</span>
        <code className="rounded bg-white px-1.5 py-0.5 font-mono text-[11px] ring-1 ring-inset ring-slate-200">
          {entry.request_id}
        </code>
        <button
          type="button"
          onClick={() => void copyRequestId()}
          className="inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-slate-500 hover:bg-white hover:text-slate-700"
          aria-label="Copy request ID"
        >
          <CopyIcon />
          {copied ? 'Copied' : 'Copy'}
        </button>
        {entry.user_agent !== undefined && entry.user_agent !== '' && (
          <span className="truncate text-slate-500">UA: {entry.user_agent}</span>
        )}
      </div>

      {entry.error_message !== undefined && entry.error_message !== '' && (
        <div className="rounded-md border border-rose-200 bg-rose-50 p-2 text-rose-700">
          {entry.error_message}
        </div>
      )}

      {entry.request_body !== undefined && entry.request_body !== '' && (
        <div>
          <div className="mb-1 flex items-center justify-between">
            <div className="text-[11px] font-semibold uppercase tracking-wide text-slate-500">
              Request body
            </div>
            <button
              type="button"
              onClick={() => setRawReq((r) => !r)}
              className="text-[11px] text-slate-500 hover:text-slate-700"
            >
              {rawReq ? 'Formatted' : 'Raw'}
            </button>
          </div>
          <pre className="max-h-64 overflow-auto rounded-md border border-slate-200 bg-white p-2 font-mono text-[11px] text-slate-800">
            {formatBody(entry.request_body, rawReq)}
          </pre>
        </div>
      )}

      {entry.response_body !== undefined && entry.response_body !== '' && (
        <div>
          <div className="mb-1 flex items-center justify-between">
            <div className="text-[11px] font-semibold uppercase tracking-wide text-slate-500">
              Response body
            </div>
            <button
              type="button"
              onClick={() => setRawRes((r) => !r)}
              className="text-[11px] text-slate-500 hover:text-slate-700"
            >
              {rawRes ? 'Formatted' : 'Raw'}
            </button>
          </div>
          <pre className="max-h-64 overflow-auto rounded-md border border-slate-200 bg-white p-2 font-mono text-[11px] text-slate-800">
            {formatBody(entry.response_body, rawRes)}
          </pre>
        </div>
      )}
    </div>
  );
}

function statusBucketRange(bucket: StatusBucket): { min?: number; max?: number } {
  switch (bucket) {
    case '2xx':
      return { min: 200, max: 299 };
    case '3xx':
      return { min: 300, max: 399 };
    case '4xx':
      return { min: 400, max: 499 };
    case '5xx':
      return { min: 500, max: 599 };
    default:
      return {};
  }
}

function LogTab({ id, range }: { id: string; range: MetricsRange }) {
  const [method, setMethod] = useState<MethodFilter>('');
  const [status, setStatus] = useState<StatusBucket>('');
  const [expanded, setExpanded] = useState<string | null>(null);

  const filter = useMemo<UsageFilter>(() => {
    const f: UsageFilter = {
      since: `${range.from}T00:00:00Z`,
      until: `${range.to}T23:59:59Z`,
      limit: 50,
    };
    const bucket = statusBucketRange(status);
    if (bucket.min !== undefined) f.status_min = bucket.min;
    if (bucket.max !== undefined) f.status_max = bucket.max;
    return f;
  }, [range.from, range.to, status]);

  const usage = useAPITokenUsage(id, filter);

  const allItems = useMemo(() => {
    const pages = usage.data?.pages ?? [];
    const rows: UsageEntry[] = [];
    for (const p of pages) rows.push(...p.items);
    if (method === '') return rows;
    return rows.filter((r) => r.method.toUpperCase() === method);
  }, [usage.data, method]);

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex items-center gap-1">
          <span className="text-[11px] font-medium uppercase tracking-wide text-slate-500">Method</span>
          {(['', 'GET', 'POST', 'PUT', 'PATCH', 'DELETE'] as MethodFilter[]).map((m) => (
            <button
              key={m === '' ? 'all-m' : m}
              type="button"
              onClick={() => setMethod(m)}
              className={
                method === m
                  ? 'rounded-md bg-slate-900 px-2 py-0.5 text-[11px] font-medium text-white'
                  : 'rounded-md bg-slate-100 px-2 py-0.5 text-[11px] text-slate-600 hover:bg-slate-200'
              }
            >
              {m === '' ? 'All' : m}
            </button>
          ))}
        </div>
        <div className="flex items-center gap-1">
          <span className="text-[11px] font-medium uppercase tracking-wide text-slate-500">Status</span>
          {(['', '2xx', '3xx', '4xx', '5xx'] as StatusBucket[]).map((s) => (
            <button
              key={s === '' ? 'all-s' : s}
              type="button"
              onClick={() => setStatus(s)}
              className={
                status === s
                  ? 'rounded-md bg-slate-900 px-2 py-0.5 text-[11px] font-medium text-white'
                  : 'rounded-md bg-slate-100 px-2 py-0.5 text-[11px] text-slate-600 hover:bg-slate-200'
              }
            >
              {s === '' ? 'All' : s}
            </button>
          ))}
        </div>
      </div>

      {usage.isError && (
        <div className="rounded-lg border border-rose-200 bg-rose-50 p-3 text-xs text-rose-700">
          Failed to load usage log:{' '}
          {usage.error instanceof ApiError
            ? (usage.error.problem.detail ?? usage.error.message)
            : 'unknown error'}
        </div>
      )}

      <div className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm">
        {usage.isPending ? (
          <div className="flex items-center justify-center py-8">
            <Spinner className="h-4 w-4 text-slate-500" label="Loading usage log" />
          </div>
        ) : allItems.length === 0 ? (
          <div className="px-3 py-6 text-center text-xs text-slate-500">
            No requests match the current filters.
          </div>
        ) : (
          <table className="min-w-full divide-y divide-slate-100 text-xs">
            <thead className="bg-slate-50 text-left text-[10px] font-semibold uppercase tracking-wide text-slate-500">
              <tr>
                <th className="w-4 px-2 py-1.5"></th>
                <th className="px-2 py-1.5">When</th>
                <th className="px-2 py-1.5">Method</th>
                <th className="px-2 py-1.5">Path</th>
                <th className="px-2 py-1.5">Status</th>
                <th className="px-2 py-1.5 text-right">Latency</th>
                <th className="px-2 py-1.5">IP</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100">
              {allItems.map((row) => {
                const open = expanded === row.id;
                return (
                  <Fragment key={row.id}>
                    <tr
                      className="cursor-pointer hover:bg-slate-50"
                      onClick={() => setExpanded(open ? null : row.id)}
                    >
                      <td className="px-2 py-1.5 text-slate-400">
                        <ChevronIcon open={open} />
                      </td>
                      <td className="whitespace-nowrap px-2 py-1.5 text-slate-500">
                        {formatRelative(row.occurred_at)}
                      </td>
                      <td className="px-2 py-1.5">
                        <MethodChip method={row.method} />
                      </td>
                      <td className="max-w-[220px] truncate px-2 py-1.5">
                        <code className="font-mono text-[11px] text-slate-700">{row.path}</code>
                      </td>
                      <td className="px-2 py-1.5">
                        <StatusChip code={row.status_code} />
                      </td>
                      <td className="px-2 py-1.5 text-right tabular-nums text-slate-600">
                        {row.latency_ms}ms
                      </td>
                      <td className="whitespace-nowrap px-2 py-1.5 font-mono text-[11px] text-slate-500">
                        {row.remote_ip}
                      </td>
                    </tr>
                    {open && (
                      <tr>
                        <td colSpan={7} className="p-0">
                          <LogRowDetail entry={row} />
                        </td>
                      </tr>
                    )}
                  </Fragment>
                );
              })}
            </tbody>
          </table>
        )}
      </div>

      {usage.hasNextPage && (
        <div className="flex justify-center">
          <button
            type="button"
            onClick={() => void usage.fetchNextPage()}
            disabled={usage.isFetchingNextPage}
            className="rounded-md border border-slate-200 bg-white px-3 py-1.5 text-xs font-medium text-slate-700 shadow-sm hover:bg-slate-50 disabled:opacity-50"
          >
            {usage.isFetchingNextPage ? 'Loading…' : 'Load more'}
          </button>
        </div>
      )}
    </div>
  );
}

// APITokenUsageDrawer is a right-side slide-in panel that shows aggregate
// metrics + the raw execution log for a single API token. Matches the
// pattern used by SettingsDrawer in features/integrations.
export function APITokenUsageDrawer({
  token,
  onClose,
}: {
  token: APIToken | null;
  onClose: () => void;
}) {
  const [tab, setTab] = useState<Tab>('overview');
  const [range, setRange] = useState<MetricsRange>(() => defaultMetricsRange());

  useEffect(() => {
    if (token === null) return;
    setTab('overview');
    setRange(defaultMetricsRange());
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [token, onClose]);

  const metrics = useAPITokenMetrics(token?.id ?? null, range);

  if (token === null) return null;

  return createPortal(
    <div
      className="fixed inset-0 z-[100] flex bg-slate-900/40"
      role="presentation"
      onClick={onClose}
    >
      <aside
        role="dialog"
        aria-modal="true"
        aria-labelledby="api-token-usage-title"
        className="ml-auto flex h-full w-full max-w-[640px] flex-col border-l border-slate-200 bg-white shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <header className="flex items-start justify-between gap-3 border-b border-slate-200 px-5 py-3">
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-2">
              <h2 id="api-token-usage-title" className="truncate text-base font-semibold text-slate-900">
                {token.name}
              </h2>
              <TokenStatusChip token={token} />
            </div>
            <code className="mt-1 inline-block rounded-md bg-slate-50 px-1.5 py-0.5 font-mono text-[11px] text-slate-600 ring-1 ring-inset ring-slate-200">
              {token.prefix}
              <span className="text-slate-400">…</span>
            </code>
          </div>
          <button
            type="button"
            aria-label="Close usage panel"
            onClick={onClose}
            className="rounded-full p-1 text-slate-500 hover:bg-slate-100"
          >
            <CloseIcon />
          </button>
        </header>

        <nav className="flex gap-1 border-b border-slate-200 px-3 pt-2" aria-label="Usage tabs">
          <TabButton active={tab === 'overview'} onClick={() => setTab('overview')}>
            Overview
          </TabButton>
          <TabButton active={tab === 'log'} onClick={() => setTab('log')}>
            Log
          </TabButton>
        </nav>

        <div className="flex-1 overflow-y-auto px-5 py-5">
          {tab === 'overview' && (
            <OverviewTab
              metrics={metrics.data}
              isPending={metrics.isPending}
              isError={metrics.isError}
              error={metrics.error ?? null}
              range={range}
              onRangeChange={setRange}
            />
          )}
          {tab === 'log' && <LogTab id={token.id} range={range} />}
        </div>
      </aside>
    </div>,
    document.body,
  );
}

function TabButton({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  const base = 'rounded-t-lg px-3 py-2 text-sm font-medium transition';
  const cls = active
    ? `${base} bg-emerald-50 text-emerald-700 border border-b-0 border-slate-200`
    : `${base} text-slate-600 hover:bg-slate-50`;
  return (
    <button type="button" onClick={onClick} className={cls}>
      {children}
    </button>
  );
}
