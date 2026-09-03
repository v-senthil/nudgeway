import type { MessageStatus } from '../../../lib/messages';

type Props = {
  status: MessageStatus;
  className?: string;
};

/**
 * TickIcon renders a WhatsApp-style status indicator for an outbound
 * message.
 *
 * - queued / sending → three grey dots (message still in the local queue)
 * - sent            → single grey check (accepted by the provider)
 * - delivered       → double grey check (delivered to the customer)
 * - read            → double blue check (customer opened the thread)
 * - failed          → red exclamation (send permanently failed)
 *
 * SVGs use `currentColor` for the stroke so callers can override the tint
 * with a Tailwind class.
 */
export function TickIcon({ status, className }: Props) {
  const label = `status: ${status}`;
  const base = 'inline-block h-3.5 w-[18px] align-middle';
  const cls = className === undefined ? base : `${base} ${className}`;

  switch (status) {
    case 'queued':
    case 'sending':
      return (
        <svg
          aria-label={label}
          role="img"
          viewBox="0 0 18 14"
          className={`${cls} text-slate-300`}
          fill="none"
          xmlns="http://www.w3.org/2000/svg"
        >
          <circle cx="4" cy="7" r="1.4" fill="currentColor" />
          <circle cx="9" cy="7" r="1.4" fill="currentColor" />
          <circle cx="14" cy="7" r="1.4" fill="currentColor" />
        </svg>
      );
    case 'sent':
      return (
        <svg
          aria-label={label}
          role="img"
          viewBox="0 0 18 14"
          className={`${cls} text-slate-300`}
          fill="none"
          xmlns="http://www.w3.org/2000/svg"
        >
          <path
            d="M3 8l3.2 3L14 4"
            stroke="currentColor"
            strokeWidth="1.8"
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </svg>
      );
    case 'delivered':
      return (
        <svg
          aria-label={label}
          role="img"
          viewBox="0 0 18 14"
          className={`${cls} text-slate-300`}
          fill="none"
          xmlns="http://www.w3.org/2000/svg"
        >
          <path
            d="M1 8l3.2 3L11.5 4"
            stroke="currentColor"
            strokeWidth="1.8"
            strokeLinecap="round"
            strokeLinejoin="round"
          />
          <path
            d="M6 8l3.2 3L17 4"
            stroke="currentColor"
            strokeWidth="1.8"
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </svg>
      );
    case 'read':
      return (
        <svg
          aria-label={label}
          role="img"
          viewBox="0 0 18 14"
          className={`${cls} text-sky-400`}
          fill="none"
          xmlns="http://www.w3.org/2000/svg"
        >
          <path
            d="M1 8l3.2 3L11.5 4"
            stroke="currentColor"
            strokeWidth="1.8"
            strokeLinecap="round"
            strokeLinejoin="round"
          />
          <path
            d="M6 8l3.2 3L17 4"
            stroke="currentColor"
            strokeWidth="1.8"
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </svg>
      );
    case 'failed':
      return (
        <svg
          aria-label={label}
          role="img"
          viewBox="0 0 18 14"
          className={`${cls} text-rose-400`}
          fill="none"
          xmlns="http://www.w3.org/2000/svg"
        >
          <circle cx="9" cy="7" r="6" stroke="currentColor" strokeWidth="1.5" />
          <path d="M9 4v3.5" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
          <circle cx="9" cy="10" r="0.9" fill="currentColor" />
        </svg>
      );
  }
}
