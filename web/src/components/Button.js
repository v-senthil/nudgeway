import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { Spinner } from './Spinner';
const base = 'inline-flex items-center justify-center gap-2 rounded-xl px-4 py-2 text-sm font-medium transition ' +
    'focus:outline-none focus-visible:ring-2 focus-visible:ring-emerald-500 focus-visible:ring-offset-2 ' +
    'disabled:cursor-not-allowed';
const variants = {
    primary: 'bg-emerald-600 text-white shadow-sm hover:bg-emerald-500 disabled:bg-slate-300 disabled:text-slate-500',
    secondary: 'bg-white text-slate-700 border border-slate-200 shadow-sm hover:bg-slate-50 disabled:bg-slate-50 disabled:text-slate-400',
    ghost: 'bg-transparent text-slate-700 hover:bg-slate-100 disabled:text-slate-400',
};
export function Button({ variant = 'primary', loading = false, disabled, className = '', children, type, ...rest }) {
    return (_jsxs("button", { type: type ?? 'button', className: `${base} ${variants[variant]} ${className}`, disabled: disabled === true || loading, ...rest, children: [loading && _jsx(Spinner, { className: "h-4 w-4" }), children] }));
}
