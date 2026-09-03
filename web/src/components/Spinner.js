import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
export function Spinner({ className = 'h-4 w-4', label = 'Loading' }) {
    return (_jsxs("svg", { className: `animate-spin ${className}`, xmlns: "http://www.w3.org/2000/svg", fill: "none", viewBox: "0 0 24 24", role: "status", "aria-label": label, children: [_jsx("circle", { className: "opacity-25", cx: "12", cy: "12", r: "10", stroke: "currentColor", strokeWidth: "4" }), _jsx("path", { className: "opacity-75", fill: "currentColor", d: "M4 12a8 8 0 018-8v4a4 4 0 00-4 4H4z" })] }));
}
