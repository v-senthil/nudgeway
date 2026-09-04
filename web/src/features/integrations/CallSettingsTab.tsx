import { useEffect, useState } from 'react';
import { Button } from '../../components/Button';
import { Spinner } from '../../components/Spinner';
import { ApiError } from '../../lib/api';
import {
  type CallSettings,
  type WeeklyHours,
  DAYS_OF_WEEK,
  TIMEZONES,
  displayToHHMM,
  hhmmToDisplay,
  useCallSettings,
  useUpdateCallSettings,
} from '../../lib/integration-settings';

// CallSettingsTab renders the "Call settings" section of the drawer.
// Only the base surface is exposed here — SIP, holiday_schedule, and
// restrict_to_user_countries are follow-ups noted in the report.
export function CallSettingsTab({ integrationID, active }: { integrationID: string; active: boolean }) {
  const query = useCallSettings(integrationID, active);
  const update = useUpdateCallSettings(integrationID);

  const [form, setForm] = useState<CallSettings>({});
  const [rowMap, setRowMap] = useState<Record<string, { open: string; close: string; enabled: boolean }>>({});
  const [savedFlash, setSavedFlash] = useState(false);

  useEffect(() => {
    if (query.data === undefined) return;
    setForm(query.data);
    // Seed the day rows from the fetched weekly hours; days not present
    // are shown as disabled with default 09:00–17:00.
    const next: Record<string, { open: string; close: string; enabled: boolean }> = {};
    for (const d of DAYS_OF_WEEK) {
      next[d] = { open: '09:00', close: '17:00', enabled: false };
    }
    const weekly = query.data.call_hours?.weekly_operating_hours ?? [];
    for (const w of weekly) {
      next[w.day_of_week] = {
        open: hhmmToDisplay(w.open_time),
        close: hhmmToDisplay(w.close_time),
        enabled: true,
      };
    }
    setRowMap(next);
  }, [query.data]);

  const setDay = (day: string, patch: Partial<{ open: string; close: string; enabled: boolean }>) => {
    setRowMap((prev) => ({ ...prev, [day]: { ...(prev[day] ?? { open: '09:00', close: '17:00', enabled: false }), ...patch } }));
  };

  const onSave = async () => {
    setSavedFlash(false);
    const weekly: WeeklyHours[] = DAYS_OF_WEEK.filter((d) => rowMap[d]?.enabled === true).map((d) => ({
      day_of_week: d,
      open_time: displayToHHMM(rowMap[d]?.open ?? '09:00'),
      close_time: displayToHHMM(rowMap[d]?.close ?? '17:00'),
    }));
    const payload: CallSettings = {
      status: form.status ?? 'DISABLED',
      call_icon_visibility: form.call_icon_visibility ?? 'DEFAULT',
      callback_permission_status: form.callback_permission_status ?? 'DISABLED',
      call_hours: {
        status: form.call_hours?.status ?? 'DISABLED',
        timezone_id: form.call_hours?.timezone_id ?? 'UTC',
        weekly_operating_hours: weekly,
      },
    };
    try {
      await update.mutateAsync(payload);
      setSavedFlash(true);
      window.setTimeout(() => setSavedFlash(false), 2500);
    } catch {
      // rendered below
    }
  };

  if (query.isPending) {
    return (
      <div className="flex items-center justify-center py-12">
        <Spinner className="h-6 w-6 text-slate-500" label="Loading call settings" />
      </div>
    );
  }
  if (query.isError) {
    const detail = query.error.problem.detail ?? query.error.problem.title ?? 'Failed to load call settings';
    return (
      <div role="alert" className="rounded-lg border border-rose-200 bg-rose-50 p-3 text-sm text-rose-800">
        {detail}
      </div>
    );
  }

  const saveErr = update.error instanceof ApiError
    ? update.error.problem.detail ?? update.error.problem.title ?? 'Save failed'
    : null;

  const hoursEnabled = form.call_hours?.status === 'ENABLED';

  return (
    <form
      className="space-y-5"
      onSubmit={(e) => {
        e.preventDefault();
        void onSave();
      }}
    >
      <ToggleRow
        label="Calling enabled"
        description="Master switch. Disabling hides the call surface entirely for this number."
        checked={form.status === 'ENABLED'}
        onChange={(v) => setForm({ ...form, status: v ? 'ENABLED' : 'DISABLED' })}
      />

      <ToggleRow
        label="Show call icon"
        description="Icon visibility in the customer's chat header."
        checked={(form.call_icon_visibility ?? 'DEFAULT') === 'DEFAULT'}
        onChange={(v) => setForm({ ...form, call_icon_visibility: v ? 'DEFAULT' : 'DISABLE_ALL' })}
      />

      <ToggleRow
        label="Callback permission"
        description="Allow customers to request a callback from within the chat."
        checked={form.callback_permission_status === 'ENABLED'}
        onChange={(v) => setForm({ ...form, callback_permission_status: v ? 'ENABLED' : 'DISABLED' })}
      />

      <div className="rounded-lg border border-slate-200">
        <div className="flex items-center justify-between px-3 py-2">
          <div>
            <p className="text-sm font-semibold text-slate-800">Call hours</p>
            <p className="text-xs text-slate-500">
              Weekly window when the in-app call button surfaces.
            </p>
          </div>
          <Toggle
            checked={hoursEnabled}
            onChange={(v) =>
              setForm({ ...form, call_hours: { ...(form.call_hours ?? {}), status: v ? 'ENABLED' : 'DISABLED' } })
            }
            ariaLabel="Enable call hours"
          />
        </div>
        {hoursEnabled && (
          <div className="border-t border-slate-200 p-3 space-y-3">
            <label className="block">
              <span className="mb-1 block text-xs font-semibold uppercase tracking-wide text-slate-600">Timezone</span>
              <select
                value={form.call_hours?.timezone_id ?? 'UTC'}
                onChange={(e) =>
                  setForm({ ...form, call_hours: { ...(form.call_hours ?? {}), timezone_id: e.target.value } })
                }
                className={fieldClass}
              >
                {TIMEZONES.map((tz) => (
                  <option key={tz} value={tz}>
                    {tz}
                  </option>
                ))}
              </select>
            </label>

            <div className="divide-y divide-slate-100 rounded-lg border border-slate-200 bg-white">
              {DAYS_OF_WEEK.map((day) => {
                const row = rowMap[day] ?? { open: '09:00', close: '17:00', enabled: false };
                return (
                  <div key={day} className="grid grid-cols-[6rem_1fr_1fr_3rem] items-center gap-2 px-3 py-2">
                    <span className="text-xs font-medium text-slate-700">{day.slice(0, 3)}</span>
                    <input
                      type="time"
                      value={row.open}
                      onChange={(e) => setDay(day, { open: e.target.value })}
                      className={fieldClass + ' !py-1 text-xs'}
                      disabled={!row.enabled}
                    />
                    <input
                      type="time"
                      value={row.close}
                      onChange={(e) => setDay(day, { close: e.target.value })}
                      className={fieldClass + ' !py-1 text-xs'}
                      disabled={!row.enabled}
                    />
                    <Toggle
                      checked={row.enabled}
                      onChange={(v) => setDay(day, { enabled: v })}
                      ariaLabel={`Enable ${day}`}
                    />
                  </div>
                );
              })}
            </div>
          </div>
        )}
      </div>

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

      <div className="flex justify-end pt-2">
        <Button type="submit" variant="primary" loading={update.isPending}>
          Save
        </Button>
      </div>
    </form>
  );
}

const fieldClass =
  'block w-full rounded-lg border border-slate-200 bg-white px-3 py-2 text-sm text-slate-900 shadow-sm ' +
  'focus:border-emerald-500 focus:outline-none focus:ring-1 focus:ring-emerald-500 disabled:bg-slate-50 disabled:text-slate-400';

function ToggleRow({
  label,
  description,
  checked,
  onChange,
}: {
  label: string;
  description: string;
  checked: boolean;
  onChange: (v: boolean) => void;
}) {
  return (
    <div className="flex items-center justify-between rounded-lg border border-slate-200 px-3 py-2">
      <div>
        <p className="text-sm font-medium text-slate-800">{label}</p>
        <p className="text-xs text-slate-500">{description}</p>
      </div>
      <Toggle checked={checked} onChange={onChange} ariaLabel={label} />
    </div>
  );
}

function Toggle({
  checked,
  onChange,
  ariaLabel,
}: {
  checked: boolean;
  onChange: (v: boolean) => void;
  ariaLabel: string;
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={ariaLabel}
      onClick={() => onChange(!checked)}
      className={`relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full transition ${
        checked ? 'bg-emerald-600' : 'bg-slate-300'
      }`}
    >
      <span
        className={`inline-block h-4 w-4 transform rounded-full bg-white shadow transition ${
          checked ? 'translate-x-[1.125rem]' : 'translate-x-0.5'
        } mt-0.5`}
      />
    </button>
  );
}
