import { useEffect, useState, useCallback } from 'react';

// Theme handling is intentionally tiny — one HTML class, one localStorage
// key, and a listener for cross-tab sync. Kept separate from TanStack Query
// because the theme is a UI-only concern that never round-trips the server.

export type Theme = 'light' | 'dark';

const STORAGE_KEY = 'fullwa.theme';
const EVENT_NAME = 'fullwa:theme-change';

function readStored(): Theme | null {
  try {
    const v = localStorage.getItem(STORAGE_KEY);
    if (v === 'light' || v === 'dark') return v;
  } catch {
    // localStorage may throw in sandboxed contexts — treat as no preference.
  }
  return null;
}

function systemPreference(): Theme {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return 'light';
  }
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

// resolveInitial picks the initial theme: stored preference first, then
// system, then light. Called once at boot from main.tsx to avoid a flash.
export function resolveInitial(): Theme {
  return readStored() ?? systemPreference();
}

// applyTheme toggles the `dark` class on <html> so Tailwind's class-based
// dark-mode variants take effect.
export function applyTheme(theme: Theme): void {
  if (typeof document === 'undefined') return;
  const root = document.documentElement;
  if (theme === 'dark') {
    root.classList.add('dark');
  } else {
    root.classList.remove('dark');
  }
}

// setTheme persists and applies. Fires a same-tab event so any mounted
// useTheme() hooks re-render immediately.
export function setTheme(theme: Theme): void {
  try {
    localStorage.setItem(STORAGE_KEY, theme);
  } catch {
    // ignore
  }
  applyTheme(theme);
  if (typeof window !== 'undefined') {
    window.dispatchEvent(new CustomEvent(EVENT_NAME, { detail: theme }));
  }
}

function currentAppliedTheme(): Theme {
  if (typeof document === 'undefined') return 'light';
  return document.documentElement.classList.contains('dark') ? 'dark' : 'light';
}

export function useTheme(): { theme: Theme; toggle: () => void; setTheme: (t: Theme) => void } {
  const [theme, setLocal] = useState<Theme>(() => currentAppliedTheme());

  useEffect(() => {
    const onChange = (e: Event) => {
      const detail = (e as CustomEvent<Theme>).detail;
      if (detail === 'light' || detail === 'dark') {
        setLocal(detail);
      } else {
        setLocal(currentAppliedTheme());
      }
    };
    const onStorage = (e: StorageEvent) => {
      if (e.key !== STORAGE_KEY) return;
      const v = e.newValue;
      if (v === 'light' || v === 'dark') {
        applyTheme(v);
        setLocal(v);
      }
    };
    window.addEventListener(EVENT_NAME, onChange as EventListener);
    window.addEventListener('storage', onStorage);
    return () => {
      window.removeEventListener(EVENT_NAME, onChange as EventListener);
      window.removeEventListener('storage', onStorage);
    };
  }, []);

  const toggle = useCallback(() => {
    setTheme(theme === 'dark' ? 'light' : 'dark');
  }, [theme]);

  return { theme, toggle, setTheme };
}
