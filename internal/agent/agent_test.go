package agent

import (
	"context"
	"testing"
)

// TestAIAgentRunOnceWithStaticResponder verifies that RunOnce forwards the
// user message to the client and returns the assistant reply.
func TestAIAgentRunOnceWithStaticResponder(t *testing.T) {
	a := NewAIAgent(StaticResponder{})
	got, err := a.RunOnce(context.Background(), "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Role != RoleAssistant {
		t.Errorf("Role = %q, want %q", got.Role, RoleAssistant)
	}
	want := "hermes-go (static): hello"
	if got.Content != want {
		t.Errorf("Content = %q, want %q", got.Content, want)
	}
}

// TestAIAgentRunOnceEmptyMessage verifies that an empty user message does not
// panic and is forwarded to the client unchanged.
func TestAIAgentRunOnceEmptyMessage(t *testing.T) {
	a := NewAIAgent(StaticResponder{})
	got, err := a.RunOnce(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "hermes-go (static): "
	if got.Content != want {
		t.Errorf("Content = %q, want %q", got.Content, want)
	}
}

// TestAIAgentRunOnceBuildsMsgList verifies that RunOnce passes only the user
// message (no system message when systemPrompt is empty) to the client.
func TestAIAgentRunOnceBuildsMsgList(t *testing.T) {
	var captured []Message
	spy := spyClient{capture: &captured, reply: Message{Role: RoleAssistant, Content: "ok"}}

	a := NewAIAgent(spy)
	if _, err := a.RunOnce(context.Background(), "ping"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(captured) != 1 {
		t.Fatalf("got %d messages, want 1", len(captured))
	}
	if captured[0].Role != RoleUser {
		t.Errorf("messages[0].role = %q, want %q", captured[0].Role, RoleUser)
	}
	if captured[0].Content != "ping" {
		t.Errorf("messages[0].content = %q, want %q", captured[0].Content, "ping")
	}
}

// spyClient is a test double that records the messages list passed to Respond.
type spyClient struct {
	capture *[]Message
	reply   Message
}

func (s spyClient) Respond(_ context.Context, msgs []Message) (Message, error) {
	*s.capture = append(*s.capture, msgs...)
	return s.reply, nil
}
