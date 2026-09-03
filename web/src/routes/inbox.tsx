import { useEffect } from 'react';
import { createRoute, useNavigate } from '@tanstack/react-router';
import { rootRoute } from './__root';
import { useMe } from '../lib/auth';
import { Header } from '../features/inbox/Header';
import { ConversationList } from '../features/inbox/ConversationList';
import { Thread } from '../features/inbox/Thread';
import { ContactPanel } from '../features/inbox/ContactPanel';
import { Spinner } from '../components/Spinner';

function InboxPage() {
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
    <div className="flex h-screen flex-col">
      <Header me={me.data} />
      <div className="flex flex-1 overflow-hidden">
        <ConversationList />
        <Thread />
        <ContactPanel />
      </div>
    </div>
  );
}

export const inboxRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/inbox',
  component: InboxPage,
});
