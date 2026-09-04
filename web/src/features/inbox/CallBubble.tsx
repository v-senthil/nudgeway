import type { Call } from '../../lib/calls';
import { formatDuration } from '../../lib/calls';

/**
 * CallBubble renders a compact system-style row for a call event inside a
 * conversation thread. Not a chat bubble — centered, muted, WhatsApp-style
 * "missed call" / "You called" pill.
 */
export function CallBubble({ call }: { call: Call }) {
  const outbound = call.direction === 'outbound';
  const missed =
    call.status === 'missed' ||
    call.status === 'no_answer' ||
    (outbound && call.status === 'declined');
  const failed = call.status === 'failed';
  const answered =
    call.status === 'answered' ||
    call.status === 'completed' ||
    call.status === 'in_progress';

  let label: string;
  if (failed) {
    label = 'Call failed';
  } else if (outbound) {
    label = answered
      ? `You called · ${formatDuration(call.duration_seconds)}`
      : 'You called · no answer';
  } else {
    label = answered
      ? `Incoming call · ${formatDuration(call.duration_seconds)}`
      : 'Missed call';
  }

  const tone =
    failed || missed
      ? 'bg-rose-50 text-rose-700 ring-rose-100'
      : 'bg-slate-100 text-slate-700 ring-slate-200';

  const when = call.created_at;
  const timeStr = ((): string => {
    const d = new Date(when);
    if (Number.isNaN(d.getTime())) return '';
    return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  })();

  return (
    <div className="flex justify-center py-1">
      <div
        className={`inline-flex items-center gap-2 rounded-full px-3 py-1 text-xs ring-1 ring-inset ${tone}`}
        aria-label={label}
      >
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
        {timeStr !== '' && <span className="text-[10px] opacity-70">{timeStr}</span>}
      </div>
    </div>
  );
}
