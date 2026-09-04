import { useEffect } from 'react';
import { Link, Outlet, createRoute, useNavigate } from '@tanstack/react-router';
import { rootRoute } from './__root';
import { useMe } from '../lib/auth';
import { Header } from '../features/inbox/Header';
import { Spinner } from '../components/Spinner';

const sections: Array<{ label: string; to?: string; disabled?: boolean }> = [
  { label: 'Profile', disabled: true },
  { label: 'Organization', disabled: true },
  { label: 'Users & roles', disabled: true },
  { label: 'Integrations', to: '/settings/integrations' },
  { label: 'Templates', to: '/settings/templates' },
  { label: 'Groups', to: '/settings/groups' },
  { label: 'Audit log', to: '/settings/audit' },
  { label: 'Meta API logs', to: '/settings/provider-calls' },
  { label: 'Billing', disabled: true },
  { label: 'API keys', disabled: true },
];

function SettingsShell() {
  const me = useMe();
  const navigate = useNavigate();

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

  if (me.data === null || me.data === undefined) {
    return null;
  }

  return (
    <div className="flex h-screen flex-col overflow-hidden">
      <Header me={me.data} />
      <div className="flex flex-1 overflow-hidden">
        <nav
          aria-label="Settings sections"
          className="flex w-[240px] flex-shrink-0 flex-col border-r border-slate-200 bg-white px-3 pb-4"
        >
          <p className="px-3 pb-2 pt-4 text-xs font-semibold uppercase tracking-wide text-slate-400">
            Settings
          </p>
          <ul className="space-y-0.5">
            {sections.map((s) => (
              <li key={s.label}>
                {s.disabled === true || s.to === undefined ? (
                  <span
                    aria-disabled="true"
                    className="block cursor-not-allowed rounded-lg px-3 py-2 text-sm text-slate-400"
                  >
                    {s.label}
                  </span>
                ) : (
                  <Link
                    to={s.to}
                    className="block rounded-lg px-3 py-2 text-sm text-slate-700 hover:bg-slate-100"
                    activeProps={{ className: 'block rounded-lg px-3 py-2 text-sm bg-emerald-50 text-emerald-700 font-medium' }}
                  >
                    {s.label}
                  </Link>
                )}
              </li>
            ))}
          </ul>
        </nav>
        <main className="flex-1 overflow-y-auto bg-slate-50 px-8 py-6">
          <Outlet />
        </main>
      </div>
    </div>
  );
}

export const settingsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/settings',
  component: SettingsShell,
});
