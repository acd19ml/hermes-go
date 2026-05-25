package agent

import (
	"context"
	"fmt"
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
// Fields:
//   - client       — injected at construction; satisfies chatClient
//   - systemPrompt — hardcoded at construction; byte-static after construction
//   - budget       — caps the number of LLM calls; Phase 1 sets Max=1
type AIAgent struct {
	client       chatClient
	systemPrompt string         // byte-static after construction
	budget       IterationBudget
}

// defaultSystemPrompt is the hardcoded system prompt injected at the start of
// every conversation. It is set once at construction and never changed
// (byte-static invariant): the same bytes appear in every call within the
// same AIAgent lifetime, enabling Anthropic prompt prefix caching (Phase 4).
const defaultSystemPrompt = "You are hermes-go, a helpful AI assistant."

// NewAIAgent constructs an AIAgent with the provided client, injects the
// default system prompt, and sets the iteration budget to 1 (Phase 1
// single-turn). Dependency injection allows tests to pass StaticResponder
// without a real key.
func NewAIAgent(client chatClient) *AIAgent {
	return &AIAgent{
		client:       client,
		systemPrompt: defaultSystemPrompt,
		budget:       IterationBudget{Max: 1},
	}
}

// RunOnce sends a single user message to the LLM and returns the assistant
// reply. It enforces the iteration budget, then prepends the system prompt
// as the first message before calling the client.
//
// Returns an error if the budget is exhausted (client is never called) or if
// the underlying client call fails.
func (a *AIAgent) RunOnce(ctx context.Context, userMsg string) (Message, error) {
	if err := a.budget.Consume(); err != nil {
		return Message{}, fmt.Errorf("agent: %w", err)
	}

	msgs := []Message{}
	if a.systemPrompt != "" {
		msgs = append(msgs, Message{Role: RoleSystem, Content: a.systemPrompt})
	}
	msgs = append(msgs, Message{Role: RoleUser, Content: userMsg})

	return a.client.Respond(ctx, msgs)
}
