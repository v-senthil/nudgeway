import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
export function EmptyState({ icon, title, description, action, className = '' }) {
    return (_jsxs("div", { className: `flex flex-col items-center justify-center gap-3 px-6 py-10 text-center ${className}`, children: [icon !== undefined && (_jsx("div", { className: "flex h-12 w-12 items-center justify-center rounded-xl bg-slate-100 text-slate-400", children: icon })), _jsxs("div", { className: "space-y-1", children: [_jsx("p", { className: "text-sm font-medium text-slate-700", children: title }), description !== undefined && (_jsx("p", { className: "text-xs text-slate-500 max-w-xs", children: description }))] }), action !== undefined && _jsx("div", { className: "pt-1", children: action })] }));
}
