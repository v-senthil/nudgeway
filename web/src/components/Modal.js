import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { useEffect, useRef } from 'react';
export function Modal({ open, onClose, title, children, footer }) {
    const dialogRef = useRef(null);
    const previouslyFocused = useRef(null);
    useEffect(() => {
        if (!open)
            return;
        previouslyFocused.current = document.activeElement;
        const onKey = (e) => {
            if (e.key === 'Escape') {
                e.stopPropagation();
                onClose();
            }
            if (e.key === 'Tab' && dialogRef.current !== null) {
                const focusables = dialogRef.current.querySelectorAll('button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])');
                if (focusables.length === 0)
                    return;
                const first = focusables[0];
                const last = focusables[focusables.length - 1];
                if (first === undefined || last === undefined)
                    return;
                if (e.shiftKey && document.activeElement === first) {
                    e.preventDefault();
                    last.focus();
                }
                else if (!e.shiftKey && document.activeElement === last) {
                    e.preventDefault();
                    first.focus();
                }
            }
        };
        document.addEventListener('keydown', onKey);
        // focus first focusable
        const timer = window.setTimeout(() => {
            const el = dialogRef.current?.querySelector('button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])');
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
    if (!open)
        return null;
    return (_jsx("div", { className: "fixed inset-0 z-50 flex items-center justify-center bg-slate-900/40 p-4", onClick: onClose, role: "presentation", children: _jsxs("div", { ref: dialogRef, role: "dialog", "aria-modal": "true", "aria-labelledby": "modal-title", className: "w-full max-w-md rounded-xl border border-slate-200 bg-white p-6 shadow-lg", onClick: (e) => e.stopPropagation(), children: [_jsx("h2", { id: "modal-title", className: "text-lg font-semibold text-slate-900", children: title }), _jsx("div", { className: "mt-3 text-sm text-slate-600", children: children }), _jsx("div", { className: "mt-6 flex justify-end gap-2", children: footer ?? (_jsx("button", { type: "button", onClick: onClose, className: "rounded-xl border border-slate-200 bg-white px-4 py-2 text-sm font-medium text-slate-700 shadow-sm hover:bg-slate-50 focus:outline-none focus-visible:ring-2 focus-visible:ring-emerald-500 focus-visible:ring-offset-2", children: "Close" })) })] }) }));
}
