import { useEffect, useState } from 'react';

type Health = { status: string };

export function App() {
  const [health, setHealth] = useState<Health | null>(null);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    fetch('/healthz')
      .then((r) => (r.ok ? (r.json() as Promise<Health>) : Promise.reject(new Error(String(r.status)))))
      .then(setHealth)
      .catch((e: Error) => setErr(e.message));
  }, []);

  return (
    <main className="min-h-screen flex items-center justify-center">
      <div className="rounded-2xl border bg-white p-8 shadow-sm text-center max-w-md">
        <h1 className="text-2xl font-semibold">Nudgeway</h1>
        <p className="text-sm text-slate-500 mt-1">Phase 0 — walking skeleton.</p>
        <div className="mt-6 text-sm">
          {err !== null && <div className="text-rose-600">/healthz error: {err}</div>}
          {err === null && health === null && <div className="text-slate-400">checking backend…</div>}
          {err === null && health !== null && (
            <div className="text-emerald-600">backend {health.status}</div>
          )}
        </div>
      </div>
    </main>
  );
}
