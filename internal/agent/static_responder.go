package agent

import "context"

// StaticResponder is a concrete chatClient that always returns a fixed
// assistant message without calling any external API. It serves two purposes:
//
//  1. End-to-end verification of the CLI --msg path before a real LLM client
//     exists (Phase 0.3).
//  2. A test stand-in (test double) injected via NewAIAgent in unit tests,
//     eliminating the need for a real API key during testing.
//
// StaticResponder satisfies the chatClient interface extracted in Phase 1 c2.
type StaticResponder struct{}

// Respond accepts the full message list (including any leading system message)
// and returns a fixed assistant Message. Only the last user message's Content
// is echoed; system messages are ignored.
// It never returns a non-nil error.
func (s StaticResponder) Respond(_ context.Context, msgs []Message) (Message, error) {
	// Find the last user message to echo; fall back to empty content.
	content := ""
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == RoleUser {
			content = msgs[i].Content
			break
		}
	}
	return Message{
		Role:    RoleAssistant,
		Content: "hermes-go (static): " + content,
	}, nil
}
