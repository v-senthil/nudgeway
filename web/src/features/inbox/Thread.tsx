import { useEffect, useRef } from 'react';
import { useConversationMessages } from '../../lib/messages';
import type { Message } from '../../lib/messages';
import { Composer } from './Composer';
import { Spinner } from '../../components/Spinner';
import { ApiError } from '../../lib/api';

type Props = {
  conversationID: string | null;
  orgID: string;
};

function formatTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}

function statusIcon(status: Message['status']): string {
  switch (status) {
    case 'sending':
      return '…';
    case 'sent':
      return '✓';
    case 'delivered':
      return '✓✓';
    case 'read':
      return '✓✓';
    case 'failed':
      return '!';
    case 'queued':
      return '·';
  }
}

function Bubble({ msg }: { msg: Message }) {
  const isOut = msg.direction === 'outbound';
  const isRead = msg.status === 'read';
  const isFailed = msg.status === 'failed';
  return (
    <div className={'flex ' + (isOut ? 'justify-end' : 'justify-start')}>
      <div
        className={
          'max-w-[70%] rounded-2xl px-3 py-2 text-sm shadow-sm ' +
          (isOut
            ? 'rounded-br-md bg-emerald-600 text-white'
            : 'rounded-bl-md bg-white text-slate-900 ring-1 ring-slate-200')
        }
      >
        {msg.type === 'text' && <p className="whitespace-pre-wrap break-words">{msg.text ?? ''}</p>}
        {msg.type !== 'text' && (
          <p className="italic opacity-80">
            [{msg.type}] {msg.media_caption ?? 'media message'}
          </p>
        )}
        <div className={'mt-1 flex items-center justify-end gap-1 text-[10px] ' + (isOut ? 'text-emerald-100' : 'text-slate-400')}>
          <time dateTime={msg.created_at}>{formatTime(msg.created_at)}</time>
          {isOut && (
            <span
              aria-label={`status: ${msg.status}`}
              className={
                isFailed
                  ? 'text-rose-200'
                  : isRead
                    ? 'text-sky-200'
                    : ''
              }
            >
              {statusIcon(msg.status)}
            </span>
          )}
        </div>
      </div>
    </div>
  );
}

export function Thread({ conversationID, orgID }: Props) {
  const messages = useConversationMessages(conversationID);
  const scrollRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (scrollRef.current === null) return;
    scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
  }, [messages.data]);

  if (conversationID === null) {
    return (
      <section className="flex flex-1 flex-col bg-slate-50">
        <div className="flex flex-1 items-center justify-center">
          <p className="text-sm text-slate-500">Select a conversation to view messages.</p>
        </div>
      </section>
    );
  }

  const isPermDenied = messages.error instanceof ApiError && messages.error.status === 403;
  const isNotFound = messages.error instanceof ApiError && messages.error.status === 404;
  const isOffline = messages.isError && typeof navigator !== 'undefined' && !navigator.onLine;

  return (
    <section className="flex flex-1 flex-col bg-slate-50">
      <div ref={scrollRef} className="flex-1 space-y-2 overflow-y-auto px-6 py-4">
        {messages.isPending && (
          <div className="flex items-center justify-center py-10">
            <Spinner className="h-5 w-5 text-slate-500" label="Loading messages" />
          </div>
        )}
        {messages.isError && (
          <div role="alert" className="rounded-xl border border-rose-200 bg-rose-50 p-3 text-sm text-rose-800">
            {isPermDenied
              ? "You don't have permission to view this conversation."
              : isNotFound
                ? 'Conversation not found.'
                : isOffline
                  ? "You're offline."
                  : 'Could not load messages.'}
            {!isPermDenied && !isNotFound && (
              <button
                type="button"
                onClick={() => void messages.refetch()}
                className="ml-2 rounded-md bg-white px-2 py-0.5 text-xs font-medium text-rose-700 ring-1 ring-inset ring-rose-200 hover:bg-rose-100"
              >
                Retry
              </button>
            )}
          </div>
        )}
        {messages.data !== undefined && messages.data.length === 0 && (
          <div className="flex items-center justify-center py-10">
            <p className="text-sm text-slate-500">No messages in this conversation yet. Say hello 👋</p>
          </div>
        )}
        {messages.data !== undefined && messages.data.map((m) => <Bubble key={m.id} msg={m} />)}
      </div>
      <Composer conversationID={conversationID} orgID={orgID} />
    </section>
  );
}
