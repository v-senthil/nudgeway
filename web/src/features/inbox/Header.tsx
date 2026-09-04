import { useEffect, useRef, useState } from 'react';
import { Link, useNavigate } from '@tanstack/react-router';
import type { Me } from '../../lib/auth';
import { useLogout } from '../../lib/auth';

function initials(name: string, email: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  if (parts.length >= 2 && parts[0] !== undefined && parts[1] !== undefined) {
    return `${parts[0][0] ?? ''}${parts[1][0] ?? ''}`.toUpperCase();
  }
  if (parts.length === 1 && parts[0] !== undefined && parts[0].length > 0) {
    return parts[0].slice(0, 2).toUpperCase();
  }
  return email.slice(0, 2).toUpperCase();
}

type Props = { me: Me };

export function Header({ me }: Props) {
  const [open, setOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);
  const navigate = useNavigate();
  const logout = useLogout();

  useEffect(() => {
    if (!open) return;
    const onClick = (e: MouseEvent) => {
      if (menuRef.current !== null && !menuRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOpen(false);
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
    } finally {
      setOpen(false);
      await navigate({ to: '/login' });
    }
  };

  return (
    <header className="flex h-14 flex-shrink-0 items-center justify-between border-b border-slate-200 bg-white px-4">
      <div className="flex items-center gap-2">
        <div className="flex h-7 w-7 items-center justify-center rounded-lg bg-emerald-600 text-white">
          <span className="text-[11px] font-bold">fW</span>
        </div>
        <span className="text-sm font-semibold tracking-tight text-slate-900">fullWA</span>
      </div>

      <nav aria-label="Primary" className="flex items-center gap-1">
        <Link
          to="/inbox"
          className="rounded-lg px-3 py-1.5 text-sm text-slate-600 hover:bg-slate-100 hover:text-slate-900"
          activeProps={{ className: 'rounded-lg px-3 py-1.5 text-sm bg-emerald-50 text-emerald-700 font-medium' }}
        >
          Inbox
        </Link>
        <Link
          to="/calls"
          className="rounded-lg px-3 py-1.5 text-sm text-slate-600 hover:bg-slate-100 hover:text-slate-900"
          activeProps={{ className: 'rounded-lg px-3 py-1.5 text-sm bg-emerald-50 text-emerald-700 font-medium' }}
        >
          Calls
        </Link>
        <Link
          to="/analytics"
          className="rounded-lg px-3 py-1.5 text-sm text-slate-600 hover:bg-slate-100 hover:text-slate-900"
          activeProps={{ className: 'rounded-lg px-3 py-1.5 text-sm bg-emerald-50 text-emerald-700 font-medium' }}
        >
          Analytics
        </Link>
        <span className="ml-3 text-sm font-medium text-slate-600">{me.org_name}</span>
      </nav>

      <div className="flex items-center gap-2">
        <button
          type="button"
          aria-label="Open settings"
          onClick={() => void navigate({ to: '/settings/integrations' })}
          className="flex h-9 w-9 items-center justify-center rounded-full text-slate-500 hover:bg-slate-100 hover:text-slate-700 focus:outline-none focus-visible:ring-2 focus-visible:ring-emerald-500 focus-visible:ring-offset-2"
        >
          <svg
            aria-hidden="true"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.8"
            strokeLinecap="round"
            strokeLinejoin="round"
            className="h-5 w-5"
          >
            <path d="M12 15.5a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7Z" />
            <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06A1.65 1.65 0 0 0 15 19.4a1.65 1.65 0 0 0-1 1.51V21a2 2 0 1 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.6 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 1 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.6a1.65 1.65 0 0 0 1-1.51V3a2 2 0 1 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 1 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1Z" />
          </svg>
        </button>
        <div className="relative" ref={menuRef}>
        <button
          type="button"
          aria-label="Open user menu"
          aria-haspopup="menu"
          aria-expanded={open}
          onClick={() => setOpen((v) => !v)}
          className="flex h-9 w-9 items-center justify-center rounded-full bg-slate-100 text-xs font-semibold text-slate-700 hover:bg-slate-200 focus:outline-none focus-visible:ring-2 focus-visible:ring-emerald-500 focus-visible:ring-offset-2"
        >
          {initials(me.display_name, me.email)}
        </button>
        {open && (
          <div
            role="menu"
            className="absolute right-0 mt-2 w-64 rounded-xl border border-slate-200 bg-white p-2 shadow-lg"
          >
            <div className="px-3 py-2">
              <p className="text-xs text-slate-500">Signed in as</p>
              <p className="truncate text-sm font-medium text-slate-900">{me.email}</p>
            </div>
            <div className="my-1 h-px bg-slate-100" />
            <button
              type="button"
              role="menuitem"
              onClick={onLogout}
              disabled={logout.isPending}
              className="flex w-full items-center rounded-lg px-3 py-2 text-left text-sm text-slate-700 hover:bg-slate-50 disabled:text-slate-400"
            >
              {logout.isPending ? 'Signing out…' : 'Logout'}
            </button>
          </div>
        )}
        </div>
      </div>
    </header>
  );
}
