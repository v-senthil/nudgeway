import { jsx as _jsx, jsxs as _jsxs } from "react/jsx-runtime";
import { useEffect, useState } from 'react';
import { createRoute, useNavigate } from '@tanstack/react-router';
import { rootRoute } from './__root';
import { Button } from '../components/Button';
import { Input } from '../components/Input';
import { Card } from '../components/Card';
import { ApiError, ensureCsrf } from '../lib/api';
import { useLogin, useMe } from '../lib/auth';
function LoginPage() {
    const navigate = useNavigate();
    const me = useMe();
    const login = useLogin();
    const [email, setEmail] = useState('');
    const [password, setPassword] = useState('');
    const [emailError, setEmailError] = useState(undefined);
    const [passwordError, setPasswordError] = useState(undefined);
    const [formError, setFormError] = useState(undefined);
    useEffect(() => {
        void ensureCsrf();
    }, []);
    useEffect(() => {
        if (me.data !== undefined && me.data !== null) {
            void navigate({ to: '/inbox' });
        }
    }, [me.data, navigate]);
    const validate = () => {
        let ok = true;
        if (email.trim().length === 0) {
            setEmailError('Email is required.');
            ok = false;
        }
        else if (!/^\S+@\S+\.\S+$/.test(email)) {
            setEmailError('Enter a valid email address.');
            ok = false;
        }
        else {
            setEmailError(undefined);
        }
        if (password.length === 0) {
            setPasswordError('Password is required.');
            ok = false;
        }
        else {
            setPasswordError(undefined);
        }
        return ok;
    };
    const onSubmit = async (e) => {
        e.preventDefault();
        setFormError(undefined);
        if (!validate())
            return;
        try {
            await login.mutateAsync({ email: email.trim(), password });
            await navigate({ to: '/inbox' });
        }
        catch (err) {
            if (err instanceof ApiError) {
                setFormError(err.problem.detail ?? err.problem.title ?? 'Sign-in failed. Please try again.');
            }
            else {
                setFormError('Unexpected error. Please try again.');
            }
        }
    };
    return (_jsx("main", { className: "flex min-h-screen items-center justify-center px-4", children: _jsxs(Card, { className: "w-full max-w-md p-8", children: [_jsxs("div", { className: "mb-6 text-center", children: [_jsx("div", { className: "mx-auto mb-3 flex h-10 w-10 items-center justify-center rounded-xl bg-emerald-600 text-white", children: _jsx("span", { className: "text-sm font-bold", children: "fW" }) }), _jsx("h1", { className: "text-xl font-semibold text-slate-900", children: "Sign in to fullWA" }), _jsx("p", { className: "mt-1 text-sm text-slate-500", children: "Welcome back. Enter your credentials to continue." })] }), _jsxs("form", { onSubmit: onSubmit, noValidate: true, className: "space-y-4", children: [_jsx(Input, { label: "Email", type: "email", autoComplete: "email", required: true, value: email, onChange: (e) => setEmail(e.target.value), error: emailError, placeholder: "you@company.com" }), _jsx(Input, { label: "Password", type: "password", autoComplete: "current-password", required: true, value: password, onChange: (e) => setPassword(e.target.value), error: passwordError, placeholder: "\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022" }), formError !== undefined && (_jsx("div", { role: "alert", className: "rounded-xl border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700", children: formError })), _jsx(Button, { type: "submit", variant: "primary", loading: login.isPending, className: "w-full", children: login.isPending ? 'Signing in…' : 'Sign in' })] }), _jsx("p", { className: "mt-6 text-center text-xs text-slate-500", children: "Multi-tenant WhatsApp Business Platform." })] }) }));
}
export const loginRoute = createRoute({
    getParentRoute: () => rootRoute,
    path: '/login',
    component: LoginPage,
});
