import { useEffect, useRef, useState } from 'react';
import { useAnswerCall, useCallSession, useEndCall, useRejectCall } from '../../lib/calls';
import { clearIncomingCallIf, useIncomingCall } from '../../lib/incoming-call';
import { acceptOffer, hangup, toggleMute, type AcceptSession } from '../../lib/webrtc';
import { ApiError } from '../../lib/api';

/**
 * IncomingCallPopup is the fixed bottom-right toast that announces a
 * ringing inbound call and — on Accept — walks the operator through the
 * WebRTC handshake:
 *
 *   1. GET /api/v1/calls/{id}/session to pull the SDP offer Meta shipped
 *      on the `connect` webhook.
 *   2. Run acceptOffer(offer) which prompts the mic-permission,
 *      builds the local answer, and gathers ICE.
 *   3. POST /api/v1/calls/{id}/answer with the answer SDP + recording /
 *      transcription selections.
 *
 * After the handshake succeeds we stay mounted as a live call panel
 * (State B) with an elapsed timer, a mute toggle, an autoplay <audio>
 * element bound to the remote MediaStream, and an End button that
 * closes the peer + POSTs /end.
 */
export function IncomingCallPopup() {
  const call = useIncomingCall();
  // Preload the SDP offer as soon as we have a call id — the ringing
  // popup often outlives the fetch, so we can accept instantly.
  const sessionQ = useCallSession(call?.id ?? null);
  const answer = useAnswerCall();
  const reject = useRejectCall();
  const end = useEndCall();

  const [elapsed, setElapsed] = useState<number>(0);
  const [recordOn, setRecordOn] = useState<boolean>(false);
  const [transcribeOn, setTranscribeOn] = useState<boolean>(false);
  const [accepting, setAccepting] = useState<boolean>(false);
  const [inCall, setInCall] = useState<boolean>(false);
  const [muted, setMuted] = useState<boolean>(false);
  const [error, setError] = useState<string | null>(null);
  const remoteAudioRef = useRef<HTMLAudioElement | null>(null);
  const sessionRef = useRef<AcceptSession | null>(null);

  // Tick the elapsed counter; anchor to startedAt when the popup first
  // opens, then re-anchor to the accept moment once the call connects.
  useEffect(() => {
    if (call === null) {
      setElapsed(0);
      return;
    }
    const anchorSource = inCall ? Date.now() : Date.parse(call.startedAt);
    const anchor = Number.isNaN(anchorSource) ? Date.now() : anchorSource;
    setElapsed(Math.max(0, Math.floor((Date.now() - anchor) / 1000)));
    const t = window.setInterval(() => {
      setElapsed(Math.max(0, Math.floor((Date.now() - anchor) / 1000)));
    }, 1_000);
    return () => {
      window.clearInterval(t);
    };
  }, [call, inCall]);

  // Reset per-call ephemeral state whenever the tracked call id changes
  // out from under us (e.g. a fresh ringing frame replaces the old one).
  useEffect(() => {
    setInCall(false);
    setMuted(false);
    setError(null);
    setRecordOn(false);
    setTranscribeOn(false);
    setAccepting(false);
  }, [call?.id]);

  // Release WebRTC resources whenever the popup unmounts or the tracked
  // call id changes underneath us. WS `call.ended` / `call.failed`
  // frames clear the store via clearIncomingCallIf → call becomes null
  // → this effect fires the cleanup.
  useEffect(() => {
    return () => {
      hangup(sessionRef.current);
      sessionRef.current = null;
    };
  }, [call?.id]);

  // Wire the remote MediaStream to the audio element as soon as State B
  // mounts. Deferred to an effect so the <audio> ref exists. Kept ABOVE
  // the early `return null` below — every hook must run on every render.
  useEffect(() => {
    if (!inCall) return;
    const el = remoteAudioRef.current;
    const s = sessionRef.current;
    if (el === null || s === null) return;
    el.srcObject = s.remoteStream;
    // Browsers block autoplay for un-muted media without a user gesture
    // — Accept was that gesture, so this should resolve. If it rejects
    // we still keep the peer up; the operator can click Unmute (which
    // triggers a fresh play() implicitly on state change) as a fallback.
    void el.play().catch(() => {
      // ignore; the audio element remains bound and playback will
      // resume on the next user interaction.
    });
  }, [inCall]);

  if (call === null) return null;

  const label =
    call.contactName !== undefined && call.contactName.length > 0
      ? call.contactName
      : call.from.length > 0
        ? call.from
        : 'Unknown caller';

  const busy = accepting || answer.isPending || reject.isPending || end.isPending;

  const handleAccept = async (): Promise<void> => {
    setError(null);
    setAccepting(true);
    try {
      // 1. Fetch the SDP offer captured on the connect webhook. Prefer
      //    the cached query result; refetch on-demand if it's missing.
      let sess = sessionQ.data;
      if (sess === undefined) {
        const refetched = await sessionQ.refetch();
        if (refetched.data === undefined) {
          const e = refetched.error;
          if (e instanceof ApiError && e.status === 404) {
            throw new Error('No SDP offer available for this call. Try again.');
          }
          throw (e ?? new Error('Could not load call session'));
        }
        sess = refetched.data;
      }
      if (sess.sdp === '') {
        throw new Error('No SDP offer available for this call. Try again.');
      }

      // 2. Build the local answer + gather ICE. This triggers the
      //    mic-permission prompt on this user-gesture-anchored click.
      const result = await acceptOffer(sess.sdp);
      sessionRef.current = result;

      // 3. Post the answer + recording / transcription toggles.
      await answer.mutateAsync({
        id: call.id,
        sdp: result.answerSDP,
        ...(recordOn ? { recording: { enabled: true } } : {}),
        ...(transcribeOn ? { transcription: { enabled: true } } : {}),
      });

      // 4. Swap to State B and wire the remote stream to the hidden
      //    <audio> element. srcObject is set on the ref that mounts as
      //    soon as inCall flips true — see the effect below.
      setInCall(true);
    } catch (err) {
      // Tear down any partial state.
      hangup(sessionRef.current);
      sessionRef.current = null;
      const message =
        err instanceof ApiError
          ? (err.problem.detail ?? err.message)
          : err instanceof Error
            ? err.message
            : 'Accept failed';
      setError(message);
    } finally {
      setAccepting(false);
    }
  };

  const handleReject = (): void => {
    reject.mutate(
      { id: call.id },
      {
        onSuccess: () => {
          clearIncomingCallIf(call.id);
        },
      },
    );
  };

  const handleHangup = (): void => {
    // Local teardown first — no reason to hold onto the mic while the
    // /end POST is in flight.
    hangup(sessionRef.current);
    sessionRef.current = null;
    end.mutate(call.id, {
      onSettled: () => {
        clearIncomingCallIf(call.id);
      },
    });
  };

  const handleToggleMute = (): void => {
    const s = sessionRef.current;
    if (s === null) return;
    setMuted(toggleMute(s));
  };

  const mm = Math.floor(elapsed / 60).toString().padStart(2, '0');
  const ss = (elapsed % 60).toString().padStart(2, '0');

  return (
    <div
      role="dialog"
      aria-label={inCall ? 'Active call' : 'Incoming call'}
      className="pointer-events-auto fixed bottom-6 right-6 z-50 w-80 rounded-xl bg-white p-4 shadow-lg ring-1 ring-slate-200"
    >
      <div className="flex items-start gap-3">
        <div
          aria-hidden="true"
          className={
            'flex h-10 w-10 shrink-0 items-center justify-center rounded-full text-xl ' +
            (inCall
              ? 'bg-sky-100 text-sky-700'
              : 'animate-pulse bg-emerald-100 text-emerald-700')
          }
        >
          {inCall ? '🔊' : '📞'}
        </div>
        <div className="min-w-0 flex-1">
          <p className="text-xs font-medium uppercase tracking-wide text-slate-500">
            {inCall ? 'On call' : 'Incoming call from'}
          </p>
          <p className="truncate text-sm font-semibold text-slate-900" title={label}>
            {label}
          </p>
          <p className="mt-0.5 text-xs tabular-nums text-slate-500">
            {mm}:{ss}
          </p>
        </div>
      </div>

      {/* Hidden audio element bound to the remote MediaStream. Kept out
          of the flow but present in the DOM so the browser routes audio
          to the operator's speakers. */}
      <audio ref={remoteAudioRef} autoPlay className="sr-only" aria-hidden="true" />

      {!inCall && (
        <div className="mt-3 space-y-1.5 text-xs text-slate-700">
          <label className="flex items-center gap-2">
            <input
              type="checkbox"
              checked={recordOn}
              onChange={(e) => setRecordOn(e.target.checked)}
              disabled={busy}
              className="h-3.5 w-3.5 rounded border-slate-300 text-emerald-600 focus:ring-emerald-500"
            />
            Record this call
          </label>
          <label className="flex items-center gap-2">
            <input
              type="checkbox"
              checked={transcribeOn}
              onChange={(e) => setTranscribeOn(e.target.checked)}
              disabled={busy}
              className="h-3.5 w-3.5 rounded border-slate-300 text-emerald-600 focus:ring-emerald-500"
            />
            Transcribe this call
          </label>
        </div>
      )}

      <div className="mt-4 flex gap-2">
        {!inCall ? (
          <>
            <button
              type="button"
              disabled={busy}
              onClick={handleReject}
              className="flex-1 rounded-md bg-rose-600 px-3 py-2 text-sm font-medium text-white shadow-sm transition hover:bg-rose-700 focus:outline-none focus:ring-2 focus:ring-rose-500 focus:ring-offset-1 disabled:cursor-not-allowed disabled:opacity-60"
            >
              {reject.isPending ? 'Rejecting…' : 'Reject'}
            </button>
            <button
              type="button"
              disabled={busy}
              onClick={() => {
                void handleAccept();
              }}
              className="flex-1 rounded-md bg-emerald-600 px-3 py-2 text-sm font-medium text-white shadow-sm transition hover:bg-emerald-700 focus:outline-none focus:ring-2 focus:ring-emerald-500 focus:ring-offset-1 disabled:cursor-not-allowed disabled:opacity-60"
            >
              {accepting || answer.isPending ? 'Connecting…' : 'Accept'}
            </button>
          </>
        ) : (
          <>
            <button
              type="button"
              onClick={handleToggleMute}
              className={
                'flex-1 rounded-md px-3 py-2 text-sm font-medium shadow-sm transition focus:outline-none focus:ring-2 focus:ring-offset-1 ' +
                (muted
                  ? 'bg-slate-700 text-white hover:bg-slate-800 focus:ring-slate-500'
                  : 'bg-slate-100 text-slate-800 hover:bg-slate-200 focus:ring-slate-400')
              }
            >
              {muted ? 'Unmute' : 'Mute'}
            </button>
            <button
              type="button"
              disabled={end.isPending}
              onClick={handleHangup}
              className="flex-1 rounded-md bg-rose-600 px-3 py-2 text-sm font-medium text-white shadow-sm transition hover:bg-rose-700 focus:outline-none focus:ring-2 focus:ring-rose-500 focus:ring-offset-1 disabled:cursor-not-allowed disabled:opacity-60"
            >
              {end.isPending ? 'Ending…' : 'End call'}
            </button>
          </>
        )}
      </div>
      {error !== null && <p className="mt-2 text-xs text-rose-600">{error}</p>}
      {(answer.isError || reject.isError || end.isError) && error === null && (
        <p className="mt-2 text-xs text-rose-600">
          {answer.error?.message ??
            reject.error?.message ??
            end.error?.message ??
            'Call action failed'}
        </p>
      )}
    </div>
  );
}
