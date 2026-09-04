import { useMemo, useState } from 'react';
import { createRoute } from '@tanstack/react-router';
import { settingsRoute } from '../settings';
import { Spinner } from '../../components/Spinner';
import { EmptyState } from '../../components/EmptyState';
import { Button } from '../../components/Button';
import {
  PROVIDER_CALL_OPERATIONS,
  statusRangeFromPreset,
  useProviderCalls,
  type ProviderCall,
  type ProviderCallFilter,
  type ProviderCallStatusPreset,
} from '../../lib/provider-calls';
import { useIntegrations } from '../../lib/integrations';
import { ApiError } from '../../lib/api';

function toDateInputValue(iso: string): string {
  return iso.slice(0, 16);
}

function fromDateInputValue(local: string): string {
  if (local === '') return '';
  const d = new Date(local);
  if (Number.isNaN(d.getTime())) return '';
  return d.toISOString();
}

function formatOccurred(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString();
}

// StatusBadge colours the status_code cell so failures pop at a glance.
function StatusBadge({ code }: { code: number }) {
  let cls = 'bg-slate-100 text-slate-700 ring-slate-200';
  if (code === 0) cls = 'bg-orange-50 text-orange-700 ring-orange-100';
  else if (code >= 200 && code < 300) cls = 'bg-emerald-50 text-emerald-700 ring-emerald-100';
  else if (code >= 400 && code < 500) cls = 'bg-amber-50 text-amber-700 ring-amber-100';
  else if (code >= 500) cls = 'bg-rose-50 text-rose-700 ring-rose-100';
  return (
    <span
      className={`inline-flex items-center rounded-md px-2 py-0.5 text-xs font-mono ring-1 ring-inset ${cls}`}
    >
      {code === 0 ? '—' : code}
    </span>
  );
}

// OperationChip renders the adapter operation as a small labelled pill.
function OperationChip({ operation }: { operation: string }) {
  return (
    <span className="inline-flex items-center rounded-md bg-indigo-50 px-2 py-0.5 text-xs font-medium text-indigo-700 ring-1 ring-inset ring-indigo-100">
      {operation}
    </span>
  );
}

// decodeBase64ToText attempts a UTF-8 decode of a base64 payload. Returns
// null on failure so the row can fall back to raw base64.
function decodeBase64ToText(b64: string): string | null {
  try {
    // atob() is browser-native and returns a byte string; wrap in TextDecoder
    // through Uint8Array to handle UTF-8 correctly.
    const bin = atob(b64);
    const bytes = new Uint8Array(bin.length);
    for (let i = 0; i < bin.length; i += 1) {
      bytes[i] = bin.charCodeAt(i);
    }
    return new TextDecoder('utf-8', { fatal: false }).decode(bytes);
  } catch {
    return null;
  }
}

// prettyJSON pretty-prints JSON text when parseable; otherwise returns
// the string unchanged.
function prettyJSON(text: string): string {
  try {
    return JSON.stringify(JSON.parse(text), null, 2);
  } catch {
    return text;
  }
}

function BodyBlock({
  label,
  base64,
  text,
}: {
  label: string;
  base64: string | undefined;
  text: string | undefined;
}) {
  let body: string | null = null;
  if (text !== undefined && text !== '') {
    body = prettyJSON(text);
  } else if (base64 !== undefined && base64 !== '') {
    const decoded = decodeBase64ToText(base64);
    if (decoded !== null) {
      body = prettyJSON(decoded);
    } else {
      body = base64;
    }
  }
  if (body === null) return null;
  return (
    <div>
      <div className="mb-1 text-xs font-semibold uppercase tracking-wide text-slate-500">
        {label}
      </div>
      <pre className="max-h-96 overflow-auto rounded-lg bg-slate-900 p-3 text-xs text-slate-100">
        {body}
      </pre>
    </div>
  );
}

function EntryRow({ entry }: { entry: ProviderCall }) {
  const [expanded, setExpanded] = useState(false);
  const hasBody =
    (entry.request_body !== undefined && entry.request_body !== '') ||
    (entry.response_body !== undefined && entry.response_body !== '');
  return (
    <>
      <tr className="border-t border-slate-100 hover:bg-slate-50">
        <td className="whitespace-nowrap px-3 py-2 text-xs text-slate-500">
          {formatOccurred(entry.occurred_at)}
        </td>
        <td className="px-3 py-2">
          <OperationChip operation={entry.operation} />
        </td>
        <td className="px-3 py-2 font-mono text-xs text-slate-600">{entry.method}</td>
        <td className="px-3 py-2 font-mono text-xs text-slate-500">
          <div className="max-w-md truncate" title={entry.url}>
            {entry.url}
          </div>
        </td>
        <td className="px-3 py-2">
          <StatusBadge code={entry.status_code} />
        </td>
        <td className="whitespace-nowrap px-3 py-2 text-xs text-slate-600">
          {entry.latency_ms} ms
        </td>
        <td className="px-3 py-2 text-xs text-rose-700">
          {entry.error_class !== undefined && entry.error_class !== '' && (
            <span
              className="inline-flex items-center rounded-md bg-rose-50 px-2 py-0.5 font-medium ring-1 ring-inset ring-rose-100"
              title={entry.error_message}
            >
              {entry.error_class}
            </span>
          )}
        </td>
        <td className="whitespace-nowrap px-3 py-2 pr-4 text-right">
          {(hasBody ||
            (entry.trace_id !== undefined && entry.trace_id !== '') ||
            (entry.error_message !== undefined && entry.error_message !== '')) && (
            <button
              type="button"
              onClick={() => setExpanded((v) => !v)}
              className="text-xs font-medium text-emerald-700 hover:text-emerald-800"
            >
              {expanded ? 'Hide' : 'Details'}
            </button>
          )}
        </td>
      </tr>
      {expanded && (
        <tr className="border-t border-slate-100 bg-slate-50">
          <td colSpan={8} className="space-y-3 px-3 py-3">
            <div className="grid grid-cols-1 gap-2 text-xs text-slate-600 sm:grid-cols-2">
              {entry.trace_id !== undefined && entry.trace_id !== '' && (
                <div>
                  <span className="font-semibold text-slate-500">Meta fbtrace_id:</span>{' '}
                  <span className="font-mono">{entry.trace_id}</span>
                </div>
              )}
              {entry.correlation_id !== undefined && entry.correlation_id !== '' && (
                <div>
                  <span className="font-semibold text-slate-500">Correlation:</span>{' '}
                  <span className="font-mono">{entry.correlation_id}</span>
                </div>
              )}
              {entry.integration_id !== undefined && entry.integration_id !== '' && (
                <div>
                  <span className="font-semibold text-slate-500">Integration:</span>{' '}
                  <span className="font-mono">{entry.integration_id}</span>
                </div>
              )}
              {entry.error_message !== undefined && entry.error_message !== '' && (
                <div className="sm:col-span-2 text-rose-700">
                  <span className="font-semibold">Error:</span> {entry.error_message}
                </div>
              )}
            </div>
            <BodyBlock
              label="Request body"
              base64={entry.request_body}
              text={entry.request_body_text}
            />
            <BodyBlock
              label="Response body"
              base64={entry.response_body}
              text={entry.response_body_text}
            />
          </td>
        </tr>
      )}
    </>
  );
}

function ProviderCallsPage() {
  const [integrationID, setIntegrationID] = useState<string>('');
  const [operation, setOperation] = useState<string>('');
  const [statusPreset, setStatusPreset] = useState<ProviderCallStatusPreset>('all');
  const [since, setSince] = useState<string>('');
  const [until, setUntil] = useState<string>('');

  const integrations = useIntegrations();

  const filter: ProviderCallFilter = useMemo(() => {
    const range = statusRangeFromPreset(statusPreset);
    return {
      integration_id: integrationID,
      operation,
      ...range,
      since: fromDateInputValue(since),
      until: fromDateInputValue(until),
      limit: 50,
    };
  }, [integrationID, operation, statusPreset, since, until]);

  const query = useProviderCalls(filter);

  const isPermDenied = query.error instanceof ApiError && query.error.status === 403;
  const isOffline = query.isError && typeof navigator !== 'undefined' && !navigator.onLine;

  const items = useMemo(() => {
    if (query.data === undefined) return [];
    const out: ProviderCall[] = [];
    for (const page of query.data.pages) out.push(...page.items);
    return out;
  }, [query.data]);

  const clearFilters = () => {
    setIntegrationID('');
    setOperation('');
    setStatusPreset('all');
    setSince('');
    setUntil('');
  };

  const anyFilter =
    integrationID !== '' ||
    operation !== '' ||
    statusPreset !== 'all' ||
    since !== '' ||
    until !== '';

  return (
    <div className="mx-auto max-w-6xl space-y-6">
      <header className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold text-slate-900">Meta API execution logs</h1>
          <p className="mt-1 text-sm text-slate-500">
            Every outbound HTTP call the WhatsApp adapter makes to Meta is recorded here — request
            body, response body, status, latency, and Meta's fbtrace_id. Use this when a send
            silently fails or a template call returns an unexpected 4xx.
          </p>
        </div>
      </header>

      <section
        aria-label="Filters"
        className="grid gap-3 rounded-xl border border-slate-200 bg-white p-4 sm:grid-cols-2 lg:grid-cols-5"
      >
        <label className="flex flex-col gap-1 text-xs font-medium text-slate-600">
          Integration
          <select
            value={integrationID}
            onChange={(e) => setIntegrationID(e.target.value)}
            className="rounded-lg border border-slate-200 bg-white px-2 py-1.5 text-sm text-slate-800"
          >
            <option value="">All</option>
            {(integrations.data ?? []).map((i) => (
              <option key={i.id} value={i.id}>
                {i.name}
              </option>
            ))}
          </select>
        </label>
        <label className="flex flex-col gap-1 text-xs font-medium text-slate-600">
          Operation
          <select
            value={operation}
            onChange={(e) => setOperation(e.target.value)}
            className="rounded-lg border border-slate-200 bg-white px-2 py-1.5 text-sm text-slate-800"
          >
            <option value="">All</option>
            {PROVIDER_CALL_OPERATIONS.map((op) => (
              <option key={op} value={op}>
                {op}
              </option>
            ))}
          </select>
        </label>
        <div className="flex flex-col gap-1 text-xs font-medium text-slate-600">
          <span>Status</span>
          <div className="flex gap-1">
            {(['all', '2xx', '4xx', '5xx'] as const).map((p) => (
              <button
                key={p}
                type="button"
                onClick={() => setStatusPreset(p)}
                className={`rounded-md px-2 py-1 text-xs font-medium ring-1 ring-inset ${
                  statusPreset === p
                    ? 'bg-emerald-50 text-emerald-700 ring-emerald-200'
                    : 'bg-white text-slate-600 ring-slate-200 hover:bg-slate-50'
                }`}
              >
                {p}
              </button>
            ))}
          </div>
        </div>
        <label className="flex flex-col gap-1 text-xs font-medium text-slate-600">
          Since
          <input
            type="datetime-local"
            value={since === '' ? '' : toDateInputValue(since)}
            onChange={(e) => setSince(e.target.value)}
            className="rounded-lg border border-slate-200 bg-white px-2 py-1.5 text-sm text-slate-800"
          />
        </label>
        <label className="flex flex-col gap-1 text-xs font-medium text-slate-600">
          Until
          <input
            type="datetime-local"
            value={until === '' ? '' : toDateInputValue(until)}
            onChange={(e) => setUntil(e.target.value)}
            className="rounded-lg border border-slate-200 bg-white px-2 py-1.5 text-sm text-slate-800"
          />
        </label>
        {anyFilter && (
          <div className="sm:col-span-2 lg:col-span-5 flex justify-end">
            <button
              type="button"
              onClick={clearFilters}
              className="text-xs font-medium text-slate-600 hover:text-slate-900"
            >
              Clear filters
            </button>
          </div>
        )}
      </section>

      {query.isPending && (
        <div className="flex items-center justify-center rounded-xl border border-slate-200 bg-white py-12">
          <Spinner className="h-6 w-6 text-slate-500" label="Loading provider calls" />
        </div>
      )}

      {query.isError && (
        <div
          role="alert"
          className="rounded-xl border border-rose-200 bg-rose-50 p-4 text-sm text-rose-800"
        >
          <p className="font-medium">
            {isPermDenied
              ? "You don't have permission to view provider calls."
              : isOffline
                ? "You're offline. Reconnect to see provider calls."
                : 'Could not load provider calls.'}
          </p>
          {!isPermDenied && (
            <button
              type="button"
              onClick={() => void query.refetch()}
              className="mt-2 rounded-lg bg-white px-3 py-1 text-xs font-medium text-rose-700 ring-1 ring-inset ring-rose-200 hover:bg-rose-100"
            >
              Retry
            </button>
          )}
        </div>
      )}

      {!query.isPending && !query.isError && items.length === 0 && (
        <EmptyState
          title="No provider calls yet."
          description={
            anyFilter
              ? 'No entries match the current filters. Try clearing them.'
              : 'Entries will appear here after the first outbound call.'
          }
        />
      )}

      {items.length > 0 && (
        <div className="overflow-x-auto rounded-xl border border-slate-200 bg-white">
          <table className="w-full min-w-[900px] text-left">
            <thead className="bg-slate-50 text-xs uppercase tracking-wide text-slate-500">
              <tr>
                <th className="px-3 py-2 font-medium">When</th>
                <th className="px-3 py-2 font-medium">Operation</th>
                <th className="px-3 py-2 font-medium">Method</th>
                <th className="px-3 py-2 font-medium">URL</th>
                <th className="px-3 py-2 font-medium">Status</th>
                <th className="px-3 py-2 font-medium">Latency</th>
                <th className="px-3 py-2 font-medium">Error</th>
                <th className="w-20 px-3 py-2 pr-4" />
              </tr>
            </thead>
            <tbody>
              {items.map((entry) => (
                <EntryRow key={entry.id} entry={entry} />
              ))}
            </tbody>
          </table>
        </div>
      )}

      {query.hasNextPage === true && (
        <div className="flex justify-center">
          <Button
            variant="secondary"
            onClick={() => {
              void query.fetchNextPage();
            }}
            disabled={query.isFetchingNextPage}
          >
            {query.isFetchingNextPage ? 'Loading…' : 'Load more'}
          </Button>
        </div>
      )}
    </div>
  );
}

export const settingsProviderCallsRoute = createRoute({
  getParentRoute: () => settingsRoute,
  path: '/provider-calls',
  component: ProviderCallsPage,
});
