import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { forwardRef, useId } from 'react';
export const Input = forwardRef(function Input({ label, error, hint, id, className = '', ...rest }, ref) {
    const generatedId = useId();
    const inputId = id ?? generatedId;
    const describedBy = error !== undefined ? `${inputId}-error` : hint !== undefined ? `${inputId}-hint` : undefined;
    const invalid = error !== undefined && error.length > 0;
    return (_jsxs("div", { className: "flex flex-col gap-1.5", children: [_jsx("label", { htmlFor: inputId, className: "text-sm font-medium text-slate-700", children: label }), _jsx("input", { ref: ref, id: inputId, "aria-invalid": invalid || undefined, "aria-describedby": describedBy, className: 'w-full rounded-xl border bg-white px-3 py-2 text-sm text-slate-900 placeholder:text-slate-400 ' +
                    'focus:outline-none focus:ring-2 focus:ring-offset-0 ' +
                    (invalid
                        ? 'border-rose-400 focus:ring-rose-300 '
                        : 'border-slate-200 focus:border-emerald-500 focus:ring-emerald-200 ') +
                    className, ...rest }), hint !== undefined && error === undefined && (_jsx("p", { id: `${inputId}-hint`, className: "text-xs text-slate-500", children: hint })), invalid && (_jsx("p", { id: `${inputId}-error`, className: "text-xs text-rose-600", children: error }))] }));
});
