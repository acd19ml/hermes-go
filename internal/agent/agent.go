package agent

import (
	"context"
)

// chatClient is the internal interface satisfied by all LLM provider clients.
//
// Defined here after Phase 1 establishes two production implementations:
// StaticResponder (Phase 0, test double) and OpenAIChatClient (Phase 1, c1).
// Unexported so callers outside the package wire via concrete types.
//
// msgs is the complete conversation history including the system message
// prepended by AIAgent; clients receive the full list and need not manage it.
type chatClient interface {
	Respond(ctx context.Context, msgs []Message) (Message, error)
}

// AIAgent orchestrates a single LLM interaction.
//
// Fields are populated across Phase 1 commits:
//   - client       — injected at construction (c2)
//   - systemPrompt — hardcoded at construction (c3); byte-static after construction
//   - budget       — IterationBudget controlling max LLM calls (c4)
type AIAgent struct {
	client       chatClient
	systemPrompt string // set in c3; byte-static after construction
}

// NewAIAgent constructs an AIAgent with the provided client.
// Dependency injection allows tests to pass StaticResponder without a real key.
func NewAIAgent(client chatClient) *AIAgent {
	return &AIAgent{client: client}
}

// RunOnce sends a single user message to the LLM and returns the assistant
// reply. It prepends the system prompt when non-empty (c3).
//
// Budget enforcement is added in c4.
func (a *AIAgent) RunOnce(ctx context.Context, userMsg string) (Message, error) {
	msgs := []Message{}
	if a.systemPrompt != "" {
		msgs = append(msgs, Message{Role: RoleSystem, Content: a.systemPrompt})
	}
	msgs = append(msgs, Message{Role: RoleUser, Content: userMsg})

	return a.client.Respond(ctx, msgs)
}
