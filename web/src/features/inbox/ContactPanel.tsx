import { useConversations } from '../../lib/messages';

type Props = {
  conversationID: string | null;
};

function initials(name: string | undefined): string {
  if (name === undefined || name.length === 0) return '?';
  const parts = name.trim().split(/\s+/);
  const first = parts[0];
  const second = parts[1];
  if (first !== undefined && second !== undefined) return `${first[0] ?? ''}${second[0] ?? ''}`.toUpperCase();
  if (first !== undefined) return first.slice(0, 2).toUpperCase();
  return '?';
}

export function ContactPanel({ conversationID }: Props) {
  const conversations = useConversations();
  const selected = conversationID === null ? undefined : conversations.data?.find((c) => c.id === conversationID);

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
            <p className="text-base font-semibold text-slate-900">{selected.contact_name ?? 'Unknown contact'}</p>
            <p className="text-xs uppercase tracking-wide text-slate-500">{selected.channel}</p>
          </div>
          <dl className="space-y-2 text-sm">
            <div className="flex justify-between gap-3">
              <dt className="text-slate-500">Status</dt>
              <dd className="font-medium text-slate-800 capitalize">{selected.status}</dd>
            </div>
            {selected.last_message_at !== undefined && (
              <div className="flex justify-between gap-3">
                <dt className="text-slate-500">Last message</dt>
                <dd className="font-medium text-slate-800">
                  {new Date(selected.last_message_at).toLocaleString()}
                </dd>
              </div>
            )}
          </dl>
          <p className="mt-auto text-[11px] text-slate-400">
            Contact enrichment, tickets and notes ship in a later Phase 1 task.
          </p>
        </div>
      )}
    </aside>
  );
}
