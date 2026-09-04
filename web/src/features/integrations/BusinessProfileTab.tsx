import { useEffect, useState } from 'react';
import { Button } from '../../components/Button';
import { Spinner } from '../../components/Spinner';
import { ApiError } from '../../lib/api';
import {
  type BusinessProfile,
  VERTICALS,
  useBusinessProfile,
  useUpdateBusinessProfile,
} from '../../lib/integration-settings';

// BusinessProfileTab renders the "Business profile" section of the
// integration settings drawer. All fields except the profile picture
// URL are editable — picture upload is a follow-up (see report).
export function BusinessProfileTab({ integrationID, active }: { integrationID: string; active: boolean }) {
  const query = useBusinessProfile(integrationID, active);
  const update = useUpdateBusinessProfile(integrationID);

  const [form, setForm] = useState<BusinessProfile>({});
  const [savedFlash, setSavedFlash] = useState(false);

  useEffect(() => {
    if (query.data !== undefined) {
      setForm({
        about: query.data.about ?? '',
        address: query.data.address ?? '',
        description: query.data.description ?? '',
        email: query.data.email ?? '',
        profile_picture_url: query.data.profile_picture_url ?? '',
        vertical: query.data.vertical ?? '',
        websites: query.data.websites ?? [],
      });
    }
  }, [query.data]);

  const onSave = async () => {
    setSavedFlash(false);
    // Trim empty website slots before POSTing so Meta doesn't see the empties.
    const websites = (form.websites ?? []).map((s) => s.trim()).filter((s) => s.length > 0);
    const payload: BusinessProfile = {
      about: form.about ?? '',
      address: form.address ?? '',
      description: form.description ?? '',
      email: form.email ?? '',
      vertical: form.vertical ?? '',
      websites,
    };
    try {
      await update.mutateAsync(payload);
      setSavedFlash(true);
      window.setTimeout(() => setSavedFlash(false), 2500);
    } catch {
      // ApiError bubbles into update.error; rendered below.
    }
  };

  if (query.isPending) {
    return (
      <div className="flex items-center justify-center py-12">
        <Spinner className="h-6 w-6 text-slate-500" label="Loading business profile" />
      </div>
    );
  }
  if (query.isError) {
    const detail = query.error.problem.detail ?? query.error.problem.title ?? 'Failed to load business profile';
    return (
      <div role="alert" className="rounded-lg border border-rose-200 bg-rose-50 p-3 text-sm text-rose-800">
        {detail}
      </div>
    );
  }

  const websites = form.websites ?? [];
  const setWebsite = (idx: number, v: string) => {
    const next = [...websites];
    next[idx] = v;
    setForm({ ...form, websites: next });
  };
  const removeWebsite = (idx: number) => {
    const next = websites.filter((_, i) => i !== idx);
    setForm({ ...form, websites: next });
  };
  const addWebsite = () => {
    if (websites.length >= 2) return;
    setForm({ ...form, websites: [...websites, ''] });
  };

  const saveErr = update.error instanceof ApiError
    ? update.error.problem.detail ?? update.error.problem.title ?? 'Save failed'
    : null;

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault();
        void onSave();
      }}
    >
      <Field label="About" hint={`${(form.about ?? '').length}/139`}>
        <textarea
          rows={2}
          maxLength={139}
          value={form.about ?? ''}
          onChange={(e) => setForm({ ...form, about: e.target.value })}
          className={fieldClass}
        />
      </Field>

      <Field label="Address" hint={`${(form.address ?? '').length}/256`}>
        <input
          type="text"
          maxLength={256}
          value={form.address ?? ''}
          onChange={(e) => setForm({ ...form, address: e.target.value })}
          className={fieldClass}
        />
      </Field>

      <Field label="Description" hint={`${(form.description ?? '').length}/512`}>
        <textarea
          rows={3}
          maxLength={512}
          value={form.description ?? ''}
          onChange={(e) => setForm({ ...form, description: e.target.value })}
          className={fieldClass}
        />
      </Field>

      <Field label="Email">
        <input
          type="email"
          value={form.email ?? ''}
          onChange={(e) => setForm({ ...form, email: e.target.value })}
          className={fieldClass}
        />
      </Field>

      <Field label="Vertical">
        <select
          value={form.vertical ?? ''}
          onChange={(e) => setForm({ ...form, vertical: e.target.value })}
          className={fieldClass}
        >
          <option value="">— select —</option>
          {VERTICALS.map((v) => (
            <option key={v} value={v}>
              {v}
            </option>
          ))}
        </select>
      </Field>

      <Field label="Websites" hint="Up to 2">
        <div className="space-y-2">
          {websites.map((w, idx) => (
            <div key={idx} className="flex gap-2">
              <input
                type="url"
                value={w}
                placeholder="https://example.com"
                onChange={(e) => setWebsite(idx, e.target.value)}
                className={fieldClass}
              />
              <button
                type="button"
                onClick={() => removeWebsite(idx)}
                className="rounded-lg px-2 text-xs text-rose-700 hover:bg-rose-50"
                aria-label={`Remove website ${idx + 1}`}
              >
                Remove
              </button>
            </div>
          ))}
          {websites.length < 2 && (
            <button
              type="button"
              onClick={addWebsite}
              className="text-xs font-medium text-emerald-700 hover:text-emerald-800"
            >
              + Add website
            </button>
          )}
        </div>
      </Field>

      <Field label="Profile picture URL" hint="Read-only for now — upload coming soon">
        <input
          type="url"
          value={form.profile_picture_url ?? ''}
          readOnly
          className={fieldClass + ' bg-slate-50 text-slate-500'}
        />
      </Field>

      {saveErr !== null && (
        <div role="alert" className="rounded-lg border border-rose-200 bg-rose-50 p-3 text-sm text-rose-800">
          {saveErr}
        </div>
      )}
      {savedFlash && (
        <div role="status" className="rounded-lg border border-emerald-200 bg-emerald-50 p-3 text-sm text-emerald-800">
          Saved.
        </div>
      )}

      <div className="flex justify-end gap-2 pt-2">
        <Button type="submit" variant="primary" loading={update.isPending}>
          Save
        </Button>
      </div>
    </form>
  );
}

const fieldClass =
  'block w-full rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm text-slate-900 shadow-sm ' +
  'focus:border-emerald-500 focus:outline-none focus:ring-1 focus:ring-emerald-500';

function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return (
    <label className="block">
      <div className="mb-1 flex items-baseline justify-between">
        <span className="text-xs font-semibold uppercase tracking-wide text-slate-600">{label}</span>
        {hint !== undefined && <span className="text-[11px] text-slate-400">{hint}</span>}
      </div>
      {children}
    </label>
  );
}
