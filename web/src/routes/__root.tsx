import { Outlet, createRootRoute } from '@tanstack/react-router';

function RootComponent() {
  return (
    <div className="min-h-screen bg-slate-50 text-slate-900">
      <Outlet />
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
