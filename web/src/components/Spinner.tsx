type Props = { className?: string; label?: string };

/** Spinner is a clean two-arc circular indicator — a faint background ring
 * plus a bright rotating arc. Uses currentColor so the caller's text-color
 * controls the tint. */
export function Spinner({ className = 'h-4 w-4', label = 'Loading' }: Props) {
  return (
    <svg
      className={`animate-spin ${className}`}
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 24 24"
      fill="none"
      role="status"
      aria-label={label}
    >
      <circle
        cx="12"
        cy="12"
        r="9"
        stroke="currentColor"
        strokeWidth="2.5"
        strokeOpacity="0.18"
      />
      <path
        d="M21 12a9 9 0 0 1-9 9"
        stroke="currentColor"
        strokeWidth="2.5"
        strokeLinecap="round"
      />
    </svg>
  );
}
