import { useEffect, useSyncExternalStore } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import type { QueryClient } from '@tanstack/react-query';
import { InboxEvent, isInboxFrame } from './events';
import type { InboxFrame } from './events';

type Status = 'idle' | 'connecting' | 'open' | 'closed';

type Listener = (frame: InboxFrame) => void;

type WSStore = {
  status: Status;
  lastFrame: InboxFrame | null;
};

const initialStore: WSStore = { status: 'idle', lastFrame: null };

/**
 * Module-level connection so multiple hook mounts share one socket.
 * Keyed by org id — a new org id closes the previous socket.
 */
type Connection = {
  orgID: string;
  ws: WebSocket | null;
  refCount: number;
  reconnectAttempts: number;
  reconnectTimer: number | null;
  closedByUser: boolean;
  store: WSStore;
  subscribers: Set<() => void>;
  listeners: Set<Listener>;
  qc: QueryClient | null;
};

let conn: Connection | null = null;

function emit(): void {
  if (conn === null) return;
  for (const s of conn.subscribers) s();
}

function setStatus(next: Status): void {
  if (conn === null) return;
  if (conn.store.status === next) return;
  conn.store = { ...conn.store, status: next };
  emit();
}

function scheduleReconnect(): void {
  if (conn === null) return;
  if (conn.closedByUser) return;
  const attempt = conn.reconnectAttempts;
  conn.reconnectAttempts = attempt + 1;
  const base = Math.min(30_000, 500 * 2 ** attempt);
  const jitter = Math.floor(Math.random() * 250);
  const delay = base + jitter;
  if (conn.reconnectTimer !== null) window.clearTimeout(conn.reconnectTimer);
  conn.reconnectTimer = window.setTimeout(() => {
    openSocket();
  }, delay);
}

function handleFrame(raw: string): void {
  if (conn === null) return;
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return;
  }
  if (!isInboxFrame(parsed)) return;

  conn.store = { ...conn.store, lastFrame: parsed };
  emit();

  for (const l of conn.listeners) {
    try {
      l(parsed);
    } catch {
      // isolate listener failures
    }
  }

  const qc = conn.qc;
  if (qc === null) return;

  switch (parsed.type) {
    case InboxEvent.MessageReceived:
    case InboxEvent.MessageSent:
    case InboxEvent.MessageStatus: {
      const convID = typeof parsed.payload['conversation_id'] === 'string' ? parsed.payload['conversation_id'] : null;
      if (convID !== null) {
        void qc.invalidateQueries({ queryKey: ['messages', convID] });
      }
      void qc.invalidateQueries({ queryKey: ['conversations'] });
      break;
    }
    case InboxEvent.ConversationCreated:
    case InboxEvent.ConversationUpdated: {
      void qc.invalidateQueries({ queryKey: ['conversations'] });
      break;
    }
    case InboxEvent.IntegrationStatus: {
      void qc.invalidateQueries({ queryKey: ['integrations'] });
      break;
    }
    default:
      break;
  }
}

function openSocket(): void {
  if (conn === null) return;
  const c = conn;
  if (c.ws !== null && (c.ws.readyState === WebSocket.OPEN || c.ws.readyState === WebSocket.CONNECTING)) {
    return;
  }
  setStatus('connecting');
  const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const url = `${proto}//${window.location.host}/ws/inbox`;
  const ws = new WebSocket(url);
  c.ws = ws;
  ws.onopen = () => {
    if (conn === null) return;
    conn.reconnectAttempts = 0;
    setStatus('open');
  };
  ws.onmessage = (ev) => {
    if (typeof ev.data === 'string') handleFrame(ev.data);
  };
  ws.onerror = () => {
    // let onclose handle reconnect
  };
  ws.onclose = () => {
    if (conn === null) return;
    setStatus('closed');
    conn.ws = null;
    scheduleReconnect();
  };
}

function ensureConnection(orgID: string, qc: QueryClient): Connection {
  if (conn !== null && conn.orgID !== orgID) {
    // Tenant change: tear down.
    conn.closedByUser = true;
    if (conn.reconnectTimer !== null) window.clearTimeout(conn.reconnectTimer);
    if (conn.ws !== null) conn.ws.close();
    conn = null;
  }
  if (conn === null) {
    conn = {
      orgID,
      ws: null,
      refCount: 0,
      reconnectAttempts: 0,
      reconnectTimer: null,
      closedByUser: false,
      store: { ...initialStore },
      subscribers: new Set(),
      listeners: new Set(),
      qc,
    };
    openSocket();
  } else {
    conn.qc = qc;
  }
  return conn;
}

function acquire(orgID: string, qc: QueryClient): () => void {
  const c = ensureConnection(orgID, qc);
  c.refCount += 1;
  return () => {
    c.refCount -= 1;
    if (c.refCount <= 0 && conn === c) {
      c.closedByUser = true;
      if (c.reconnectTimer !== null) window.clearTimeout(c.reconnectTimer);
      if (c.ws !== null) c.ws.close();
      conn = null;
    }
  };
}

function subscribe(cb: () => void): () => void {
  if (conn === null) return () => undefined;
  const c = conn;
  c.subscribers.add(cb);
  return () => {
    c.subscribers.delete(cb);
  };
}

function getSnapshot(): WSStore {
  return conn === null ? initialStore : conn.store;
}

/**
 * useInboxSocket opens a single shared WebSocket to `/ws/inbox`, reconnects with
 * exponential backoff, and invalidates relevant TanStack Query caches on frames.
 * Callers get the connection status and the most recent frame.
 */
export function useInboxSocket(orgID: string | null | undefined): { status: Status; lastFrame: InboxFrame | null } {
  const qc = useQueryClient();

  useEffect(() => {
    if (orgID === null || orgID === undefined || orgID.length === 0) return;
    const release = acquire(orgID, qc);
    return () => release();
  }, [orgID, qc]);

  const snap = useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
  return snap;
}

/**
 * addInboxListener registers a callback for every incoming frame. Returns unsubscribe.
 * Use for optimistic-update reconciliation (e.g., matching `message.sent` on send).
 */
export function addInboxListener(listener: Listener): () => void {
  if (conn === null) {
    // stash until connection exists
    const pending: Listener = listener;
    const timer = window.setInterval(() => {
      if (conn !== null) {
        conn.listeners.add(pending);
        window.clearInterval(timer);
      }
    }, 200);
    return () => {
      window.clearInterval(timer);
      if (conn !== null) conn.listeners.delete(pending);
    };
  }
  conn.listeners.add(listener);
  const c = conn;
  return () => {
    c.listeners.delete(listener);
  };
}
