import type { Message } from '../../../lib/messages';

type Props = {
  msg: Message;
  footer?: React.ReactNode;
};

/**
 * UnknownBubble is the catch-all renderer for message types we don't
 * yet have a specialised bubble for. The provider adapter preserved the
 * raw payload upstream — we simply tell the operator that.
 */
export function UnknownBubble({ msg, footer }: Props) {
  return (
    <div className="flex flex-col gap-1">
      <p className="font-medium">Unsupported message type</p>
      <p className="text-[11px] opacity-70">
        {`type=${msg.type}. Provider preserved raw payload.`}
      </p>
      {footer}
    </div>
  );
}
