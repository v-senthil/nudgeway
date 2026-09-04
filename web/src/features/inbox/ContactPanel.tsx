import { useMemo, useState } from 'react';
import { Link } from '@tanstack/react-router';
import { useConversationMessages, useConversations } from '../../lib/messages';
import { useConversationCalls, formatDuration, type Call } from '../../lib/calls';

type Props = {
  conversationID: string | null;
};

function initials(name: string | undefined): string {
  if (name === undefined || name.length === 0) return '?';
  const parts = name.trim().split(/\s+/);
  const first = parts[0];
  const second = parts[1];
  if (first !== undefined && second !== undefined)
    return `${first[0] ?? ''}${second[0] ?? ''}`.toUpperCase();
  if (first !== undefined) return first.slice(0, 2).toUpperCase();
  return '?';
}

// CopyButton renders a compact "Copy" chip that puts the given value on
// the clipboard. Silent no-op when the clipboard API is blocked (the
// value stays visible next to the button either way).
function CopyButton({ value }: { value: string }) {
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1200);
    } catch {
      // Clipboard blocked — no-op.
    }
  };
  return (
    <button
      type="button"
      onClick={() => void copy()}
      className="rounded border border-slate-200 bg-white px-1.5 py-0.5 text-[10px] font-medium text-slate-600 hover:bg-slate-100"
      title="Copy"
    >
      {copied ? 'Copied' : 'Copy'}
    </button>
  );
}

// MonoField renders a labelled row with a mono value + copy button. When
// the value is missing, the whole row is elided so the panel stays clean
// for freshly-created conversations that haven't been enriched yet.
function MonoField({ label, value }: { label: string; value: string | undefined }) {
  if (value === undefined || value === '') return null;
  return (
    <div className="flex items-start justify-between gap-3">
      <dt className="pt-0.5 text-[11px] font-medium uppercase tracking-wide text-slate-500">
        {label}
      </dt>
      <dd className="flex min-w-0 items-center gap-1.5">
        <span className="truncate font-mono text-xs text-slate-800">{value}</span>
        <CopyButton value={value} />
      </dd>
    </div>
  );
}

function directionArrow(direction: Call['direction']): string {
  return direction === 'inbound' ? '↙' : '↗';
}

function statusColor(status: Call['status']): string {
  switch (status) {
    case 'completed':
      return 'bg-emerald-50 text-emerald-700 ring-emerald-100';
    case 'answered':
    case 'in_progress':
      return 'bg-sky-50 text-sky-700 ring-sky-100';
    case 'ringing':
    case 'queued':
      return 'bg-amber-50 text-amber-700 ring-amber-100';
    case 'failed':
    case 'declined':
    case 'no_answer':
    case 'missed':
      return 'bg-rose-50 text-rose-700 ring-rose-100';
  }
}

export function ContactPanel({ conversationID }: Props) {
  const conversations = useConversations();
  const selected =
    conversationID === null ? undefined : conversations.data?.find((c) => c.id === conversationID);
  const messages = useConversationMessages(conversationID);
  const calls = useConversationCalls(conversationID);

  // BSUID fallback: when the conversation record doesn't surface a BSUID
  // directly, mine it from the most recent inbound message's from_user_id
  // (Meta stamps the wa_id / BSUID there on inbound webhooks).
  const derivedBSUID = useMemo(() => {
    if (messages.data === undefined) return undefined;
    // messages come newest-first from cursor pagination; walk in order.
    for (const m of messages.data) {
      if (m.direction === 'inbound' && m.from_user_id !== undefined && m.from_user_id !== '') {
        return m.from_user_id;
      }
    }
    return undefined;
  }, [messages.data]);

  return (
    <aside className="flex w-[320px] flex-shrink-0 flex-col border-l border-slate-200 bg-white">
      <div className="border-b border-slate-200 px-4 py-3">
        <h2 className="text-sm font-semibold text-slate-900">Contact</h2>
      </div>
      {selected === undefined ? (
        <div className="flex flex-1 items-center justify-center px-6">
          <p className="text-center text-sm text-slate-500">
            Select a conversation to see contact details.
          </p>
        </div>
      ) : (
        <div className="flex flex-1 flex-col gap-4 overflow-y-auto px-4 py-4">
          <div className="flex flex-col items-center gap-2 text-center">
            <div className="flex h-16 w-16 items-center justify-center rounded-full bg-slate-200 text-lg font-semibold text-slate-700">
              {initials(selected.contact_name)}
            </div>
            <p className="text-base font-semibold text-slate-900">
              {selected.contact_name ?? selected.subject ?? 'Unknown contact'}
            </p>
            <p className="text-xs uppercase tracking-wide text-slate-500">{selected.channel}</p>
          </div>

          <dl className="space-y-2">
            <MonoField label="Conversation ID" value={selected.id} />
            <MonoField label="Integration" value={selected.channel} />
            <MonoField label="Contact ID" value={selected.contact_id} />
            <MonoField label="BSUID" value={derivedBSUID} />
            <MonoField label="Phone" value={undefined /* conversation record has no phone yet */} />
          </dl>

          <section className="mt-2 border-t border-slate-100 pt-3">
            <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-slate-500">
              Calls in this conversation
            </h3>
            {calls.isPending ? (
              <p className="text-xs text-slate-400">Loading calls…</p>
            ) : calls.data === undefined || calls.data.items.length === 0 ? (
              <p className="text-xs text-slate-400">No calls in this conversation.</p>
            ) : (
              <ul className="space-y-1.5">
                {calls.data.items.map((c) => (
                  <li key={c.id}>
                    <Link
                      to="/calls"
                      search={{ id: c.id }}
                      className="flex items-center justify-between gap-2 rounded-md border border-slate-200 bg-white px-2 py-1.5 text-xs hover:bg-slate-50"
                    >
                      <span className="flex min-w-0 items-center gap-2">
                        <span
                          aria-label={
                            c.direction === 'inbound' ? 'inbound call' : 'outbound call'
                          }
                          className="text-slate-400"
                        >
                          {directionArrow(c.direction)}
                        </span>
                        <span
                          className={`inline-flex items-center rounded px-1.5 py-0.5 text-[10px] font-medium ring-1 ring-inset ${statusColor(
                            c.status,
                          )}`}
                        >
                          {c.status}
                        </span>
                      </span>
                      <span className="whitespace-nowrap font-mono text-[11px] text-slate-500">
                        {formatDuration(c.duration_seconds)}
                      </span>
                    </Link>
                  </li>
                ))}
              </ul>
            )}
          </section>
        </div>
      )}
    </aside>
  );
}
