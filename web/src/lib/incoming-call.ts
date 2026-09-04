import { useSyncExternalStore } from 'react';

/**
 * IncomingCall is the minimal shape the global popup needs to render a
 * ringing inbound call. It is populated from a WS `call.ringing` /
 * `call.initiated` frame and cleared when the same call reaches a terminal
 * (answered / ended / failed) state or when the operator answers /
 * rejects via the popup buttons.
 */
export type IncomingCall = {
  /** Canonical Call.ID (ULID string) resolved by the backend. */
  id: string;
  /** Caller identity — E.164 phone or BSUID. Best-effort display value. */
  from: string;
  /** Human-friendly caller label, when the backend surfaced one. */
  contactName?: string;
  /** Integration id that received the call (for /answer + /reject routing). */
  integrationID?: string;
  /** Provider registry key ("whatsapp", …). */
  provider: string;
  /** ISO-8601 timestamp of when the call started ringing. */
  startedAt: string;
  /** Optional conversation the call is stitched to. */
  conversationID?: string;
};

type Listener = () => void;

let current: IncomingCall | null = null;
const listeners = new Set<Listener>();

function emit(): void {
  for (const l of listeners) {
    try {
      l();
    } catch {
      // isolate subscriber failures
    }
  }
}

/**
 * setIncomingCall replaces the currently-ringing call. Passing null clears.
 * A new ringing call replaces any existing one — only one popup is shown
 * at a time. Idempotent when the id matches: silently no-ops rather than
 * re-rendering.
 */
export function setIncomingCall(next: IncomingCall | null): void {
  if (next === null) {
    if (current === null) return;
    current = null;
    emit();
    return;
  }
  if (current !== null && current.id === next.id) return;
  current = next;
  emit();
}

/**
 * clearIncomingCallIf clears the store only when the tracked call id
 * matches. Used by terminal frames (answered / ended / failed) so a stale
 * terminal event for an older call never wipes a fresher ringing one.
 */
export function clearIncomingCallIf(callID: string): void {
  if (current !== null && current.id === callID) {
    current = null;
    emit();
  }
}

/** getIncomingCall returns the current ringing call, or null. */
export function getIncomingCall(): IncomingCall | null {
  return current;
}

function subscribe(cb: Listener): () => void {
  listeners.add(cb);
  return () => {
    listeners.delete(cb);
  };
}

/**
 * useIncomingCall subscribes React to the incoming-call store. Returns the
 * ringing call (or null) and re-renders on every update.
 */
export function useIncomingCall(): IncomingCall | null {
  return useSyncExternalStore(subscribe, getIncomingCall, getIncomingCall);
}
