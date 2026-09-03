import { useNavigate } from '@tanstack/react-router';
import { EmptyState } from '../../components/EmptyState';
import { Button } from '../../components/Button';

export function ConversationList() {
  const navigate = useNavigate();

  return (
    <aside className="flex w-[280px] flex-shrink-0 flex-col border-r border-slate-200 bg-white">
      <div className="flex items-center justify-between border-b border-slate-200 px-4 py-3">
        <h2 className="text-sm font-semibold text-slate-900">Conversations</h2>
      </div>
      <div className="border-b border-slate-200 px-3 py-2">
        <label htmlFor="conv-search" className="sr-only">
          Search conversations
        </label>
        <input
          id="conv-search"
          type="search"
          placeholder="Search conversations…"
          className="w-full rounded-xl border border-slate-200 bg-slate-50 px-3 py-1.5 text-sm placeholder:text-slate-400 focus:border-emerald-500 focus:bg-white focus:outline-none focus:ring-2 focus:ring-emerald-200"
        />
      </div>
      <div className="flex-1 overflow-y-auto">
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
                d="M7.5 8.25h9m-9 3.75h6M4.5 6A2.25 2.25 0 016.75 3.75h10.5A2.25 2.25 0 0119.5 6v9a2.25 2.25 0 01-2.25 2.25H12l-4.5 3v-3H6.75A2.25 2.25 0 014.5 15V6z"
              />
            </svg>
          }
          title="No conversations yet"
          description="Connect a channel to get started."
          action={
            <Button
              variant="primary"
              onClick={() => void navigate({ to: '/settings/integrations' })}
            >
              Connect an integration
            </Button>
          }
        />
      </div>
    </aside>
  );
}
