import { useEffect, useMemo, useRef } from 'react';
import { Link } from '@tanstack/react-router';
import { useConversationMessages, useConversations, useMarkConversationRead } from '../../lib/messages';
import type { Message } from '../../lib/messages';
import { formatDuration } from '../../lib/calls';
import { Composer } from './Composer';
import { Spinner } from '../../components/Spinner';
import { ApiError } from '../../lib/api';
import { LocationBubble } from './renderers/LocationBubble';
import { ContactCardBubble } from './renderers/ContactCardBubble';
import { InteractiveBubble } from './renderers/InteractiveBubble';
import { TemplateBubble } from './renderers/TemplateBubble';
import { ReactionBadge } from './renderers/ReactionBadge';
import { UnknownBubble } from './renderers/UnknownBubble';
import { MediaBubble } from './renderers/MediaBubble';
import { TickIcon } from './renderers/TickIcon';

type Props = {
  conversationID: string | null;
  orgID: string;
};

function formatTime(iso: string): string {
  // Defensive: some server paths emit RFC3339 with Z, others emit naive
  // datetime strings that JS parses as LOCAL time — that would double-
  // shift the display. If the timezone marker is missing, append Z so
  // parsing is always UTC-first; toLocaleTimeString then converts to
  // the operator's actual local wall-clock time.
  const s = /Z|[+-]\d{2}:?\d{2}$/.test(iso) ? iso : `${iso}Z`;
  const d = new Date(s);
  if (Number.isNaN(d.getTime())) return '';
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', hour12: false });
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
 * callMessageLabel derives the human-readable label + tone for a
 * message of type "call" from its metadata bag. The backend stamps
 * { call_id, direction, status, duration_seconds } — we defensively
 * coerce each field.
 */
function callMessageLabel(md: Record<string, unknown> | undefined): {
  label: string;
  tone: 'muted' | 'bad';
} {
  const direction = typeof md?.['direction'] === 'string' ? (md['direction'] as string) : '';
  const status = typeof md?.['status'] === 'string' ? (md['status'] as string) : '';
  const duration =
    typeof md?.['duration_seconds'] === 'number' ? (md['duration_seconds'] as number) : 0;
  const outbound = direction === 'outbound';
  const missed = status === 'missed' || status === 'no_answer' || (outbound && status === 'declined');
  const failed = status === 'failed';
  const answered = status === 'answered' || status === 'completed' || status === 'in_progress';

  let label: string;
  if (failed) label = 'Call failed';
  else if (outbound)
    label = answered ? `You called · ${formatDuration(duration)}` : 'You called · no answer';
  else label = answered ? `Incoming call · ${formatDuration(duration)}` : 'Missed call';

  return { label, tone: failed || missed ? 'bad' : 'muted' };
}

/**
 * CallMessageRow renders a compact system-style row for a call-type
 * message in the thread. The row is a Link to /calls?id=<call_id> so
 * clicking jumps to the calls page with that call auto-selected.
 * Falls back to a non-interactive card if the metadata is missing a
 * call_id.
 */
function CallMessageRow({ msg }: { msg: Message }) {
  const md = msg.metadata;
  const callID = typeof md?.['call_id'] === 'string' ? (md['call_id'] as string) : '';
  const { label, tone } = callMessageLabel(md);
  const toneClasses =
    tone === 'bad'
      ? 'bg-rose-50 text-rose-700 ring-rose-100'
      : 'bg-slate-100 text-slate-700 ring-slate-200';
  const shortID = callID.length > 8 ? `${callID.slice(0, 8)}…` : callID;

  const inner = (
    <div
      className={`inline-flex flex-col items-center gap-0.5 rounded-2xl px-3 py-1.5 text-xs ring-1 ring-inset ${toneClasses}`}
    >
      <span className="inline-flex items-center gap-2">
        <svg
          aria-hidden="true"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.8"
          strokeLinecap="round"
          strokeLinejoin="round"
          className="h-3.5 w-3.5"
        >
          <path d="M22 16.92v3a2 2 0 0 1-2.18 2 19.79 19.79 0 0 1-8.63-3.07 19.5 19.5 0 0 1-6-6A19.79 19.79 0 0 1 2.12 4.18 2 2 0 0 1 4.11 2h3a2 2 0 0 1 2 1.72c.13.96.36 1.9.7 2.81a2 2 0 0 1-.45 2.11L8.09 9.91a16 16 0 0 0 6 6l1.27-1.27a2 2 0 0 1 2.11-.45c.91.34 1.85.57 2.81.7A2 2 0 0 1 22 16.92z" />
        </svg>
        <span>{label}</span>
      </span>
      {callID !== '' && (
        <span className="font-mono text-[10px] opacity-70">Call ID: {shortID}</span>
      )}
    </div>
  );

  return (
    <div className="flex justify-center py-1">
      {callID !== '' ? (
        <Link
          to="/calls"
          search={{ id: callID }}
          aria-label={`Open call ${label}`}
          className="no-underline hover:opacity-90"
        >
          {inner}
        </Link>
      ) : (
        inner
      )}
    </div>
  );
}

/**
 * InfoMessageRow renders a thin centered pill for a call-status info
 * message. One info row per status transition (ringing / answered /
 * completed / failed / missed / ...). Terminal statuses wrap in a Link
 * to /calls?id=<call_id> so operators can drill into call details. Non-
 * terminal rows render as plain pills with a tooltip.
 */
function InfoMessageRow({ msg }: { msg: Message }) {
  const md = msg.metadata;
  const label = typeof md?.['label'] === 'string' ? (md['label'] as string) : '';
  const callID = typeof md?.['call_id'] === 'string' ? (md['call_id'] as string) : '';
  const callStatus = typeof md?.['call_status'] === 'string' ? (md['call_status'] as string) : '';
  const terminal = md?.['terminal'] === true;

  let tone: string;
  switch (callStatus) {
    case 'failed':
    case 'missed':
    case 'declined':
    case 'no_answer':
      tone = 'bg-rose-50 text-rose-700 ring-rose-100';
      break;
    case 'answered':
      tone = 'bg-sky-50 text-sky-700 ring-sky-100';
      break;
    case 'completed':
    case 'ended':
    case 'recording_available':
      tone = 'bg-emerald-50 text-emerald-700 ring-emerald-100';
      break;
    default:
      tone = 'bg-slate-100 text-slate-600 ring-slate-200';
  }

  const shortID = callID.length > 8 ? `${callID.slice(0, 8)}…` : callID;
  const displayLabel = label !== '' ? label : callStatus !== '' ? callStatus : 'Call event';

  const inner = (
    <div
      className={`inline-flex flex-col items-center gap-0.5 rounded-full px-3 py-1 text-xs ring-1 ring-inset ${tone}`}
    >
      <span className="inline-flex items-center gap-2">
        <svg
          aria-hidden="true"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.8"
          strokeLinecap="round"
          strokeLinejoin="round"
          className="h-3 w-3"
        >
          <path d="M22 16.92v3a2 2 0 0 1-2.18 2 19.79 19.79 0 0 1-8.63-3.07 19.5 19.5 0 0 1-6-6A19.79 19.79 0 0 1 2.12 4.18 2 2 0 0 1 4.11 2h3a2 2 0 0 1 2 1.72c.13.96.36 1.9.7 2.81a2 2 0 0 1-.45 2.11L8.09 9.91a16 16 0 0 0 6 6l1.27-1.27a2 2 0 0 1 2.11-.45c.91.34 1.85.57 2.81.7A2 2 0 0 1 22 16.92z" />
        </svg>
        <span>{displayLabel}</span>
      </span>
      {callID !== '' && (
        <span className="font-mono text-[10px] opacity-60">Call ID: {shortID}</span>
      )}
    </div>
  );

  return (
    <div className="flex justify-center py-1">
      {terminal && callID !== '' ? (
        <Link
          to="/calls"
          search={{ id: callID }}
          aria-label={`Open call ${displayLabel}`}
          className="no-underline hover:opacity-90"
        >
          {inner}
        </Link>
      ) : (
        <span
          title={
            terminal
              ? undefined
              : 'Call still in progress — details available when the call ends.'
          }
        >
          {inner}
        </span>
      )}
    </div>
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
  // Call-status info events render as thin centered pills. The backend
  // emits one info message per call status transition (ringing / answered
  // / completed / failed / missed / ...). Terminal statuses become
  // clickable and deep-link to /calls?id=<call_id>.
  if (msg.type === 'info') {
    return <InfoMessageRow msg={msg} />;
  }
  // Legacy: pre-existing rows with the deprecated "call" type continue
  // to render via the old centered bubble.
  if (msg.type === 'call') {
    return <CallMessageRow msg={msg} />;
  }
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
    case 'template':
      body = <TemplateBubble msg={msg} footer={footer} />;
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
  const conversationsQ = useConversations();
  const current = useMemo(
    () => conversationsQ.data?.find((c) => c.id === conversationID) ?? null,
    [conversationsQ.data, conversationID],
  );
  const isGroup = current?.type === 'group';
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

  // The backend now stamps a Message of type "call" for every call event,
  // so we no longer merge a parallel useConversationCalls stream into the
  // thread — that would render duplicate bubbles. The single source of
  // truth is `messages`, and `BubbleDispatch` routes `type === 'call'`
  // rows through `CallMessageRow`.

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
            <p className="text-sm text-slate-500">No messages in this conversation yet.</p>
          </div>
        )}
        {grouped !== null &&
          grouped.bubbles.map((m) => (
            <BubbleDispatch
              key={`m-${m.id}`}
              msg={m}
              overlayReactions={
                m.provider_message_id !== undefined
                  ? (grouped.overlays.get(m.provider_message_id) ?? [])
                  : []
              }
            />
          ))}
        {/* Reactions whose target message isn't in the loaded window are
            deliberately not rendered — WhatsApp shows reactions only on
            the message they react to. Load the target via pagination if
            needed. */}
      </div>
      {isGroup ? (
        <div
          className="border-t border-slate-200 bg-white px-6 py-4 text-center text-xs text-slate-500"
          role="status"
        >
          Group messaging composer coming soon.
        </div>
      ) : (
        <Composer conversationID={conversationID} orgID={orgID} />
      )}
    </section>
  );
}
