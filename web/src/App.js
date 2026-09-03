import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { useEffect, useState } from 'react';
export function App() {
    const [health, setHealth] = useState(null);
    const [err, setErr] = useState(null);
    useEffect(() => {
        fetch('/healthz')
            .then((r) => (r.ok ? r.json() : Promise.reject(new Error(String(r.status)))))
            .then(setHealth)
            .catch((e) => setErr(e.message));
    }, []);
    return (_jsx("main", { className: "min-h-screen flex items-center justify-center", children: _jsxs("div", { className: "rounded-2xl border bg-white p-8 shadow-sm text-center max-w-md", children: [_jsx("h1", { className: "text-2xl font-semibold", children: "fullWA" }), _jsx("p", { className: "text-sm text-slate-500 mt-1", children: "Phase 0 \u2014 walking skeleton." }), _jsxs("div", { className: "mt-6 text-sm", children: [err !== null && _jsxs("div", { className: "text-rose-600", children: ["/healthz error: ", err] }), err === null && health === null && _jsx("div", { className: "text-slate-400", children: "checking backend\u2026" }), err === null && health !== null && (_jsxs("div", { className: "text-emerald-600", children: ["backend ", health.status] }))] })] }) }));
}
