import { useEffect, useMemo, useState } from 'react';
import { createRoute, useNavigate, useSearch } from '@tanstack/react-router';
import { rootRoute } from './__root';
import {
  CALL_STATUSES,
  formatDuration,
  recordingURL,
  transcriptURL,
  useCall,
  useCallTranscript,
  useCalls,
  useEndCall,
  useInitiateCall,
  useRejectCall,
  type Call,
  type CallFilter,
  type CallTranscript,
  type CallTranscriptSegment,
} from '../lib/calls';
import { useIntegrations } from '../lib/integrations';
import {
  useCallPermission,
  useSendPermissionRequest,
  type CallPermission,
} from '../lib/integration-settings';
import { Spinner } from '../components/Spinner';
import { EmptyState } from '../components/EmptyState';
import { Button } from '../components/Button';
import { ApiError } from '../lib/api';
import { Header } from '../features/inbox/Header';
import { useMe } from '../lib/auth';
import { setIncomingCall } from '../lib/incoming-call';

function formatWhen(iso: string | undefined): string {
  if (iso === undefined || iso === '') return '—';
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString();
}

function statusColor(status: Call['status']): string {
  switch (status) {
    case 'completed':
      return 'bg-emerald-50 text-emerald-700 ring-emerald-100';
    case 'answered':
    case 'in_progress':
      return 'bg-sky-50 text-sky-700 ring-sky-100';
    case 'ringing':
    case 'queued':
      return 'bg-amber-50 text-amber-700 ring-amber-100';
    case 'failed':
    case 'declined':
    case 'no_answer':
    case 'missed':
      return 'bg-rose-50 text-rose-700 ring-rose-100';
  }
}

function directionArrow(direction: Call['direction']): string {
  return direction === 'inbound' ? '↙' : '↗';
}

// primaryIdentity returns the best display string for a call row: the
// contact's resolved name if available, otherwise the BSUID, otherwise
// the raw phone, otherwise the legacy `from`/`to` peer.
function primaryIdentity(call: Call): string {
  if (call.contact_name !== undefined && call.contact_name !== '') return call.contact_name;
  if (call.bsuid !== undefined && call.bsuid !== '') return call.bsuid;
  if (call.phone !== undefined && call.phone !== '') return call.phone;
  const peer = call.direction === 'inbound' ? call.from : call.to;
  return peer ?? '—';
}

function CallRow({
  call,
  selected,
  onSelect,
}: {
  call: Call;
  selected: boolean;
  onSelect: () => void;
}) {
  const primary = primaryIdentity(call);
  const hasName = call.contact_name !== undefined && call.contact_name !== '';
  const bsuid = call.bsuid !== undefined && call.bsuid !== '' ? call.bsuid : undefined;
  const phone = call.phone !== undefined && call.phone !== '' ? call.phone : undefined;
  // Secondary line rules:
  //  - When we have a name, show BSUID prominently and phone muted.
  //  - When primary IS the BSUID, only surface phone if present.
  //  - When primary is the phone, drop the secondary line entirely.
  const showSecondary = hasName || (bsuid !== undefined && phone !== undefined);
  return (
    <button
      type="button"
      onClick={onSelect}
      className={`flex w-full items-start gap-3 border-t border-slate-100 px-3 py-2 text-left transition ${
        selected ? 'bg-emerald-50' : 'hover:bg-slate-50'
      }`}
    >
      <span
        className="mt-0.5 text-lg leading-none text-slate-400"
        aria-label={call.direction === 'inbound' ? 'inbound call' : 'outbound call'}
      >
        {directionArrow(call.direction)}
      </span>
      <div className="min-w-0 flex-1">
        <div className="flex items-center justify-between gap-2">
          <span className="truncate font-medium text-slate-800">{primary}</span>
          <span className="whitespace-nowrap text-xs text-slate-500">
            {formatWhen(call.created_at)}
          </span>
        </div>
        {showSecondary && (
          <div className="mt-0.5 flex items-center gap-2 font-mono text-[11px] leading-tight">
            {bsuid !== undefined && (
              <span className="truncate text-slate-600">{bsuid}</span>
            )}
            {phone !== undefined && (
              <span className="truncate text-slate-400">{phone}</span>
            )}
          </div>
        )}
        <div className="mt-1 flex items-center gap-2">
          <span
            className={`inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium ring-1 ring-inset ${statusColor(
              call.status,
            )}`}
          >
            {call.status}
          </span>
          <span className="text-xs text-slate-500">
            {formatDuration(call.duration_seconds)}
          </span>
        </div>
      </div>
    </button>
  );
}

// CopyRow renders a mono value alongside a compact copy button. Used in
// the call detail's identity def-list.
function CopyRow({ value }: { value: string }) {
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1200);
    } catch {
      // Clipboard blocked — the value is already visible.
    }
  };
  return (
    <span className="inline-flex items-center gap-1.5">
      <span className="font-mono text-xs text-slate-800">{value}</span>
      <button
        type="button"
        onClick={() => void copy()}
        className="rounded border border-slate-200 bg-white px-1.5 py-0.5 text-[10px] font-medium text-slate-600 hover:bg-slate-100"
        title="Copy"
      >
        {copied ? 'Copied' : 'Copy'}
      </button>
    </span>
  );
}

// formatSecondsRange renders `mm:ss - mm:ss` for a segment's start/end.
function formatSecondsRange(start: number | undefined, end: number | undefined): string {
  const fmt = (n: number | undefined): string => {
    if (n === undefined || Number.isNaN(n)) return '00:00';
    const total = Math.max(0, Math.floor(n));
    const m = Math.floor(total / 60);
    const s = total % 60;
    return `${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`;
  };
  return `${fmt(start)} - ${fmt(end)}`;
}

// speakerBadge returns the {label, classes} tuple for a segment's
// speaker label. Meta typically emits "business" / "customer" (case
// varies). We treat "business" as blue and everything else as
// customer-emerald so unknown speakers still render.
function speakerBadge(speaker: string | undefined): { label: string; classes: string } {
  const raw = (speaker ?? '').toLowerCase();
  if (raw === 'business' || raw === 'agent') {
    return {
      label: 'Business',
      classes: 'bg-sky-50 text-sky-700 ring-sky-200',
    };
  }
  return {
    label: raw === '' ? 'Speaker' : raw.charAt(0).toUpperCase() + raw.slice(1),
    classes: 'bg-emerald-50 text-emerald-700 ring-emerald-200',
  };
}

// TranscriptSection renders the transcript header + segments list. Uses
// useCallTranscript directly so it stays a self-contained component the
// call detail can drop in.
function TranscriptSection({ call }: { call: Call }) {
  const t = useCallTranscript(call.id, call.transcription_ref);

  const hasRef = call.transcription_ref !== undefined && call.transcription_ref !== '';
  if (!hasRef) {
    return (
      <section className="rounded-lg border border-slate-200 bg-slate-50 p-4">
        <h3 className="mb-2 text-sm font-medium text-slate-700">Transcript</h3>
        <p className="text-xs text-slate-500">Transcript not available yet.</p>
      </section>
    );
  }

  if (t.isPending) {
    return (
      <section className="rounded-lg border border-slate-200 bg-slate-50 p-4">
        <h3 className="mb-2 text-sm font-medium text-slate-700">Transcript</h3>
        <div className="flex items-center gap-2 text-xs text-slate-500">
          <Spinner className="h-4 w-4" label="Loading transcript" />
          Loading transcript…
        </div>
      </section>
    );
  }

  if (t.isError || t.data === undefined) {
    const msg =
      t.error instanceof ApiError
        ? t.error.problem.detail ?? t.error.message
        : 'Could not load the transcript.';
    return (
      <section className="rounded-lg border border-rose-200 bg-rose-50 p-4">
        <h3 className="mb-2 text-sm font-medium text-rose-800">Transcript</h3>
        <p className="text-xs text-rose-700">{msg}</p>
      </section>
    );
  }

  const data: CallTranscript = t.data;
  const meta = data.transcript ?? {};
  const segments: CallTranscriptSegment[] = meta.segments ?? [];
  const language = meta.language ?? '—';
  const duration = meta.duration;
  const confidencePct =
    typeof meta.confidence === 'number' ? `${Math.round(meta.confidence * 100)}%` : '—';

  const downloadHref = ((): string => {
    try {
      const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
      return URL.createObjectURL(blob);
    } catch {
      return transcriptURL(call.id);
    }
  })();

  return (
    <section className="rounded-lg border border-slate-200 bg-slate-50 p-4">
      <div className="mb-3 flex items-start justify-between gap-3">
        <div>
          <h3 className="text-sm font-medium text-slate-700">Transcript</h3>
          <p className="mt-0.5 text-xs text-slate-500">
            Language: {language}
            {duration !== undefined && ` · Duration: ${duration.toFixed(1)}s`}
            {` · Confidence: ${confidencePct}`}
          </p>
        </div>
        <a
          href={downloadHref}
          download={`transcript-${call.id}.json`}
          target="_blank"
          rel="noreferrer"
          className="inline-flex items-center rounded-lg bg-white px-2 py-1 text-xs font-medium text-slate-700 ring-1 ring-inset ring-slate-200 hover:bg-slate-100"
        >
          Download JSON
        </a>
      </div>

      {segments.length === 0 ? (
        <p className="text-xs text-slate-500">No segments in transcript.</p>
      ) : (
        <ul className="space-y-2">
          {segments.map((seg, idx) => {
            const badge = speakerBadge(seg.speaker);
            return (
              <li
                key={idx}
                className="flex items-start gap-3 rounded-md border border-slate-200 bg-white p-2.5"
              >
                <span
                  className={`inline-flex shrink-0 items-center rounded-md px-2 py-0.5 text-[11px] font-medium ring-1 ring-inset ${badge.classes}`}
                >
                  {badge.label}
                </span>
                <div className="min-w-0 flex-1">
                  <p className="text-[11px] font-mono text-slate-500">
                    {formatSecondsRange(seg.start, seg.end)}
                  </p>
                  <p className="mt-0.5 text-sm text-slate-800">{seg.text ?? ''}</p>
                </div>
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}

function CallDetail({ callID }: { callID: string | null }) {
  const detail = useCall(callID);
  const endMut = useEndCall();
  const rejectMut = useRejectCall();

  if (callID === null || callID === '') {
    return (
      <div className="flex h-full items-center justify-center rounded-xl border border-dashed border-slate-200 bg-white text-sm text-slate-400">
        Select a call to see the details.
      </div>
    );
  }
  if (detail.isPending) {
    return (
      <div className="flex h-full items-center justify-center rounded-xl border border-slate-200 bg-white">
        <Spinner className="h-6 w-6 text-slate-500" label="Loading call" />
      </div>
    );
  }
  if (detail.isError || detail.data === undefined) {
    const msg =
      detail.error instanceof ApiError
        ? detail.error.problem.detail ?? detail.error.message
        : 'Could not load the call.';
    return (
      <div className="rounded-xl border border-rose-200 bg-rose-50 p-4 text-sm text-rose-800">
        {msg}
      </div>
    );
  }
  const c = detail.data;
  const isTerminal = ['completed', 'failed', 'declined', 'no_answer', 'missed'].includes(c.status);
  const isRingingInbound = c.direction === 'inbound' && c.status === 'ringing';

  // Answering from the calls tab reuses the global IncomingCallPopup by
  // dispatching a synthetic entry into the incoming-call store — the
  // popup then drives the WebRTC handshake (SDP fetch, mic prompt,
  // /answer POST) exactly as it does for WS-delivered ringing frames.
  // Keeping a single accept path avoids duplicating the flow in two
  // places.
  const handleAnswer = (): void => {
    setIncomingCall({
      id: c.id,
      from: c.from ?? '',
      provider: c.provider,
      startedAt: c.started_at ?? c.created_at,
      ...(c.integration_id !== undefined ? { integrationID: c.integration_id } : {}),
      ...(c.conversation_id !== undefined ? { conversationID: c.conversation_id } : {}),
    });
  };

  return (
    <div className="flex h-full flex-col gap-4 rounded-xl border border-slate-200 bg-white p-6">
      <header className="flex items-start justify-between gap-4">
        <div>
          <div className="flex items-center gap-2">
            <span
              className={`inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium ring-1 ring-inset ${statusColor(
                c.status,
              )}`}
            >
              {c.status}
            </span>
            <span className="text-xs uppercase tracking-wide text-slate-500">
              {c.direction}
            </span>
          </div>
          <h2 className="mt-1 text-xl font-semibold text-slate-900">
            {primaryIdentity(c)}
          </h2>
          <p className="text-xs text-slate-500">Provider call id: {c.provider_call_id}</p>
        </div>
        <div className="flex items-center gap-2">
          {isRingingInbound && (
            <>
              <button
                type="button"
                onClick={handleAnswer}
                className="inline-flex items-center rounded-md bg-emerald-600 px-3 py-1.5 text-sm font-medium text-white shadow-sm hover:bg-emerald-700 focus:outline-none focus:ring-2 focus:ring-emerald-500 focus:ring-offset-1"
              >
                Answer
              </button>
              <button
                type="button"
                disabled={rejectMut.isPending}
                onClick={() => rejectMut.mutate({ id: c.id })}
                className="inline-flex items-center rounded-md bg-rose-600 px-3 py-1.5 text-sm font-medium text-white shadow-sm hover:bg-rose-700 focus:outline-none focus:ring-2 focus:ring-rose-500 focus:ring-offset-1 disabled:cursor-not-allowed disabled:opacity-60"
              >
                {rejectMut.isPending ? 'Rejecting…' : 'Reject'}
              </button>
            </>
          )}
          {!isTerminal && !isRingingInbound && (
            <Button
              variant="secondary"
              onClick={() => endMut.mutate(c.id)}
              loading={endMut.isPending}
            >
              End call
            </Button>
          )}
        </div>
      </header>

      <section className="rounded-lg border border-slate-200 bg-slate-50 p-4">
        <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-slate-500">
          Identity
        </h3>
        <dl className="grid grid-cols-[max-content_minmax(0,1fr)] gap-x-4 gap-y-1.5 text-sm">
          {c.contact_id !== undefined && c.contact_id !== '' && (
            <>
              <dt className="text-slate-500">Contact ID</dt>
              <dd><CopyRow value={c.contact_id} /></dd>
            </>
          )}
          {c.bsuid !== undefined && c.bsuid !== '' && (
            <>
              <dt className="text-slate-500">BSUID</dt>
              <dd><CopyRow value={c.bsuid} /></dd>
            </>
          )}
          {c.phone !== undefined && c.phone !== '' && (
            <>
              <dt className="text-slate-500">Phone</dt>
              <dd><CopyRow value={c.phone} /></dd>
            </>
          )}
        </dl>
      </section>

      <dl className="grid grid-cols-2 gap-x-6 gap-y-1.5 text-sm">
        <dt className="text-slate-500">Started</dt>
        <dd className="text-slate-800">{formatWhen(c.started_at)}</dd>
        <dt className="text-slate-500">Answered</dt>
        <dd className="text-slate-800">{formatWhen(c.answered_at)}</dd>
        <dt className="text-slate-500">Ended</dt>
        <dd className="text-slate-800">{formatWhen(c.ended_at)}</dd>
        <dt className="text-slate-500">Duration</dt>
        <dd className="text-slate-800">{formatDuration(c.duration_seconds)}</dd>
        <dt className="text-slate-500">From</dt>
        <dd className="truncate font-mono text-xs text-slate-800">{c.from ?? '—'}</dd>
        <dt className="text-slate-500">To</dt>
        <dd className="truncate font-mono text-xs text-slate-800">{c.to ?? '—'}</dd>
        {c.hangup_reason !== undefined && c.hangup_reason !== '' && (
          <>
            <dt className="text-slate-500">Hangup</dt>
            <dd className="text-slate-800">{c.hangup_reason}</dd>
          </>
        )}
      </dl>

      {c.recording_url !== undefined && c.recording_url !== '' && (
        <section className="rounded-lg border border-slate-200 bg-slate-50 p-4">
          <h3 className="mb-2 text-sm font-medium text-slate-700">Recording</h3>
          <audio controls src={recordingURL(c.id)} className="w-full">
            Your browser does not support the audio element.
          </audio>
        </section>
      )}

      <TranscriptSection call={c} />

      {c.metadata !== undefined && Object.keys(c.metadata).length > 0 && (
        <section>
          <h3 className="mb-2 text-sm font-medium text-slate-700">Metadata</h3>
          <pre className="overflow-x-auto rounded-lg bg-slate-900 p-3 text-xs text-slate-100">
            {JSON.stringify(c.metadata, null, 2)}
          </pre>
        </section>
      )}
    </div>
  );
}

function CallsPage() {
  const me = useMe();
  const navigate = useNavigate();
  const search = useSearch({ from: callsRoute.id });
  const [status, setStatus] = useState<NonNullable<CallFilter['status']>>('');
  const [direction, setDirection] = useState<NonNullable<CallFilter['direction']>>('');
  const [selectedID, setSelectedID] = useState<string | null>(search.id ?? null);

  useEffect(() => {
    if (!me.isPending && (me.data === null || me.data === undefined)) {
      void navigate({ to: '/login' });
    }
  }, [me.isPending, me.data, navigate]);

  // Deep-link: when the URL carries ?id=<call.id> (e.g. from a Thread
  // call-bubble click-through), select that call in the detail pane on
  // navigation. Silent no-op when the id is already the selected one.
  useEffect(() => {
    const wanted = search.id ?? null;
    if (wanted !== null && wanted !== selectedID) {
      setSelectedID(wanted);
    }
  }, [search.id, selectedID]);

  const filter: CallFilter = useMemo(
    () => ({ status, direction, limit: 50 }),
    [status, direction],
  );

  const query = useCalls(filter);

  const items = useMemo(() => {
    if (query.data === undefined) return [];
    const out: Call[] = [];
    for (const page of query.data.pages) out.push(...page.items);
    return out;
  }, [query.data]);

  const isPermDenied = query.error instanceof ApiError && query.error.status === 403;

  if (me.isPending) {
    return (
      <div className="flex min-h-screen items-center justify-center text-slate-500">
        <Spinner className="h-6 w-6" label="Loading session" />
      </div>
    );
  }
  if (me.data === null || me.data === undefined) return null;

  return (
    <div className="min-h-screen bg-slate-50">
      <Header me={me.data} />
      <div className="mx-auto max-w-7xl space-y-6 p-6">
      <header className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold text-slate-900">Calls</h1>
          <p className="mt-1 text-sm text-slate-500">
            Voice call log across every WhatsApp integration in this workspace. Newest first.
          </p>
        </div>
        <NewCallButton onCreated={() => void query.refetch()} />
      </header>

      <section
        aria-label="Filters"
        className="grid gap-3 rounded-xl border border-slate-200 bg-white p-4 sm:grid-cols-3"
      >
        <label className="flex flex-col gap-1 text-xs font-medium text-slate-600">
          Status
          <select
            value={status}
            onChange={(e) => setStatus(e.target.value as NonNullable<CallFilter['status']>)}
            className="rounded-lg border border-slate-200 bg-white px-2 py-1.5 text-sm text-slate-800"
          >
            <option value="">All</option>
            {CALL_STATUSES.map((s) => (
              <option key={s} value={s}>
                {s}
              </option>
            ))}
          </select>
        </label>
        <label className="flex flex-col gap-1 text-xs font-medium text-slate-600">
          Direction
          <select
            value={direction}
            onChange={(e) => setDirection(e.target.value as NonNullable<CallFilter['direction']>)}
            className="rounded-lg border border-slate-200 bg-white px-2 py-1.5 text-sm text-slate-800"
          >
            <option value="">All</option>
            <option value="inbound">Inbound</option>
            <option value="outbound">Outbound</option>
          </select>
        </label>
      </section>

      {query.isError && (
        <div
          role="alert"
          className="rounded-xl border border-rose-200 bg-rose-50 p-4 text-sm text-rose-800"
        >
          <p className="font-medium">
            {isPermDenied
              ? "You don't have permission to view the call log."
              : 'Could not load calls.'}
          </p>
        </div>
      )}

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,2fr)]">
        <div className="overflow-hidden rounded-xl border border-slate-200 bg-white">
          {query.isPending ? (
            <div className="flex items-center justify-center py-12">
              <Spinner className="h-6 w-6 text-slate-500" label="Loading calls" />
            </div>
          ) : items.length === 0 ? (
            <EmptyState
              title="No calls yet."
              description="Calls will appear here as soon as an inbound or outbound call is placed."
            />
          ) : (
            <div className="max-h-[70vh] overflow-y-auto">
              {items.map((c) => (
                <CallRow
                  key={c.id}
                  call={c}
                  selected={c.id === selectedID}
                  onSelect={() => setSelectedID(c.id)}
                />
              ))}
              {query.hasNextPage === true && (
                <div className="flex justify-center border-t border-slate-100 py-3">
                  <Button
                    variant="secondary"
                    onClick={() => {
                      void query.fetchNextPage();
                    }}
                    disabled={query.isFetchingNextPage}
                  >
                    {query.isFetchingNextPage ? 'Loading…' : 'Load more'}
                  </Button>
                </div>
              )}
            </div>
          )}
        </div>
        <CallDetail callID={selectedID} />
      </div>
      </div>
    </div>
  );
}

// permissionChipClasses maps a permission status to a Tailwind chip style.
// Green = permanent, amber = temporary, red = missing / unknown.
function permissionChipClasses(status: string | undefined): string {
  switch (status) {
    case 'permanent':
      return 'bg-emerald-50 text-emerald-700 ring-emerald-200';
    case 'temporary':
      return 'bg-amber-50 text-amber-700 ring-amber-200';
    case 'no_permission':
    case '':
    case undefined:
      return 'bg-rose-50 text-rose-700 ring-rose-200';
    default:
      return 'bg-slate-50 text-slate-700 ring-slate-200';
  }
}

// permissionChipLabel renders the status + expiration for temporary grants
// as a compact human-readable label.
function permissionChipLabel(pm: CallPermission | undefined): string {
  if (pm === undefined) return 'Checking…';
  const s = pm.status ?? '';
  if (s === 'permanent') return 'Permanent';
  if (s === 'temporary') {
    if (pm.expiration_time !== undefined && pm.expiration_time > 0) {
      const nowSec = Math.floor(Date.now() / 1000);
      const hours = Math.max(0, Math.ceil((pm.expiration_time - nowSec) / 3600));
      return `Temporary (expires in ${hours}h)`;
    }
    return 'Temporary';
  }
  if (s === 'no_permission' || s === '') return 'No permission granted';
  return s;
}

const DEFAULT_PERMISSION_PROMPT =
  "We'd like to call you regarding your recent conversation.";

function NewCallButton({ onCreated }: { onCreated: () => void }) {
  const [open, setOpen] = useState(false);
  const [integrationID, setIntegrationID] = useState('');
  const [to, setTo] = useState('');
  const [recordingEnabled, setRecordingEnabled] = useState(false);
  const [transcriptionEnabled, setTranscriptionEnabled] = useState(false);
  const [prompt, setPrompt] = useState(DEFAULT_PERMISSION_PROMPT);
  const [requestSent, setRequestSent] = useState(false);
  const integrations = useIntegrations();
  const initiate = useInitiateCall();
  const sendRequest = useSendPermissionRequest();

  const waIntegrations = useMemo(
    () =>
      (integrations.data ?? []).filter(
        (i) => i.provider === 'whatsapp' && i.status === 'connected',
      ),
    [integrations.data],
  );

  const trimmedTo = to.trim();
  const looksLikePhone = /^\+?\d{6,}/.test(trimmedTo);
  const canSubmit = integrationID !== '' && looksLikePhone;

  // Query the recipient's permission whenever both integration + a
  // well-formed phone are present.
  const permQuery = useCallPermission({
    integrationID,
    to: trimmedTo,
    enabled: integrationID !== '' && looksLikePhone,
  });
  const permission = permQuery.data;
  const permStatus = permission?.status ?? '';

  // Detect the 428 permission_missing problem+json from the initiate call
  // so the CTA switches to "Send permission request" even before the
  // preflight check has come back.
  const initiateErr = initiate.error;
  const initiate428 =
    initiateErr instanceof ApiError &&
    initiateErr.status === 428 &&
    (initiateErr.problem.title === 'permission_missing' ||
      initiateErr.problem.type?.endsWith('/permission_missing') === true);

  const permissionBlocks =
    initiate428 === true ||
    (permQuery.isSuccess && (permStatus === '' || permStatus === 'no_permission'));

  const submit = async () => {
    if (!canSubmit) return;
    try {
      await initiate.mutateAsync({
        integration_id: integrationID,
        to: trimmedTo,
        ...(recordingEnabled ? { recording: { enabled: true } } : {}),
        ...(transcriptionEnabled ? { transcription: { enabled: true } } : {}),
      });
      setOpen(false);
      setTo('');
      setRequestSent(false);
      onCreated();
    } catch {
      // error surfaced by mutation state below
    }
  };

  const sendPermission = async () => {
    if (integrationID === '' || !looksLikePhone) return;
    try {
      const trimmedPrompt = prompt.trim();
      await sendRequest.mutateAsync({
        integration_id: integrationID,
        to: trimmedTo,
        ...(trimmedPrompt === '' ? {} : { prompt: trimmedPrompt }),
      });
      setRequestSent(true);
    } catch {
      // error surfaced by mutation state below
    }
  };

  return (
    <div className="relative">
      <Button variant="primary" onClick={() => setOpen((v) => !v)}>
        + New Call
      </Button>
      {open && (
        <div className="absolute right-0 top-full z-10 mt-2 flex max-h-[calc(100vh-8rem)] w-96 flex-col overflow-y-auto rounded-xl border border-slate-200 bg-white p-4 shadow-lg">
          <h2 className="text-sm font-semibold text-slate-900">Start a new call</h2>
          <p className="mt-1 text-xs text-slate-500">
            Meta requires the recipient to have granted call permission first.
          </p>

          <label className="mt-3 flex flex-col gap-1 text-xs font-medium text-slate-600">
            Integration
            <select
              value={integrationID}
              onChange={(e) => setIntegrationID(e.target.value)}
              className="rounded-lg border border-slate-200 bg-white px-2 py-1.5 text-sm text-slate-800"
            >
              <option value="">Select…</option>
              {waIntegrations.map((i) => (
                <option key={i.id} value={i.id}>
                  {i.name}
                </option>
              ))}
            </select>
          </label>

          <label className="mt-3 flex flex-col gap-1 text-xs font-medium text-slate-600">
            Recipient phone (E.164)
            <input
              type="tel"
              value={to}
              onChange={(e) => {
                setTo(e.target.value);
                setRequestSent(false);
              }}
              placeholder="+12185552828"
              className="rounded-lg border border-slate-200 bg-white px-2 py-1.5 text-sm text-slate-800"
            />
          </label>

          {integrationID !== '' && looksLikePhone && (
            <div className="mt-2 flex flex-wrap items-center gap-2 text-xs">
              <span className="text-slate-500">Permission:</span>
              <span
                className={`inline-flex items-center rounded-md px-2 py-0.5 font-medium ring-1 ring-inset ${permissionChipClasses(
                  initiate428 === true ? 'no_permission' : permission?.status,
                )}`}
              >
                {permQuery.isPending
                  ? 'Checking…'
                  : permQuery.isError
                  ? 'Check failed'
                  : permissionChipLabel(
                      initiate428 === true
                        ? { status: 'no_permission' }
                        : permission,
                    )}
              </span>
            </div>
          )}

          <label className="mt-3 flex items-center gap-2 text-xs text-slate-700">
            <input
              type="checkbox"
              checked={recordingEnabled}
              onChange={(e) => setRecordingEnabled(e.target.checked)}
            />
            Record this call
          </label>

          <label className="mt-2 flex items-center gap-2 text-xs text-slate-700">
            <input
              type="checkbox"
              checked={transcriptionEnabled}
              onChange={(e) => setTranscriptionEnabled(e.target.checked)}
            />
            Transcribe this call
          </label>

          {permissionBlocks && (
            <label className="mt-3 flex flex-col gap-1 text-xs font-medium text-slate-600">
              Permission request message
              <textarea
                value={prompt}
                onChange={(e) => setPrompt(e.target.value)}
                rows={2}
                className="rounded-lg border border-slate-200 bg-white px-2 py-1.5 text-sm text-slate-800"
              />
            </label>
          )}

          {initiate.isError && !initiate428 && (
            <div
              role="alert"
              className="mt-3 max-h-40 overflow-y-auto rounded-lg border border-rose-200 bg-rose-50 p-2.5 text-xs leading-snug text-rose-700"
            >
              <span className="break-words">
                {(initiate.error as Error).message}
              </span>
            </div>
          )}

          {initiate428 && (
            <div
              role="alert"
              className="mt-3 rounded-lg border border-amber-200 bg-amber-50 p-2.5 text-xs leading-snug text-amber-800"
            >
              {initiateErr instanceof ApiError
                ? initiateErr.problem.detail ??
                  'The recipient has not granted call permission yet. Send a permission request first.'
                : 'The recipient has not granted call permission yet.'}
            </div>
          )}

          {sendRequest.isError && (
            <div
              role="alert"
              className="mt-3 rounded-lg border border-rose-200 bg-rose-50 p-2.5 text-xs leading-snug text-rose-700"
            >
              <span className="break-words">
                {(sendRequest.error as Error).message}
              </span>
            </div>
          )}

          {requestSent && (
            <div
              role="status"
              className="mt-3 rounded-lg border border-emerald-200 bg-emerald-50 p-2.5 text-xs leading-snug text-emerald-800"
            >
              Request sent — waiting for user to grant.
            </div>
          )}

          <div className="mt-4 flex justify-end gap-2">
            <Button variant="ghost" onClick={() => setOpen(false)}>
              Cancel
            </Button>
            {permissionBlocks ? (
              <Button
                variant="primary"
                onClick={() => void sendPermission()}
                disabled={!canSubmit || sendRequest.isPending}
              >
                {sendRequest.isPending ? 'Sending…' : 'Send permission request'}
              </Button>
            ) : (
              <Button
                variant="primary"
                onClick={() => void submit()}
                disabled={!canSubmit || initiate.isPending}
              >
                {initiate.isPending ? 'Placing…' : 'Call'}
              </Button>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

type CallsSearch = {
  id?: string;
};

export const callsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/calls',
  component: CallsPage,
  // Enables /calls?id=<call.id> deep-linking from Thread call bubbles.
  validateSearch: (raw: Record<string, unknown>): CallsSearch => {
    const id = raw['id'];
    return typeof id === 'string' && id.length > 0 ? { id } : {};
  },
});
