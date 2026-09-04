import { useState } from 'react';
import { createRoute } from '@tanstack/react-router';
import { settingsRoute } from './settings';
import { IntegrationList } from '../features/settings/IntegrationList';
import { ConnectWhatsAppModal } from '../features/settings/ConnectWhatsAppModal';
import { SettingsDrawer } from '../features/integrations/SettingsDrawer';
import { EmptyState } from '../components/EmptyState';
import { Button } from '../components/Button';
import { Spinner } from '../components/Spinner';
import { useIntegrations, type Integration } from '../lib/integrations';
import { ApiError } from '../lib/api';

function IntegrationsPage() {
  const [connectOpen, setConnectOpen] = useState(false);
  const [drawerFor, setDrawerFor] = useState<Integration | null>(null);
  const integrations = useIntegrations();

  const isPermDenied = integrations.error instanceof ApiError && integrations.error.status === 403;
  const isOffline = integrations.isError && typeof navigator !== 'undefined' && !navigator.onLine;

  return (
    <div className="mx-auto max-w-4xl space-y-6">
      <header className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold text-slate-900">Integrations</h1>
          <p className="mt-1 text-sm text-slate-500">
            Connect channels to receive and send conversations from your workspace.
          </p>
        </div>
        {integrations.data !== undefined && integrations.data.length > 0 && (
          <Button variant="primary" onClick={() => setConnectOpen(true)}>
            Connect WhatsApp
          </Button>
        )}
      </header>

      {integrations.isPending && (
        <div className="flex items-center justify-center rounded-xl border border-slate-200 bg-white py-12">
          <Spinner className="h-6 w-6 text-slate-500" label="Loading integrations" />
        </div>
      )}

      {integrations.isError && (
        <div
          role="alert"
          className="rounded-xl border border-rose-200 bg-rose-50 p-4 text-sm text-rose-800"
        >
          <p className="font-medium">
            {isPermDenied
              ? "You don't have permission to view integrations."
              : isOffline
                ? "You're offline. Reconnect to see integrations."
                : 'Could not load integrations.'}
          </p>
          {!isPermDenied && (
            <button
              type="button"
              onClick={() => void integrations.refetch()}
              className="mt-2 rounded-lg bg-white px-3 py-1 text-xs font-medium text-rose-700 ring-1 ring-inset ring-rose-200 hover:bg-rose-100"
            >
              Retry
            </button>
          )}
        </div>
      )}

      {integrations.data !== undefined && integrations.data.length === 0 && (
        <EmptyState
          icon={
            <svg
              xmlns="http://www.w3.org/2000/svg"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.75"
              className="h-6 w-6"
              aria-hidden="true"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="M15.75 9V5.25A2.25 2.25 0 0013.5 3h-6A2.25 2.25 0 005.25 5.25v13.5A2.25 2.25 0 007.5 21h6a2.25 2.25 0 002.25-2.25V15M12 12h9m0 0l-3-3m3 3l-3 3"
              />
            </svg>
          }
          title="No integrations connected yet."
          description="Connect the WhatsApp Business Cloud API to start receiving conversations."
          action={
            <Button variant="primary" onClick={() => setConnectOpen(true)}>
              Connect WhatsApp
            </Button>
          }
        />
      )}

      {integrations.data !== undefined && integrations.data.length > 0 && (
        <IntegrationList items={integrations.data} onOpenSettings={setDrawerFor} />
      )}

      <ConnectWhatsAppModal open={connectOpen} onClose={() => setConnectOpen(false)} />
      <SettingsDrawer integration={drawerFor} onClose={() => setDrawerFor(null)} />
    </div>
  );
}

export const settingsIntegrationsRoute = createRoute({
  getParentRoute: () => settingsRoute,
  path: '/integrations',
  component: IntegrationsPage,
});

export const settingsIndexRoute = createRoute({
  getParentRoute: () => settingsRoute,
  path: '/',
  component: IntegrationsPage,
});
