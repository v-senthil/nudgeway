import { createRouter, createRoute, redirect } from '@tanstack/react-router';
import { rootRoute } from './routes/__root';
import { loginRoute } from './routes/login';
import { inboxRoute } from './routes/inbox';
import { settingsRoute } from './routes/settings';
import { settingsIntegrationsRoute, settingsIndexRoute } from './routes/settings.integrations';

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  beforeLoad: () => {
    throw redirect({ to: '/inbox' });
  },
});

const routeTree = rootRoute.addChildren([
  indexRoute,
  loginRoute,
  inboxRoute,
  settingsRoute.addChildren([settingsIndexRoute, settingsIntegrationsRoute]),
]);

export const router = createRouter({
  routeTree,
  defaultPreload: 'intent',
});

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router;
  }
}
