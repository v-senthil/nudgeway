import type { Message } from '../../../lib/messages';

type Props = {
  msg: Message;
  footer?: React.ReactNode;
};

/**
 * InteractiveBubble renders the customer's reply to an interactive
 * prompt: a list selection, a reply button tap, a template quick-reply
 * button, or a call permission (accept / reject) response.
 */
export function InteractiveBubble({ msg, footer }: Props) {
  const it = msg.interactive;
  if (it === undefined) {
    return <p className="italic opacity-70">[interactive payload missing]</p>;
  }
  if (it.kind === 'call_permission_reply') {
    return <CallPermissionReplyBody it={it} footer={footer} />;
  }
  const label = labelFor(it.kind, msg.type);
  const title = it.title ?? it.id ?? '(no title)';
  return (
    <div className="flex flex-col gap-1">
      <p className="text-[11px] uppercase tracking-wide opacity-70">{label}</p>
      <p className="font-semibold break-words">{title}</p>
      {it.description !== undefined && it.description !== '' && (
        <p className="text-xs opacity-80 break-words">{it.description}</p>
      )}
      {footer}
    </div>
  );
}

/**
 * CallPermissionReplyBody renders the accept/decline result of a WhatsApp
 * call permission prompt as a tinted pill with an optional caption for
 * response source + expiry.
 */
function CallPermissionReplyBody({
  it,
  footer,
}: {
  it: NonNullable<Message['interactive']>;
  footer?: React.ReactNode;
}) {
  const response = (it.response ?? '').toLowerCase();
  const isAccept = response === 'accept';
  const isReject =
    response === 'reject' || response === 'decline' || response === 'declined';
  const isPermanent = it.is_permanent === true;

  let title: string;
  let subtitle = '';
  let tone: string;
  if (isAccept) {
    if (isPermanent) {
      title = '✅ Permission granted';
      subtitle = 'Permanent — the customer allows calls anytime.';
    } else if (it.expiration_timestamp !== undefined && it.expiration_timestamp > 0) {
      const d = new Date(it.expiration_timestamp * 1000);
      const when = Number.isNaN(d.getTime()) ? '' : d.toLocaleString();
      title = '✅ Permission granted';
      subtitle = when !== '' ? `Temporary — expires ${when}.` : 'Temporary permission.';
    } else {
      title = '✅ Permission granted';
      subtitle = 'Temporary permission.';
    }
    tone = 'text-emerald-700';
  } else if (isReject) {
    title = '⛔ Permission declined';
    subtitle = 'The customer chose not to allow calls.';
    tone = 'text-rose-700';
  } else {
    title = 'Call permission reply';
    tone = 'text-slate-700';
  }

  return (
    <div className="flex flex-col gap-1">
      <p className="text-[11px] uppercase tracking-wide opacity-70">Call permission</p>
      <p className={`font-semibold break-words ${tone}`}>{title}</p>
      {subtitle !== '' && (
        <p className="text-xs opacity-80 break-words">{subtitle}</p>
      )}
      {it.response_source !== undefined && it.response_source !== '' && (
        <p className="text-[11px] opacity-60 break-words">via {it.response_source}</p>
      )}
      {footer}
    </div>
  );
}

function labelFor(kind: string, type: Message['type']): string {
  if (type === 'button') return 'Replied via button:';
  switch (kind) {
    case 'button_reply':
      return 'Replied via button:';
    case 'list_reply':
      return 'Replied via list:';
    case 'nfm_reply':
      return 'Flow response:';
    default:
      return `Replied via ${kind || 'interactive'}:`;
  }
}
