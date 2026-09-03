import { useEffect, useRef } from 'react';
import type { ReactNode } from 'react';

type Props = {
  open: boolean;
  onClose: () => void;
  title: string;
  children: ReactNode;
  footer?: ReactNode;
};

export function Modal({ open, onClose, title, children, footer }: Props) {
  const dialogRef = useRef<HTMLDivElement>(null);
  const previouslyFocused = useRef<Element | null>(null);

  useEffect(() => {
    if (!open) return;
    previouslyFocused.current = document.activeElement;

    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.stopPropagation();
        onClose();
      }
      if (e.key === 'Tab' && dialogRef.current !== null) {
        const focusables = dialogRef.current.querySelectorAll<HTMLElement>(
          'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])',
        );
        if (focusables.length === 0) return;
        const first = focusables[0];
        const last = focusables[focusables.length - 1];
        if (first === undefined || last === undefined) return;
        if (e.shiftKey && document.activeElement === first) {
          e.preventDefault();
          last.focus();
        } else if (!e.shiftKey && document.activeElement === last) {
          e.preventDefault();
          first.focus();
        }
      }
    };

    document.addEventListener('keydown', onKey);
    // focus first focusable
    const timer = window.setTimeout(() => {
      const el = dialogRef.current?.querySelector<HTMLElement>(
        'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])',
      );
      el?.focus();
    }, 0);

    return () => {
      document.removeEventListener('keydown', onKey);
      window.clearTimeout(timer);
      if (previouslyFocused.current instanceof HTMLElement) {
        previouslyFocused.current.focus();
      }
    };
  }, [open, onClose]);

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/40 p-4"
      onClick={onClose}
      role="presentation"
    >
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby="modal-title"
        className="w-full max-w-md rounded-xl border border-slate-200 bg-white p-6 shadow-lg"
        onClick={(e) => e.stopPropagation()}
      >
        <h2 id="modal-title" className="text-lg font-semibold text-slate-900">
          {title}
        </h2>
        <div className="mt-3 text-sm text-slate-600">{children}</div>
        <div className="mt-6 flex justify-end gap-2">
          {footer ?? (
            <button
              type="button"
              onClick={onClose}
              className="rounded-xl border border-slate-200 bg-white px-4 py-2 text-sm font-medium text-slate-700 shadow-sm hover:bg-slate-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-emerald-500 focus-visible:ring-offset-2"
            >
              Close
            </button>
          )}
        </div>
      </div>
    </div>
  );
}
