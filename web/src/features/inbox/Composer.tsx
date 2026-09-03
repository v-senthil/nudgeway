import { useEffect, useRef, useState } from 'react';
import { Button } from '../../components/Button';
import { useSendMessage } from '../../lib/messages';
import type { Message, MediaMessageType, SendMessageInput } from '../../lib/messages';
import { ApiError } from '../../lib/api';
import { useUploadAttachment, mediaKindFromContentType, type UploadResult } from '../../lib/attachments';
import { useQueryClient } from '@tanstack/react-query';
import { addInboxListener } from '../../lib/ws';
import { InboxEvent } from '../../lib/events';
import { ComposerAttach } from './renderers/ComposerAttach';

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
  const [selectedFile, setSelectedFile] = useState<File | null>(null);
  const [attachment, setAttachment] = useState<UploadResult | null>(null);
  const [uploadError, setUploadError] = useState<string | null>(null);
  const send = useSendMessage();
  const upload = useUploadAttachment();
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

  const handleFileSelected = async (file: File) => {
    setUploadError(null);
    setSelectedFile(file);
    setAttachment(null);
    try {
      const res = await upload.mutateAsync(file);
      setAttachment(res);
    } catch (err) {
      const msg =
        err instanceof ApiError
          ? err.problem.detail ?? err.problem.title ?? 'Upload failed'
          : 'Upload failed';
      setUploadError(msg);
      setSelectedFile(null);
      setAttachment(null);
    }
  };

  const handleClearAttachment = () => {
    setSelectedFile(null);
    setAttachment(null);
    setUploadError(null);
  };

  const handleSend = async () => {
    const trimmed = text.trim();
    const hasText = trimmed.length > 0;
    const hasAttachment = attachment !== null;
    if (!hasText && !hasAttachment) return;
    // Disallow send while an upload is still in flight.
    if (selectedFile !== null && attachment === null) return;
    setError(null);
    const clientRef = makeClientRef();
    const now = new Date().toISOString();

    let input: SendMessageInput;
    let optimistic: Message;

    if (hasAttachment) {
      const kind: MediaMessageType = mediaKindFromContentType(attachment.content_type);
      input = {
        conversation_id: conversationID,
        type: kind,
        media: {
          url: attachment.media_url,
          ...(hasText ? { caption: trimmed } : {}),
          ...(kind === 'document' && attachment.filename !== undefined
            ? { filename: attachment.filename }
            : {}),
        },
        client_reference_id: clientRef,
      };
      optimistic = {
        id: clientRef,
        org_id: orgID,
        conversation_id: conversationID,
        direction: 'outbound',
        type: kind,
        status: 'sending',
        media_url: attachment.media_url,
        ...(hasText ? { media_caption: trimmed } : {}),
        client_reference_id: clientRef,
        created_at: now,
      };
    } else {
      input = {
        conversation_id: conversationID,
        type: 'text',
        text: trimmed,
        client_reference_id: clientRef,
      };
      optimistic = {
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
    }

    qc.setQueryData<Message[] | undefined>(['messages', conversationID], (prev) => {
      if (prev === undefined) return [optimistic];
      return [...prev, optimistic];
    });

    // Snapshot for rollback so we can restore composer state on failure.
    const prevText = text;
    const prevAttachment = attachment;
    const prevFile = selectedFile;
    setText('');
    setSelectedFile(null);
    setAttachment(null);

    try {
      const res = await send.mutateAsync(input);
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
      // Restore composer contents so the operator can retry without re-picking.
      setText(prevText);
      setSelectedFile(prevFile);
      setAttachment(prevAttachment);
    }
  };

  const onKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      void handleSend();
    }
  };

  const uploading = upload.isPending;
  const canSend =
    !uploading &&
    !send.isPending &&
    (text.trim().length > 0 || attachment !== null) &&
    // Block if a file is picked but hasn't finished uploading.
    !(selectedFile !== null && attachment === null);

  return (
    <div className="border-t border-slate-200 bg-white px-4 py-3">
      {error !== null && (
        <p role="alert" className="mb-2 rounded-lg bg-rose-50 px-3 py-1.5 text-xs text-rose-700 ring-1 ring-inset ring-rose-200">
          {error}
        </p>
      )}
      <div className="flex items-end gap-2">
        <ComposerAttach
          onSelected={(f) => void handleFileSelected(f)}
          onClear={handleClearAttachment}
          file={selectedFile}
          uploading={uploading}
          error={uploadError}
        />
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
          placeholder={
            attachment !== null
              ? 'Add an optional caption… (Enter to send)'
              : 'Type a message… (Enter to send, Shift+Enter for newline)'
          }
          className="min-h-[44px] flex-1 resize-none rounded-xl border border-slate-200 bg-slate-50 px-3 py-2 text-sm placeholder:text-slate-400 focus:border-emerald-500 focus:bg-white focus:outline-none focus:ring-2 focus:ring-emerald-200"
        />
        <Button
          variant="primary"
          onClick={() => void handleSend()}
          loading={send.isPending || uploading}
          disabled={!canSend}
          aria-label="Send message"
        >
          Send
        </Button>
      </div>
    </div>
  );
}
