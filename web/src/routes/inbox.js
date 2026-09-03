import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { useEffect } from 'react';
import { createRoute, useNavigate } from '@tanstack/react-router';
import { rootRoute } from './__root';
import { useMe } from '../lib/auth';
import { Header } from '../features/inbox/Header';
import { ConversationList } from '../features/inbox/ConversationList';
import { Thread } from '../features/inbox/Thread';
import { ContactPanel } from '../features/inbox/ContactPanel';
import { Spinner } from '../components/Spinner';
function InboxPage() {
    const me = useMe();
    const navigate = useNavigate();
    useEffect(() => {
        if (!me.isPending && (me.data === null || me.data === undefined)) {
            void navigate({ to: '/login' });
        }
    }, [me.isPending, me.data, navigate]);
    if (me.isPending) {
        return (_jsx("div", { className: "flex min-h-screen items-center justify-center text-slate-500", children: _jsx(Spinner, { className: "h-6 w-6", label: "Loading session" }) }));
    }
    if (me.data === null || me.data === undefined) {
        return null;
    }
    return (_jsxs("div", { className: "flex h-screen flex-col", children: [_jsx(Header, { me: me.data }), _jsxs("div", { className: "flex flex-1 overflow-hidden", children: [_jsx(ConversationList, {}), _jsx(Thread, {}), _jsx(ContactPanel, {})] })] }));
}
export const inboxRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '/inbox',
    component: InboxPage,
});
