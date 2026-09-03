import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { useEffect, useRef, useState } from 'react';
import { useNavigate } from '@tanstack/react-router';
import { useLogout } from '../../lib/auth';
function initials(name, email) {
    const parts = name.trim().split(/\s+/).filter(Boolean);
    if (parts.length >= 2 && parts[0] !== undefined && parts[1] !== undefined) {
        return `${parts[0][0] ?? ''}${parts[1][0] ?? ''}`.toUpperCase();
    }
    if (parts.length === 1 && parts[0] !== undefined && parts[0].length > 0) {
        return parts[0].slice(0, 2).toUpperCase();
    }
    return email.slice(0, 2).toUpperCase();
}
export function Header({ me }) {
    const [open, setOpen] = useState(false);
    const menuRef = useRef(null);
    const navigate = useNavigate();
    const logout = useLogout();
    useEffect(() => {
        if (!open)
            return;
        const onClick = (e) => {
            if (menuRef.current !== null && !menuRef.current.contains(e.target)) {
                setOpen(false);
            }
        };
        const onKey = (e) => {
            if (e.key === 'Escape')
                setOpen(false);
        };
        document.addEventListener('mousedown', onClick);
        document.addEventListener('keydown', onKey);
        return () => {
            document.removeEventListener('mousedown', onClick);
            document.removeEventListener('keydown', onKey);
        };
    }, [open]);
    const onLogout = async () => {
        try {
            await logout.mutateAsync();
        }
        finally {
            setOpen(false);
            await navigate({ to: '/login' });
        }
    };
    return (_jsxs("header", { className: "flex h-14 flex-shrink-0 items-center justify-between border-b border-slate-200 bg-white px-4", children: [_jsxs("div", { className: "flex items-center gap-2", children: [_jsx("div", { className: "flex h-7 w-7 items-center justify-center rounded-lg bg-emerald-600 text-white", children: _jsx("span", { className: "text-[11px] font-bold", children: "fW" }) }), _jsx("span", { className: "text-sm font-semibold tracking-tight text-slate-900", children: "fullWA" })] }), _jsx("div", { className: "text-sm font-medium text-slate-600", children: me.org_name }), _jsxs("div", { className: "relative", ref: menuRef, children: [_jsx("button", { type: "button", "aria-label": "Open user menu", "aria-haspopup": "menu", "aria-expanded": open, onClick: () => setOpen((v) => !v), className: "flex h-9 w-9 items-center justify-center rounded-full bg-slate-100 text-xs font-semibold text-slate-700 hover:bg-slate-200 focus:outline-none focus-visible:ring-2 focus-visible:ring-emerald-500 focus-visible:ring-offset-2", children: initials(me.display_name, me.email) }), open && (_jsxs("div", { role: "menu", className: "absolute right-0 mt-2 w-64 rounded-xl border border-slate-200 bg-white p-2 shadow-lg", children: [_jsxs("div", { className: "px-3 py-2", children: [_jsx("p", { className: "text-xs text-slate-500", children: "Signed in as" }), _jsx("p", { className: "truncate text-sm font-medium text-slate-900", children: me.email })] }), _jsx("div", { className: "my-1 h-px bg-slate-100" }), _jsx("button", { type: "button", role: "menuitem", onClick: onLogout, disabled: logout.isPending, className: "flex w-full items-center rounded-lg px-3 py-2 text-left text-sm text-slate-700 hover:bg-slate-50 disabled:text-slate-400", children: logout.isPending ? 'Signing out…' : 'Logout' })] }))] })] }));
}
