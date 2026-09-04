/**
 * Canonical WebSocket event names the frontend cares about.
 * These mirror the server-side canonical event types published on `/ws/inbox`.
 */
export const InboxEvent = {
  MessageReceived: 'message.received',
  MessageSent: 'message.sent',
  MessageDelivered: 'message.delivered',
  MessageRead: 'message.read',
  MessageFailed: 'message.failed',
  MessageStatus: 'message.status',
  ConversationCreated: 'conversation.created',
  ConversationUpdated: 'conversation.updated',
  IntegrationStatus: 'integration.status',
  CallInitiated: 'call.initiated',
  CallRinging: 'call.ringing',
  CallAnswered: 'call.answered',
  CallEnded: 'call.ended',
  CallEndedDetailed: 'call.ended.detailed',
  CallFailed: 'call.failed',
  CallRecordingCreated: 'call.recording_created',
} as const;

export type InboxEventName = (typeof InboxEvent)[keyof typeof InboxEvent];

export type InboxFrame = {
  type: string;
  payload: Record<string, unknown>;
};

export type MessageReceivedPayload = {
  message_id: string;
  conversation_id: string;
  org_id: string;
  direction: 'inbound' | 'outbound';
  type: string;
  text?: string;
  from?: string;
  received_at: string;
};

export type MessageSentPayload = {
  message_id: string;
  client_reference_id?: string;
  conversation_id: string;
  provider_message_id?: string;
  status: string;
  sent_at: string;
};

export type MessageStatusPayload = {
  message_id: string;
  conversation_id: string;
  status: 'queued' | 'sent' | 'delivered' | 'read' | 'failed';
  at: string;
};

export type ConversationUpdatedPayload = {
  conversation_id: string;
  last_message_at: string;
  last_message_preview?: string;
};

export function isInboxFrame(x: unknown): x is InboxFrame {
  if (typeof x !== 'object' || x === null) return false;
  const obj = x as Record<string, unknown>;
  return typeof obj['type'] === 'string' && typeof obj['payload'] === 'object' && obj['payload'] !== null;
}
