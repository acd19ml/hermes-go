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

// ── c3: system prompt tests ────────────────────────────────────────────────

// TestAIAgentSystemPromptPrepended verifies that RunOnce puts a system message
// as the first element of the slice sent to the client, followed by the user
// message.
func TestAIAgentSystemPromptPrepended(t *testing.T) {
	var captured []Message
	spy := spyClient{capture: &captured, reply: Message{Role: RoleAssistant, Content: "ok"}}

	a := NewAIAgent(spy)
	if _, err := a.RunOnce(context.Background(), "ping"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(captured) != 2 {
		t.Fatalf("got %d messages, want 2 (system + user)", len(captured))
	}
	if captured[0].Role != RoleSystem {
		t.Errorf("messages[0].role = %q, want %q", captured[0].Role, RoleSystem)
	}
	if captured[0].Content != defaultSystemPrompt {
		t.Errorf("messages[0].content = %q, want %q", captured[0].Content, defaultSystemPrompt)
	}
	if captured[1].Role != RoleUser {
		t.Errorf("messages[1].role = %q, want %q", captured[1].Role, RoleUser)
	}
	if captured[1].Content != "ping" {
		t.Errorf("messages[1].content = %q, want %q", captured[1].Content, "ping")
	}
}

// TestAIAgentSystemPromptByteStatic verifies the byte-static invariant: two
// successive RunOnce calls on the same AIAgent produce identical system message
// bytes. This is required for Anthropic prompt prefix caching (Phase 4).
func TestAIAgentSystemPromptByteStatic(t *testing.T) {
	var calls [][]Message
	spy := spyClient{
		captureAll: &calls,
		reply:      Message{Role: RoleAssistant, Content: "ok"},
	}

	a := NewAIAgent(spy)
	for i := 0; i < 2; i++ {
		if _, err := a.RunOnce(context.Background(), "hello"); err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
	}

	if len(calls) != 2 {
		t.Fatalf("got %d calls, want 2", len(calls))
	}
	sys0 := calls[0][0].Content
	sys1 := calls[1][0].Content
	if sys0 != sys1 {
		t.Errorf("system prompt changed between calls:\n  call 0: %q\n  call 1: %q", sys0, sys1)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────

// spyClient is a test double that records the messages list passed to Respond.
type spyClient struct {
	capture    *[]Message   // appended to on each call (for single-call tests)
	captureAll *[][]Message // a slice-per-call (for multi-call tests)
	reply      Message
}

func (s spyClient) Respond(_ context.Context, msgs []Message) (Message, error) {
	if s.capture != nil {
		*s.capture = append(*s.capture, msgs...)
	}
	if s.captureAll != nil {
		cp := make([]Message, len(msgs))
		copy(cp, msgs)
		*s.captureAll = append(*s.captureAll, cp)
	}
	return s.reply, nil
}
