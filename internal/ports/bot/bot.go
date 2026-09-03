// Package bot is the port for bot/AI-agent providers (Azure Bot, Zoho Zia,
// Dialogflow, OpenAI, Anthropic, Google AI, Custom).
package bot

import "context"

// Message is the canonical bot message shape.
type Message struct {
	ConversationID string
	Text           string
	Metadata       map[string]string
}

// Reply is what the bot returns.
type Reply struct {
	Text       string
	Handoff    bool
	Confidence float64
}

// Provider is implemented by every bot adapter.
type Provider interface {
	StartConversation(ctx context.Context, conversationID string) error
	HandleMessage(ctx context.Context, msg Message) (Reply, error)
	EndConversation(ctx context.Context, conversationID string) error
}
