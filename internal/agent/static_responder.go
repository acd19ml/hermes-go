package agent

// StaticResponder is a concrete Responder that always returns a fixed
// assistant message without calling any external API. It serves two purposes:
//
//  1. End-to-end verification of the CLI --msg path before a real LLM client
//     exists (Phase 0.3).
//  2. A test stand-in for Phase 1 unit tests once the minimal chatClient
//     interface is extracted (Phase 1.2 requires two production implementations;
//     StaticResponder + AnthropicClient will be those two).
//
// Respond signature carries error so it aligns with the future chatClient
// interface without requiring a breaking change to StaticResponder.
type StaticResponder struct{}

// Respond accepts a user Message and returns a fixed assistant Message.
// The response content is "hermes-go (static): " followed by the input
// content, making it easy to verify round-trip behaviour in tests.
// It never returns a non-nil error.
func (s StaticResponder) Respond(msg Message) (Message, error) {
	return Message{
		Role:    RoleAssistant,
		Content: "hermes-go (static): " + msg.Content,
	}, nil
}
