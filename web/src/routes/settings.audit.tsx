import { useMemo, useState } from 'react';
import { createRoute } from '@tanstack/react-router';
import { settingsRoute } from './settings';
import { Spinner } from '../components/Spinner';
import { EmptyState } from '../components/EmptyState';
import { Button } from '../components/Button';
import {
  AUDIT_ACTIONS,
  AUDIT_RESOURCE_TYPES,
  useAuditLogs,
  type AuditLog,
  type AuditLogFilter,
} from '../lib/audit';
import { ApiError } from '../lib/api';

function toDateInputValue(iso: string): string {
  // <input type="datetime-local"> requires "YYYY-MM-DDTHH:mm" with no
  // trailing Z / offset. RFC3339 → input.
  return iso.slice(0, 16);
}

function fromDateInputValue(local: string): string {
  if (local === '') return '';
  // Interpret the operator's local time and convert to RFC3339 UTC.
  const d = new Date(local);
  if (Number.isNaN(d.getTime())) return '';
  return d.toISOString();
}

function formatOccurred(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString();
}

function ActionBadge({ action }: { action: string }) {
  // Colour-code by verb family so the eye can scan a mixed list.
  const family = action.split('.')[0] ?? action;
  const colours: Record<string, string> = {
    integration: 'bg-indigo-50 text-indigo-700 ring-indigo-100',
    message: 'bg-emerald-50 text-emerald-700 ring-emerald-100',
    conversation: 'bg-teal-50 text-teal-700 ring-teal-100',
    attachment: 'bg-amber-50 text-amber-700 ring-amber-100',
    user: 'bg-slate-100 text-slate-700 ring-slate-200',
  };
  const cls = colours[family] ?? 'bg-slate-100 text-slate-700 ring-slate-200';
  return (
    <span
      className={`inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium ring-1 ring-inset ${cls}`}
    >
      {action}
    </span>
  );
}

function EntryRow({ entry }: { entry: AuditLog }) {
  const [expanded, setExpanded] = useState(false);
  const hasMeta = entry.metadata !== undefined && Object.keys(entry.metadata).length > 0;
  return (
    <>
      <tr className="border-t border-slate-100 hover:bg-slate-50">
        <td className="whitespace-nowrap px-3 py-2 text-xs text-slate-500">
          {formatOccurred(entry.occurred_at)}
        </td>
        <td className="px-3 py-2">
          <ActionBadge action={entry.action} />
        </td>
        <td className="px-3 py-2 text-sm text-slate-700">
          <div className="font-medium">{entry.resource_type}</div>
          {entry.resource_id !== undefined && entry.resource_id !== '' && (
            <div className="font-mono text-xs text-slate-500">{entry.resource_id}</div>
          )}
        </td>
        <td className="px-3 py-2 font-mono text-xs text-slate-500">
          {entry.actor_user_id ?? <span className="italic text-slate-400">system</span>}
        </td>
        <td className="px-3 py-2 text-xs text-slate-500">{entry.ip ?? '—'}</td>
        <td className="px-3 py-2 text-right">
          {hasMeta && (
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
      {expanded && hasMeta && (
        <tr className="border-t border-slate-100 bg-slate-50">
          <td colSpan={6} className="px-3 py-3">
            <pre className="overflow-x-auto rounded-lg bg-slate-900 p-3 text-xs text-slate-100">
              {JSON.stringify(entry.metadata, null, 2)}
            </pre>
          </td>
        </tr>
      )}
    </>
  );
}

function AuditPage() {
  const [action, setAction] = useState<string>('');
  const [resourceType, setResourceType] = useState<string>('');
  const [actorUserID, setActorUserID] = useState<string>('');
  const [since, setSince] = useState<string>('');
  const [until, setUntil] = useState<string>('');

  const filter: AuditLogFilter = useMemo(
    () => ({
      action,
      resource_type: resourceType,
      actor_user_id: actorUserID,
      since: fromDateInputValue(since),
      until: fromDateInputValue(until),
      limit: 50,
    }),
    [action, resourceType, actorUserID, since, until],
  );

  const query = useAuditLogs(filter);

  const isPermDenied = query.error instanceof ApiError && query.error.status === 403;
  const isOffline = query.isError && typeof navigator !== 'undefined' && !navigator.onLine;

  const items = useMemo(() => {
    if (query.data === undefined) return [];
    const out: AuditLog[] = [];
    for (const page of query.data.pages) out.push(...page.items);
    return out;
  }, [query.data]);

  const clearFilters = () => {
    setAction('');
    setResourceType('');
    setActorUserID('');
    setSince('');
    setUntil('');
  };

  const anyFilter =
    action !== '' || resourceType !== '' || actorUserID !== '' || since !== '' || until !== '';

  return (
    <div className="mx-auto max-w-6xl space-y-6">
      <header className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold text-slate-900">Audit log</h1>
          <p className="mt-1 text-sm text-slate-500">
            Every mutation on your workspace is recorded here. Filter by verb, resource, actor, or
            time window.
          </p>
        </div>
      </header>

      <section
        aria-label="Filters"
        className="grid gap-3 rounded-xl border border-slate-200 bg-white p-4 sm:grid-cols-2 lg:grid-cols-5"
      >
        <label className="flex flex-col gap-1 text-xs font-medium text-slate-600">
          Action
          <select
            value={action}
            onChange={(e) => setAction(e.target.value)}
            className="rounded-lg border border-slate-200 bg-white px-2 py-1.5 text-sm text-slate-800"
          >
            <option value="">All</option>
            {AUDIT_ACTIONS.map((a) => (
              <option key={a} value={a}>
                {a}
              </option>
            ))}
          </select>
        </label>
        <label className="flex flex-col gap-1 text-xs font-medium text-slate-600">
          Resource type
          <select
            value={resourceType}
            onChange={(e) => setResourceType(e.target.value)}
            className="rounded-lg border border-slate-200 bg-white px-2 py-1.5 text-sm text-slate-800"
          >
            <option value="">All</option>
            {AUDIT_RESOURCE_TYPES.map((t) => (
              <option key={t} value={t}>
                {t}
              </option>
            ))}
          </select>
        </label>
        <label className="flex flex-col gap-1 text-xs font-medium text-slate-600">
          Actor user ID
          <input
            type="text"
            value={actorUserID}
            onChange={(e) => setActorUserID(e.target.value)}
            placeholder="ULID"
            className="rounded-lg border border-slate-200 bg-white px-2 py-1.5 font-mono text-xs text-slate-800"
          />
        </label>
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
          <Spinner className="h-6 w-6 text-slate-500" label="Loading audit log" />
        </div>
      )}

      {query.isError && (
        <div
          role="alert"
          className="rounded-xl border border-rose-200 bg-rose-50 p-4 text-sm text-rose-800"
        >
          <p className="font-medium">
            {isPermDenied
              ? "You don't have permission to view the audit log."
              : isOffline
                ? "You're offline. Reconnect to see the audit log."
                : 'Could not load the audit log.'}
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
          title="No audit entries yet."
          description={
            anyFilter
              ? 'No entries match the current filters. Try clearing them.'
              : 'Audit entries will appear here after the first mutation.'
          }
        />
      )}

      {items.length > 0 && (
        <div className="overflow-hidden rounded-xl border border-slate-200 bg-white">
          <table className="w-full text-left">
            <thead className="bg-slate-50 text-xs uppercase tracking-wide text-slate-500">
              <tr>
                <th className="px-3 py-2 font-medium">When</th>
                <th className="px-3 py-2 font-medium">Action</th>
                <th className="px-3 py-2 font-medium">Resource</th>
                <th className="px-3 py-2 font-medium">Actor</th>
                <th className="px-3 py-2 font-medium">IP</th>
                <th className="px-3 py-2" />
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

export const settingsAuditRoute = createRoute({
  getParentRoute: () => settingsRoute,
  path: '/audit',
  component: AuditPage,
});
