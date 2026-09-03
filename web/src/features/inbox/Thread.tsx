import { useEffect, useMemo, useRef } from 'react';
import { useConversationMessages, useMarkConversationRead } from '../../lib/messages';
import type { Message } from '../../lib/messages';
import { Composer } from './Composer';
import { Spinner } from '../../components/Spinner';
import { ApiError } from '../../lib/api';
import { LocationBubble } from './renderers/LocationBubble';
import { ContactCardBubble } from './renderers/ContactCardBubble';
import { InteractiveBubble } from './renderers/InteractiveBubble';
import { ReactionBadge } from './renderers/ReactionBadge';
import { UnknownBubble } from './renderers/UnknownBubble';
import { MediaBubble } from './renderers/MediaBubble';
import { TickIcon } from './renderers/TickIcon';

type Props = {
  conversationID: string | null;
  orgID: string;
};

function formatTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}

/**
 * BubbleFooter renders the trailing time + status-tick strip shared by
 * every non-media bubble variant. Kept small + reusable so specialised
 * renderers can drop it in verbatim.
 */
function BubbleFooter({ msg, isOut }: { msg: Message; isOut: boolean }) {
  const isFailed = msg.status === 'failed';
  const isRead = msg.status === 'read';
  return (
    <div
      className={
        'mt-1 flex items-center justify-end gap-1 text-[10px] ' +
        (isOut ? 'text-emerald-100' : 'text-slate-400')
      }
    >
      <time dateTime={msg.created_at}>{formatTime(msg.created_at)}</time>
      {isOut && (
        <TickIcon
          status={msg.status}
          className={isFailed ? 'text-rose-200' : isRead ? 'text-sky-200' : ''}
        />
      )}
    </div>
  );
}

/**
 * TextBubble renders a plain-text WhatsApp message with the standard
 * timestamp + status-tick footer.
 */
function TextBubble({ msg, isOut }: { msg: Message; isOut: boolean }) {
  return (
    <>
      <p className="whitespace-pre-wrap break-words">{msg.text ?? ''}</p>
      <BubbleFooter msg={msg} isOut={isOut} />
    </>
  );
}

/**
 * bubbleContainerClass returns the tailwind-class string for the outer
 * wrapper shared by all bubble variants. Split out so the dispatcher can
 * apply the correct outbound / inbound styling in one place.
 */
function bubbleContainerClass(isOut: boolean): string {
  return (
    'relative max-w-[70%] rounded-2xl px-3 py-2 text-sm shadow-sm ' +
    (isOut
      ? 'rounded-br-md bg-emerald-600 text-white'
      : 'rounded-bl-md bg-white text-slate-900 ring-1 ring-slate-200')
  );
}

/**
 * BubbleDispatch is the render-tree entry point for a single message.
 * It type-switches on `msg.type` and hands off to the specialised
 * renderer, wrapping the result in the shared bubble container.
 *
 * Reactions are handled by the caller (Thread) and are NOT rendered as
 * their own bubbles — they overlay onto the referenced message.
 */
function BubbleDispatch({
  msg,
  overlayReactions,
}: {
  msg: Message;
  overlayReactions: Message[];
}) {
  const isOut = msg.direction === 'outbound';
  const footer = <BubbleFooter msg={msg} isOut={isOut} />;

  let body: React.ReactNode;
  switch (msg.type) {
    case 'text':
      body = <TextBubble msg={msg} isOut={isOut} />;
      break;
    case 'image':
    case 'video':
    case 'audio':
    case 'document':
    case 'sticker':
      // MediaBubble (Agent A) renders its own footer/caption; we still
      // stamp the status tick + timestamp inside it via the footer slot.
      body = <MediaBubble msg={msg} footer={footer} />;
      break;
    case 'location':
      body = <LocationBubble msg={msg} footer={footer} />;
      break;
    case 'contacts':
      body = <ContactCardBubble msg={msg} footer={footer} />;
      break;
    case 'interactive':
    case 'button':
      body = <InteractiveBubble msg={msg} footer={footer} />;
      break;
    case 'reaction':
      // Should never hit — Thread pre-filters reactions. Fall through
      // to the standalone badge just in case.
      return <ReactionBadge msg={msg} asBubble />;
    default:
      body = <UnknownBubble msg={msg} footer={footer} />;
  }

  return (
    <div className={'flex ' + (isOut ? 'justify-end' : 'justify-start')}>
      <div className={bubbleContainerClass(isOut)}>
        {body}
        {overlayReactions.map((r) => (
          <ReactionBadge key={r.id} msg={r} />
        ))}
      </div>
    </div>
  );
}

/**
 * groupReactions splits the loaded message window into non-reaction
 * bubbles and a map from `provider_message_id` → reactions that target
 * it. Reactions whose target is NOT in the loaded window are returned in
 * the third slot so the caller can render them as fallback bubbles.
 */
function groupReactions(msgs: Message[]): {
  bubbles: Message[];
  overlays: Map<string, Message[]>;
  fallback: Message[];
} {
  const bubbles: Message[] = [];
  const reactions: Message[] = [];
  const providerIds = new Set<string>();
  for (const m of msgs) {
    if (m.type === 'reaction') {
      reactions.push(m);
    } else {
      bubbles.push(m);
      if (m.provider_message_id !== undefined && m.provider_message_id !== '') {
        providerIds.add(m.provider_message_id);
      }
    }
  }
  const overlays = new Map<string, Message[]>();
  const fallback: Message[] = [];
  for (const r of reactions) {
    const target = r.reaction?.message_id ?? r.reply_to_provider_message_id;
    if (target !== undefined && target !== '' && providerIds.has(target)) {
      const bucket = overlays.get(target) ?? [];
      bucket.push(r);
      overlays.set(target, bucket);
    } else {
      fallback.push(r);
    }
  }
  return { bubbles, overlays, fallback };
}

export function Thread({ conversationID, orgID }: Props) {
  const messages = useConversationMessages(conversationID);
  const scrollRef = useRef<HTMLDivElement>(null);
  const markConvRead = useMarkConversationRead();
  const lastMarkedRef = useRef<{ id: string; at: number } | null>(null);

  // Auto-fire mark-as-read when the thread mounts or the conversation
  // changes AND there is at least one unread inbound message. Throttled to
  // once per 5s per conversation so quick tab-switches do not spam Meta.
  useEffect(() => {
    if (conversationID === null || messages.data === undefined) return;
    const hasUnread = messages.data.some((m) => m.direction === 'inbound' && m.status !== 'read');
    if (!hasUnread) return;
    const now = Date.now();
    const last = lastMarkedRef.current;
    if (last !== null && last.id === conversationID && now - last.at < 5000) return;
    lastMarkedRef.current = { id: conversationID, at: now };
    markConvRead.mutate({ conversationID });
  }, [conversationID, messages.data, markConvRead]);

  useEffect(() => {
    if (scrollRef.current === null) return;
    scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
  }, [messages.data]);

  // Backend returns newest-first for cursor pagination; render oldest-first.
  const grouped = useMemo(() => {
    if (messages.data === undefined) return null;
    return groupReactions([...messages.data].reverse());
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
        {grouped !== null &&
          grouped.bubbles.map((m) => (
            <BubbleDispatch
              key={m.id}
              msg={m}
              overlayReactions={
                m.provider_message_id !== undefined
                  ? (grouped.overlays.get(m.provider_message_id) ?? [])
                  : []
              }
            />
          ))}
        {grouped !== null &&
          grouped.fallback.map((r) => <ReactionBadge key={r.id} msg={r} asBubble />)}
      </div>
      <Composer conversationID={conversationID} orgID={orgID} />
    </section>
  );
}
