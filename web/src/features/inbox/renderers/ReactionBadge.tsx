import type { Message } from '../../../lib/messages';

type Props = {
  msg: Message;
  /**
   * When true, render as a standalone bubble (for reactions whose target
   * is outside the loaded window). When false (default), render as an
   * absolute-positioned overlay chip meant to sit inside a target bubble
   * wrapper (which must be `relative`).
   */
  asBubble?: boolean;
};

/**
 * ReactionBadge renders an emoji reaction. Two rendering modes:
 *
 *   - Overlay (default): a small chip absolutely positioned at the
 *     bottom-right of the target bubble. The Thread groups reactions by
 *     their `reply_to_provider_message_id` and stacks these inside the
 *     wrapper of the reacted-to bubble.
 *   - Fallback bubble (`asBubble`): a standalone "Reacted <emoji>" bubble
 *     shown when the target message is outside the loaded window and we
 *     have nothing to overlay on.
 */
export function ReactionBadge({ msg, asBubble }: Props) {
  const emoji = msg.reaction?.emoji ?? '';
  if (asBubble === true) {
    return (
      <div className="flex justify-start">
        <div className="max-w-[70%] rounded-2xl bg-white px-3 py-2 text-xs shadow-sm ring-1 ring-slate-200">
          <span className="opacity-70">Reacted </span>
          <span aria-label={`reaction ${emoji}`}>{emoji || '—'}</span>
        </div>
      </div>
    );
  }
  if (emoji === '') return null;
  return (
    <span
      aria-label={`reaction ${emoji}`}
      className="absolute -bottom-2 right-2 z-10 rounded-full bg-white px-1.5 py-0.5 text-xs shadow ring-1 ring-slate-200"
    >
      {emoji}
    </span>
  );
}
