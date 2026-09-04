/**
 * webrtc.ts — minimal WebRTC accept flow for inbound WhatsApp calls.
 *
 * The provider (Meta) ships an SDP offer inline on the `connect` webhook
 * (session.sdp = RFC 8866 SDP with ICE candidates baked in as
 * `a=candidate:` lines). This module builds a local answer against that
 * offer, gathers ICE for a short window, and returns the resulting SDP
 * plus the live peer connection + streams the caller mounts.
 *
 * Design constraints (distilled from the callsConsole.html reference):
 *  - Single Google STUN server. No TURN. No /iceServers endpoint round trip.
 *  - One-shot ICE gather; wait for `complete` or a 3s timeout, whichever
 *    fires first, then hand back the local description as-is.
 *  - Meta's offer carries the far-end ICE candidates inline in the SDP,
 *    so we parse `a=candidate:` lines and feed them to addIceCandidate
 *    (mirrors the callsConsole flow at lines 1683-1724).
 *  - The audio-only shape mirrors WhatsApp voice calls; video would be
 *    additive and out of scope here.
 */

const STUN_SERVERS: RTCIceServer[] = [
  { urls: 'stun:stun.l.google.com:19302' },
];

const ICE_GATHER_TIMEOUT_MS = 3_000;

/**
 * AcceptSession bundles the live artefacts of a successful accept flow:
 * the peer connection, the local mic stream, the remote-audio
 * MediaStream (already populated by `ontrack`), and the local answer SDP
 * the caller must POST to /api/v1/calls/{id}/answer.
 */
export type AcceptSession = {
  pc: RTCPeerConnection;
  localStream: MediaStream;
  remoteStream: MediaStream;
  answerSDP: string;
};

/**
 * normalizeSdp collapses line endings. Meta occasionally ships bare `\n`
 * SDP; the WebRTC spec expects `\r\n`. Also appends a trailing CRLF if
 * missing so parsers that key off line terminators don't drop the last
 * attribute.
 */
export function normalizeSdp(sdp: string): string {
  const withCRLF = sdp.replace(/\r\n/g, '\n').replace(/\n/g, '\r\n');
  return withCRLF.endsWith('\r\n') ? withCRLF : `${withCRLF}\r\n`;
}

/**
 * acceptOffer opens the mic, builds an RTCPeerConnection against Meta's
 * SDP offer, replays inline ICE candidates, creates a local answer, and
 * waits for ICE gather to complete (or 3s, whichever fires first).
 *
 * Because it calls `navigator.mediaDevices.getUserMedia`, it MUST be
 * invoked from a user gesture (button click) — otherwise the browser
 * denies the permission prompt and/or autoplay policy blocks audio.
 */
export async function acceptOffer(offerSDP: string): Promise<AcceptSession> {
  if (offerSDP.length === 0) {
    throw new Error('acceptOffer: empty offer SDP');
  }

  const normalized = normalizeSdp(offerSDP);
  const pc = new RTCPeerConnection({ iceServers: STUN_SERVERS });

  // Prepare a remote MediaStream so we can pipe ontrack audio into an
  // <audio> element the caller mounts. Created up-front so tracks that
  // arrive before mount aren't dropped.
  const remoteStream = new MediaStream();
  pc.ontrack = (ev): void => {
    for (const track of ev.streams[0]?.getTracks() ?? [ev.track]) {
      // Guard against double-adding the same track — some browsers fire
      // ontrack twice on renegotiation.
      if (!remoteStream.getTracks().includes(track)) {
        remoteStream.addTrack(track);
      }
    }
  };

  // Acquire mic. This is the prompt the user sees.
  const localStream = await navigator.mediaDevices.getUserMedia({ audio: true });
  for (const track of localStream.getTracks()) {
    pc.addTrack(track, localStream);
  }

  // Apply Meta's offer.
  await pc.setRemoteDescription({ type: 'offer', sdp: normalized });

  // Meta ships the far-end ICE candidates inline in the SDP as
  // `a=candidate:` lines. Most impls consume them during
  // setRemoteDescription, but we replay defensively via addIceCandidate
  // for parity with the reference client. Individual failures are
  // non-fatal — duplicates and unsupported types should not abort the
  // handshake.
  for (const cand of parseInlineCandidates(normalized)) {
    try {
      await pc.addIceCandidate(cand);
    } catch (err) {
      // eslint-disable-next-line no-console
      console.warn('acceptOffer: addIceCandidate skipped', err);
    }
  }

  // Answer.
  const answer = await pc.createAnswer({ offerToReceiveAudio: true });
  await pc.setLocalDescription(answer);

  // Gather ICE for up to ICE_GATHER_TIMEOUT_MS, whichever comes first.
  await waitForIceGather(pc);

  const rawSDP = pc.localDescription?.sdp ?? answer.sdp ?? '';
  if (rawSDP === '') {
    // Clean up mic before bailing out.
    for (const track of localStream.getTracks()) track.stop();
    try {
      pc.close();
    } catch {
      // ignore
    }
    throw new Error('acceptOffer: local description missing after ICE gather');
  }
  const finalSDP = sanitizeAnswerSDP(rawSDP);
  return { pc, localStream, remoteStream, answerSDP: finalSDP };
}

/**
 * sanitizeAnswerSDP strips lines Meta's WhatsApp Calling server rejects:
 *
 *  - mDNS candidates (`.local` hostnames) — Chrome hides the local IP
 *    behind a UUID `.local` name as a privacy feature, but Meta cannot
 *    resolve those. Meta returns error 138008 (SDP Validation error,
 *    subcode 2593093) when they are present. The public server-reflexive
 *    (srflx) candidate that STUN produced is retained.
 *  - `a=ice-options:trickle` — Meta expects full-ICE (non-trickle) since
 *    it ships all its candidates inline in the offer. Removing this flag
 *    keeps browsers that autoselect trickle from tripping the validator.
 *
 * Blank lines produced by removals are collapsed so the SDP structure
 * remains parseable.
 */
export function sanitizeAnswerSDP(sdp: string): string {
  const out: string[] = [];
  for (const rawLine of sdp.split(/\r?\n/)) {
    const line = rawLine;
    if (line === '') {
      continue;
    }
    if (line.startsWith('a=candidate:') && line.includes('.local ')) {
      continue;
    }
    if (line.startsWith('a=ice-options:')) {
      continue;
    }
    out.push(line);
  }
  // Ensure the SDP ends with a CRLF (WebRTC spec + Meta parser expect it).
  return out.join('\r\n') + '\r\n';
}

/**
 * hangup releases mic + peer resources. Idempotent — safe to call
 * multiple times, and safe to call with a null session (no-op).
 */
export function hangup(session: AcceptSession | null): void {
  if (session === null) return;
  try {
    for (const track of session.localStream.getTracks()) {
      track.stop();
    }
  } catch {
    // ignore — stream may already be released
  }
  try {
    for (const sender of session.pc.getSenders()) {
      if (sender.track !== null) sender.track.stop();
    }
  } catch {
    // ignore
  }
  try {
    session.pc.close();
  } catch {
    // ignore
  }
}

/**
 * toggleMute flips the enabled flag on every local audio track and
 * returns the new muted state (true = muted).
 */
export function toggleMute(session: AcceptSession): boolean {
  // Derive current muted state from the first audio track — if it's
  // disabled we're muted, otherwise live.
  const first = session.localStream.getAudioTracks()[0];
  const currentlyMuted = first !== undefined ? !first.enabled : false;
  const nextMuted = !currentlyMuted;
  for (const track of session.localStream.getTracks()) {
    track.enabled = !nextMuted;
  }
  return nextMuted;
}

/**
 * parseInlineCandidates extracts `a=candidate:` lines from an SDP blob
 * and returns them as RTCIceCandidateInit records suitable for
 * addIceCandidate. sdpMid / sdpMLineIndex are attributed to the m= line
 * the candidate is nested under.
 */
function parseInlineCandidates(sdp: string): RTCIceCandidateInit[] {
  const out: RTCIceCandidateInit[] = [];
  const lines = sdp.split(/\r?\n/);
  let mLineIndex = -1;
  let mid = '';
  for (const line of lines) {
    if (line.startsWith('m=')) {
      mLineIndex += 1;
      // Reset mid so we pick up the next a=mid: for this m= section.
      mid = '';
      continue;
    }
    if (line.startsWith('a=mid:')) {
      mid = line.slice('a=mid:'.length).trim();
      continue;
    }
    if (line.startsWith('a=candidate:') || line.startsWith('candidate:')) {
      const candidate = line.startsWith('a=') ? line.slice('a='.length) : line;
      const init: RTCIceCandidateInit = { candidate };
      if (mid !== '') init.sdpMid = mid;
      if (mLineIndex >= 0) init.sdpMLineIndex = mLineIndex;
      out.push(init);
    }
  }
  return out;
}

/**
 * waitForIceGather resolves when the peer connection's iceGatheringState
 * reaches 'complete' or the timeout fires — whichever is first. Meta's
 * SDP already includes remote candidates inline, so a 3s ceiling keeps
 * the accept latency bounded even on networks where trickle takes longer.
 */
function waitForIceGather(pc: RTCPeerConnection): Promise<void> {
  if (pc.iceGatheringState === 'complete') {
    return Promise.resolve();
  }
  return new Promise<void>((resolve) => {
    let done = false;
    const finish = (): void => {
      if (done) return;
      done = true;
      pc.removeEventListener('icegatheringstatechange', onChange);
      resolve();
    };
    const onChange = (): void => {
      if (pc.iceGatheringState === 'complete') finish();
    };
    pc.addEventListener('icegatheringstatechange', onChange);
    window.setTimeout(finish, ICE_GATHER_TIMEOUT_MS);
  });
}
