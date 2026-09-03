import type { Message } from '../../../lib/messages';

type Props = {
  msg: Message;
  footer?: React.ReactNode;
};

/**
 * ContactCardBubble renders one or more shared vCards. Each card shows
 * the name (bold), phones as `tel:` links, and emails as `mailto:` links.
 */
export function ContactCardBubble({ msg, footer }: Props) {
  const contacts = msg.contacts;
  if (contacts === undefined || contacts.length === 0) {
    return <p className="italic opacity-70">[contacts payload missing]</p>;
  }
  return (
    <div className="flex flex-col gap-2">
      <p className="text-[11px] uppercase tracking-wide opacity-70">Contact card</p>
      {contacts.map((c, i) => (
        <div key={i} className="flex flex-col gap-0.5">
          <p className="font-semibold">{c.name !== '' ? c.name : 'Unnamed contact'}</p>
          {c.phones !== undefined &&
            c.phones.map((p, j) => (
              <a
                key={`p-${j}`}
                href={`tel:${p}`}
                className="text-xs underline break-all"
              >
                {p}
              </a>
            ))}
          {c.emails !== undefined &&
            c.emails.map((e, j) => (
              <a
                key={`e-${j}`}
                href={`mailto:${e}`}
                className="text-xs underline break-all"
              >
                {e}
              </a>
            ))}
        </div>
      ))}
      {footer}
    </div>
  );
}
