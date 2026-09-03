import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { ApiError, api } from './api';

export type MessageDirection = 'inbound' | 'outbound';

export type MessageStatus = 'queued' | 'sending' | 'sent' | 'delivered' | 'read' | 'failed';

export type MessageType = 'text' | 'image' | 'video' | 'audio' | 'document' | 'sticker' | 'location' | 'contacts' | 'interactive' | 'template' | 'reaction' | 'unknown';

export type Message = {
  id: string;
  org_id: string;
  conversation_id: string;
  direction: MessageDirection;
  type: MessageType;
  status: MessageStatus;
  text?: string;
  media_url?: string;
  media_caption?: string;
  provider_message_id?: string;
  client_reference_id?: string;
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

export type SendMessageInput = {
  conversation_id: string;
  type: 'text';
  text: string;
  client_reference_id?: string;
};

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

export function useSendMessage() {
  const qc = useQueryClient();
  return useMutation<SendMessageResponse, ApiError, SendMessageInput>({
    mutationFn: async (input) => {
      return api<SendMessageResponse>('/messages', { method: 'POST', body: input });
    },
    onSuccess: async (_data, vars) => {
      await qc.invalidateQueries({ queryKey: ['messages', vars.conversation_id] });
      await qc.invalidateQueries({ queryKey: CONVERSATIONS_KEY });
    },
  });
}
