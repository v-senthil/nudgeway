import { useState } from 'react';
import { createRoute } from '@tanstack/react-router';
import { settingsRoute } from './settings';
import { EmptyState } from '../components/EmptyState';
import { Button } from '../components/Button';
import { Spinner } from '../components/Spinner';
import { CreateAPITokenModal } from '../features/settings/CreateAPITokenModal';
import { APITokenCreatedModal } from '../features/settings/APITokenCreatedModal';
import { RevokeTokenModal } from '../features/settings/RevokeTokenModal';
import { APITokenUsageDrawer } from '../features/settings/APITokenUsageDrawer';
import {
  useAPITokens,
  useRevokeAPIToken,
  type APIToken,
  type CreateAPITokenResponse,
} from '../lib/api-tokens';
import { ApiError } from '../lib/api';

function formatDate(iso: string | null | undefined): string {
  if (iso === null || iso === undefined || iso === '') return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString();
}

function formatRelative(iso: string | null | undefined): string {
  if (iso === null || iso === undefined || iso === '') return 'Never';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString();
}

function KeyIcon() {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.75"
      strokeLinecap="round"
      strokeLinejoin="round"
      className="h-6 w-6"
      aria-hidden="true"
    >
      <circle cx="7.5" cy="15.5" r="4.5" />
      <path d="m21 2-9.6 9.6" />
      <path d="m15.5 7.5 3 3L22 7l-3-3" />
    </svg>
  );
}

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

function APITokensPage() {
  const [createOpen, setCreateOpen] = useState(false);
  const [createdToken, setCreatedToken] = useState<CreateAPITokenResponse | null>(null);
  const [pendingRevoke, setPendingRevoke] = useState<APIToken | null>(null);
  const [revokeError, setRevokeError] = useState<string | null>(null);
  const [usageToken, setUsageToken] = useState<APIToken | null>(null);

  const tokens = useAPITokens();
  const revoke = useRevokeAPIToken();

  const isPermDenied = tokens.error instanceof ApiError && tokens.error.status === 403;
  const isOffline = tokens.isError && typeof navigator !== 'undefined' && !navigator.onLine;

  const confirmRevoke = async () => {
    if (pendingRevoke === null) return;
    setRevokeError(null);
    try {
      await revoke.mutateAsync(pendingRevoke.id);
      setPendingRevoke(null);
    } catch (err) {
      setRevokeError(
        err instanceof ApiError
          ? err.problem.detail ?? err.problem.title ?? 'Revoke failed'
          : 'Revoke failed',
      );
    }
  };

  const handleCreated = (res: CreateAPITokenResponse) => {
    setCreateOpen(false);
    setCreatedToken(res);
  };

  return (
    <div className="mx-auto max-w-5xl space-y-6">
      <header className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold text-slate-900">API tokens</h1>
          <p className="mt-1 text-sm text-slate-500">
            Personal access tokens for programmatic use — MCP servers, CI jobs, custom scripts.
            Cookies stay for the web UI; tokens replace them everywhere else.
          </p>
        </div>
        {tokens.data !== undefined && tokens.data.length > 0 && (
          <Button variant="primary" onClick={() => setCreateOpen(true)}>
            New token
          </Button>
        )}
      </header>

      {tokens.isPending && (
        <div className="flex items-center justify-center rounded-xl border border-slate-200 bg-white py-12">
          <Spinner className="h-6 w-6 text-slate-500" label="Loading API tokens" />
        </div>
      )}

      {tokens.isError && (
        <div
          role="alert"
          className="rounded-xl border border-rose-200 bg-rose-50 p-4 text-sm text-rose-800"
        >
          <p className="font-medium">
            {isPermDenied
              ? "You don't have permission to view API tokens."
              : isOffline
                ? "You're offline. Reconnect to see API tokens."
                : 'Could not load API tokens.'}
          </p>
          {!isPermDenied && (
            <button
              type="button"
              onClick={() => void tokens.refetch()}
              className="mt-2 rounded-lg bg-white px-3 py-1 text-xs font-medium text-rose-700 ring-1 ring-inset ring-rose-200 hover:bg-rose-100"
            >
              Retry
            </button>
          )}
        </div>
      )}

      {tokens.data !== undefined && tokens.data.length === 0 && (
        <div className="rounded-xl border border-slate-200 bg-white">
          <EmptyState
            icon={<KeyIcon />}
            title="No API tokens yet."
            description="Mint a token to authenticate the MCP server, a CI pipeline, or a custom script against the Nudgeway API."
            action={
              <Button variant="primary" onClick={() => setCreateOpen(true)}>
                New token
              </Button>
            }
          />
        </div>
      )}

      {tokens.data !== undefined && tokens.data.length > 0 && (
        <div className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm">
          <table className="min-w-full divide-y divide-slate-200">
            <thead className="bg-slate-50">
              <tr>
                <th className="px-4 py-2 text-left text-xs font-semibold uppercase tracking-wide text-slate-500">
                  Name
                </th>
                <th className="px-4 py-2 text-left text-xs font-semibold uppercase tracking-wide text-slate-500">
                  Prefix
                </th>
                <th className="px-4 py-2 text-left text-xs font-semibold uppercase tracking-wide text-slate-500">
                  Last used
                </th>
                <th className="px-4 py-2 text-left text-xs font-semibold uppercase tracking-wide text-slate-500">
                  Expires
                </th>
                <th className="px-4 py-2 text-left text-xs font-semibold uppercase tracking-wide text-slate-500">
                  Created
                </th>
                <th className="px-4 py-2 text-right text-xs font-semibold uppercase tracking-wide text-slate-500">
                  <span className="sr-only">Actions</span>
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100">
              {tokens.data.map((t) => {
                const revoked =
                  t.revoked_at !== null && t.revoked_at !== undefined && t.revoked_at !== '';
                const rowClass = revoked
                  ? 'cursor-pointer text-sm opacity-60 hover:bg-slate-50'
                  : 'cursor-pointer text-sm hover:bg-slate-50';
                return (
                  <tr
                    key={t.id}
                    className={rowClass}
                    onClick={() => setUsageToken(t)}
                  >
                    <td className="px-4 py-3">
                      <div className="flex items-center gap-2">
                        <span className="font-medium text-slate-900">{t.name}</span>
                        <TokenStatusChip token={t} />
                      </div>
                    </td>
                    <td className="px-4 py-3">
                      <code className="rounded-md bg-slate-50 px-1.5 py-0.5 font-mono text-xs text-slate-700 ring-1 ring-inset ring-slate-200">
                        {t.prefix}
                        <span className="text-slate-400">…</span>
                      </code>
                    </td>
                    <td className="px-4 py-3 text-xs text-slate-500">
                      {formatRelative(t.last_used_at)}
                    </td>
                    <td className="px-4 py-3 text-xs text-slate-500">
                      {formatRelative(t.expires_at)}
                    </td>
                    <td className="px-4 py-3 text-xs text-slate-500">{formatDate(t.created_at)}</td>
                    <td
                      className="px-4 py-3 text-right"
                      onClick={(e) => e.stopPropagation()}
                    >
                      <div className="flex items-center justify-end gap-1">
                        <Button
                          variant="ghost"
                          onClick={() => setUsageToken(t)}
                          aria-label={`View usage for token ${t.name}`}
                        >
                          View usage
                        </Button>
                        {!revoked && (
                          <Button
                            variant="ghost"
                            onClick={() => {
                              setRevokeError(null);
                              setPendingRevoke(t);
                            }}
                            aria-label={`Revoke token ${t.name}`}
                            className="text-rose-700 hover:bg-rose-50"
                          >
                            Revoke
                          </Button>
                        )}
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      <CreateAPITokenModal
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        onCreated={handleCreated}
      />

      <APITokenCreatedModal
        open={createdToken !== null}
        onClose={() => setCreatedToken(null)}
        plaintext={createdToken?.plaintext ?? ''}
        name={createdToken?.name ?? ''}
      />

      <RevokeTokenModal
        open={pendingRevoke !== null}
        onClose={() => {
          setPendingRevoke(null);
          setRevokeError(null);
        }}
        onConfirm={() => void confirmRevoke()}
        tokenName={pendingRevoke?.name}
        loading={revoke.isPending}
        errorMessage={revokeError ?? undefined}
      />

      <APITokenUsageDrawer token={usageToken} onClose={() => setUsageToken(null)} />
    </div>
  );
}

export const settingsAPITokensRoute = createRoute({
  getParentRoute: () => settingsRoute,
  path: '/api-tokens',
  component: APITokensPage,
});
