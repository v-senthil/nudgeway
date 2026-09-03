import type { Message } from '../../../lib/messages';

type Props = {
  msg: Message;
  footer?: React.ReactNode;
};

/**
 * InteractiveBubble renders the customer's reply to an interactive
 * prompt: a list selection, a reply button tap, or a template
 * quick-reply button. Shows a muted label + the chosen title (bold).
 */
export function InteractiveBubble({ msg, footer }: Props) {
  const it = msg.interactive;
  if (it === undefined) {
    return <p className="italic opacity-70">[interactive payload missing]</p>;
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
