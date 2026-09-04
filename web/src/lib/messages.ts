import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ApiError, api } from './api';

export type MessageDirection = 'inbound' | 'outbound';

export type MessageStatus = 'queued' | 'sending' | 'sent' | 'delivered' | 'read' | 'failed';

export type MessageType =
  | 'text'
  | 'image'
  | 'video'
  | 'audio'
  | 'document'
  | 'sticker'
  | 'location'
  | 'contacts'
  | 'interactive'
  | 'button'
  | 'template'
  | 'reaction'
  | 'system'
  | 'unknown';

export type Message = {
  id: string;
  org_id: string;
  conversation_id: string;
  direction: MessageDirection;
  type: MessageType;
  status: MessageStatus;
  text?: string;
  media_url?: string;
  content_type?: string;
  media_caption?: string;
  provider_message_id?: string;
  client_reference_id?: string;
  location?: {
    latitude: number;
    longitude: number;
    name?: string;
    address?: string;
    url?: string;
  };
  contacts?: Array<{
    name: string;
    phones?: string[];
    emails?: string[];
  }>;
  reaction?: {
    emoji: string;
    message_id: string;
  };
  interactive?: {
    kind: string;
    id?: string;
    title?: string;
    description?: string;
  };
  reply_to_provider_message_id?: string;
  created_at: string;
  updated_at?: string;
};

export type Conversation = {
  id: string;
  org_id: string;
  contact_id: string;
  contact_name?: string;
  contact_avatar_url?: string;
  status: 'open' | 'pending' | 'resolved' | 'closed';
  last_message_at?: string;
  last_message_preview?: string;
  unread_count?: number;
  channel: string;
  created_at: string;
};

/**
 * SendMessageInput is the discriminated union the Composer hands to
 * useSendMessage. Text sends carry a plain `text`; media sends carry a
 * `media` object with the URL returned by POST /api/v1/attachments.
 * The `client_reference_id` is optimistic-UI plumbing echoed back on the
 * accept response so the optimistic row can be reconciled with the server id.
 */
export type SendMessageInput =
  | {
      conversation_id: string;
      type: 'text';
      text: string;
      client_reference_id?: string;
    }
  | {
      conversation_id: string;
      type: 'image' | 'video' | 'audio' | 'document';
      media: {
        /** Meta media_id from Media Upload; preferred over url. */
        id?: string;
        url?: string;
        caption?: string;
        filename?: string;
      };
      client_reference_id?: string;
    };

/** MediaMessageType lists the outbound media kinds the Composer supports. */
export type MediaMessageType = 'image' | 'video' | 'audio' | 'document';

export type SendMessageResponse = {
  id: string;
  client_reference_id?: string;
  status: MessageStatus;
  accepted_at: string;
};

type ListResponse<T> = { items: T[] } | T[];

function normalize<T>(res: ListResponse<T>): T[] {
  if (Array.isArray(res)) return res;
  return res.items;
}

const CONVERSATIONS_KEY = ['conversations'] as const;

export function useConversations() {
  return useQuery<Conversation[], ApiError>({
    queryKey: CONVERSATIONS_KEY,
    queryFn: async () => {
      const res = await api<ListResponse<Conversation>>('/conversations');
      return normalize(res);
    },
    staleTime: 10_000,
    retry: (fc, err) => {
      if (err instanceof ApiError && err.status >= 400 && err.status < 500) return false;
      return fc < 2;
    },
  });
}

export function useConversationMessages(conversationID: string | null) {
  return useQuery<Message[], ApiError>({
    queryKey: ['messages', conversationID ?? ''],
    enabled: conversationID !== null && conversationID.length > 0,
    queryFn: async () => {
      if (conversationID === null) return [];
      const res = await api<ListResponse<Message>>(`/conversations/${conversationID}/messages`);
      return normalize(res);
    },
    staleTime: 5_000,
    retry: (fc, err) => {
      if (err instanceof ApiError && err.status >= 400 && err.status < 500) return false;
      return fc < 2;
    },
  });
}

/**
 * useMarkMessageRead marks a single inbound message as read to the provider
 * (Meta shows the blue double-tick). Idempotent — the server absorbs
 * outbound / already-read / no-wamid messages as no-ops.
 */
export function useMarkMessageRead() {
  const qc = useQueryClient();
  return useMutation<void, ApiError, { messageID: string; conversationID: string }>({
    mutationFn: async ({ messageID }) => {
      await api<void>(`/messages/${messageID}/read`, { method: 'POST' });
    },
    onSuccess: async (_data, vars) => {
      await qc.invalidateQueries({ queryKey: ['messages', vars.conversationID] });
      await qc.invalidateQueries({ queryKey: CONVERSATIONS_KEY });
    },
  });
}

/**
 * useMarkConversationRead marks the newest ~50 inbound-with-wamid unread
 * messages in a conversation as read on the provider side. The frontend
 * fires this when the operator opens a conversation.
 */
export function useMarkConversationRead() {
  const qc = useQueryClient();
  return useMutation<void, ApiError, { conversationID: string }>({
    mutationFn: async ({ conversationID }) => {
      await api<void>(`/conversations/${conversationID}/read`, { method: 'POST' });
    },
    onSuccess: async (_data, vars) => {
      await qc.invalidateQueries({ queryKey: ['messages', vars.conversationID] });
      await qc.invalidateQueries({ queryKey: CONVERSATIONS_KEY });
    },
  });
}

/**
 * useSendMessage builds the correct POST /api/v1/messages body from the
 * discriminated SendMessageInput. Text sends land the raw string under
 * `text` (the backend normalises bare strings into `{body}`). Media
 * sends land the URL + optional caption/filename under `media`.
 */
export function useSendMessage() {
  const qc = useQueryClient();
  return useMutation<SendMessageResponse, ApiError, SendMessageInput>({
    mutationFn: async (input) => {
      const body: Record<string, unknown> = {
        conversation_id: input.conversation_id,
        type: input.type,
      };
      if (input.client_reference_id !== undefined) {
        body['client_reference_id'] = input.client_reference_id;
        body['idempotency_key'] = input.client_reference_id;
      }
      if (input.type === 'text') {
        body['text'] = input.text;
      } else {
        body['media'] = input.media;
      }
      return api<SendMessageResponse>('/messages', { method: 'POST', body });
    },
    onSuccess: async (_data, vars) => {
      await qc.invalidateQueries({ queryKey: ['messages', vars.conversation_id] });
      await qc.invalidateQueries({ queryKey: CONVERSATIONS_KEY });
    },
  });
}
