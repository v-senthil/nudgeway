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
  const [emailError, setEmailError] = useState<string | undefined>(undefined);
  const [passwordError, setPasswordError] = useState<string | undefined>(undefined);
  const [formError, setFormError] = useState<string | undefined>(undefined);

  useEffect(() => {
    void ensureCsrf();
  }, []);

  useEffect(() => {
    if (me.data !== undefined && me.data !== null) {
      void navigate({ to: '/inbox' });
    }
  }, [me.data, navigate]);

  const validate = (): boolean => {
    let ok = true;
    if (email.trim().length === 0) {
      setEmailError('Email is required.');
      ok = false;
    } else if (!/^\S+@\S+\.\S+$/.test(email)) {
      setEmailError('Enter a valid email address.');
      ok = false;
    } else {
      setEmailError(undefined);
    }
    if (password.length === 0) {
      setPasswordError('Password is required.');
      ok = false;
    } else {
      setPasswordError(undefined);
    }
    return ok;
  };

  const onSubmit = async (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    setFormError(undefined);
    if (!validate()) return;
    try {
      await login.mutateAsync({ email: email.trim(), password });
      await navigate({ to: '/inbox' });
    } catch (err) {
      if (err instanceof ApiError) {
        setFormError(err.problem.detail ?? err.problem.title ?? 'Sign-in failed. Please try again.');
      } else {
        setFormError('Unexpected error. Please try again.');
      }
    }
  };

  return (
    <main className="flex min-h-screen items-center justify-center px-4">
      <Card className="w-full max-w-md p-8">
        <div className="mb-6 text-center">
          <div className="mx-auto mb-3 flex h-10 w-10 items-center justify-center rounded-xl bg-emerald-600 text-white">
            <span className="text-sm font-bold">fW</span>
          </div>
          <h1 className="text-xl font-semibold text-slate-900">Sign in to fullWA</h1>
          <p className="mt-1 text-sm text-slate-500">Welcome back. Enter your credentials to continue.</p>
        </div>

        <form onSubmit={onSubmit} noValidate className="space-y-4">
          <Input
            label="Email"
            type="email"
            autoComplete="email"
            required
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            error={emailError}
            placeholder="you@company.com"
          />
          <Input
            label="Password"
            type="password"
            autoComplete="current-password"
            required
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            error={passwordError}
            placeholder="••••••••"
          />

          {formError !== undefined && (
            <div
              role="alert"
              className="rounded-xl border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700"
            >
              {formError}
            </div>
          )}

          <Button
            type="submit"
            variant="primary"
            loading={login.isPending}
            className="w-full"
          >
            {login.isPending ? 'Signing in…' : 'Sign in'}
          </Button>
        </form>

        <p className="mt-6 text-center text-xs text-slate-500">
          Multi-tenant WhatsApp Business Platform.
        </p>
      </Card>
    </main>
  );
}

export const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/login',
  component: LoginPage,
});
