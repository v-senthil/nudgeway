// Package aiport is the port for raw AI/LLM providers used by the AI
// orchestrator (as distinct from full turnkey bot providers).
//
// Named "aiport" (not "ai") to avoid clashing with the domain "ai" package.
package aiport

import "context"

// Request is a provider-neutral prompt.
type Request struct {
	SystemPrompt string
	UserPrompt   string
	Metadata     map[string]string
}

// Response is a provider-neutral completion.
type Response struct {
	Text        string
	InputTokens int
	OutputTokens int
	FinishReason string
}

// Provider is implemented by every AI adapter.
type Provider interface {
	Complete(ctx context.Context, req Request) (Response, error)
}
