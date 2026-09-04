import { Outlet, createRootRoute } from '@tanstack/react-router';
import { useMe } from '../lib/auth';
import { useInboxSocket } from '../lib/ws';
import { IncomingCallPopup } from '../features/inbox/IncomingCallPopup';

function RootComponent() {
  // Open the shared inbox WebSocket as soon as we know the org id. This
  // keeps the connection live across route transitions (previously only
  // the inbox page opened it) so the global incoming-call popup fires on
  // every screen — calls, analytics, settings, etc. useMe short-circuits
  // to null on 401 so anonymous visitors never open a socket.
  const me = useMe();
  useInboxSocket(me.data?.org_id ?? null);
  return (
    <div className="min-h-screen bg-slate-50 text-slate-900">
      <Outlet />
      <IncomingCallPopup />
    </div>
  );
}

function NotFound() {
  return (
    <div className="flex min-h-screen items-center justify-center">
      <div className="text-center">
        <p className="text-sm font-medium text-slate-500">404</p>
        <h1 className="mt-2 text-2xl font-semibold text-slate-900">Page not found</h1>
      </div>
    </div>
  );
}

export const rootRoute = createRootRoute({
  component: RootComponent,
  notFoundComponent: NotFound,
});
