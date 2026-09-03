import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { Outlet, createRootRoute } from '@tanstack/react-router';
function RootComponent() {
    return (_jsx("div", { className: "min-h-screen bg-slate-50 text-slate-900", children: _jsx(Outlet, {}) }));
}
function NotFound() {
    return (_jsx("div", { className: "flex min-h-screen items-center justify-center", children: _jsxs("div", { className: "text-center", children: [_jsx("p", { className: "text-sm font-medium text-slate-500", children: "404" }), _jsx("h1", { className: "mt-2 text-2xl font-semibold text-slate-900", children: "Page not found" })] }) }));
}
export const rootRoute = createRootRoute({
    component: RootComponent,
    notFoundComponent: NotFound,
});
