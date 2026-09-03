import { jsx as _jsx } from "react/jsx-runtime";
export function Card({ children, className = '', ...rest }) {
    return (_jsx("div", { className: `rounded-xl border border-slate-200 bg-white shadow-sm ${className}`, ...rest, children: children }));
}
