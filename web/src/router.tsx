import { createRouter, createRoute, redirect } from '@tanstack/react-router';
import { rootRoute } from './routes/__root';
import { loginRoute } from './routes/login';
import { inboxRoute } from './routes/inbox';
import { settingsRoute } from './routes/settings';
import { settingsIntegrationsRoute, settingsIndexRoute } from './routes/settings.integrations';
import { settingsAuditRoute } from './routes/settings.audit';
import { settingsProviderCallsRoute } from './routes/settings/provider-calls';
import { settingsTemplatesRoute } from './routes/settings.templates';
import { settingsGroupsRoute } from './routes/settings.groups';
import { settingsAPITokensRoute } from './routes/settings.api-tokens';
import { callsRoute } from './routes/calls';
import { analyticsRoute } from './routes/analytics';

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
  callsRoute,
  analyticsRoute,
  settingsRoute.addChildren([
    settingsIndexRoute,
    settingsIntegrationsRoute,
    settingsAuditRoute,
    settingsProviderCallsRoute,
    settingsTemplatesRoute,
    settingsGroupsRoute,
    settingsAPITokensRoute,
  ]),
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
