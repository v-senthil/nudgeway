package events

import "time"

// Type enumerates every canonical event kind. New events land here first, and
// only then get published by application code. The wire format (protobuf)
// mirrors these definitions.
type Type string

// Canonical event types. See docs/flows/ for the flows they drive.
const (
	MessageReceived      Type = "message.received"
	MessageCreated       Type = "message.created"
	MessageSendRequested Type = "message.send_requested"
	MessageSent          Type = "message.sent"
	MessageFailed        Type = "message.failed"
	MessageDelivered     Type = "message.delivered"
	MessageRead          Type = "message.read"

	ContactCreated Type = "contact.created"
	ContactUpdated Type = "contact.updated"
	ContactTagged  Type = "contact.tagged"

	SessionCreated Type = "session.created"
	SessionClosed  Type = "session.closed"

	ConversationCreated  Type = "conversation.created"
	ConversationUpdated  Type = "conversation.updated"
	ConversationAssigned Type = "conversation.assigned"
	ConversationResolved Type = "conversation.resolved"
	ConversationReopened Type = "conversation.reopened"

	TicketCreated Type = "ticket.created"
	TicketUpdated Type = "ticket.updated"
	TicketClosed  Type = "ticket.closed"
	TicketSynced  Type = "ticket.synced"

	BotStarted          Type = "bot.started"
	BotMessageReceived  Type = "bot.message_received"
	BotCompleted        Type = "bot.completed"
	BotHandoffRequested Type = "bot.handoff_requested"

	CampaignCreated       Type = "campaign.created"
	CampaignStarted       Type = "campaign.started"
	CampaignMessageQueued Type = "campaign.message_queued"
	CampaignMessageSent   Type = "campaign.message_sent"
	CampaignMessageFailed Type = "campaign.message_failed"
	CampaignCompleted     Type = "campaign.completed"

	CallStarted          Type = "call.started"
	CallEnded            Type = "call.ended"
	CallRecordingCreated Type = "call.recording_created"

	TemplateCreated Type = "template.created"
	TemplateUpdated Type = "template.updated"
	TemplateDeleted Type = "template.deleted"

	WebhookReceived  Type = "webhook.received"
	WebhookProcessed Type = "webhook.processed"

	IntegrationCreated Type = "integration.created"
	IntegrationUpdated Type = "integration.updated"
	IntegrationFailed  Type = "integration.failed"
)

// Envelope wraps every event with tenant + correlation metadata. The
// concrete payload lives in Payload; the type selects the deserialiser.
type Envelope struct {
	Type          Type
	OrganizationID string
	OccurredAt    time.Time
	CorrelationID string
	CausationID   string
	Payload       any
}

// Handler processes one event envelope.
type Handler func(Envelope) error
