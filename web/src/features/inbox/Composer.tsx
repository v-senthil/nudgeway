import { useEffect, useRef, useState } from 'react';
import { Button } from '../../components/Button';
import { useSendMessage } from '../../lib/messages';
import type { Message } from '../../lib/messages';
import { ApiError } from '../../lib/api';
import { useQueryClient } from '@tanstack/react-query';
import { addInboxListener } from '../../lib/ws';
import { InboxEvent } from '../../lib/events';

type Props = {
  conversationID: string;
  orgID: string;
};

function makeClientRef(): string {
  const rand = Math.random().toString(36).slice(2, 10);
  return `c-${Date.now().toString(36)}-${rand}`;
}

export function Composer({ conversationID, orgID }: Props) {
  const [text, setText] = useState('');
  const [error, setError] = useState<string | null>(null);
  const send = useSendMessage();
  const qc = useQueryClient();
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  // Listen for `message.sent` frames matching optimistic sends and mark delivered.
  useEffect(() => {
    const off = addInboxListener((frame) => {
      if (frame.type !== InboxEvent.MessageSent && frame.type !== InboxEvent.MessageStatus) return;
      const convID = typeof frame.payload['conversation_id'] === 'string' ? frame.payload['conversation_id'] : null;
      const clientRef =
        typeof frame.payload['client_reference_id'] === 'string' ? frame.payload['client_reference_id'] : null;
      const messageID = typeof frame.payload['message_id'] === 'string' ? frame.payload['message_id'] : null;
      const status = typeof frame.payload['status'] === 'string' ? frame.payload['status'] : 'sent';
      if (convID === null) return;

      const key = ['messages', convID];
      qc.setQueryData<Message[] | undefined>(key, (prev) => {
        if (prev === undefined) return prev;
        return prev.map((m) => {
          if (clientRef !== null && m.client_reference_id === clientRef) {
            return { ...m, id: messageID ?? m.id, status: status as Message['status'] };
          }
          if (messageID !== null && m.id === messageID) {
            return { ...m, status: status as Message['status'] };
          }
          return m;
        });
      });
    });
    return off;
  }, [qc]);

  const handleSend = async () => {
    const trimmed = text.trim();
    if (trimmed.length === 0) return;
    setError(null);
    const clientRef = makeClientRef();
    const now = new Date().toISOString();

    // Optimistic append
    const optimistic: Message = {
      id: clientRef,
      org_id: orgID,
      conversation_id: conversationID,
      direction: 'outbound',
      type: 'text',
      status: 'sending',
      text: trimmed,
      client_reference_id: clientRef,
      created_at: now,
    };
    qc.setQueryData<Message[] | undefined>(['messages', conversationID], (prev) => {
      if (prev === undefined) return [optimistic];
      return [...prev, optimistic];
    });
    setText('');

    try {
      const res = await send.mutateAsync({
        conversation_id: conversationID,
        type: 'text',
        text: trimmed,
        client_reference_id: clientRef,
      });
      // Reconcile: swap the optimistic entry with the accepted id + status.
      qc.setQueryData<Message[] | undefined>(['messages', conversationID], (prev) => {
        if (prev === undefined) return prev;
        return prev.map((m) => (m.client_reference_id === clientRef ? { ...m, id: res.id, status: res.status } : m));
      });
    } catch (err) {
      const msg = err instanceof ApiError ? err.problem.detail ?? err.problem.title ?? 'Failed to send' : 'Failed to send';
      setError(msg);
      qc.setQueryData<Message[] | undefined>(['messages', conversationID], (prev) => {
        if (prev === undefined) return prev;
        return prev.map((m) => (m.client_reference_id === clientRef ? { ...m, status: 'failed' } : m));
      });
      setText(trimmed);
    }
  };

  const onKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      void handleSend();
    }
  };

  return (
    <div className="border-t border-slate-200 bg-white px-4 py-3">
      {error !== null && (
        <p role="alert" className="mb-2 rounded-lg bg-rose-50 px-3 py-1.5 text-xs text-rose-700 ring-1 ring-inset ring-rose-200">
          {error}
        </p>
      )}
      <div className="flex items-end gap-2">
        <label htmlFor="composer-text" className="sr-only">
          Message
        </label>
        <textarea
          id="composer-text"
          ref={textareaRef}
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={onKeyDown}
          rows={2}
          placeholder="Type a message… (Enter to send, Shift+Enter for newline)"
          className="min-h-[44px] flex-1 resize-none rounded-xl border border-slate-200 bg-slate-50 px-3 py-2 text-sm placeholder:text-slate-400 focus:border-emerald-500 focus:bg-white focus:outline-none focus:ring-2 focus:ring-emerald-200"
        />
        <Button
          variant="primary"
          onClick={() => void handleSend()}
          loading={send.isPending}
          disabled={text.trim().length === 0}
          aria-label="Send message"
        >
          Send
        </Button>
      </div>
    </div>
  );
}
