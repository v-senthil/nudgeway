import { useEffect, useState } from 'react';
import { createPortal } from 'react-dom';
import type { Integration } from '../../lib/integrations';
import { BusinessProfileTab } from './BusinessProfileTab';
import { CallSettingsTab } from './CallSettingsTab';
import { DetailsTab } from './DetailsTab';
import { OBATab } from './OBATab';
import { UsernameTab } from './UsernameTab';

// SettingsDrawer is a right-side slide-in panel that hosts the per-
// integration settings tabs. Dims the rest of the page while open.
export function SettingsDrawer({
  integration,
  onClose,
}: {
  integration: Integration | null;
  onClose: () => void;
}) {
  const [tab, setTab] = useState<'details' | 'business-profile' | 'call-settings' | 'oba-status' | 'username'>('details');

  useEffect(() => {
    if (integration === null) return;
    setTab('details');
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [integration, onClose]);

  if (integration === null) return null;

  // Portal to document.body so no ancestor's transform / overflow / stacking
  // context can clip the drawer or leave the app header peeking through the
  // top of the panel. z-[100] beats every in-app chrome layer.
  return createPortal(
    <div
      className="fixed inset-0 z-[100] flex bg-slate-900/40"
      role="presentation"
      onClick={onClose}
    >
      <aside
        role="dialog"
        aria-modal="true"
        aria-labelledby="integration-settings-title"
        className="ml-auto flex h-full w-full max-w-xl flex-col border-l border-slate-200 bg-white shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <header className="flex items-start justify-between gap-3 border-b border-slate-200 px-5 py-3">
          <div className="min-w-0 flex-1">
            <h2 id="integration-settings-title" className="text-base font-semibold text-slate-900">
              {integration.name}
            </h2>
            <p className="text-xs text-slate-500 capitalize">{integration.provider} settings</p>
          </div>
          <button
            type="button"
            aria-label="Close settings"
            onClick={onClose}
            className="rounded-full p-1 text-slate-500 hover:bg-slate-100"
          >
            <svg
              aria-hidden="true"
              viewBox="0 0 20 20"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.75"
              className="h-5 w-5"
            >
              <path strokeLinecap="round" strokeLinejoin="round" d="M5 5l10 10M15 5L5 15" />
            </svg>
          </button>
        </header>

        <nav className="flex gap-1 border-b border-slate-200 px-3 pt-2" aria-label="Settings tabs">
          <TabButton active={tab === 'details'} onClick={() => setTab('details')}>
            Details
          </TabButton>
          <TabButton active={tab === 'business-profile'} onClick={() => setTab('business-profile')}>
            Business profile
          </TabButton>
          <TabButton active={tab === 'call-settings'} onClick={() => setTab('call-settings')}>
            Call settings
          </TabButton>
          <TabButton active={tab === 'oba-status'} onClick={() => setTab('oba-status')}>
            OBA status
          </TabButton>
          <TabButton active={tab === 'username'} onClick={() => setTab('username')}>
            Username
          </TabButton>
        </nav>

        <div className="flex-1 overflow-y-auto px-5 py-5">
          {tab === 'details' && <DetailsTab integration={integration} />}
          {tab === 'business-profile' && (
            <BusinessProfileTab integrationID={integration.id} active={tab === 'business-profile'} />
          )}
          {tab === 'call-settings' && (
            <CallSettingsTab integrationID={integration.id} active={tab === 'call-settings'} />
          )}
          {tab === 'oba-status' && (
            <OBATab integrationID={integration.id} active={tab === 'oba-status'} />
          )}
          {tab === 'username' && (
            <UsernameTab integrationID={integration.id} active={tab === 'username'} />
          )}
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
