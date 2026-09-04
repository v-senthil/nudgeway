import { useMemo, useState } from 'react';
import { useNavigate } from '@tanstack/react-router';
import { EmptyState } from '../../components/EmptyState';
import { Button } from '../../components/Button';
import { Spinner } from '../../components/Spinner';
import { useConversations } from '../../lib/messages';
import type { Conversation } from '../../lib/messages';
import { useIntegrations } from '../../lib/integrations';
import { ApiError } from '../../lib/api';

type Props = {
  selectedID: string | null;
};

function formatTime(iso: string | undefined): string {
  if (iso === undefined) return '';
  // Force UTC parsing when the server omits the timezone marker; JS
  // otherwise treats naive datetimes as local, double-shifting the display.
  const s = /Z|[+-]\d{2}:?\d{2}$/.test(iso) ? iso : `${iso}Z`;
  const d = new Date(s);
  if (Number.isNaN(d.getTime())) return '';
  const now = new Date();
  const sameDay = d.toDateString() === now.toDateString();
  if (sameDay) {
    return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', hour12: false });
  }
  return d.toLocaleDateString([], { month: 'short', day: 'numeric' });
}

function initials(name: string | undefined): string {
  if (name === undefined || name.length === 0) return '?';
  const parts = name.trim().split(/\s+/);
  const first = parts[0];
  const second = parts[1];
  if (first !== undefined && second !== undefined) return `${first[0] ?? ''}${second[0] ?? ''}`.toUpperCase();
  if (first !== undefined) return first.slice(0, 2).toUpperCase();
  return '?';
}

export function ConversationList({ selectedID }: Props) {
  const [query, setQuery] = useState('');
  const conversations = useConversations();
  const integrations = useIntegrations();
  const navigate = useNavigate();

  const filtered = useMemo<Conversation[]>(() => {
    const items = conversations.data ?? [];
    if (query.trim().length === 0) return items;
    const q = query.trim().toLowerCase();
    return items.filter((c) => {
      const name = c.contact_name?.toLowerCase() ?? '';
      const subject = c.subject?.toLowerCase() ?? '';
      const preview = c.last_message_preview?.toLowerCase() ?? '';
      return name.includes(q) || subject.includes(q) || preview.includes(q);
    });
  }, [conversations.data, query]);

  // titleFor returns the row's display title. Group threads show the group
  // subject; 1-to-1 threads show the contact name.
  const titleFor = (c: Conversation): string => {
    if (c.type === 'group') return c.subject ?? 'Unnamed group';
    return c.contact_name ?? 'Unknown contact';
  };

  const selectConversation = (id: string) => {
    void navigate({ to: '/inbox', search: { c: id } });
  };

  const isPermDenied = conversations.error instanceof ApiError && conversations.error.status === 403;
  const isOffline = conversations.isError && typeof navigator !== 'undefined' && !navigator.onLine;
  const hasIntegration = (integrations.data ?? []).length > 0;

  return (
    <aside className="flex w-[300px] flex-shrink-0 flex-col border-r border-slate-200 bg-white">
      <div className="flex items-center justify-between border-b border-slate-200 px-4 py-3">
        <h2 className="text-sm font-semibold text-slate-900">Conversations</h2>
        {conversations.data !== undefined && (
          <span className="text-xs text-slate-500" aria-label={`${filtered.length} conversations`}>
            {filtered.length}
          </span>
        )}
      </div>
      <div className="border-b border-slate-200 px-3 py-2">
        <label htmlFor="conv-search" className="sr-only">
          Search conversations
        </label>
        <input
          id="conv-search"
          type="search"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Search conversations…"
          className="w-full rounded-xl border border-slate-200 bg-slate-50 px-3 py-1.5 text-sm placeholder:text-slate-400 focus:border-emerald-500 focus:bg-white focus:outline-none focus:ring-2 focus:ring-emerald-200"
        />
      </div>
      <div className="flex-1 overflow-y-auto">
        {conversations.isPending && (
          <div className="flex items-center justify-center py-10">
            <Spinner className="h-5 w-5 text-slate-500" label="Loading conversations" />
          </div>
        )}

        {conversations.isError && (
          <div role="alert" className="m-3 rounded-lg bg-rose-50 p-3 text-xs text-rose-700 ring-1 ring-inset ring-rose-200">
            {isPermDenied
              ? "You don't have permission to view conversations."
              : isOffline
                ? "You're offline. Reconnect to see conversations."
                : 'Could not load conversations.'}
            {!isPermDenied && (
              <button
                type="button"
                onClick={() => void conversations.refetch()}
                className="mt-2 block rounded-md bg-white px-2 py-1 text-xs font-medium text-rose-700 ring-1 ring-inset ring-rose-200 hover:bg-rose-100"
              >
                Retry
              </button>
            )}
          </div>
        )}

        {conversations.data !== undefined && filtered.length === 0 && query.length > 0 && (
          <EmptyState title="No matches" description={`Nothing matches “${query}”.`} />
        )}

        {conversations.data !== undefined && conversations.data.length === 0 && (
          <EmptyState
            title="No conversations yet"
            description={
              hasIntegration
                ? 'Once contacts message your WhatsApp number they will appear here.'
                : 'Connect WhatsApp to start receiving conversations.'
            }
            action={
              !hasIntegration ? (
                <Button variant="primary" onClick={() => void navigate({ to: '/settings/integrations' })}>
                  Go to integrations
                </Button>
              ) : undefined
            }
          />
        )}

        {conversations.data !== undefined && filtered.length > 0 && (
          <ul role="listbox" aria-label="Conversations" className="divide-y divide-slate-100">
            {filtered.map((c) => {
              const active = c.id === selectedID;
              return (
                <li key={c.id}>
                  <button
                    type="button"
                    role="option"
                    aria-selected={active}
                    onClick={() => selectConversation(c.id)}
                    className={
                      'flex w-full items-start gap-3 px-3 py-2.5 text-left transition ' +
                      (active ? 'bg-emerald-50' : 'hover:bg-slate-50')
                    }
                  >
                    <div
                      className={
                        'flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-full text-xs font-semibold ' +
                        (c.type === 'group'
                          ? 'bg-indigo-100 text-indigo-700'
                          : 'bg-slate-200 text-slate-700')
                      }
                    >
                      {initials(titleFor(c))}
                    </div>
                    <div className="min-w-0 flex-1">
                      <div className="flex items-baseline justify-between gap-2">
                        <p className={'truncate text-sm ' + (active ? 'font-semibold text-emerald-900' : 'font-medium text-slate-900')}>
                          {titleFor(c)}
                          {c.type === 'group' && (
                            <span
                              className="ml-2 inline-flex items-center rounded-full bg-indigo-100 px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-indigo-700"
                              aria-label="Group conversation"
                            >
                              Group
                            </span>
                          )}
                        </p>
                        <span className="flex-shrink-0 text-[11px] text-slate-500">{formatTime(c.last_message_at)}</span>
                      </div>
                      <p className="truncate text-xs text-slate-500">
                        {c.last_message_preview ?? 'No messages yet'}
                      </p>
                    </div>
                    {c.unread_count !== undefined && c.unread_count > 0 && (
                      <span
                        aria-label={`${c.unread_count} unread`}
                        className="ml-1 inline-flex h-5 min-w-5 items-center justify-center rounded-full bg-emerald-600 px-1.5 text-[11px] font-semibold text-white"
                      >
                        {c.unread_count}
                      </span>
                    )}
                  </button>
                </li>
              );
            })}
          </ul>
        )}
      </div>
    </aside>
  );
}
