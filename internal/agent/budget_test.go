package agent

import (
	"context"
	"strings"
	"testing"
)

// ── IterationBudget unit tests ────────────────────────────────────────────────

func TestIterationBudgetConsumeWithinLimit(t *testing.T) {
	b := IterationBudget{Max: 3}
	for i := 0; i < 3; i++ {
		if err := b.Consume(); err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
	}
	if b.Remaining() != 0 {
		t.Errorf("Remaining = %d, want 0 after 3 of 3 consumed", b.Remaining())
	}
}

func TestIterationBudgetConsumeExhausted(t *testing.T) {
	b := IterationBudget{Max: 1}
	if err := b.Consume(); err != nil {
		t.Fatalf("first consume: unexpected error: %v", err)
	}
	err := b.Consume()
	if err == nil {
		t.Fatal("second consume: expected error when budget exhausted, got nil")
	}
	if !strings.Contains(err.Error(), "exhausted") {
		t.Errorf("error %q should mention 'exhausted'", err.Error())
	}
}

func TestIterationBudgetRemaining(t *testing.T) {
	b := IterationBudget{Max: 5}
	if b.Remaining() != 5 {
		t.Errorf("Remaining = %d, want 5 initially", b.Remaining())
	}
	_ = b.Consume()
	if b.Remaining() != 4 {
		t.Errorf("Remaining = %d, want 4 after one consume", b.Remaining())
	}
}

func TestIterationBudgetZeroMax(t *testing.T) {
	b := IterationBudget{Max: 0}
	if err := b.Consume(); err == nil {
		t.Fatal("expected error for Max=0 budget, got nil")
	}
}

// ── AIAgent budget integration tests ─────────────────────────────────────────

// TestAIAgentBudgetExhaustedDoesNotCallLLM verifies that RunOnce returns an
// error and does NOT call the underlying client when the budget is exhausted.
func TestAIAgentBudgetExhaustedDoesNotCallLLM(t *testing.T) {
	calls := 0
	counter := callCounterClient{count: &calls, reply: Message{Role: RoleAssistant, Content: "ok"}}

	a := NewAIAgent(counter)
	// First call should succeed (Max=1).
	if _, err := a.RunOnce(context.Background(), "first"); err != nil {
		t.Fatalf("first RunOnce: unexpected error: %v", err)
	}
	// Second call must fail without calling the client.
	_, err := a.RunOnce(context.Background(), "second")
	if err == nil {
		t.Fatal("second RunOnce: expected budget error, got nil")
	}
	if calls != 1 {
		t.Errorf("client called %d times, want exactly 1 (budget should block second call)", calls)
	}
}

// callCounterClient counts how many times Respond is invoked.
type callCounterClient struct {
	count *int
	reply Message
}

func (c callCounterClient) Respond(_ context.Context, _ []Message) (Message, error) {
	*c.count++
	return c.reply, nil
}
