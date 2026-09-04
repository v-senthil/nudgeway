import { useMemo, useState } from 'react';
import { createRoute } from '@tanstack/react-router';
import { settingsRoute } from './settings';
import { Spinner } from '../components/Spinner';
import { EmptyState } from '../components/EmptyState';
import { Button } from '../components/Button';
import { useIntegrations } from '../lib/integrations';
import { ApiError } from '../lib/api';
import {
  useGroups,
  useGroupMembers,
  useSyncGroups,
  useCreateGroup,
  useSendGroupMessage,
  type Group,
} from '../lib/groups';

function formatOccurred(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString();
}

function RoleBadge({ role }: { role: string }) {
  const colours: Record<string, string> = {
    superadmin: 'bg-indigo-50 text-indigo-700 ring-indigo-100',
    admin: 'bg-emerald-50 text-emerald-700 ring-emerald-100',
    member: 'bg-slate-100 text-slate-700 ring-slate-200',
  };
  const cls = colours[role] ?? colours.member;
  return (
    <span
      className={`inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium ring-1 ring-inset ${cls}`}
    >
      {role}
    </span>
  );
}

function GroupRow({
  group,
  selected,
  onSelect,
}: {
  group: Group;
  selected: boolean;
  onSelect: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onSelect}
      className={`flex w-full flex-col items-start gap-1 rounded-lg border px-3 py-2 text-left transition ${
        selected
          ? 'border-emerald-300 bg-emerald-50'
          : 'border-transparent hover:border-slate-200 hover:bg-slate-50'
      }`}
    >
      <div className="flex w-full items-center justify-between gap-2">
        <span className="truncate text-sm font-medium text-slate-800">
          {group.subject === '' ? <span className="italic text-slate-400">(no subject)</span> : group.subject}
        </span>
        <span className="whitespace-nowrap text-xs text-slate-500">{group.size} members</span>
      </div>
      <span className="truncate font-mono text-[10px] text-slate-400">{group.provider_group_id}</span>
    </button>
  );
}

function GroupDetail({ group }: { group: Group }) {
  const members = useGroupMembers(group.id);
  const [composerType] = useState<'text'>('text');
  const [composerBody, setComposerBody] = useState('');
  const send = useSendGroupMessage(group.id);
  const [flash, setFlash] = useState<string | null>(null);

  const activeMembers = useMemo(() => {
    if (members.data === undefined) return [];
    return members.data.items.filter((m) => m.left_at === undefined);
  }, [members.data]);

  const onSend = async () => {
    if (composerBody.trim() === '') return;
    try {
      await send.mutateAsync({ type: composerType, text: { body: composerBody } });
      setComposerBody('');
      setFlash('Message queued.');
      setTimeout(() => setFlash(null), 3000);
    } catch (e) {
      setFlash(e instanceof ApiError ? e.problem.detail ?? 'Send failed' : 'Send failed');
    }
  };

  return (
    <section className="flex flex-col gap-4 rounded-xl border border-slate-200 bg-white p-5">
      <header className="flex flex-col gap-1 border-b border-slate-100 pb-3">
        <h2 className="text-lg font-semibold text-slate-900">
          {group.subject === '' ? '(no subject)' : group.subject}
        </h2>
        {group.description !== undefined && group.description !== '' && (
          <p className="text-sm text-slate-600">{group.description}</p>
        )}
        <p className="font-mono text-xs text-slate-400">{group.provider_group_id}</p>
        <div className="mt-2 flex flex-wrap gap-2 text-xs text-slate-500">
          <span>Size: {group.size}</span>
          <span>Admin: {group.is_admin ? 'yes' : 'no'}</span>
          <span>Updated: {formatOccurred(group.updated_at)}</span>
        </div>
      </header>

      <div>
        <h3 className="mb-2 text-sm font-medium text-slate-700">
          Members {activeMembers.length > 0 && <span className="text-slate-400">({activeMembers.length})</span>}
        </h3>
        {members.isPending && (
          <div className="flex items-center justify-center py-6">
            <Spinner className="h-5 w-5 text-slate-500" label="Loading members" />
          </div>
        )}
        {members.isError && (
          <p className="rounded-lg bg-rose-50 p-3 text-xs text-rose-800">Could not load members.</p>
        )}
        {members.data !== undefined && activeMembers.length === 0 && (
          <p className="rounded-lg bg-slate-50 p-3 text-xs text-slate-500">No active members recorded.</p>
        )}
        {activeMembers.length > 0 && (
          <ul className="divide-y divide-slate-100 rounded-lg border border-slate-200">
            {activeMembers.map((m) => (
              <li key={m.id} className="flex items-center justify-between px-3 py-2">
                <div className="flex flex-col">
                  <span className="text-sm text-slate-800">
                    {m.wa_id !== undefined && m.wa_id !== ''
                      ? `+${m.wa_id}`
                      : (m.bsuid ?? '(unknown)')}
                  </span>
                  {m.contact_id !== undefined && (
                    <span className="font-mono text-[10px] text-slate-400">{m.contact_id}</span>
                  )}
                </div>
                <RoleBadge role={m.role} />
              </li>
            ))}
          </ul>
        )}
      </div>

      <div>
        <h3 className="mb-2 text-sm font-medium text-slate-700">Send message</h3>
        <textarea
          value={composerBody}
          onChange={(e) => setComposerBody(e.target.value)}
          placeholder="Type a message to send to the whole group…"
          rows={3}
          className="w-full rounded-lg border border-slate-200 p-2 text-sm text-slate-800 focus:border-emerald-400 focus:outline-none focus:ring-2 focus:ring-emerald-100"
        />
        <div className="mt-2 flex items-center justify-between">
          {flash !== null ? (
            <p className="text-xs text-emerald-700">{flash}</p>
          ) : (
            <span />
          )}
          <Button
            variant="primary"
            onClick={() => {
              void onSend();
            }}
            disabled={composerBody.trim() === '' || send.isPending}
          >
            {send.isPending ? 'Sending…' : 'Send'}
          </Button>
        </div>
      </div>
    </section>
  );
}

function NewGroupDialog({
  onClose,
  onCreated,
  whatsappIntegrations,
  defaultIntegrationID,
}: {
  onClose: () => void;
  onCreated: (g: Group) => void;
  whatsappIntegrations: Array<{ id: string; name: string }>;
  defaultIntegrationID: string;
}) {
  const [integrationID, setIntegrationID] = useState<string>(
    defaultIntegrationID !== '' ? defaultIntegrationID : (whatsappIntegrations[0]?.id ?? ''),
  );
  const [subject, setSubject] = useState('');
  const [description, setDescription] = useState('');
  const [mode, setMode] = useState<'auto_approve' | 'approval_required'>('auto_approve');
  const [err, setErr] = useState<string | null>(null);
  const create = useCreateGroup();

  const submit = async () => {
    setErr(null);
    if (integrationID === '' || subject.trim() === '') {
      setErr('Integration + subject are required.');
      return;
    }
    try {
      const g = await create.mutateAsync({
        integration_id: integrationID,
        subject: subject.trim(),
        ...(description.trim() !== '' ? { description: description.trim() } : {}),
        join_approval_mode: mode,
      });
      onCreated(g);
    } catch (e) {
      setErr(e instanceof ApiError ? (e.problem.detail ?? 'Create failed') : 'Create failed');
    }
  };

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="new-group-title"
      className="absolute right-0 top-full z-20 mt-2 w-[380px] rounded-xl border border-slate-200 bg-white p-4 shadow-lg"
    >
      <h2 id="new-group-title" className="mb-3 text-sm font-semibold text-slate-900">
        New group
      </h2>
      <div className="flex flex-col gap-3">
        <label className="flex flex-col gap-1 text-xs font-medium text-slate-600">
          Integration
          <select
            value={integrationID}
            onChange={(e) => setIntegrationID(e.target.value)}
            className="rounded-lg border border-slate-200 bg-white px-2 py-1.5 text-sm text-slate-800"
          >
            {whatsappIntegrations.length === 0 && <option value="">(none available)</option>}
            {whatsappIntegrations.map((i) => (
              <option key={i.id} value={i.id}>
                {i.name}
              </option>
            ))}
          </select>
        </label>
        <label className="flex flex-col gap-1 text-xs font-medium text-slate-600">
          Subject
          <input
            type="text"
            value={subject}
            onChange={(e) => setSubject(e.target.value)}
            maxLength={128}
            placeholder="Group name"
            className="rounded-lg border border-slate-200 bg-white px-2 py-1.5 text-sm text-slate-800"
          />
        </label>
        <label className="flex flex-col gap-1 text-xs font-medium text-slate-600">
          Description (optional)
          <textarea
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            maxLength={2048}
            rows={3}
            className="rounded-lg border border-slate-200 bg-white px-2 py-1.5 text-sm text-slate-800"
          />
        </label>
        <fieldset className="flex flex-col gap-1 text-xs font-medium text-slate-600">
          <legend>Join approval</legend>
          <label className="flex items-center gap-2 font-normal text-slate-700">
            <input
              type="radio"
              name="join-mode"
              checked={mode === 'auto_approve'}
              onChange={() => setMode('auto_approve')}
            />
            Auto approve
          </label>
          <label className="flex items-center gap-2 font-normal text-slate-700">
            <input
              type="radio"
              name="join-mode"
              checked={mode === 'approval_required'}
              onChange={() => setMode('approval_required')}
            />
            Approval required
          </label>
        </fieldset>
        {err !== null && (
          <p role="alert" className="rounded-lg bg-rose-50 px-2 py-1 text-xs text-rose-800">
            {err}
          </p>
        )}
        <div className="mt-1 flex items-center justify-end gap-2">
          <Button variant="secondary" onClick={onClose} disabled={create.isPending}>
            Cancel
          </Button>
          <Button
            variant="primary"
            onClick={() => {
              void submit();
            }}
            disabled={create.isPending || subject.trim() === '' || integrationID === ''}
          >
            {create.isPending ? 'Creating…' : 'Create'}
          </Button>
        </div>
      </div>
    </div>
  );
}

function GroupsPage() {
  const [integrationFilter, setIntegrationFilter] = useState<string>('');
  const [q, setQ] = useState('');
  const [selectedID, setSelectedID] = useState<string | undefined>(undefined);
  const [showNew, setShowNew] = useState(false);

  const integrations = useIntegrations();
  const groups = useGroups({ integration_id: integrationFilter, q, limit: 100 });
  const sync = useSyncGroups();
  const [syncFlash, setSyncFlash] = useState<string | null>(null);

  const items = groups.data?.items ?? [];
  const selected = useMemo(
    () => items.find((g) => g.id === selectedID),
    [items, selectedID],
  );

  const whatsappIntegrations = useMemo(
    () => (integrations.data ?? []).filter((i) => i.provider === 'whatsapp'),
    [integrations.data],
  );

  const onSync = async () => {
    // If a filter integration is picked, sync that. Otherwise sync the
    // first WhatsApp integration on the org.
    const target =
      integrationFilter !== ''
        ? integrationFilter
        : whatsappIntegrations[0]?.id;
    if (target === undefined || target === '') {
      setSyncFlash('No WhatsApp integration to sync.');
      setTimeout(() => setSyncFlash(null), 3000);
      return;
    }
    try {
      const res = await sync.mutateAsync({ integration_id: target });
      setSyncFlash(
        `Synced ${res.groups_upserted} groups (${res.members_upserted} members).`,
      );
      setTimeout(() => setSyncFlash(null), 4000);
    } catch (e) {
      setSyncFlash(e instanceof ApiError ? e.problem.detail ?? 'Sync failed' : 'Sync failed');
      setTimeout(() => setSyncFlash(null), 5000);
    }
  };

  const isPermDenied = groups.error instanceof ApiError && groups.error.status === 403;

  return (
    <div className="mx-auto max-w-6xl space-y-6">
      <header className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold text-slate-900">Groups</h1>
          <p className="mt-1 text-sm text-slate-500">
            WhatsApp groups your integration participates in. Sync to pull the current roster from
            Meta, then send group messages from here.
          </p>
        </div>
        <div className="relative flex flex-col items-end gap-1">
          <div className="flex items-center gap-2">
            <Button
              variant="secondary"
              onClick={() => setShowNew((v) => !v)}
              disabled={whatsappIntegrations.length === 0}
            >
              New Group
            </Button>
            <Button
              variant="primary"
              onClick={() => {
                void onSync();
              }}
              disabled={sync.isPending}
            >
              {sync.isPending ? 'Syncing…' : 'Sync now'}
            </Button>
          </div>
          {syncFlash !== null && (
            <p className="text-xs text-slate-500">{syncFlash}</p>
          )}
          {showNew && (
            <NewGroupDialog
              onClose={() => setShowNew(false)}
              onCreated={(g) => {
                setShowNew(false);
                setSelectedID(g.id);
                void groups.refetch();
              }}
              whatsappIntegrations={whatsappIntegrations}
              defaultIntegrationID={integrationFilter}
            />
          )}
        </div>
      </header>

      <section
        aria-label="Filters"
        className="grid gap-3 rounded-xl border border-slate-200 bg-white p-4 sm:grid-cols-2"
      >
        <label className="flex flex-col gap-1 text-xs font-medium text-slate-600">
          Integration
          <select
            value={integrationFilter}
            onChange={(e) => setIntegrationFilter(e.target.value)}
            className="rounded-lg border border-slate-200 bg-white px-2 py-1.5 text-sm text-slate-800"
          >
            <option value="">All</option>
            {whatsappIntegrations.map((i) => (
              <option key={i.id} value={i.id}>
                {i.name}
              </option>
            ))}
          </select>
        </label>
        <label className="flex flex-col gap-1 text-xs font-medium text-slate-600">
          Search
          <input
            type="text"
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="subject substring"
            className="rounded-lg border border-slate-200 bg-white px-2 py-1.5 text-sm text-slate-800"
          />
        </label>
      </section>

      {groups.isPending && (
        <div className="flex items-center justify-center rounded-xl border border-slate-200 bg-white py-12">
          <Spinner className="h-6 w-6 text-slate-500" label="Loading groups" />
        </div>
      )}

      {groups.isError && (
        <div role="alert" className="rounded-xl border border-rose-200 bg-rose-50 p-4 text-sm text-rose-800">
          <p className="font-medium">
            {isPermDenied ? "You don't have permission to view groups." : 'Could not load groups.'}
          </p>
          {!isPermDenied && (
            <button
              type="button"
              onClick={() => void groups.refetch()}
              className="mt-2 rounded-lg bg-white px-3 py-1 text-xs font-medium text-rose-700 ring-1 ring-inset ring-rose-200 hover:bg-rose-100"
            >
              Retry
            </button>
          )}
        </div>
      )}

      {!groups.isPending && !groups.isError && items.length === 0 && (
        <EmptyState
          title="No groups yet."
          description="Click 'Sync now' to pull your WhatsApp groups from Meta."
        />
      )}

      {items.length > 0 && (
        <div className="grid gap-4 lg:grid-cols-[320px,1fr]">
          <div className="flex flex-col gap-1 rounded-xl border border-slate-200 bg-white p-3">
            {items.map((g) => (
              <GroupRow
                key={g.id}
                group={g}
                selected={g.id === selectedID}
                onSelect={() => setSelectedID(g.id)}
              />
            ))}
          </div>
          {selected === undefined ? (
            <EmptyState
              title="Pick a group"
              description="Select a group on the left to see its members and send a message."
            />
          ) : (
            <GroupDetail group={selected} />
          )}
        </div>
      )}
    </div>
  );
}

export const settingsGroupsRoute = createRoute({
  getParentRoute: () => settingsRoute,
  path: '/groups',
  component: GroupsPage,
});
